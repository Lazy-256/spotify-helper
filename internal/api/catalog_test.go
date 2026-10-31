package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Lazy-256/spotify-helper/internal/output"
	"github.com/Lazy-256/spotify-helper/internal/spotifyuri"
)

func mustURI(t *testing.T, s string) spotifyuri.URI {
	t.Helper()
	u, err := spotifyuri.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

const albumHead = `{"id":"alb","uri":"spotify:album:alb","name":"The Album","release_date":"2021-05-01","total_tracks":12,
 "artists":[{"name":"Band","uri":"spotify:artist:b"}],
 "images":[{"url":"https://i/640","width":640},{"url":"https://i/300","width":300},{"url":"https://i/64","width":64}]}`

func albumRoutes(t *testing.T, tracks string, seen *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.URL.RequestURI())
		switch r.URL.Path {
		case "/v1/albums/alb":
			jsonReply(w, 200, albumHead)
		case "/v1/albums/alb/tracks":
			jsonReply(w, 200, tracks)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(599)
		}
	}
}

func TestAlbum(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, albumRoutes(t, `{"total":12,"items":[
	  {"id":"t1","uri":"spotify:track:t1","name":"One","type":"track","duration_ms":100,"explicit":true,"artists":[{"name":"Band","uri":"spotify:artist:b"}]},
	  null,
	  {"id":"t2","uri":"spotify:track:t2","name":"Two","duration_ms":200,"artists":[{"name":"Band","uri":"spotify:artist:b"}]}]}`, &seen))

	res, err := c.Album(context.Background(), mustURI(t, "spotify:album:alb"), 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "/v1/albums/alb" || seen[1] != "/v1/albums/alb/tracks?limit=10&offset=10" {
		t.Errorf("requests: %v", seen)
	}
	if res.Album.URI != "spotify:album:alb" || res.Album.TotalTracks != 12 || res.Album.URL == nil || *res.Album.URL != "https://i/300" {
		t.Errorf("album: %+v", res.Album)
	}
	if res.Total != 12 || res.Offset != 10 || res.Limit != 10 {
		t.Errorf("paging: total %d offset %d limit %d", res.Total, res.Offset, res.Limit)
	}
	if len(res.Tracks) != 2 {
		t.Fatalf("null entries must be dropped, got %+v", res.Tracks)
	}
	for i, tr := range res.Tracks {
		if tr.Type != "track" || tr.Album.URI != "spotify:album:alb" || tr.Album.Name != "The Album" ||
			tr.URL == nil || *tr.URL != "https://i/300" || len(tr.Artists) != 1 {
			t.Errorf("track %d: %+v", i, tr)
		}
	}
	if res.Tracks[0].ID != "t1" || !res.Tracks[0].Explicit || res.Tracks[1].DurationMS != 200 {
		t.Errorf("tracks: %+v", res.Tracks)
	}
}

func TestAlbumWithoutTracks(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, albumRoutes(t, `{"total":0,"items":[]}`, &seen))
	res, err := c.Album(context.Background(), mustURI(t, "spotify:album:alb"), 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tracks == nil || len(res.Tracks) != 0 || res.Total != 0 {
		t.Errorf("want an empty, non-nil list: %+v", res)
	}
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), `"tracks":[]`) {
		t.Errorf("json: %s", b)
	}
}

func TestCatalogNotFound(t *testing.T) {
	c, n, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, 404, `{"error":{"status":404,"message":"Non existing id"}}`)
	})
	for name, call := range map[string]func() error{
		"album": func() error {
			_, err := c.Album(context.Background(), mustURI(t, "spotify:album:x"), 50, 0)
			return err
		},
		"artist": func() error {
			_, err := c.Artist(context.Background(), mustURI(t, "spotify:artist:x"), 50, 0)
			return err
		},
		"playlist": func() error {
			_, err := c.Playlist(context.Background(), mustURI(t, "spotify:playlist:x"), 50, 0)
			return err
		},
	} {
		before := n.Load()
		oe := asErr(t, call())
		if oe.Code != output.CodeAPIError || !strings.Contains(oe.Message, "404") {
			t.Errorf("%s: %+v", name, oe)
		}
		if n.Load()-before != 1 {
			t.Errorf("%s: a missing item must stop after the header request, sent %d", name, n.Load()-before)
		}
	}
}

func TestArtist(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		switch r.URL.Path {
		case "/v1/artists/art":
			jsonReply(w, 200, `{"id":"art","uri":"spotify:artist:art","name":"Band","images":[{"url":"https://i/a","width":320}],"followers":{"total":5},"popularity":9}`)
		case "/v1/artists/art/albums":
			jsonReply(w, 200, `{"total":25,"items":[null,
			  {"id":"a1","uri":"spotify:album:a1","name":"First","album_type":"album","release_date":"2019","total_tracks":9,
			   "artists":[{"name":"Band","uri":"spotify:artist:art"}],"images":[{"url":"https://i/1","width":300}]},
			  {"id":"a2","uri":"spotify:album:a2","name":"Single","album_type":"single","release_date":"2020-02-02","total_tracks":1,
			   "artists":[{"name":"Band","uri":"spotify:artist:art"}],"images":[]}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	})
	res, err := c.Artist(context.Background(), mustURI(t, "spotify:artist:art"), 20, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[1] != "/v1/artists/art/albums?include_groups=album%2Csingle&limit=20&offset=5" {
		t.Errorf("requests: %v", seen)
	}
	if res.Artist.URI != "spotify:artist:art" || res.Artist.Name != "Band" || *res.Artist.URL != "https://i/a" {
		t.Errorf("artist: %+v", res.Artist)
	}
	if res.Total != 25 || res.Offset != 5 || res.Limit != 20 || len(res.Albums) != 2 {
		t.Errorf("page: %+v", res)
	}
	if res.Albums[0].Name != "First" || res.Albums[0].TotalTracks != 9 || res.Albums[1].URL != nil || res.Albums[1].ReleaseDate != "2020-02-02" {
		t.Errorf("albums: %+v", res.Albums)
	}
	b, _ := json.Marshal(res)
	for _, banned := range []string{"followers", "popularity", "top_tracks"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("output must not contain %q: %s", banned, b)
		}
	}
}

func TestArtistWithoutReleases(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/artists/art" {
			jsonReply(w, 200, `{"id":"art","uri":"spotify:artist:art","name":"Band"}`)
			return
		}
		jsonReply(w, 200, `{"total":0,"items":[]}`)
	})
	res, err := c.Artist(context.Background(), mustURI(t, "spotify:artist:art"), 50, 0)
	if err != nil || res.Albums == nil || len(res.Albums) != 0 || res.Total != 0 || res.Artist.URL != nil {
		t.Errorf("res %+v err %v", res, err)
	}
}

const playlistHead = `{"id":"pl","uri":"spotify:playlist:pl","name":"Mix","collaborative":false,
 "owner":{"id":"me","display_name":"Me"},"images":[{"url":"https://i/pl","width":300}]`

const trackWire = `{"type":"track","id":"t1","uri":"spotify:track:t1","name":"Song","duration_ms":10,
 "artists":[{"name":"Band","uri":"spotify:artist:b"}],
 "album":{"name":"Alb","uri":"spotify:album:alb","images":[{"url":"https://i/alb","width":300}]}}`

func playlistServer(t *testing.T, head, items string, itemsStatus int, seen *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.URL.RequestURI())
		switch r.URL.Path {
		case "/v1/playlists/pl":
			jsonReply(w, 200, head)
		case "/v1/playlists/pl/items":
			jsonReply(w, itemsStatus, items)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}
}

func TestPlaylistOwn(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, playlistServer(t,
		playlistHead+`,"items":{"total":30,"href":"x"}}`,
		`{"total":30,"items":[{"is_local":false,"item":`+trackWire+`}]}`, 200, &seen))
	res, err := c.Playlist(context.Background(), mustURI(t, "spotify:playlist:pl"), 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || !strings.HasPrefix(seen[1], "/v1/playlists/pl/items?") ||
		!strings.Contains(seen[1], "limit=50") || !strings.Contains(seen[1], "offset=0") {
		t.Errorf("requests: %v", seen)
	}
	if !res.TracksAvailable || res.Total != 30 || len(res.Tracks) != 1 || res.Limit != 50 || res.Offset != 0 {
		t.Errorf("res: %+v", res)
	}
	p := res.Playlist
	if p.URI != "spotify:playlist:pl" || p.OwnerName != "Me" || p.Collaborative || *p.URL != "https://i/pl" {
		t.Errorf("playlist: %+v", p)
	}
	if tr := res.Tracks[0]; tr.Type != "track" || tr.URI != "spotify:track:t1" || tr.Album.URI != "spotify:album:alb" || *tr.URL != "https://i/alb" {
		t.Errorf("track: %+v", tr)
	}
}

func TestPlaylistWithoutItems(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, playlistServer(t, playlistHead+`,"collaborative":false}`, ``, 200, &seen))
	res, err := c.Playlist(context.Background(), mustURI(t, "spotify:playlist:pl"), 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 {
		t.Errorf("the items request must not be sent, requests: %v", seen)
	}
	if res.TracksAvailable || res.Tracks == nil || len(res.Tracks) != 0 || res.Total != 0 || res.Playlist.Name != "Mix" || res.Playlist.OwnerName != "Me" {
		t.Errorf("res: %+v", res)
	}
}

func TestPlaylistItemsNullIsAbsent(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, playlistServer(t, playlistHead+`,"items":null,"tracks":null}`, ``, 200, &seen))
	res, err := c.Playlist(context.Background(), mustURI(t, "spotify:playlist:pl"), 50, 0)
	if err != nil || res.TracksAvailable || len(seen) != 1 {
		t.Errorf("res %+v err %v requests %v", res, err, seen)
	}
}

func TestPlaylistItemsForbidden(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, playlistServer(t,
		playlistHead+`,"collaborative":true,"items":{"total":4}}`,
		`{"error":{"status":403,"message":"Forbidden"}}`, 403, &seen))
	res, err := c.Playlist(context.Background(), mustURI(t, "spotify:playlist:pl"), 50, 0)
	if err != nil {
		t.Fatalf("a 403 on the items is an answer, not an error: %v", err)
	}
	if res.TracksAvailable || len(res.Tracks) != 0 || res.Total != 0 || !res.Playlist.Collaborative || len(seen) != 2 {
		t.Errorf("res %+v requests %v", res, seen)
	}
}

func TestPlaylistItemsOtherFailureIsAnError(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, playlistServer(t, playlistHead+`,"items":{"total":4}}`, `{"error":{"message":"boom"}}`, 500, &seen))
	_, err := c.Playlist(context.Background(), mustURI(t, "spotify:playlist:pl"), 50, 0)
	if oe := asErr(t, err); oe.Code != output.CodeAPIError || !strings.Contains(oe.Message, "500") {
		t.Errorf("got %+v", oe)
	}
}

func TestPlaylistEntryKinds(t *testing.T) {
	var seen []string
	items := `{"total":6,"items":[
	  null,
	  {"is_local":false,"item":null},
	  {"is_local":true,"item":{"type":"track","uri":"spotify:local:a:b:c:1","name":"Local","artists":[],"album":{"name":"","uri":""}}},
	  {"is_local":false,"item":{"type":"episode","id":"e1","uri":"spotify:episode:e1","name":"Ep","duration_ms":3000,
	     "show":{"name":"Show","uri":"spotify:show:s1","images":[{"url":"https://i/show","width":300}]},"images":[]}},
	  {"is_local":false,"track":` + trackWire + `},
	  {"is_local":false,"item":{"type":"ad","uri":"spotify:ad:1","name":"Ad"}}]}`
	c, _, _ := newClient(t, playlistServer(t, playlistHead+`,"items":{"total":6}}`, items, 200, &seen))
	res, err := c.Playlist(context.Background(), mustURI(t, "spotify:playlist:pl"), 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 6 || len(res.Tracks) != 2 {
		t.Fatalf("want the episode and the track under the old key, got %+v", res.Tracks)
	}
	ep := res.Tracks[0]
	if ep.Type != "episode" || ep.URI != "spotify:episode:e1" || ep.Album.URI != "spotify:show:s1" || len(ep.Artists) != 0 || *ep.URL != "https://i/show" {
		t.Errorf("episode: %+v", ep)
	}
	if tr := res.Tracks[1]; tr.Type != "track" || tr.URI != "spotify:track:t1" {
		t.Errorf("track under the old key: %+v", tr)
	}
}

func TestPlaylistReadsOldTracksField(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, playlistServer(t, playlistHead+`,"tracks":{"total":1}}`,
		`{"total":1,"items":[{"track":`+trackWire+`}]}`, 200, &seen))
	res, err := c.Playlist(context.Background(), mustURI(t, "spotify:playlist:pl"), 50, 0)
	if err != nil || !res.TracksAvailable || len(res.Tracks) != 1 {
		t.Errorf("res %+v err %v", res, err)
	}
}

func TestPlayFromSendsContextAndOffset(t *testing.T) {
	c, n, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/me/player/play" || r.URL.Query().Get("device_id") != "D1" {
			t.Errorf("request %s %s", r.Method, r.URL)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		offset, _ := body["offset"].(map[string]any)
		if len(body) != 2 || body["context_uri"] != "spotify:album:alb" || len(offset) != 1 || offset["uri"] != "spotify:track:t7" {
			t.Errorf("body %v", body)
		}
		w.WriteHeader(204)
	})
	err := c.PlayFrom(context.Background(), mustURI(t, "spotify:track:t7"), mustURI(t, "spotify:album:alb"), "D1")
	if err != nil || n.Load() != 1 {
		t.Errorf("err %v requests %d", err, n.Load())
	}
}

const unplayableTracks = `{"total":3,"items":[
  {"id":"t1","uri":"spotify:track:t1","name":"One","type":"track","is_playable":true},
  {"id":"t2","uri":"spotify:track:t2","name":"Two","type":"track","is_playable":false},
  {"id":"t3","uri":"spotify:track:t3","name":"Three","type":"track"}]}`

func TestAlbumMarketFiltersOnlyMarkedFalse(t *testing.T) {
	var seen []string
	c, _, _ := newClient(t, albumRoutes(t, unplayableTracks, &seen))

	res, err := c.Album(context.Background(), mustURI(t, "spotify:album:alb"), 10, 0)
	if err != nil || len(res.Tracks) != 3 || strings.Contains(seen[1], "market") {
		t.Fatalf("plain Album: %v %+v %v", err, res.Tracks, seen)
	}

	res, err = c.album(context.Background(), mustURI(t, "spotify:album:alb"), 10, 0, "from_token")
	if err != nil || len(res.Tracks) != 2 || res.Tracks[0].ID != "t1" || res.Tracks[1].ID != "t3" {
		t.Fatalf("album with market: %v %+v", err, res.Tracks)
	}
	if !strings.Contains(seen[3], "market=from_token") || strings.Contains(seen[2], "market") {
		t.Errorf("requests: %v", seen)
	}
}

func TestPlaylistMarketFiltersOnlyMarkedFalse(t *testing.T) {
	var seen []string
	entry := func(mark string) string {
		return `{"is_local":false,"item":{"id":"t","uri":"spotify:track:t","name":"T","type":"track"` + mark + `}}`
	}
	items := `{"total":3,"items":[` + entry(`,"is_playable":true`) + `,` + entry(`,"is_playable":false`) + `,` + entry("") + `]}`
	c, _, _ := newClient(t, playlistServer(t, playlistHead+`,"items":{"total":3}}`, items, 200, &seen))

	res, err := c.Playlist(context.Background(), mustURI(t, "spotify:playlist:pl"), 50, 0)
	if err != nil || len(res.Tracks) != 3 || strings.Contains(seen[1], "market") {
		t.Fatalf("plain Playlist: %v %+v %v", err, res.Tracks, seen)
	}

	res, err = c.playlist(context.Background(), mustURI(t, "spotify:playlist:pl"), 50, 0, "from_token")
	if err != nil || len(res.Tracks) != 2 {
		t.Fatalf("playlist with market: %v %+v", err, res.Tracks)
	}
	if !strings.Contains(seen[3], "market=from_token") {
		t.Errorf("requests: %v", seen)
	}
}
