package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func trackJSON(n int) string {
	return fmt.Sprintf(`{"type":"track","id":"T%[1]d","uri":"spotify:track:T%[1]d","name":"Track %[1]d","duration_ms":1000,"explicit":true,
	  "artists":[{"name":"Daft Punk","uri":"spotify:artist:A"}],
	  "album":{"name":"Discovery","uri":"spotify:album:B","images":[{"url":"https://i.scdn.co/a%[1]d","width":300}]}}`, n)
}

const searchFixture = `{
 "tracks":{"items":[%s]},
 "albums":{"items":[{"id":"B","uri":"spotify:album:B","name":"Discovery","release_date":"2001-03-12","total_tracks":14,
   "artists":[{"name":"Daft Punk","uri":"spotify:artist:A"}],"images":[{"url":"https://i.scdn.co/alb","width":640}]}]},
 "artists":{"items":[{"id":"A","uri":"spotify:artist:A","name":"Daft Punk","images":[]}]},
 "playlists":{"items":[null,{"id":"P","uri":"spotify:playlist:P","name":"This Is Daft Punk","owner":{"display_name":"Spotify","id":"spotify"},"images":[{"url":"https://i.scdn.co/pl"}]},null]}
}`

func searchEnv(t *testing.T) (*Env, *fakeSpotify, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /search", 200, fmt.Sprintf(searchFixture, trackJSON(1)+","+trackJSON(2)))
	return env, f, out, errb
}

func TestSearchDefaultAllTypes(t *testing.T) {
	env, f, out, errb := searchEnv(t)
	raw := runOK(t, env, out, errb, "search", "daft", "punk")
	res := parse(t, raw)

	if res["query"] != "daft punk" {
		t.Errorf("query %v", res["query"])
	}
	for _, k := range []string{"tracks", "albums", "artists", "playlists"} {
		if _, ok := res[k].([]any); !ok {
			t.Errorf("%s missing or not an array: %v", k, res[k])
		}
	}
	c := f.callsTo("GET /search")[0]
	if c.Query.Get("q") != "daft punk" || c.Query.Get("type") != "track,album,artist,playlist" || c.Query.Get("limit") != "10" {
		t.Errorf("request query %v", c.Query)
	}

	tr := res["tracks"].([]any)[0].(map[string]any)
	if tr["uri"] != "spotify:track:T1" || tr["name"] != "Track 1" || tr["explicit"] != true ||
		tr["duration_ms"] != float64(1000) || tr["cover_url"] != "https://i.scdn.co/a1" ||
		tr["album"].(map[string]any)["name"] != "Discovery" || tr["artists"].([]any)[0].(map[string]any)["name"] != "Daft Punk" {
		t.Errorf("track: %v", tr)
	}
	al := res["albums"].([]any)[0].(map[string]any)
	if al["release_date"] != "2001-03-12" || al["total_tracks"] != float64(14) || al["uri"] != "spotify:album:B" {
		t.Errorf("album: %v", al)
	}
	ar := res["artists"].([]any)[0].(map[string]any)
	if ar["name"] != "Daft Punk" || ar["cover_url"] != nil || !strings.Contains(raw, `"cover_url":null`) {
		t.Errorf("artist without images must have null cover_url: %v", ar)
	}
}

func TestSearchDropsNullEntries(t *testing.T) {
	env, _, out, errb := searchEnv(t)
	res := parse(t, runOK(t, env, out, errb, "search", "x"))
	pls := res["playlists"].([]any)
	if len(pls) != 1 {
		t.Fatalf("null playlists must be dropped, got %v", pls)
	}
	pl := pls[0].(map[string]any)
	if pl["name"] != "This Is Daft Punk" || pl["owner_name"] != "Spotify" || pl["uri"] != "spotify:playlist:P" ||
		pl["cover_url"] != "https://i.scdn.co/pl" {
		t.Errorf("playlist: %v", pl)
	}
}

func TestSearchOnlyRequestedTypes(t *testing.T) {
	env, f, out, errb := searchEnv(t)
	res := parse(t, runOK(t, env, out, errb, "search", "daft", "punk", "--type", "artist"))
	if _, ok := res["artists"]; !ok {
		t.Error("artists missing")
	}
	for _, k := range []string{"tracks", "albums", "playlists"} {
		if _, ok := res[k]; ok {
			t.Errorf("%s must be absent when not requested", k)
		}
	}
	if got := f.callsTo("GET /search")[0].Query.Get("type"); got != "artist" {
		t.Errorf("type sent to Spotify: %q", got)
	}

	// Canonical order, case and spacing do not matter.
	runOK(t, env, out, errb, "search", "x", "--type", "Playlist, track")
	if got := f.callsTo("GET /search")[1].Query.Get("type"); got != "track,playlist" {
		t.Errorf("type sent to Spotify: %q", got)
	}
}

func TestSearchRequestedTypeWithNoMatchesIsEmptyArray(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /search", 200, `{"tracks":{"items":[]}}`)
	raw := runOK(t, env, out, errb, "search", "zzz", "--type", "track,album")
	if !strings.Contains(raw, `"tracks":[]`) || !strings.Contains(raw, `"albums":[]`) {
		t.Errorf("want empty arrays, got %s", raw)
	}
}

func TestSearchLimit(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	var items []string
	for i := 1; i <= 15; i++ { // a misbehaving server returning more than asked
		items = append(items, trackJSON(i))
	}
	f.reply("GET /search", 200, fmt.Sprintf(searchFixture, strings.Join(items, ",")))

	res := parse(t, runOK(t, env, out, errb, "search", "x", "--limit", "50"))
	if got := f.callsTo("GET /search")[0].Query.Get("limit"); got != "10" {
		t.Errorf("limit sent to Spotify: %q, want 10", got)
	}
	if n := len(res["tracks"].([]any)); n != 10 {
		t.Errorf("%d tracks, want at most 10", n)
	}

	runOK(t, env, out, errb, "search", "x", "--limit", "3")
	if got := f.callsTo("GET /search")[1].Query.Get("limit"); got != "3" {
		t.Errorf("limit sent to Spotify: %q, want 3", got)
	}
}

func TestSearchUsageErrors(t *testing.T) {
	env, f, out, errb := searchEnv(t)
	runFail(t, env, out, errb, 2, "usage", "search")
	runFail(t, env, out, errb, 2, "usage", "search", "  ")
	runFail(t, env, out, errb, 2, "usage", "search", "x", "--type", "podcast")
	runFail(t, env, out, errb, 2, "usage", "search", "x", "--limit", "0")
	runFail(t, env, out, errb, 2, "usage", "search", "x", "--limit", "-4")
	runFail(t, env, out, errb, 2, "usage", "search", "x", "--limit", "many")
	if f.total() != 0 {
		t.Errorf("usage errors must not reach Spotify (%d requests)", f.total())
	}
}

func TestSearchNotLoggedIn(t *testing.T) {
	env, out, errb := testEnv(t)
	_ = out
	if code := Run([]string{"search", "x"}, env); code != 3 || errorCode(t, errb) != "not_configured" {
		t.Errorf("exit %d stderr %q", code, errb.String())
	}
}

func TestSearchResultIsPlayable(t *testing.T) {
	env, f, out, errb := searchEnv(t)
	res := parse(t, runOK(t, env, out, errb, "search", "x"))
	f.reply("PUT /me/player/play", 204, "")
	for _, k := range []string{"tracks", "albums", "artists", "playlists"} {
		uri := res[k].([]any)[0].(map[string]any)["uri"].(string)
		runOK(t, env, out, errb, "play-uri", uri)
	}
}
