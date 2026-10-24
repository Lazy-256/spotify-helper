package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/Lazy-256/spotify-helper/internal/auth"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

func TestRecentCursorsAndPlays(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/player/recently-played" || r.URL.Query().Get("limit") != "3" || r.URL.Query().Get("before") != "99" {
			t.Errorf("request %s", r.URL.String())
		}
		jsonReply(w, 200, `{"next":"https://x/next","cursors":{"after":"200","before":"100"},"items":[
		 {"played_at":"2026-10-24T09:00:00Z","context":{"type":"playlist","uri":"spotify:playlist:p"},
		  "track":{"id":"t","uri":"spotify:track:t","name":"T","artists":[],"album":{"uri":"spotify:album:a","name":"A","images":[]}}},
		 {"played_at":"x","track":null}]}`)
	})
	before := int64(99)
	res, err := c.Recent(context.Background(), 3, &before, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Limit != 3 || res.Cursors.Before == nil || *res.Cursors.Before != 100 || res.Cursors.After == nil || *res.Cursors.After != 200 {
		t.Errorf("cursors: %+v", res)
	}
	if len(res.Items) != 1 || res.Items[0].Track.Type != "track" || res.Items[0].Context == nil || res.Items[0].Context.Type != "playlist" {
		t.Errorf("items: %+v", res.Items)
	}
}

func TestRecentWithoutNextPageHasNoOlderCursor(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, 200, `{"next":null,"cursors":{"after":"200","before":"100"},"items":[]}`)
	})
	res, err := c.Recent(context.Background(), 20, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cursors.Before != nil || res.Cursors.After == nil || res.Items == nil || len(res.Items) != 0 {
		t.Errorf("res: %+v", res)
	}
}

func TestRecentAndTopWithoutScopeSendNothing(t *testing.T) {
	c, n, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) { jsonReply(w, 200, `{"items":[]}`) })
	c.Tokens = scopedToken{"TOK", []string{"user-library-read"}}
	_, err1 := c.Recent(context.Background(), 20, nil, nil)
	_, err2 := c.Top(context.Background(), "tracks", "medium", 20, 0)
	for _, err := range []error{err1, err2} {
		if oe := asErr(t, err); oe.Code != output.CodeScopesMissing {
			t.Errorf("got %+v", oe)
		}
	}
	if n.Load() != 0 {
		t.Errorf("%d requests were sent", n.Load())
	}
	c.Tokens = scopedToken{"TOK", []string{auth.ScopeRecentlyPlayed, auth.ScopeTopRead}}
	if _, err := c.Recent(context.Background(), 20, nil, nil); err != nil {
		t.Errorf("recent with its scope: %v", err)
	}
	if _, err := c.Top(context.Background(), "artists", "long", 20, 0); err != nil {
		t.Errorf("top with its scope: %v", err)
	}
	if n.Load() != 2 {
		t.Errorf("%d requests", n.Load())
	}
}

func TestTopRangesAndRateLimit(t *testing.T) {
	for short, long := range map[string]string{"short": "short_term", "medium": "medium_term", "long": "long_term"} {
		c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("time_range") != long {
				t.Errorf("%s: time_range %q", short, r.URL.Query().Get("time_range"))
			}
			jsonReply(w, 200, `{"total":0,"items":[]}`)
		})
		if _, err := c.Top(context.Background(), "tracks", short, 5, 0); err != nil {
			t.Errorf("%s: %v", short, err)
		}
	}
	c, n, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(429)
	})
	_, err := c.Top(context.Background(), "tracks", "medium", 5, 0)
	if oe := asErr(t, err); oe.Code != output.CodeRateLimited || n.Load() != 1 {
		t.Errorf("got %+v after %d requests", oe, n.Load())
	}
}
