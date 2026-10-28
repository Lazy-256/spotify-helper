package cli

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// playlistFixtures serves a playlist of n tracks, paged by limit and offset,
// with one local file at position localAt (-1 for none).
func playlistFixtures(f *fakeSpotify, n, localAt int) {
	f.reply("GET /playlists/pl", 200, `{"id":"pl","uri":"spotify:playlist:pl","name":"Mix","owner":{"display_name":"Me"},
	 "items":{"total":`+strconv.Itoa(n)+`},"images":[]}`)
	f.on("GET /playlists/pl/items", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		var items []string
		for i := offset; i < min(offset+limit, n); i++ {
			if i == localAt {
				items = append(items, `{"is_local":true,"item":{"uri":"spotify:local:a:b:c:1","name":"L","type":"track"}}`)
				continue
			}
			items = append(items, fmt.Sprintf(`{"is_local":false,"item":{"type":"track","id":"t%[1]d","uri":"spotify:track:t%[1]d","name":"S%[1]d","artists":[]}}`, i))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"total":%d,"items":[%s]}`, n, strings.Join(items, ","))
	})
}

func postedURIs(f *fakeSpotify) []string {
	var out []string
	for _, c := range f.callsTo("POST /me/player/queue") {
		out = append(out, c.Query.Get("uri"))
	}
	return out
}

func TestQueueAddAllAlbum(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	albumFixtures(f, 12, "")
	f.reply("POST /me/player/queue", 204, "")
	got := parse(t, runOK(t, env, out, errb, "queue-add-all", "spotify:album:alb"))
	want := map[string]any{"ok": true, "uri": "spotify:album:alb", "added": 12.0, "skipped": 0.0, "total": 12.0, "offset": 0.0, "truncated": false}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v (output %v)", k, got[k], v, got)
		}
	}
	posted := postedURIs(f)
	if len(posted) != 12 || posted[0] != "spotify:track:t1" || posted[11] != "spotify:track:t12" {
		t.Errorf("posted %v", posted)
	}
	if f.callsTo("POST /me/player/queue")[0].Query.Has("device_id") {
		t.Error("no device was given")
	}
}

func TestQueueAddAllOffsetAndDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /albums/alb", 200, `{"id":"alb","uri":"spotify:album:alb","name":"A","total_tracks":12,"artists":[],"images":[]}`)
	var items []string
	for i := 5; i <= 12; i++ {
		items = append(items, fmt.Sprintf(`{"id":"t%[1]d","uri":"spotify:track:t%[1]d","name":"S","artists":[]}`, i))
	}
	f.reply("GET /albums/alb/tracks", 200, `{"total":12,"items":[`+strings.Join(items, ",")+`]}`)
	f.reply("POST /me/player/queue", 204, "")
	got := parse(t, runOK(t, env, out, errb, "queue-add-all", "spotify:album:alb", "--offset", "4", "--device", "D1"))
	if got["added"] != 8.0 || got["total"] != 12.0 || got["offset"] != 4.0 {
		t.Errorf("output %v", got)
	}
	if c := f.callsTo("GET /albums/alb/tracks")[0]; c.Query.Get("offset") != "4" {
		t.Errorf("tracks query %v", c.Query)
	}
	calls := f.callsTo("POST /me/player/queue")
	if calls[0].Query.Get("uri") != "spotify:track:t5" || calls[0].Query.Get("device_id") != "D1" {
		t.Errorf("first call %+v", calls[0])
	}
}

func TestQueueAddAllPlaylistOverCap(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	playlistFixtures(f, 342, -1)
	f.reply("POST /me/player/queue", 204, "")
	got := parse(t, runOK(t, env, out, errb, "queue-add-all", "spotify:playlist:pl"))
	if got["added"] != 100.0 || got["total"] != 342.0 || got["truncated"] != true {
		t.Errorf("output %v", got)
	}
	if n := len(f.callsTo("POST /me/player/queue")); n != 100 {
		t.Errorf("%d posts, want 100", n)
	}
	if n := len(f.callsTo("GET /playlists/pl/items")); n != 2 {
		t.Errorf("%d page reads, want 2", n)
	}
}

func TestQueueAddAllSkipsLocalFiles(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	playlistFixtures(f, 5, 2)
	f.reply("POST /me/player/queue", 204, "")
	got := parse(t, runOK(t, env, out, errb, "queue-add-all", "spotify:playlist:pl"))
	if got["added"] != 4.0 || got["skipped"] != 1.0 || got["total"] != 5.0 {
		t.Errorf("output %v", got)
	}
	for _, u := range postedURIs(f) {
		if strings.Contains(u, "local") {
			t.Errorf("local file posted: %s", u)
		}
	}
}

func TestQueueAddAllWebLinkIsNormalized(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	playlistFixtures(f, 1, -1)
	f.reply("POST /me/player/queue", 204, "")
	got := parse(t, runOK(t, env, out, errb, "queue-add-all", "https://open.spotify.com/playlist/pl?si=xyz"))
	if got["uri"] != "spotify:playlist:pl" {
		t.Errorf("output %v", got)
	}
}

func TestQueueAddAllUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"queue-add-all", "spotify:track:abc"},
		{"queue-add-all", "spotify:episode:abc"},
		{"queue-add-all", "spotify:artist:abc"},
		{"queue-add-all", "nonsense"},
		{"queue-add-all"},
		{"queue-add-all", "spotify:album:abc", "--offset", "-1"},
		{"queue-add-all", "spotify:album:abc", "--offset", "x"},
	} {
		env, f, out, errb := spotifyEnv(t)
		runFail(t, env, out, errb, 2, "usage", args...)
		if f.total() != 0 {
			t.Errorf("%v: nothing may be sent to Spotify", args)
		}
	}
}

func TestQueueAddAllOffsetPastEnd(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /albums/alb", 200, `{"id":"alb","uri":"spotify:album:alb","name":"A","total_tracks":12,"artists":[],"images":[]}`)
	f.reply("GET /albums/alb/tracks", 200, `{"total":12,"items":[]}`)
	got := parse(t, runOK(t, env, out, errb, "queue-add-all", "spotify:album:alb", "--offset", "40"))
	if got["added"] != 0.0 || got["total"] != 12.0 || got["truncated"] != false || got["offset"] != 40.0 {
		t.Errorf("output %v", got)
	}
	if len(f.callsTo("POST /me/player/queue")) != 0 {
		t.Error("nothing to queue")
	}
}

func TestQueueAddAllTracksUnavailable(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /playlists/pl", 200, `{"id":"pl","uri":"spotify:playlist:pl","name":"Theirs","owner":{"display_name":"Them"},"images":[]}`)
	runFail(t, env, out, errb, 1, "tracks_unavailable", "queue-add-all", "spotify:playlist:pl")
	if len(f.callsTo("POST /me/player/queue")) != 0 {
		t.Error("nothing may be queued")
	}
	if strings.Contains(errb.String(), `"added"`) {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestQueueAddAllRateLimitedMidway(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	albumFixtures(f, 30, "")
	n := 0
	f.on("POST /me/player/queue", func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 9 {
			w.Header().Set("Retry-After", "12")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(204)
	})
	runFail(t, env, out, errb, 5, "rate_limited", "queue-add-all", "spotify:album:alb")
	e := parse(t, errb.String())["error"].(map[string]any)
	if e["retry_after"] != 12.0 || e["added"] != 8.0 || e["total"] != 30.0 {
		t.Errorf("error %v", e)
	}
	if got := len(f.callsTo("POST /me/player/queue")); got != 9 {
		t.Errorf("%d posts, want 9 (eight accepted, the ninth refused, no tenth)", got)
	}

	// The cooldown is recorded: the next run stops at once, without counts.
	before := f.total()
	runFail(t, env, out, errb, 5, "rate_limited", "queue-add-all", "spotify:album:alb")
	if f.total() != before {
		t.Error("an active cooldown must not reach Spotify")
	}
	if strings.Contains(errb.String(), `"added"`) {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestQueueAddAllNoActiveDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	albumFixtures(f, 5, "")
	f.reply("POST /me/player/queue", 404, `{"error":{"status":404,"message":"Player command failed: No active device found","reason":"NO_ACTIVE_DEVICE"}}`)
	runFail(t, env, out, errb, 4, "no_active_device", "queue-add-all", "spotify:album:alb")
	if got := len(f.callsTo("POST /me/player/queue")); got != 1 {
		t.Errorf("%d posts, want 1", got)
	}
	if strings.Contains(errb.String(), `"added"`) {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestQueueAddAllNotLoggedIn(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	_ = removeFile(env.Getenv("XDG_CONFIG_HOME") + "/spotify-helper/token.json")
	runFail(t, env, out, errb, 3, "not_authenticated", "queue-add-all", "spotify:album:alb")
	if f.total() != 0 {
		t.Error("nothing may be sent without a token")
	}
}

func TestQueueAddAllNetworkError(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.srv.Close()
	runFail(t, env, out, errb, 7, "network_error", "queue-add-all", "spotify:album:alb")
}

func TestQueueAddAllHelp(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	got := runOK(t, env, out, errb, "queue-add-all", "--help")
	if !strings.Contains(got, "--offset") {
		t.Errorf("help %q", got)
	}
}
