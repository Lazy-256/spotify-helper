package cli

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Lazy-256/spotify-helper/internal/auth"
)

const playlistsFixture = `{"total":3,"limit":50,"offset":0,"items":[
 {"id":"P1","uri":"spotify:playlist:P1","name":"Mix","owner":{"display_name":"Me","id":"me"},"images":[{"url":"https://i.scdn.co/p1","width":300}]},
 null,
 {"id":"P2","uri":"spotify:playlist:P2","name":"Rock","owner":{"id":"spotify"},"images":null},
 {"id":"P3","uri":"spotify:playlist:P3","name":"Jazz","owner":{"display_name":"Me","id":"me"},"images":[]}]}`

func TestPlaylists(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/playlists", 200, playlistsFixture)
	res := parse(t, runOK(t, env, out, errb, "playlists"))

	if res["total"] != float64(3) || res["limit"] != float64(50) || res["offset"] != float64(0) {
		t.Errorf("paging fields: %v", res)
	}
	pls := res["playlists"].([]any)
	if len(pls) != 3 {
		t.Fatalf("null entries must be dropped, got %d entries", len(pls))
	}
	first := pls[0].(map[string]any)
	if first["uri"] != "spotify:playlist:P1" || first["name"] != "Mix" || first["owner_name"] != "Me" || first["cover_url"] != "https://i.scdn.co/p1" {
		t.Errorf("first: %v", first)
	}
	if pls[1].(map[string]any)["owner_name"] != "spotify" || pls[1].(map[string]any)["cover_url"] != nil {
		t.Errorf("second: %v", pls[1])
	}
	c := f.callsTo("GET /me/playlists")[0]
	if c.Query.Get("limit") != "50" || c.Query.Get("offset") != "0" {
		t.Errorf("default paging sent: %v", c.Query)
	}
}

func TestPlaylistsEmpty(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/playlists", 200, `{"total":0,"items":[]}`)
	got := runOK(t, env, out, errb, "playlists")
	if got != "{\"total\":0,\"offset\":0,\"limit\":50,\"playlists\":[]}\n" {
		t.Errorf("got %q", got)
	}
}

func TestPlaylistsSecondPage(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/playlists", 200, `{"total":57,"items":[{"id":"P","uri":"spotify:playlist:P","name":"X"}]}`)
	res := parse(t, runOK(t, env, out, errb, "playlists", "--limit", "20", "--offset", "20"))
	if res["limit"] != float64(20) || res["offset"] != float64(20) || res["total"] != float64(57) {
		t.Errorf("echo: %v", res)
	}
	if c := f.callsTo("GET /me/playlists")[0]; c.Query.Get("limit") != "20" || c.Query.Get("offset") != "20" {
		t.Errorf("query: %v", c.Query)
	}
}

func TestLibraryPagingValidation(t *testing.T) {
	for _, cmd := range []string{"playlists", "albums"} {
		for _, args := range [][]string{
			{"--limit", "0"}, {"--limit", "51"}, {"--limit", "-1"}, {"--limit", "many"},
			{"--offset", "-1"}, {"--offset", "x"}, {"extra"},
		} {
			env, f, out, errb := spotifyEnv(t)
			runFail(t, env, out, errb, 2, "usage", append([]string{cmd}, args...)...)
			if f.total() != 0 {
				t.Errorf("%s %v: a request was sent", cmd, args)
			}
		}
	}
}

func TestAlbums(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/albums", 200, `{"total":2,"items":[
	 {"added_at":"2026-01-01T00:00:00Z","album":{"id":"B","uri":"spotify:album:B","name":"Discovery","release_date":"2001-03-12","total_tracks":14,
	  "artists":[{"name":"Daft Punk","uri":"spotify:artist:A"}],"images":[{"url":"https://i.scdn.co/b","width":640}]}},
	 null,
	 {"added_at":"x","album":null}]}`)
	res := parse(t, runOK(t, env, out, errb, "albums"))
	if res["total"] != float64(2) {
		t.Errorf("total: %v", res["total"])
	}
	albums := res["albums"].([]any)
	if len(albums) != 1 {
		t.Fatalf("entries without an album are dropped, got %v", albums)
	}
	a := albums[0].(map[string]any)
	if a["uri"] != "spotify:album:B" || a["release_date"] != "2001-03-12" || a["total_tracks"] != float64(14) ||
		a["artists"].([]any)[0].(map[string]any)["name"] != "Daft Punk" || a["cover_url"] != "https://i.scdn.co/b" {
		t.Errorf("album: %v", a)
	}
}

func TestLibraryNotLoggedIn(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	_ = os.Remove(env.Getenv("XDG_CONFIG_HOME") + "/spotify-helper/token.json")
	runFail(t, env, out, errb, 3, "not_authenticated", "playlists")
}

func TestLibraryRateLimited(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.on("GET /me/albums", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(429)
	})
	runFail(t, env, out, errb, 5, "rate_limited", "albums")
	if !strings.Contains(errb.String(), `"retry_after":7`) {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestLibraryWithCovers(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	srv, _ := coverServer(t, env)
	f.reply("GET /me/playlists", 200, `{"total":2,"items":[
	 {"id":"P1","uri":"spotify:playlist:P1","name":"Mix","images":[{"url":"`+srv.URL+`/image/p1","width":300}]},
	 {"id":"P2","uri":"spotify:playlist:P2","name":"Bad","images":[{"url":"`+srv.URL+`/image/bad","width":300}]}]}`)
	f.reply("GET /me/albums", 200, `{"total":1,"items":[{"album":{"id":"B","uri":"spotify:album:B","name":"D","images":[{"url":"`+srv.URL+`/image/b","width":300}]}}]}`)

	pls := parse(t, runOK(t, env, out, errb, "playlists", "--covers"))["playlists"].([]any)
	if p, _ := pls[0].(map[string]any)["cover_path"].(string); p == "" {
		t.Errorf("playlist cover_path: %v", pls[0])
	}
	if v, has := pls[1].(map[string]any)["cover_path"]; !has || v != nil {
		t.Errorf("failed download must be null: %v", pls[1])
	}
	albums := parse(t, runOK(t, env, out, errb, "albums", "--covers"))["albums"].([]any)
	if p, _ := albums[0].(map[string]any)["cover_path"].(string); p == "" {
		t.Errorf("album cover_path: %v", albums[0])
	}
}

// ---- save, unsave, saved ----

// withScopes replaces the stored token's scopes.
func withScopes(t *testing.T, env *Env, scopes []string) {
	t.Helper()
	store := &auth.Store{Path: tokenPath(env)}
	if err := store.Save(&auth.Token{AccessToken: "TOKEN-VALUE", RefreshToken: "REFRESH-VALUE",
		ExpiresAt: env.Now().Unix() + 3600, ClientID: "test-client-id", Scopes: scopes}); err != nil {
		t.Fatal(err)
	}
}

func TestSaveTrack(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/library", 200, "")
	got := runOK(t, env, out, errb, "save", "spotify:track:abc")
	if got != "{\"ok\":true,\"uri\":\"spotify:track:abc\"}\n" {
		t.Errorf("got %q", got)
	}
	calls := f.callsTo("PUT /me/library")
	if len(calls) != 1 || calls[0].Query.Get("uris") != "spotify:track:abc" {
		t.Errorf("calls: %+v", calls)
	}
}

func TestSaveFromWebLinkAndEpisode(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/library", 200, "")
	res := parse(t, runOK(t, env, out, errb, "save", "https://open.spotify.com/track/abc?si=xyz"))
	if res["uri"] != "spotify:track:abc" {
		t.Errorf("link must be normalized: %v", res)
	}
	res = parse(t, runOK(t, env, out, errb, "save", "spotify:episode:ep1"))
	if res["uri"] != "spotify:episode:ep1" {
		t.Errorf("episode: %v", res)
	}
}

func TestSaveNeedsNoDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/library", 200, "")
	f.reply("GET /me/player", 204, "")
	runOK(t, env, out, errb, "save", "spotify:track:abc")
	if n := len(f.callsTo("GET /me/player")); n != 0 {
		t.Errorf("save must not look at the player, %d calls", n)
	}
}

func TestUnsaveTrack(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("DELETE /me/library", 200, "")
	got := runOK(t, env, out, errb, "unsave", "spotify:track:abc")
	if got != "{\"ok\":true,\"uri\":\"spotify:track:abc\"}\n" {
		t.Errorf("got %q", got)
	}
	if c := f.callsTo("DELETE /me/library"); len(c) != 1 || c[0].Query.Get("uris") != "spotify:track:abc" {
		t.Errorf("calls: %+v", c)
	}
}

func TestSaveRejectsOtherKinds(t *testing.T) {
	for _, cmd := range []string{"save", "unsave", "saved"} {
		for _, arg := range []string{"spotify:album:abc", "spotify:playlist:abc", "spotify:artist:abc", "hello"} {
			env, f, out, errb := spotifyEnv(t)
			runFail(t, env, out, errb, 2, "usage", cmd, arg)
			if f.total() != 0 {
				t.Errorf("%s %s: a request was sent", cmd, arg)
			}
		}
		env, f, out, errb := spotifyEnv(t)
		runFail(t, env, out, errb, 2, "usage", cmd)
		runFail(t, env, out, errb, 2, "usage", cmd, "spotify:track:a", "spotify:track:b")
		if f.total() != 0 {
			t.Errorf("%s: a request was sent for a bad argument count", cmd)
		}
	}
}

func TestSaveMissingScope(t *testing.T) {
	old := []string{"user-read-playback-state", "user-library-read"}
	for _, cmd := range []string{"save", "unsave"} {
		env, f, out, errb := spotifyEnv(t)
		withScopes(t, env, old)
		runFail(t, env, out, errb, 3, "scopes_missing", cmd, "spotify:track:abc")
		if f.total() != 0 {
			t.Errorf("%s: a request was sent without the scope", cmd)
		}
		if strings.Contains(errb.String(), "TOKEN-VALUE") {
			t.Errorf("%s: the error leaks the token", cmd)
		}
	}
}

func TestBadURIWinsOverMissingScope(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	withScopes(t, env, []string{"user-library-read"})
	runFail(t, env, out, errb, 2, "usage", "save", "spotify:album:abc")
}

func TestSaveNotLoggedIn(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	if err := removeFile(tokenPath(env)); err != nil {
		t.Fatal(err)
	}
	runFail(t, env, out, errb, 3, "not_authenticated", "save", "spotify:track:abc")
	if f.total() != 0 {
		t.Error("a request was sent without a token")
	}
}

func TestSaveSpotifyRefuses(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/library", 403, `{"error":{"status":403,"message":"Forbidden"}}`)
	runFail(t, env, out, errb, 1, "api_error", "save", "spotify:track:abc")
}

func TestSaved(t *testing.T) {
	for _, tt := range []struct {
		body string
		want string
	}{
		{`[true]`, "{\"uri\":\"spotify:track:abc\",\"saved\":true}\n"},
		{`[false]`, "{\"uri\":\"spotify:track:abc\",\"saved\":false}\n"},
	} {
		env, f, out, errb := spotifyEnv(t)
		f.reply("GET /me/library/contains", 200, tt.body)
		if got := runOK(t, env, out, errb, "saved", "spotify:track:abc"); got != tt.want {
			t.Errorf("%s: got %q", tt.body, got)
		}
		if c := f.callsTo("GET /me/library/contains"); len(c) != 1 || c[0].Query.Get("uris") != "spotify:track:abc" {
			t.Errorf("calls: %+v", c)
		}
	}
}

func TestSavedNeedsOnlyTheReadScope(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	withScopes(t, env, []string{"user-library-read"})
	f.reply("GET /me/library/contains", 200, `[true]`)
	res := parse(t, runOK(t, env, out, errb, "saved", "spotify:track:abc"))
	if res["saved"] != true {
		t.Errorf("got %v", res)
	}
}

func TestSavedUnexpectedAnswer(t *testing.T) {
	for _, body := range []string{`[]`, `[true,false]`, `{"saved":true}`, `["yes"]`, ``} {
		env, f, out, errb := spotifyEnv(t)
		f.reply("GET /me/library/contains", 200, body)
		runFail(t, env, out, errb, 1, "api_error", "saved", "spotify:track:abc")
	}
}

func TestSavedRateLimited(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.on("GET /me/library/contains", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(429)
	})
	runFail(t, env, out, errb, 5, "rate_limited", "saved", "spotify:track:abc")
	if res := parse(t, strings.TrimRight(errb.String(), "\n")); res["error"].(map[string]any)["retry_after"] != float64(7) {
		t.Errorf("stderr: %s", errb.String())
	}
}

func TestLibraryCommandsInHelp(t *testing.T) {
	env, out, errb := testEnv(t)
	Run([]string{"--help"}, env)
	text := out.String() + errb.String()
	for _, name := range []string{"save ", "unsave ", "saved "} {
		if !strings.Contains(text, name) {
			t.Errorf("--help does not list %q:\n%s", name, text)
		}
	}
}
