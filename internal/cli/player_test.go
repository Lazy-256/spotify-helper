package cli

import (
	"strconv"
	"strings"
	"testing"
)

const playerJSON = `{
 "device":{"id":"D1","name":"la-box","type":"Speaker","volume_percent":93,"is_active":true,"is_restricted":false},
 "repeat_state":"track","shuffle_state":true,
 "context":{"type":"playlist","uri":"spotify:playlist:P"},
 "progress_ms":250343,"is_playing":%s,
 "item":{"type":"track","id":"T","uri":"spotify:track:T","name":"Veridis Quo","duration_ms":345186,"explicit":false,
  "artists":[{"name":"Daft Punk","uri":"spotify:artist:A"}],
  "album":{"name":"Discovery","uri":"spotify:album:B","images":[{"url":"https://i.scdn.co/image/640","width":640},{"url":"https://i.scdn.co/image/300","width":300}]}}}`

func player(playing string) string { return strings.Replace(playerJSON, "%s", playing, 1) }

// ---- 5.1 state ----

func TestStateTrackPlaying(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 200, player("true"))
	st := parse(t, runOK(t, env, out, errb, "state"))

	if st["active"] != true || st["is_playing"] != true || st["shuffle"] != true || st["repeat"] != "track" {
		t.Errorf("flags: %v", st)
	}
	if st["progress_ms"] != float64(250343) || st["fetched_at_ms"] != float64(1_700_000_000_000) {
		t.Errorf("progress/fetched: %v %v", st["progress_ms"], st["fetched_at_ms"])
	}
	dev := st["device"].(map[string]any)
	if dev["id"] != "D1" || dev["name"] != "la-box" || dev["type"] != "Speaker" || dev["volume_percent"] != float64(93) {
		t.Errorf("device: %v", dev)
	}
	if ctx := st["context"].(map[string]any); ctx["type"] != "playlist" || ctx["uri"] != "spotify:playlist:P" {
		t.Errorf("context: %v", ctx)
	}
	item := st["item"].(map[string]any)
	if item["type"] != "track" || item["name"] != "Veridis Quo" || item["duration_ms"] != float64(345186) ||
		item["cover_url"] != "https://i.scdn.co/image/300" {
		t.Errorf("item: %v", item)
	}
	if _, has := item["cover_path"]; has {
		t.Error("cover_path must be absent without --covers")
	}
	if got := f.callsTo("GET /me/player")[0].Query.Get("additional_types"); got != "track,episode" {
		t.Errorf("additional_types = %q (episodes would be missing)", got)
	}
}

func TestStatePaused(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 200, player("false"))
	st := parse(t, runOK(t, env, out, errb, "state"))
	if st["active"] != true || st["is_playing"] != false || st["item"] == nil {
		t.Errorf("paused state: %v", st)
	}
}

func TestStateNothingActive(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 204, "")
	raw := runOK(t, env, out, errb, "state")
	st := parse(t, raw)
	if st["active"] != false || st["is_playing"] != false || st["progress_ms"] != float64(0) ||
		st["device"] != nil || st["context"] != nil || st["item"] != nil {
		t.Errorf("inactive state: %v", st)
	}
	for _, key := range []string{"device", "context", "item"} {
		if !strings.Contains(raw, `"`+key+`":null`) {
			t.Errorf("%s must be an explicit null: %s", key, raw)
		}
	}
}

func TestStateEpisode(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 200, `{"device":{"id":"D","name":"n","type":"Speaker","is_active":true},"is_playing":true,
	 "item":{"type":"episode","id":"E","uri":"spotify:episode:E","name":"Ep","duration_ms":1000,
	  "show":{"name":"Show","uri":"spotify:show:S"},"images":[{"url":"https://i.scdn.co/e","width":300}]}}`)
	item := parse(t, runOK(t, env, out, errb, "state"))["item"].(map[string]any)
	if item["type"] != "episode" || len(item["artists"].([]any)) != 0 || item["album"].(map[string]any)["name"] != "Show" {
		t.Errorf("episode: %v", item)
	}
}

func TestStateErrors(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 429, "")
	runFail(t, env, out, errb, 5, "rate_limited", "state")
}

// ---- 5.2 transport ----

func TestTransportCommands(t *testing.T) {
	tests := []struct {
		args   []string
		route  string
		device string
	}{
		{[]string{"play"}, "PUT /me/player/play", ""},
		{[]string{"pause"}, "PUT /me/player/pause", ""},
		{[]string{"next"}, "POST /me/player/next", ""},
		{[]string{"prev"}, "POST /me/player/previous", ""},
		{[]string{"pause", "--device", "D9"}, "PUT /me/player/pause", "D9"},
		{[]string{"next", "--device=D9"}, "POST /me/player/next", "D9"},
	}
	for _, tt := range tests {
		env, f, out, errb := spotifyEnv(t)
		f.reply(tt.route, 204, "")
		if got := runOK(t, env, out, errb, tt.args...); got != "{\"ok\":true}\n" {
			t.Errorf("%v: stdout %q", tt.args, got)
		}
		calls := f.callsTo(tt.route)
		if len(calls) != 1 || calls[0].Query.Get("device_id") != tt.device {
			t.Errorf("%v: calls %+v, want one with device_id %q", tt.args, calls, tt.device)
		}
		if calls[0].Body != "" {
			t.Errorf("%v: resume/transport must not send a body (it would change what plays): %q", tt.args, calls[0].Body)
		}
	}
}

func TestToggleBothDirections(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 200, player("true"))
	f.reply("PUT /me/player/pause", 204, "")
	f.reply("PUT /me/player/play", 204, "")
	runOK(t, env, out, errb, "toggle")
	if len(f.callsTo("PUT /me/player/pause")) != 1 || len(f.callsTo("PUT /me/player/play")) != 0 {
		t.Errorf("playing → expected one pause")
	}

	f.reply("GET /me/player", 200, player("false"))
	runOK(t, env, out, errb, "toggle")
	if len(f.callsTo("PUT /me/player/play")) != 1 {
		t.Errorf("paused → expected one play")
	}
}

func TestToggleWithNothingActive(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 204, "")
	runFail(t, env, out, errb, 4, "no_active_device", "toggle")
	if n := len(f.callsTo("PUT /me/player/play")) + len(f.callsTo("PUT /me/player/pause")); n != 0 {
		t.Errorf("%d control requests were sent", n)
	}

	// An explicit device bypasses the check and starts playback there.
	f.reply("PUT /me/player/play", 204, "")
	runOK(t, env, out, errb, "toggle", "--device", "D1")
	if c := f.callsTo("PUT /me/player/play"); len(c) != 1 || c[0].Query.Get("device_id") != "D1" {
		t.Errorf("calls: %+v", c)
	}
}

func TestToggleOtherDeviceStartsIt(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 200, player("true")) // active device is D1
	f.reply("PUT /me/player/play", 204, "")
	runOK(t, env, out, errb, "toggle", "--device", "D2")
	if len(f.callsTo("PUT /me/player/play")) != 1 || len(f.callsTo("PUT /me/player/pause")) != 0 {
		t.Error("toggling a device other than the active one should play on it")
	}
}

func TestTransportErrorMapping(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/player/pause", 404, `{"error":{"status":404,"message":"Player command failed: No active device found","reason":"NO_ACTIVE_DEVICE"}}`)
	runFail(t, env, out, errb, 4, "no_active_device", "pause")

	f.reply("POST /me/player/next", 403, `{"error":{"status":403,"message":"Player command failed: Premium required","reason":"PREMIUM_REQUIRED"}}`)
	runFail(t, env, out, errb, 6, "premium_required", "next")
}

// ---- 5.3 volume and seek ----

func TestVolumeMissingArgumentNeedsNoConfig(t *testing.T) {
	env, out, errb := testEnv(t) // no client ID, no token
	out.Reset()
	if code := Run([]string{"volume"}, env); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if got := errorCode(t, errb); got != "usage" {
		t.Errorf("code %q", got)
	}
}

func TestVolumeAbsolute(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/player/volume", 204, "")
	got := parse(t, runOK(t, env, out, errb, "volume", "40"))
	if got["ok"] != true || got["volume_percent"] != float64(40) {
		t.Errorf("output %v", got)
	}
	if c := f.callsTo("PUT /me/player/volume"); len(c) != 1 || c[0].Query.Get("volume_percent") != "40" {
		t.Errorf("calls %+v", c)
	}
	if f.total() != 1 {
		t.Errorf("absolute volume needs exactly one request, got %d", f.total())
	}
}

func TestVolumeRelativeClamps(t *testing.T) {
	devices := `{"devices":[{"id":"D0","name":"phone","type":"Smartphone","is_active":false,"volume_percent":10},
	 {"id":"D1","name":"la-box","type":"Speaker","is_active":true,"volume_percent":97}]}`
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, devices)
	f.reply("PUT /me/player/volume", 204, "")

	if got := parse(t, runOK(t, env, out, errb, "volume", "+10"))["volume_percent"]; got != float64(100) {
		t.Errorf("97+10 → %v, want 100", got)
	}
	if got := parse(t, runOK(t, env, out, errb, "volume", "-5"))["volume_percent"]; got != float64(92) {
		t.Errorf("97-5 → %v, want 92", got)
	}
	if got := parse(t, runOK(t, env, out, errb, "volume", "-500"))["volume_percent"]; got != float64(0) {
		t.Errorf("97-500 → %v, want 0", got)
	}
	// An explicit device uses its own current volume.
	if got := parse(t, runOK(t, env, out, errb, "volume", "+5", "--device", "D0"))["volume_percent"]; got != float64(15) {
		t.Errorf("D0 10+5 → %v, want 15", got)
	}
	calls := f.callsTo("PUT /me/player/volume")
	if calls[3].Query.Get("device_id") != "D0" {
		t.Errorf("device_id not forwarded: %+v", calls[3])
	}
}

func TestVolumeRelativeWithoutActiveDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, `{"devices":[{"id":"D0","name":"phone","type":"Smartphone","is_active":false,"volume_percent":10}]}`)
	runFail(t, env, out, errb, 4, "no_active_device", "volume", "+5")
}

func TestVolumeDeviceWithoutVolumeSupport(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, `{"devices":[{"id":"D1","name":"tv","type":"TV","is_active":true,"volume_percent":null}]}`)
	runFail(t, env, out, errb, 1, "api_error", "volume", "+5")
}

func TestVolumeUsageErrors(t *testing.T) {
	for _, arg := range []string{"150", "abc", "5.5", "+", "--", "101", ""} {
		env, f, out, errb := spotifyEnv(t)
		runFail(t, env, out, errb, 2, "usage", "volume", arg)
		if f.total() != 0 {
			t.Errorf("volume %q: a request was sent", arg)
		}
	}
	env, _, out, errb := spotifyEnv(t)
	runFail(t, env, out, errb, 2, "usage", "volume", "1", "2")
}

func playerAt(progress, duration int) string {
	return strings.NewReplacer(
		`"progress_ms":250343`, `"progress_ms":`+strconv.Itoa(progress),
		`"duration_ms":345186`, `"duration_ms":`+strconv.Itoa(duration),
	).Replace(player("true"))
}

func TestSeek(t *testing.T) {
	tests := []struct {
		name               string
		progress, duration int
		arg                string
		want               string
	}{
		{"absolute", 3000, 200000, "60000", "60000"},
		{"relative forward", 3000, 200000, "+10000", "13000"},
		{"relative back", 30000, 200000, "-5000", "25000"},
		{"back past the start", 3000, 200000, "-5000", "0"},
		{"past the end stops one second short", 190000, 200000, "+50000", "199000"},
		{"absolute past the end stops one second short", 3000, 200000, "999999", "199000"},
		{"exactly the last allowed position", 3000, 200000, "199000", "199000"},
		{"item shorter than the margin", 100, 800, "5000", "0"},
	}
	for _, tt := range tests {
		env, f, out, errb := spotifyEnv(t)
		f.reply("GET /me/player", 200, playerAt(tt.progress, tt.duration))
		f.reply("PUT /me/player/seek", 204, "")
		got := parse(t, runOK(t, env, out, errb, "seek", tt.arg))
		if got["position_ms"] != mustFloat(tt.want) {
			t.Errorf("%s: position_ms %v, want %s", tt.name, got["position_ms"], tt.want)
		}
		if c := f.callsTo("PUT /me/player/seek"); len(c) != 1 || c[0].Query.Get("position_ms") != tt.want {
			t.Errorf("%s: calls %+v", tt.name, c)
		}
	}
}

func mustFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(err)
	}
	return f
}

func TestSeekErrors(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	runFail(t, env, out, errb, 2, "usage", "seek")
	runFail(t, env, out, errb, 2, "usage", "seek", "soon")
	if f.total() != 0 {
		t.Error("usage errors must not reach Spotify")
	}
	f.reply("GET /me/player", 204, "")
	runFail(t, env, out, errb, 4, "no_active_device", "seek", "1000")
}

// ---- 5.4 shuffle and repeat ----

func TestShuffle(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/player/shuffle", 204, "")
	if got := parse(t, runOK(t, env, out, errb, "shuffle", "on")); got["shuffle"] != true {
		t.Errorf("%v", got)
	}
	if got := parse(t, runOK(t, env, out, errb, "shuffle", "off")); got["shuffle"] != false {
		t.Errorf("%v", got)
	}
	calls := f.callsTo("PUT /me/player/shuffle")
	if calls[0].Query.Get("state") != "true" || calls[1].Query.Get("state") != "false" {
		t.Errorf("calls %+v", calls)
	}

	f.reply("GET /me/player", 200, player("true")) // shuffle_state is true
	if got := parse(t, runOK(t, env, out, errb, "shuffle", "toggle")); got["shuffle"] != false {
		t.Errorf("toggle from on → %v", got)
	}
}

func TestRepeat(t *testing.T) {
	for _, mode := range []string{"off", "context", "track"} {
		env, f, out, errb := spotifyEnv(t)
		f.reply("PUT /me/player/repeat", 204, "")
		if got := parse(t, runOK(t, env, out, errb, "repeat", mode)); got["repeat"] != mode {
			t.Errorf("%s: %v", mode, got)
		}
		if c := f.callsTo("PUT /me/player/repeat"); c[0].Query.Get("state") != mode {
			t.Errorf("%s: %+v", mode, c)
		}
	}
}

func TestRepeatCycleOrder(t *testing.T) {
	for current, want := range map[string]string{"off": "context", "context": "track", "track": "off"} {
		env, f, out, errb := spotifyEnv(t)
		f.reply("GET /me/player", 200, strings.Replace(player("true"), `"repeat_state":"track"`, `"repeat_state":"`+current+`"`, 1))
		f.reply("PUT /me/player/repeat", 204, "")
		if got := parse(t, runOK(t, env, out, errb, "repeat", "cycle")); got["repeat"] != want {
			t.Errorf("cycle from %s → %v, want %s", current, got["repeat"], want)
		}
	}
}

func TestShuffleRepeatBadArguments(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	runFail(t, env, out, errb, 2, "usage", "shuffle", "maybe")
	runFail(t, env, out, errb, 2, "usage", "shuffle")
	runFail(t, env, out, errb, 2, "usage", "repeat", "always")
	runFail(t, env, out, errb, 2, "usage", "repeat")
	if f.total() != 0 {
		t.Error("usage errors must not reach Spotify")
	}
}

// ---- 5.5 play-uri ----

func TestPlayURIContextVersusItem(t *testing.T) {
	for _, uri := range []string{"spotify:album:abc", "spotify:playlist:abc", "spotify:artist:abc", "spotify:show:abc"} {
		env, f, out, errb := spotifyEnv(t)
		f.reply("PUT /me/player/play", 204, "")
		if got := runOK(t, env, out, errb, "play-uri", uri); got != "{\"ok\":true}\n" {
			t.Errorf("%s: %q", uri, got)
		}
		body := f.callsTo("PUT /me/player/play")[0].json(t)
		if body["context_uri"] != uri || body["uris"] != nil {
			t.Errorf("%s: body %v", uri, body)
		}
	}
	for _, uri := range []string{"spotify:track:abc", "spotify:episode:abc"} {
		env, f, out, errb := spotifyEnv(t)
		f.reply("PUT /me/player/play", 204, "")
		runOK(t, env, out, errb, "play-uri", uri)
		body := f.callsTo("PUT /me/player/play")[0].json(t)
		uris, _ := body["uris"].([]any)
		if len(uris) != 1 || uris[0] != uri || body["context_uri"] != nil {
			t.Errorf("%s: body %v", uri, body)
		}
	}
}

func TestPlayURIWebLinkAndDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/player/play", 204, "")
	runOK(t, env, out, errb, "play-uri", "https://open.spotify.com/track/abc?si=xyz", "--device", "D1")
	c := f.callsTo("PUT /me/player/play")[0]
	if uris := c.json(t)["uris"].([]any); uris[0] != "spotify:track:abc" || c.Query.Get("device_id") != "D1" {
		t.Errorf("call %+v", c)
	}
	if f.total() != 1 {
		t.Errorf("with --device no state lookup is needed, got %d requests", f.total())
	}
}

func TestPlayURIUsageErrors(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	runFail(t, env, out, errb, 2, "usage", "play-uri", "hello")
	runFail(t, env, out, errb, 2, "usage", "play-uri")
	runFail(t, env, out, errb, 2, "usage", "play-uri", "spotify:user:abc")
	if f.total() != 0 {
		t.Error("usage errors must not reach Spotify")
	}
}

func TestPlayURINoActiveDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("PUT /me/player/play", 404, `{"error":{"reason":"NO_ACTIVE_DEVICE","message":"x"}}`)
	runFail(t, env, out, errb, 4, "no_active_device", "play-uri", "spotify:album:abc")
}

// ---- cross-cutting: secrets ----

func TestTokensNeverAppearInOutput(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 500, `{"error":{"message":"boom"}}`)
	runFail(t, env, out, errb, 1, "api_error", "state")
	for _, secret := range []string{"TOKEN-VALUE", "REFRESH-VALUE", "test-client-id"} {
		if strings.Contains(out.String()+errb.String(), secret) {
			t.Errorf("output leaks %q", secret)
		}
	}
}

func TestNotLoggedIn(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	cfgDir := env.Getenv("XDG_CONFIG_HOME") + "/spotify-helper"
	_ = removeFile(cfgDir + "/token.json")
	runFail(t, env, out, errb, 3, "not_authenticated", "state")
}

func TestSeekWithoutKnownDurationHasNoUpperLimit(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 200, `{"device":{"id":"D","name":"n","type":"Speaker","is_active":true},"is_playing":true,"progress_ms":10,"item":null}`)
	f.reply("PUT /me/player/seek", 204, "")
	if got := parse(t, runOK(t, env, out, errb, "seek", "5000"))["position_ms"]; got != float64(5000) {
		t.Errorf("position_ms %v, want 5000", got)
	}
}
