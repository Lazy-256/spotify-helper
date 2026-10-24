package cli

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

// albumFixtures serves an album of n tracks whose cover is coverURL.
func albumFixtures(f *fakeSpotify, n int, coverURL string) {
	f.reply("GET /albums/alb", 200, `{"id":"alb","uri":"spotify:album:alb","name":"The Album","release_date":"2021",
	 "total_tracks":`+fmt.Sprint(n)+`,"artists":[{"name":"Band","uri":"spotify:artist:b"}],
	 "images":[{"url":"`+coverURL+`","width":300}]}`)
	var items []string
	for i := 1; i <= n; i++ {
		items = append(items, fmt.Sprintf(`{"type":"track","id":"t%[1]d","uri":"spotify:track:t%[1]d","name":"Song %[1]d",
		 "duration_ms":1000,"artists":[{"name":"Band","uri":"spotify:artist:b"}]}`, i))
	}
	f.reply("GET /albums/alb/tracks", 200, fmt.Sprintf(`{"total":%d,"items":[%s]}`, n, strings.Join(items, ",")))
}

func TestAlbumCommand(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	albumFixtures(f, 12, "https://i.scdn.co/alb")
	res := parse(t, runOK(t, env, out, errb, "album", "spotify:album:alb"))

	album := res["album"].(map[string]any)
	if album["uri"] != "spotify:album:alb" || album["name"] != "The Album" || album["total_tracks"] != float64(12) || album["cover_url"] != "https://i.scdn.co/alb" {
		t.Errorf("album: %v", album)
	}
	tracks := res["tracks"].([]any)
	if res["total"] != float64(12) || res["offset"] != float64(0) || res["limit"] != float64(50) || len(tracks) != 12 {
		t.Errorf("paging: %v", res)
	}
	first := tracks[0].(map[string]any)
	if first["uri"] != "spotify:track:t1" || first["type"] != "track" || first["cover_url"] != "https://i.scdn.co/alb" ||
		first["album"].(map[string]any)["uri"] != "spotify:album:alb" {
		t.Errorf("first track: %v", first)
	}
	if _, has := first["cover_path"]; has {
		t.Errorf("cover_path without --covers: %v", first)
	}
	if c := f.callsTo("GET /albums/alb/tracks")[0]; c.Query.Get("limit") != "50" || c.Query.Get("offset") != "0" {
		t.Errorf("default paging sent: %v", c.Query)
	}
}

func TestAlbumSecondPageAndLink(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	albumFixtures(f, 2, "")
	f.reply("GET /albums/alb/tracks", 200, `{"total":20,"items":[{"type":"track","id":"t11","uri":"spotify:track:t11","name":"Eleven","artists":[]}]}`)
	res := parse(t, runOK(t, env, out, errb, "album", "https://open.spotify.com/intl-nl/album/alb?si=xyz", "--limit", "10", "--offset", "10"))
	if res["limit"] != float64(10) || res["offset"] != float64(10) || res["total"] != float64(20) || len(res["tracks"].([]any)) != 1 {
		t.Errorf("res: %v", res)
	}
	if c := f.callsTo("GET /albums/alb/tracks")[0]; c.Query.Get("limit") != "10" || c.Query.Get("offset") != "10" {
		t.Errorf("query: %v", c.Query)
	}
}

func TestCatalogUsageErrors(t *testing.T) {
	cases := [][]string{
		{"album", "spotify:playlist:abc"}, {"album", "hello"}, {"album"}, {"album", "spotify:album:a", "spotify:album:b"},
		{"playlist", "spotify:album:abc"}, {"playlist", "spotify:artist:abc"},
		{"artist", "spotify:track:abc"}, {"artist", "https://open.spotify.com/album/abc"},
		{"artist", "spotify:artist:abc", "--limit", "0"}, {"artist", "spotify:artist:abc", "--limit", "51"},
		{"album", "spotify:album:abc", "--limit", "many"}, {"playlist", "spotify:playlist:abc", "--offset", "-1"},
	}
	for _, args := range cases {
		env, f, out, errb := spotifyEnv(t)
		runFail(t, env, out, errb, 2, "usage", args...)
		if f.total() != 0 {
			t.Errorf("%v: a request was sent", args)
		}
	}
}

func TestCatalogNotLoggedIn(t *testing.T) {
	for _, args := range [][]string{{"album", "spotify:album:abc"}, {"playlist", "spotify:playlist:abc"}, {"artist", "spotify:artist:abc"}} {
		env, _, out, errb := spotifyEnv(t)
		_ = os.Remove(env.Getenv("XDG_CONFIG_HOME") + "/spotify-helper/token.json")
		runFail(t, env, out, errb, 3, "not_authenticated", args...)
	}
}

func TestCatalogRateLimited(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.on("GET /albums/alb", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(429)
	})
	runFail(t, env, out, errb, 5, "rate_limited", "album", "spotify:album:alb")
	if !strings.Contains(errb.String(), `"retry_after":12`) {
		t.Errorf("stderr %q", errb.String())
	}
	if f.total() != 1 {
		t.Errorf("a 429 must not be retried, %d requests", f.total())
	}
}

func TestCatalogNotFound(t *testing.T) {
	for _, tc := range [][2]string{{"album", "spotify:album:nope"}, {"playlist", "spotify:playlist:nope"}, {"artist", "spotify:artist:nope"}} {
		env, f, out, errb := spotifyEnv(t)
		f.reply("GET /"+tc[0]+"s/nope", 404, `{"error":{"status":404,"message":"Non existing id"}}`)
		runFail(t, env, out, errb, 1, "api_error", tc[0], tc[1])
		if !strings.Contains(errb.String(), "404") {
			t.Errorf("%s: message should include the status: %q", tc[0], errb.String())
		}
	}
}

func TestAlbumCovers(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	srv, hits := coverServer(t, env)
	albumFixtures(f, 12, srv.URL+"/image/alb")

	res := parse(t, runOK(t, env, out, errb, "album", "spotify:album:alb", "--covers"))
	path, _ := res["album"].(map[string]any)["cover_path"].(string)
	if path == "" {
		t.Fatalf("album: %v", res["album"])
	}
	tracks := res["tracks"].([]any)
	if len(tracks) != 12 {
		t.Fatalf("tracks: %d", len(tracks))
	}
	for i, tr := range tracks {
		if got := tr.(map[string]any)["cover_path"]; got != path {
			t.Errorf("track %d cover_path %v, want %s", i, got, path)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("the album cover must be downloaded once, got %d downloads", hits.Load())
	}
}

func TestArtistCommand(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	srv, _ := coverServer(t, env)
	f.reply("GET /artists/art", 200, `{"id":"art","uri":"spotify:artist:art","name":"Band","images":[{"url":"`+srv.URL+`/image/art","width":320}]}`)
	f.reply("GET /artists/art/albums", 200, `{"total":25,"items":[null,
	 {"id":"a1","uri":"spotify:album:a1","name":"First","release_date":"2019","total_tracks":9,"artists":[],"images":[{"url":"`+srv.URL+`/image/a1","width":300}]}]}`)

	res := parse(t, runOK(t, env, out, errb, "artist", "spotify:artist:art", "--covers", "--limit", "8", "--offset", "5"))
	if res["total"] != float64(25) || res["limit"] != float64(8) || res["offset"] != float64(5) {
		t.Errorf("paging: %v", res)
	}
	albums := res["albums"].([]any)
	if len(albums) != 1 || albums[0].(map[string]any)["uri"] != "spotify:album:a1" {
		t.Errorf("albums: %v", albums)
	}
	for name, e := range map[string]map[string]any{"artist": res["artist"].(map[string]any), "album": albums[0].(map[string]any)} {
		if p, _ := e["cover_path"].(string); p == "" {
			t.Errorf("%s without cover_path: %v", name, e)
		}
	}
	c := f.callsTo("GET /artists/art/albums")[0]
	if c.Query.Get("include_groups") != "album,single" || c.Query.Get("limit") != "8" || c.Query.Get("offset") != "5" {
		t.Errorf("query: %v", c.Query)
	}
}

func TestArtistWithoutReleases(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /artists/art", 200, `{"id":"art","uri":"spotify:artist:art","name":"Band"}`)
	f.reply("GET /artists/art/albums", 200, `{"total":0,"items":[]}`)
	got := runOK(t, env, out, errb, "artist", "spotify:artist:art")
	want := `{"artist":{"uri":"spotify:artist:art","id":"art","name":"Band","cover_url":null},"albums":[],"total":0,"offset":0,"limit":10}` + "\n"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if c := f.callsTo("GET /artists/art/albums")[0]; c.Query.Get("limit") != "10" {
		t.Errorf("the default page of an artist must be 10 (Spotify refuses more): %v", c.Query)
	}
}

// Spotify answers HTTP 400 to more than 10 albums of an artist in Development
// Mode (seen on a real account), so every larger limit is reduced to 10.
func TestArtistLimitIsReducedToTen(t *testing.T) {
	for _, limit := range []string{"11", "20", "50"} {
		env, f, out, errb := spotifyEnv(t)
		f.reply("GET /artists/art", 200, `{"id":"art","uri":"spotify:artist:art","name":"Band"}`)
		f.reply("GET /artists/art/albums", 200, `{"total":33,"items":[]}`)
		res := parse(t, runOK(t, env, out, errb, "artist", "spotify:artist:art", "--limit", limit))
		if res["limit"] != float64(10) {
			t.Errorf("--limit %s: echoed %v, want 10", limit, res["limit"])
		}
		if c := f.callsTo("GET /artists/art/albums")[0]; c.Query.Get("limit") != "10" {
			t.Errorf("--limit %s: sent %v, want 10", limit, c.Query)
		}
	}
}

func TestPlaylistCommand(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /playlists/pl", 200, `{"id":"pl","uri":"spotify:playlist:pl","name":"Mix","collaborative":true,
	 "owner":{"id":"me","display_name":"Me"},"images":[],"items":{"total":2}}`)
	f.reply("GET /playlists/pl/items", 200, `{"total":2,"items":[
	 null,
	 {"is_local":false,"item":{"type":"track","id":"t1","uri":"spotify:track:t1","name":"Song","artists":[],"album":{"name":"A","uri":"spotify:album:a"}}},
	 {"is_local":false,"item":{"type":"episode","id":"e1","uri":"spotify:episode:e1","name":"Ep","show":{"name":"S","uri":"spotify:show:s"}}}]}`)

	res := parse(t, runOK(t, env, out, errb, "playlist", "https://open.spotify.com/playlist/pl?si=1"))
	pl := res["playlist"].(map[string]any)
	if pl["uri"] != "spotify:playlist:pl" || pl["owner_name"] != "Me" || pl["collaborative"] != true || res["tracks_available"] != true {
		t.Errorf("playlist: %v", res)
	}
	tracks := res["tracks"].([]any)
	if len(tracks) != 2 || tracks[1].(map[string]any)["type"] != "episode" || res["total"] != float64(2) {
		t.Errorf("tracks: %v", tracks)
	}
}

func TestPlaylistTracksNotAvailable(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /playlists/pl", 200, `{"id":"pl","uri":"spotify:playlist:pl","name":"Theirs","owner":{"id":"x","display_name":"Someone"},
	 "images":[{"url":"https://i.scdn.co/pl","width":300}]}`)
	got := runOK(t, env, out, errb, "playlist", "spotify:playlist:pl")
	want := `{"playlist":{"uri":"spotify:playlist:pl","id":"pl","name":"Theirs","owner_name":"Someone","cover_url":"https://i.scdn.co/pl","collaborative":false},` +
		`"tracks_available":false,"tracks":[],"total":0,"offset":0,"limit":50}` + "\n"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if len(f.callsTo("GET /playlists/pl/items")) != 0 {
		t.Error("no items request when the playlist has no items")
	}
}

func TestPlaylistItemsForbidden(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /playlists/pl", 200, `{"id":"pl","uri":"spotify:playlist:pl","name":"P","collaborative":true,"items":{"total":9}}`)
	f.reply("GET /playlists/pl/items", 403, `{"error":{"status":403,"message":"Forbidden"}}`)
	res := parse(t, runOK(t, env, out, errb, "playlist", "spotify:playlist:pl"))
	if res["tracks_available"] != false || res["total"] != float64(0) || len(res["tracks"].([]any)) != 0 || res["playlist"].(map[string]any)["collaborative"] != true {
		t.Errorf("res: %v", res)
	}
}

// ---- play-uri --context ----

func TestPlayURIContext(t *testing.T) {
	for _, tc := range []struct{ context, want string }{
		{"spotify:album:abc", "spotify:album:abc"},
		{"https://open.spotify.com/playlist/pl1?si=x", "spotify:playlist:pl1"},
	} {
		env, f, out, errb := spotifyEnv(t)
		f.reply("PUT /me/player/play", 204, "")
		if got := runOK(t, env, out, errb, "play-uri", "spotify:track:t7", "--context", tc.context); got != "{\"ok\":true}\n" {
			t.Errorf("%s: %q", tc.context, got)
		}
		c := f.callsTo("PUT /me/player/play")[0]
		body := c.json(t)
		offset, _ := body["offset"].(map[string]any)
		if len(body) != 2 || body["context_uri"] != tc.want || len(offset) != 1 || offset["uri"] != "spotify:track:t7" {
			t.Errorf("%s: body %v", tc.context, body)
		}
		if f.total() != 1 {
			t.Errorf("one request expected, got %d", f.total())
		}
	}
}

func TestPlayURIContextWithItemLinkAndDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/player/play", 204, "")
	runOK(t, env, out, errb, "play-uri", "--context", "spotify:album:abc", "https://open.spotify.com/episode/e1", "--device", "D1")
	c := f.callsTo("PUT /me/player/play")[0]
	if c.Query.Get("device_id") != "D1" || c.json(t)["offset"].(map[string]any)["uri"] != "spotify:episode:e1" {
		t.Errorf("call %+v", c)
	}
}

func TestPlayURIContextUsageErrors(t *testing.T) {
	cases := [][]string{
		{"play-uri", "spotify:track:t7", "--context", "spotify:artist:a1"},
		{"play-uri", "spotify:track:t7", "--context", "spotify:show:s1"},
		{"play-uri", "spotify:track:t7", "--context", "spotify:track:t8"},
		{"play-uri", "spotify:track:t7", "--context", "hello"},
		{"play-uri", "spotify:track:t7", "--context", ""},
		{"play-uri", "spotify:album:abc", "--context", "spotify:playlist:pl1"},
		{"play-uri", "spotify:artist:abc", "--context", "spotify:album:abc"},
		{"play-uri", "hello", "--context", "spotify:album:abc"},
		{"play-uri", "spotify:track:t7", "--context"},
	}
	for _, args := range cases {
		env, f, out, errb := spotifyEnv(t)
		runFail(t, env, out, errb, 2, "usage", args...)
		if f.total() != 0 {
			t.Errorf("%v: a request was sent", args)
		}
	}
}

func TestPlayURIContextSpotifyRefuses(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/player/play", 400, `{"error":{"status":400,"message":"Item not in context"}}`)
	runFail(t, env, out, errb, 1, "api_error", "play-uri", "spotify:track:t7", "--context", "spotify:album:abc")
}
