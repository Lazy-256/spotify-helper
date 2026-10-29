package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/auth"
	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/ratelimit"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

type fixedToken string

func (f fixedToken) AccessToken(context.Context) (string, error) { return string(f), nil }

type failingToken struct{ err error }

func (f failingToken) AccessToken(context.Context) (string, error) { return "", f.err }

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// newClient returns a Client talking to a test server driven by h, plus the
// number of requests that reached it.
func newClient(t *testing.T, h http.HandlerFunc) (*Client, *atomic.Int32, *clock) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	clk := &clock{t: time.Unix(1_700_000_000, 0)}
	return &Client{
		HTTP:   srv.Client(),
		Base:   srv.URL + "/v1",
		Tokens: fixedToken("TOK"),
		Gate:   &ratelimit.Gate{Path: filepath.Join(t.TempDir(), "rl.json"), Now: clk.now},
		Now:    clk.now,

		SettleDelay: time.Nanosecond,
	}, &n, clk
}

func asErr(t *testing.T, err error) *output.Error {
	t.Helper()
	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("want *output.Error, got %T %v", err, err)
	}
	return oe
}

func jsonReply(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func TestDoSendsAuthQueryAndBody(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer TOK" {
			t.Errorf("Authorization %q", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/me/player/play" || r.URL.Query().Get("device_id") != "D 1" {
			t.Errorf("request %s %s", r.Method, r.URL.String())
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["context_uri"] != "spotify:album:x" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("body %v / %s", body, r.Header.Get("Content-Type"))
		}
		jsonReply(w, 200, `{"hello":"world"}`)
	})
	var out struct{ Hello string }
	q := map[string][]string{"device_id": {"D 1"}}
	status, err := c.do(context.Background(), http.MethodPut, "/me/player/play", q, map[string]string{"context_uri": "spotify:album:x"}, &out)
	if err != nil || status != 200 || out.Hello != "world" {
		t.Errorf("status %d out %+v err %v", status, out, err)
	}
}

func TestDoNoContent(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	var out map[string]any
	status, err := c.do(context.Background(), http.MethodGet, "/me/player", nil, nil, &out)
	if err != nil || status != 204 || out != nil {
		t.Errorf("status %d out %v err %v", status, out, err)
	}
}

func TestDoTimeoutIsNetworkError(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) })
	c.HTTP = &http.Client{Timeout: 50 * time.Millisecond}
	_, err := c.do(context.Background(), http.MethodGet, "/me/player", nil, nil, nil)
	if oe := asErr(t, err); oe.Code != output.CodeNetworkError || !strings.Contains(oe.Message, "timed out") {
		t.Errorf("got %+v", oe)
	}
}

func TestDoConnectionRefusedIsNetworkError(t *testing.T) {
	c, _, _ := newClient(t, func(http.ResponseWriter, *http.Request) {})
	c.Base = "http://127.0.0.1:1/v1" // nothing listens here
	_, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	if oe := asErr(t, err); oe.Code != output.CodeNetworkError {
		t.Errorf("got %+v", oe)
	}
}

func TestTokenFailureIsReturnedUnchanged(t *testing.T) {
	c, n, _ := newClient(t, func(http.ResponseWriter, *http.Request) {})
	want := output.Errorf(output.CodeNotAuthenticated, "login please")
	c.Tokens = failingToken{want}
	_, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	if err != want || n.Load() != 0 {
		t.Errorf("err %v, requests %d", err, n.Load())
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode output.Code
		contains string
	}{
		{"no active device", 404, `{"error":{"status":404,"message":"Player command failed: No active device found","reason":"NO_ACTIVE_DEVICE"}}`, output.CodeNoActiveDevice, ""},
		{"premium required", 403, `{"error":{"status":403,"message":"Player command failed: Premium required","reason":"PREMIUM_REQUIRED"}}`, output.CodePremiumRequired, ""},
		{"unauthorized", 401, `{"error":{"status":401,"message":"The access token expired"}}`, output.CodeNotAuthenticated, ""},
		{"unauthorized oauth style", 401, `{"error":"invalid_token"}`, output.CodeNotAuthenticated, ""},
		{"other forbidden", 403, `{"error":{"status":403,"message":"User not registered in the Developer Dashboard"}}`, output.CodeAPIError, "HTTP 403: User not registered"},
		{"not found", 404, `{"error":{"status":404,"message":"Not found."}}`, output.CodeAPIError, "HTTP 404: Not found."},
		{"server error", 500, `oops`, output.CodeAPIError, "HTTP 500"},
		{"bad request string error", 400, `{"error":"invalid_request"}`, output.CodeAPIError, "HTTP 400: invalid_request"},
	}
	for _, tt := range tests {
		c, n, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) { jsonReply(w, tt.status, tt.body) })
		_, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
		oe := asErr(t, err)
		if oe.Code != tt.wantCode || !strings.Contains(oe.Message, tt.contains) {
			t.Errorf("%s: got %+v", tt.name, oe)
		}
		if n.Load() != 1 {
			t.Errorf("%s: %d requests, want exactly 1 (no retries)", tt.name, n.Load())
		}
		if strings.Contains(oe.Message, "TOK") {
			t.Errorf("%s: message leaks the token", tt.name)
		}
	}
}

func TestLongErrorMessageIsTruncated(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, 500, `{"error":{"message":"`+strings.Repeat("x", 5000)+`"}}`)
	})
	_, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	if oe := asErr(t, err); len(oe.Message) > 300 {
		t.Errorf("message length %d", len(oe.Message))
	}
}

func TestRateLimitedHeaderPresent(t *testing.T) {
	c, n, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(429)
	})
	_, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	oe := asErr(t, err)
	if oe.Code != output.CodeRateLimited || oe.RetryAfter != 12 || n.Load() != 1 {
		t.Errorf("got %+v after %d requests", oe, n.Load())
	}
}

func TestRateLimitedHeaderMissingOrMalformed(t *testing.T) {
	for _, h := range []string{"", "soon", "-3"} {
		c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			if h != "" {
				w.Header().Set("Retry-After", h)
			}
			w.WriteHeader(429)
		})
		_, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
		if oe := asErr(t, err); oe.RetryAfter != ratelimit.DefaultWait {
			t.Errorf("Retry-After %q: retry_after %d, want %d", h, oe.RetryAfter, ratelimit.DefaultWait)
		}
	}
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
	})
	_, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	if oe := asErr(t, err); oe.RetryAfter != 1 {
		t.Errorf("Retry-After 0: retry_after %d, want 1", oe.RetryAfter)
	}
}

func TestCooldownBlocksLocallyThenExpires(t *testing.T) {
	var limited atomic.Bool
	limited.Store(true)
	c, n, clk := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if limited.Load() {
			w.Header().Set("Retry-After", "12")
			w.WriteHeader(429)
			return
		}
		jsonReply(w, 200, `{}`)
	})
	_, _ = c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	if n.Load() != 1 {
		t.Fatalf("setup: %d requests", n.Load())
	}

	// 5 s later: still cooling down, and Spotify is not contacted.
	clk.t = clk.t.Add(5 * time.Second)
	_, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	if oe := asErr(t, err); oe.Code != output.CodeRateLimited || oe.RetryAfter < 1 || oe.RetryAfter > 7 {
		t.Errorf("got %+v", oe)
	}
	if n.Load() != 1 {
		t.Errorf("a request was sent during the cooldown (%d total)", n.Load())
	}

	// Past the deadline: calls go through again.
	limited.Store(false)
	clk.t = clk.t.Add(8 * time.Second)
	if _, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil); err != nil {
		t.Errorf("after cooldown: %v", err)
	}
	if n.Load() != 2 {
		t.Errorf("%d requests, want 2", n.Load())
	}
}

func TestUnreadableSuccessBody(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) { jsonReply(w, 200, `{not json`) })
	var out map[string]any
	_, err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, &out)
	if oe := asErr(t, err); oe.Code != output.CodeAPIError {
		t.Errorf("got %+v", oe)
	}
}

// ---- cover selection and mapping ----

func w(n int) *int { return &n }

func TestCoverURL(t *testing.T) {
	tests := []struct {
		name   string
		images []wireImage
		want   string // "" = nil
	}{
		{"640/300/64 picks 300", []wireImage{{"u640", w(640)}, {"u300", w(300)}, {"u64", w(64)}}, "u300"},
		{"order does not matter", []wireImage{{"u64", w(64)}, {"u640", w(640)}, {"u300", w(300)}}, "u300"},
		{"smallest above threshold", []wireImage{{"u640", w(640)}, {"u400", w(400)}}, "u400"},
		{"only small ones picks largest", []wireImage{{"u32", w(32)}, {"u64", w(64)}}, "u64"},
		{"large and small picks large", []wireImage{{"u64", w(64)}, {"u640", w(640)}}, "u640"},
		{"unknown widths pick first", []wireImage{{"first", nil}, {"second", nil}}, "first"},
		{"unknown width loses to known", []wireImage{{"nil", nil}, {"u64", w(64)}}, "u64"},
		{"empty urls skipped", []wireImage{{"", w(300)}, {"u64", w(64)}}, "u64"},
		{"none", nil, ""},
	}
	for _, tt := range tests {
		got := coverURL(tt.images)
		switch {
		case tt.want == "" && got != nil:
			t.Errorf("%s: got %q, want nil", tt.name, *got)
		case tt.want != "" && (got == nil || *got != tt.want):
			t.Errorf("%s: got %v, want %q", tt.name, got, tt.want)
		}
	}
}

func TestMapTrack(t *testing.T) {
	var wi wireItem
	src := `{"type":"track","id":"T","uri":"spotify:track:T","name":"Veridis Quo","duration_ms":345186,"explicit":true,
	  "artists":[{"name":"Daft Punk","uri":"spotify:artist:A"}],
	  "album":{"name":"Discovery","uri":"spotify:album:B","images":[{"url":"big","width":640},{"url":"mid","width":300}]}}`
	if err := json.Unmarshal([]byte(src), &wi); err != nil {
		t.Fatal(err)
	}
	it := mapItem(&wi)
	if it == nil || it.Type != "track" || it.URI != "spotify:track:T" || it.DurationMS != 345186 || !it.Explicit ||
		len(it.Artists) != 1 || it.Artists[0].Name != "Daft Punk" ||
		it.Album.Name != "Discovery" || it.Album.URI != "spotify:album:B" ||
		it.URL == nil || *it.URL != "mid" {
		t.Errorf("got %+v", it)
	}
}

func TestMapEpisode(t *testing.T) {
	var wi wireItem
	src := `{"type":"episode","id":"E","uri":"spotify:episode:E","name":"Ep 1","duration_ms":1000,
	  "show":{"name":"The Show","uri":"spotify:show:S","images":[{"url":"showimg","width":300}]},"images":[]}`
	if err := json.Unmarshal([]byte(src), &wi); err != nil {
		t.Fatal(err)
	}
	it := mapItem(&wi)
	if it == nil || it.Type != "episode" || it.Artists == nil || len(it.Artists) != 0 ||
		it.Album.Name != "The Show" || it.Album.URI != "spotify:show:S" ||
		it.URL == nil || *it.URL != "showimg" {
		t.Errorf("episode: %+v", it)
	}
	b, _ := json.Marshal(it)
	if !strings.Contains(string(b), `"artists":[]`) {
		t.Errorf("artists must serialize as []: %s", b)
	}
}

func TestMapItemUnknownOrNil(t *testing.T) {
	if mapItem(nil) != nil {
		t.Error("nil in, nil out")
	}
	if mapItem(&wireItem{Type: "ad"}) != nil {
		t.Error("advertisements are not items")
	}
}

func TestMapItemWithoutImagesHasNullCover(t *testing.T) {
	it := mapItem(&wireItem{Type: "track", Album: &wireAlbum{Name: "A"}})
	b, _ := json.Marshal(it)
	if !strings.Contains(string(b), `"cover_url":null`) || strings.Contains(string(b), "cover_path") {
		t.Errorf("got %s", b)
	}
}

func TestCoverPathSerialization(t *testing.T) {
	it := &output.Item{}
	b, _ := json.Marshal(it)
	if strings.Contains(string(b), "cover_path") {
		t.Errorf("cover_path must be absent unless requested: %s", b)
	}
	it.Path.Set("")
	b, _ = json.Marshal(it)
	if !strings.Contains(string(b), `"cover_path":null`) {
		t.Errorf("requested-but-missing must be null: %s", b)
	}
	it.Path.Set("/tmp/x.jpg")
	b, _ = json.Marshal(it)
	if !strings.Contains(string(b), `"cover_path":"/tmp/x.jpg"`) {
		t.Errorf("got %s", b)
	}
}

func TestMapPlaylistOwnerFallback(t *testing.T) {
	e := mapPlaylist(wirePlaylist{Name: "P", Owner: &struct {
		DisplayName string `json:"display_name"`
		ID          string `json:"id"`
	}{ID: "spotify"}})
	if e.OwnerName != "spotify" {
		t.Errorf("owner %q", e.OwnerName)
	}
}

func TestLibraryWritesSendTheURI(t *testing.T) {
	u, _ := spotifyuri.Parse("spotify:track:abc")
	for _, tt := range []struct {
		method string
		call   func(*Client) error
	}{
		{http.MethodPut, func(c *Client) error { return c.SaveItem(context.Background(), u) }},
		{http.MethodDelete, func(c *Client) error { return c.RemoveItem(context.Background(), u) }},
	} {
		c, n, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != tt.method || r.URL.Path != "/v1/me/library" || r.URL.Query().Get("uris") != "spotify:track:abc" {
				t.Errorf("request %s %s", r.Method, r.URL.String())
			}
			w.WriteHeader(200)
		})
		if err := tt.call(c); err != nil || n.Load() != 1 {
			t.Errorf("%s: err %v, %d requests", tt.method, err, n.Load())
		}
	}
}

// scopedToken is a token source that also knows its scopes.
type scopedToken struct {
	fixedToken
	scopes []string
}

func (s scopedToken) RequireScope(scope string) error {
	for _, have := range s.scopes {
		if have == scope {
			return nil
		}
	}
	return output.Errorf(output.CodeScopesMissing, "lacks %s", scope)
}

func TestLibraryWriteWithoutScopeSendsNothing(t *testing.T) {
	u, _ := spotifyuri.Parse("spotify:track:abc")
	c, n, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	c.Tokens = scopedToken{"TOK", []string{"user-library-read"}}
	for _, err := range []error{c.SaveItem(context.Background(), u), c.RemoveItem(context.Background(), u)} {
		if oe := asErr(t, err); oe.Code != output.CodeScopesMissing {
			t.Errorf("got %+v", oe)
		}
	}
	if n.Load() != 0 {
		t.Errorf("%d requests were sent", n.Load())
	}
	c.Tokens = scopedToken{"TOK", []string{auth.ScopeLibraryModify}}
	if err := c.SaveItem(context.Background(), u); err != nil || n.Load() != 1 {
		t.Errorf("with the scope: err %v, %d requests", err, n.Load())
	}
}

func TestItemSaved(t *testing.T) {
	u, _ := spotifyuri.Parse("spotify:track:abc")
	for body, want := range map[string]bool{`[true]`: true, `[false]`: false} {
		c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/me/library/contains" || r.URL.Query().Get("uris") != "spotify:track:abc" {
				t.Errorf("request %s %s", r.Method, r.URL.String())
			}
			jsonReply(w, 200, body)
		})
		if got, err := c.ItemSaved(context.Background(), u); err != nil || got != want {
			t.Errorf("%s: got %v, %v", body, got, err)
		}
	}
	for _, body := range []string{`[]`, `[true,true]`, `{"a":1}`, ``} {
		c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) { jsonReply(w, 200, body) })
		_, err := c.ItemSaved(context.Background(), u)
		if oe := asErr(t, err); oe.Code != output.CodeAPIError {
			t.Errorf("%q: got %+v", body, oe)
		}
	}
}
