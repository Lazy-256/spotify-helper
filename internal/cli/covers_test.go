package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// coverServer serves images over TLS (cover URLs must be https). Paths ending
// in /bad fail. It is wired into env.CoverHTTP.
func coverServer(t *testing.T, env *Env) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Errorf("credentials sent to the image server: %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/bad") {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8image" + r.URL.Path))
	}))
	t.Cleanup(srv.Close)
	env.CoverHTTP = srv.Client()
	return srv, &hits
}

func coverDir(env *Env) string {
	return filepath.Join(env.Getenv("XDG_CACHE_HOME"), "spotify-helper", "covers")
}

func TestCoverCommand(t *testing.T) {
	env, out, errb := testEnv(t) // no login needed: the URL is public
	srv, hits := coverServer(t, env)
	url := srv.URL + "/image/abc"

	first := parse(t, runOK(t, env, out, errb, "cover", url))
	path, _ := first["path"].(string)
	if first["ok"] != true || first["url"] != url || first["cached"] != false || !filepath.IsAbs(path) {
		t.Fatalf("first: %v", first)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "/image/abc") {
		t.Errorf("file: %q %v", data, err)
	}
	if filepath.Dir(path) != coverDir(env) {
		t.Errorf("path %q is outside the cover cache %q", path, coverDir(env))
	}

	second := parse(t, runOK(t, env, out, errb, "cover", url))
	if second["cached"] != true || second["path"] != path || hits.Load() != 1 {
		t.Errorf("second: %v after %d hits", second, hits.Load())
	}
}

func TestCoverCommandErrors(t *testing.T) {
	env, out, errb := testEnv(t)
	srv, hits := coverServer(t, env)
	runFail(t, env, out, errb, 2, "usage", "cover")
	runFail(t, env, out, errb, 2, "usage", "cover", "http://example.com/a.jpg")
	runFail(t, env, out, errb, 2, "usage", "cover", "a", "b")
	if hits.Load() != 0 {
		t.Error("usage errors must not download anything")
	}
	runFail(t, env, out, errb, 1, "api_error", "cover", srv.URL+"/bad")
	if entries, _ := os.ReadDir(coverDir(env)); len(entries) != 0 {
		t.Errorf("a failed download must leave nothing behind: %v", entries)
	}
}

func playerWithCover(url string) string {
	return `{"device":{"id":"D","name":"n","type":"Speaker","is_active":true},"is_playing":true,"progress_ms":1,
	 "item":{"type":"track","id":"T","uri":"spotify:track:T","name":"S","duration_ms":10,"artists":[],
	 "album":{"name":"A","uri":"spotify:album:B","images":[{"url":"` + url + `","width":300}]}}}`
}

func TestStateWithCovers(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	srv, hits := coverServer(t, env)
	f.reply("GET /me/player", 200, playerWithCover(srv.URL+"/image/now"))

	item := parse(t, runOK(t, env, out, errb, "state", "--covers"))["item"].(map[string]any)
	path, _ := item["cover_path"].(string)
	if path == "" || item["cover_url"] != srv.URL+"/image/now" {
		t.Fatalf("item: %v", item)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("cover_path must point at a real file: %v", err)
	}

	// Warm cache: the second poll costs no image request.
	before := hits.Load()
	item = parse(t, runOK(t, env, out, errb, "state", "--covers"))["item"].(map[string]any)
	if item["cover_path"] != path || hits.Load() != before {
		t.Errorf("warm cache: path %v, %d new image requests", item["cover_path"], hits.Load()-before)
	}

	// Without the flag nothing is added and nothing downloaded.
	before = hits.Load()
	item = parse(t, runOK(t, env, out, errb, "state"))["item"].(map[string]any)
	if _, has := item["cover_path"]; has || hits.Load() != before {
		t.Errorf("flag omitted: %v", item)
	}
}

func TestSearchWithCoversOneFails(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	srv, _ := coverServer(t, env)
	f.reply("GET /search", 200, `{
	 "tracks":{"items":[
	  {"type":"track","id":"1","uri":"spotify:track:1","name":"One","artists":[],"album":{"name":"A","uri":"spotify:album:A","images":[{"url":"`+srv.URL+`/image/one","width":300}]}},
	  {"type":"track","id":"2","uri":"spotify:track:2","name":"Two","artists":[],"album":{"name":"B","uri":"spotify:album:B","images":[{"url":"`+srv.URL+`/image/bad","width":300}]}},
	  {"type":"track","id":"3","uri":"spotify:track:3","name":"Three","artists":[],"album":{"name":"C","uri":"spotify:album:C","images":[]}}]},
	 "artists":{"items":[{"id":"A","uri":"spotify:artist:A","name":"Artist","images":[{"url":"`+srv.URL+`/image/one","width":300}]}]}}`)

	res := parse(t, runOK(t, env, out, errb, "search", "x", "--type", "track,artist", "--covers"))
	tracks := res["tracks"].([]any)
	one, two, three := tracks[0].(map[string]any), tracks[1].(map[string]any), tracks[2].(map[string]any)
	if p, _ := one["cover_path"].(string); p == "" {
		t.Errorf("good cover: %v", one)
	}
	for name, tr := range map[string]map[string]any{"failed download": two, "no image": three} {
		v, has := tr["cover_path"]
		if !has || v != nil {
			t.Errorf("%s: cover_path must be present and null, got %v (present=%v)", name, v, has)
		}
	}
	artist := res["artists"].([]any)[0].(map[string]any)
	if artist["cover_path"] != one["cover_path"] {
		t.Errorf("the same image shares one cached file: %v vs %v", artist["cover_path"], one["cover_path"])
	}
}

func TestQueueWithCovers(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	srv, _ := coverServer(t, env)
	f.reply("GET /me/player/queue", 200, `{"currently_playing":null,"queue":[
	 {"type":"track","id":"1","uri":"spotify:track:1","name":"One","artists":[],"album":{"name":"A","uri":"spotify:album:A","images":[{"url":"`+srv.URL+`/image/q1","width":300}]}}]}`)
	q := parse(t, runOK(t, env, out, errb, "queue", "--covers"))
	if q["now_playing"] != nil {
		t.Errorf("now_playing: %v", q["now_playing"])
	}
	if p, _ := q["queue"].([]any)[0].(map[string]any)["cover_path"].(string); p == "" {
		t.Errorf("queue item without cover_path: %v", q["queue"])
	}
}

func TestCoversDoNotUseTheCredentialedClient(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	// CoverHTTP stays the default test value (nil is replaced by the real
	// client); an image URL pointing at a plain http server is refused.
	f.reply("GET /me/player", 200, playerWithCover("http://127.0.0.1:1/image"))
	item := parse(t, runOK(t, env, out, errb, "state", "--covers"))["item"].(map[string]any)
	if v, has := item["cover_path"]; !has || v != nil {
		t.Errorf("insecure cover URLs must give null, got %v", v)
	}
}
