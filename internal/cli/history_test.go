package cli

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Lazy-256/spotify-helper/internal/auth"
)

// scopesWithout saves a token that has every scope except the given ones.
func scopesWithout(t *testing.T, env *Env, drop ...string) {
	t.Helper()
	var scopes []string
	for _, s := range auth.Scopes {
		keep := true
		for _, d := range drop {
			keep = keep && s != d
		}
		if keep {
			scopes = append(scopes, s)
		}
	}
	store := &auth.Store{Path: tokenPath(env)}
	if err := store.Save(&auth.Token{AccessToken: "TOKEN-VALUE", RefreshToken: "REFRESH-VALUE",
		ExpiresAt: env.Now().Unix() + 3600, ClientID: "test-client-id", Scopes: scopes}); err != nil {
		t.Fatal(err)
	}
}

const recentBody = `{"limit":20,"next":"https://api.spotify.com/v1/me/player/recently-played?before=1792000000000",
 "cursors":{"after":"1792000900000","before":"1792000000000"},
 "items":[
  {"played_at":"2026-10-24T09:00:00.000Z","context":{"type":"album","uri":"spotify:album:alb","href":"x"},
   "track":{"type":"track","id":"t1","uri":"spotify:track:t1","name":"One","duration_ms":1000,
    "artists":[{"name":"Band","uri":"spotify:artist:b"}],"album":{"uri":"spotify:album:alb","name":"The Album","images":[{"url":"%COVER%","width":300}]}}},
  null,
  {"played_at":"2026-10-24T08:50:00.000Z","context":null,
   "track":{"type":"track","id":"t2","uri":"spotify:track:t2","name":"Two","duration_ms":2000,
    "artists":[],"album":{"uri":"spotify:album:alb2","name":"Other","images":[]}}}
 ]}`

func TestRecentCommand(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/recently-played", 200, strings.ReplaceAll(recentBody, "%COVER%", "https://i.scdn.co/c"))
	res := parse(t, runOK(t, env, out, errb, "recent"))

	if res["limit"] != float64(20) {
		t.Errorf("limit: %v", res["limit"])
	}
	cur := res["cursors"].(map[string]any)
	if cur["before"] != float64(1792000000000) || cur["after"] != float64(1792000900000) {
		t.Errorf("cursors: %v", cur)
	}
	items := res["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("a null entry must be dropped: %v", items)
	}
	first := items[0].(map[string]any)
	ctx := first["context"].(map[string]any)
	track := first["track"].(map[string]any)
	if first["played_at"] != "2026-10-24T09:00:00.000Z" || ctx["type"] != "album" || ctx["uri"] != "spotify:album:alb" ||
		track["uri"] != "spotify:track:t1" || track["type"] != "track" || track["cover_url"] != "https://i.scdn.co/c" {
		t.Errorf("first: %v", first)
	}
	if _, has := ctx["href"]; has {
		t.Errorf("Spotify's own context fields leaked: %v", ctx)
	}
	if v, has := items[1].(map[string]any)["context"]; !has || v != nil {
		t.Errorf("a play outside a context must carry \"context\":null: %v", items[1])
	}
	c := f.callsTo("GET /me/player/recently-played")[0]
	if c.Query.Get("limit") != "20" || c.Query.Has("before") || c.Query.Has("after") {
		t.Errorf("query: %v", c.Query)
	}
}

func TestRecentOlderPageAndLimit(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/recently-played", 200, `{"cursors":null,"next":null,"items":[]}`)
	res := parse(t, runOK(t, env, out, errb, "recent", "--before", "1792000000000", "--limit", "5"))
	c := f.callsTo("GET /me/player/recently-played")[0]
	if c.Query.Get("before") != "1792000000000" || c.Query.Get("limit") != "5" || c.Query.Has("after") {
		t.Errorf("query: %v", c.Query)
	}
	if res["limit"] != float64(5) {
		t.Errorf("limit: %v", res["limit"])
	}
	env2, f2, out2, errb2 := spotifyEnv(t)
	f2.reply("GET /me/player/recently-played", 200, `{"cursors":{"after":"1792000900000","before":"1792000000000"},"next":null,"items":[]}`)
	res = parse(t, runOK(t, env2, out2, errb2, "recent", "--after", "1792000000000"))
	if c := f2.callsTo("GET /me/player/recently-played")[0]; c.Query.Get("after") != "1792000000000" || c.Query.Has("before") {
		t.Errorf("query: %v", c.Query)
	}
	if cur := res["cursors"].(map[string]any); cur["before"] != nil || cur["after"] != float64(1792000900000) {
		t.Errorf("without a next page the older cursor is null: %v", cur)
	}
}

func TestRecentNothingPlayed(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/recently-played", 200, `{"items":[],"next":null,"cursors":null}`)
	got := runOK(t, env, out, errb, "recent")
	want := `{"limit":20,"cursors":{"before":null,"after":null},"items":[]}` + "\n"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestRecentUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"recent", "--before", "1", "--after", "2"},
		{"recent", "--limit", "0"}, {"recent", "--limit", "51"}, {"recent", "--limit", "many"},
		{"recent", "--before", "-1"}, {"recent", "--after", "soon"},
		{"recent", "extra"},
	} {
		env, f, out, errb := spotifyEnv(t)
		runFail(t, env, out, errb, 2, "usage", args...)
		if f.total() != 0 {
			t.Errorf("%v: a request was sent", args)
		}
	}
}

func TestRecentCovers(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	srv, _ := coverServer(t, env)
	f.reply("GET /me/player/recently-played", 200, strings.ReplaceAll(recentBody, "%COVER%", srv.URL+"/image/c"))
	res := parse(t, runOK(t, env, out, errb, "recent", "--covers"))
	first := res["items"].([]any)[0].(map[string]any)["track"].(map[string]any)
	if p, _ := first["cover_path"].(string); p == "" {
		t.Errorf("no cover_path: %v", first)
	}
	if _, has := res["items"].([]any)[1].(map[string]any)["track"].(map[string]any)["cover_path"]; !has {
		t.Errorf("a track without an image must still carry cover_path (null)")
	}
}

func TestTopTracksDefault(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/top/tracks", 200, `{"total":120,"items":[null,
	 {"type":"track","id":"t1","uri":"spotify:track:t1","name":"One","duration_ms":1000,"popularity":80,
	  "artists":[{"name":"Band","uri":"spotify:artist:b"}],"album":{"uri":"spotify:album:alb","name":"The Album","images":[]}}]}`)
	res := parse(t, runOK(t, env, out, errb, "top", "tracks"))
	if res["type"] != "tracks" || res["range"] != "medium" || res["total"] != float64(120) || res["offset"] != float64(0) || res["limit"] != float64(20) {
		t.Errorf("res: %v", res)
	}
	if _, has := res["artists"]; has {
		t.Errorf("only the array of the requested type may appear: %v", res)
	}
	tracks := res["tracks"].([]any)
	if len(tracks) != 1 || tracks[0].(map[string]any)["uri"] != "spotify:track:t1" {
		t.Errorf("tracks: %v", tracks)
	}
	if _, has := tracks[0].(map[string]any)["popularity"]; has {
		t.Errorf("popularity leaked: %v", tracks[0])
	}
	c := f.callsTo("GET /me/top/tracks")[0]
	if c.Query.Get("time_range") != "medium_term" || c.Query.Get("limit") != "20" || c.Query.Get("offset") != "0" {
		t.Errorf("query: %v", c.Query)
	}
}

func TestTopArtistsLongRangeAndPage(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/top/artists", 200, `{"total":40,"items":[{"id":"a1","uri":"spotify:artist:a1","name":"Band","genres":["x"],"followers":{"total":5},"popularity":50,"images":[]}]}`)
	res := parse(t, runOK(t, env, out, errb, "top", "artists", "--range", "long", "--limit", "10", "--offset", "10"))
	if res["type"] != "artists" || res["range"] != "long" || res["limit"] != float64(10) || res["offset"] != float64(10) {
		t.Errorf("res: %v", res)
	}
	if _, has := res["tracks"]; has {
		t.Errorf("tracks must not appear: %v", res)
	}
	a := res["artists"].([]any)[0].(map[string]any)
	if a["uri"] != "spotify:artist:a1" || a["name"] != "Band" {
		t.Errorf("artist: %v", a)
	}
	for _, k := range []string{"genres", "followers", "popularity"} {
		if _, has := a[k]; has {
			t.Errorf("%s must not be in the output: %v", k, a)
		}
	}
	c := f.callsTo("GET /me/top/artists")[0]
	if c.Query.Get("time_range") != "long_term" || c.Query.Get("limit") != "10" || c.Query.Get("offset") != "10" {
		t.Errorf("query: %v", c.Query)
	}
}

func TestTopEmptyAndRanges(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/top/tracks", 200, `{"total":0,"items":[]}`)
	got := runOK(t, env, out, errb, "top", "tracks", "--range", "short")
	want := `{"type":"tracks","range":"short","total":0,"offset":0,"limit":20,"tracks":[]}` + "\n"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if c := f.callsTo("GET /me/top/tracks")[0]; c.Query.Get("time_range") != "short_term" {
		t.Errorf("query: %v", c.Query)
	}
}

func TestTopUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"top"}, {"top", "albums"}, {"top", "tracks", "artists"},
		{"top", "tracks", "--range", "week"}, {"top", "tracks", "--limit", "0"}, {"top", "tracks", "--limit", "51"},
		{"top", "tracks", "--offset", "-1"},
	} {
		env, f, out, errb := spotifyEnv(t)
		runFail(t, env, out, errb, 2, "usage", args...)
		if f.total() != 0 {
			t.Errorf("%v: a request was sent", args)
		}
	}
}

func TestHistoryNeedsItsScope(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	scopesWithout(t, env, auth.ScopeTopRead, auth.ScopeRecentlyPlayed)
	runFail(t, env, out, errb, 3, "scopes_missing", "top", "tracks")
	if !strings.Contains(errb.String(), "login") {
		t.Errorf("the message must say to log in again: %q", errb.String())
	}
	out.Reset()
	errb.Reset()
	runFail(t, env, out, errb, 3, "scopes_missing", "recent")
	if f.total() != 0 {
		t.Errorf("%d requests were sent without the scope", f.total())
	}
}

func TestHistoryWithOnlyOneOfTheScopes(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	scopesWithout(t, env, auth.ScopeRecentlyPlayed)
	f.reply("GET /me/top/tracks", 200, `{"total":0,"items":[]}`)
	runOK(t, env, out, errb, "top", "tracks")
	out.Reset()
	errb.Reset()
	runFail(t, env, out, errb, 3, "scopes_missing", "recent")
	if f.total() != 1 {
		t.Errorf("only top should have reached Spotify, got %d requests", f.total())
	}
}

func TestHistoryNotLoggedIn(t *testing.T) {
	for _, args := range [][]string{{"recent"}, {"top", "tracks"}} {
		env, _, out, errb := spotifyEnv(t)
		_ = os.Remove(tokenPath(env))
		runFail(t, env, out, errb, 3, "not_authenticated", args...)
	}
}

func TestHistoryRateLimited(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.on("GET /me/player/recently-played", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(429)
	})
	runFail(t, env, out, errb, 5, "rate_limited", "recent")
	if !strings.Contains(errb.String(), `"retry_after":12`) {
		t.Errorf("stderr %q", errb.String())
	}
	if f.total() != 1 {
		t.Errorf("a 429 must not be retried, %d requests", f.total())
	}
}
