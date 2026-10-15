package cli

import (
	"strings"
	"testing"
)

// ---- 7.1 queue ----

func queueItem(n string) string {
	return `{"type":"track","id":"` + n + `","uri":"spotify:track:` + n + `","name":"Song ` + n + `","duration_ms":1000,
	 "artists":[{"name":"Artist","uri":"spotify:artist:A"}],
	 "album":{"name":"Album","uri":"spotify:album:B","images":[{"url":"https://i.scdn.co/` + n + `","width":300}]}}`
}

func TestQueueWithItems(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/queue", 200, `{"currently_playing":`+queueItem("1")+`,"queue":[`+queueItem("2")+`,{"type":"ad","id":"x"},`+queueItem("3")+`]}`)
	q := parse(t, runOK(t, env, out, errb, "queue"))

	now := q["now_playing"].(map[string]any)
	if now["uri"] != "spotify:track:1" || now["cover_url"] != "https://i.scdn.co/1" {
		t.Errorf("now_playing: %v", now)
	}
	items := q["queue"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["uri"] != "spotify:track:2" || items[1].(map[string]any)["uri"] != "spotify:track:3" {
		t.Errorf("queue must keep Spotify's order and drop non-items: %v", items)
	}
}

func TestQueueNothingPlaying(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/queue", 200, `{"currently_playing":null,"queue":[]}`)
	if got := runOK(t, env, out, errb, "queue"); got != "{\"now_playing\":null,\"queue\":[]}\n" {
		t.Errorf("got %q", got)
	}
	// A missing queue field must still serialize as an empty array.
	f.reply("GET /me/player/queue", 200, `{}`)
	if got := runOK(t, env, out, errb, "queue"); got != "{\"now_playing\":null,\"queue\":[]}\n" {
		t.Errorf("got %q", got)
	}
}

func TestQueueRejectsArguments(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	runFail(t, env, out, errb, 2, "usage", "queue", "extra")
	if f.total() != 0 {
		t.Error("usage errors must not reach Spotify")
	}
}

// ---- 7.2 queue-add ----

func TestQueueAddTrack(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("POST /me/player/queue", 204, "")
	got := parse(t, runOK(t, env, out, errb, "queue-add", "spotify:track:abc"))
	if got["ok"] != true || got["uri"] != "spotify:track:abc" {
		t.Errorf("output %v", got)
	}
	c := f.callsTo("POST /me/player/queue")[0]
	if c.Query.Get("uri") != "spotify:track:abc" || c.Query.Has("device_id") || c.Body != "" {
		t.Errorf("call %+v", c)
	}
}

func TestQueueAddEpisodeAndDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("POST /me/player/queue", 204, "")
	runOK(t, env, out, errb, "queue-add", "spotify:episode:abc", "--device", "D1")
	c := f.callsTo("POST /me/player/queue")[0]
	if c.Query.Get("uri") != "spotify:episode:abc" || c.Query.Get("device_id") != "D1" {
		t.Errorf("call %+v", c)
	}
}

func TestQueueAddWebLinkIsNormalized(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("POST /me/player/queue", 204, "")
	got := parse(t, runOK(t, env, out, errb, "queue-add", "https://open.spotify.com/track/abc?si=xyz"))
	if got["uri"] != "spotify:track:abc" {
		t.Errorf("output %v", got)
	}
	if f.callsTo("POST /me/player/queue")[0].Query.Get("uri") != "spotify:track:abc" {
		t.Error("the normalized URI must be sent to Spotify")
	}
}

func TestQueueAddRejectsContexts(t *testing.T) {
	for _, uri := range []string{"spotify:album:abc", "spotify:playlist:abc", "spotify:artist:abc", "spotify:show:abc", "nonsense"} {
		env, f, out, errb := spotifyEnv(t)
		runFail(t, env, out, errb, 2, "usage", "queue-add", uri)
		if f.total() != 0 {
			t.Errorf("%s: nothing may be sent to Spotify", uri)
		}
	}
	env, _, out, errb := spotifyEnv(t)
	runFail(t, env, out, errb, 2, "usage", "queue-add")
}

func TestQueueAddNoActiveDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("POST /me/player/queue", 404, `{"error":{"status":404,"message":"Player command failed: No active device found","reason":"NO_ACTIVE_DEVICE"}}`)
	runFail(t, env, out, errb, 4, "no_active_device", "queue-add", "spotify:track:abc")
}

func TestQueueRemoveDoesNotExist(t *testing.T) {
	env, _, out, errb := spotifyEnv(t)
	runFail(t, env, out, errb, 2, "usage", "queue-remove")
}

// ---- 7.3 devices ----

const devicesJSON = `{"devices":[
 {"id":"D1","name":"la-box","type":"Speaker","is_active":true,"is_restricted":false,"volume_percent":93},
 {"id":"D2","name":"Anna's phone","type":"Smartphone","is_active":false,"is_restricted":false,"volume_percent":100},
 {"id":null,"name":"Web Player","type":"Computer","is_active":false,"is_restricted":true,"volume_percent":null}]}`

func TestDevicesList(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, devicesJSON)
	res := parse(t, runOK(t, env, out, errb, "devices"))
	devs := res["devices"].([]any)
	if len(devs) != 3 {
		t.Fatalf("devices: %v", devs)
	}
	active := 0
	for _, d := range devs {
		if d.(map[string]any)["is_active"] == true {
			active++
		}
	}
	if active != 1 {
		t.Errorf("%d active devices, want 1", active)
	}
	d0 := devs[0].(map[string]any)
	if d0["id"] != "D1" || d0["name"] != "la-box" || d0["type"] != "Speaker" || d0["volume_percent"] != float64(93) || d0["is_restricted"] != false {
		t.Errorf("first device: %v", d0)
	}
	d2 := devs[2].(map[string]any)
	if d2["id"] != nil || d2["volume_percent"] != nil || d2["is_restricted"] != true {
		t.Errorf("restricted device must have null id and volume: %v", d2)
	}
}

func TestDevicesEmpty(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, `{"devices":[]}`)
	if got := runOK(t, env, out, errb, "devices"); got != "{\"devices\":[]}\n" {
		t.Errorf("got %q", got)
	}
	f.reply("GET /me/player/devices", 200, `{}`)
	if got := runOK(t, env, out, errb, "devices"); got != "{\"devices\":[]}\n" {
		t.Errorf("missing field must still be []: %q", got)
	}
}

// ---- 7.4 transfer ----

func TestTransferByName(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, devicesJSON)
	f.reply("PUT /me/player", 204, "")
	res := parse(t, runOK(t, env, out, errb, "transfer", "ANNA'S PHONE"))
	dev := res["device"].(map[string]any)
	if res["ok"] != true || dev["id"] != "D2" || dev["name"] != "Anna's phone" {
		t.Errorf("output %v", res)
	}
	body := f.callsTo("PUT /me/player")[0].json(t)
	ids := body["device_ids"].([]any)
	if len(ids) != 1 || ids[0] != "D2" {
		t.Errorf("body %v", body)
	}
	if _, has := body["play"]; has {
		t.Errorf("without --play the playing state must be kept: %v", body)
	}
}

func TestTransferByIDWithPlay(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, devicesJSON)
	f.reply("PUT /me/player", 204, "")
	runOK(t, env, out, errb, "transfer", "D1", "--play")
	body := f.callsTo("PUT /me/player")[0].json(t)
	if body["play"] != true || body["device_ids"].([]any)[0] != "D1" {
		t.Errorf("body %v", body)
	}
}

func TestTransferIDWinsOverName(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, `{"devices":[
	 {"id":"X","name":"D2","type":"Speaker","is_active":false,"is_restricted":false},
	 {"id":"D2","name":"Other","type":"Speaker","is_active":false,"is_restricted":false}]}`)
	f.reply("PUT /me/player", 204, "")
	res := parse(t, runOK(t, env, out, errb, "transfer", "D2"))
	if res["device"].(map[string]any)["id"] != "D2" {
		t.Errorf("an exact id must win over a name match: %v", res)
	}
}

func TestTransferUnknownDevice(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, devicesJSON)
	runFail(t, env, out, errb, 4, "device_not_found", "transfer", "Kitchen")
	if len(f.callsTo("PUT /me/player")) != 0 {
		t.Error("nothing may be transferred")
	}
}

func TestTransferRestrictedDeviceIsNotEligible(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, devicesJSON)
	runFail(t, env, out, errb, 4, "device_not_found", "transfer", "Web Player")
}

func TestTransferAmbiguousName(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, `{"devices":[
	 {"id":"A","name":"Speaker","type":"Speaker","is_active":false,"is_restricted":false},
	 {"id":"B","name":"speaker","type":"Speaker","is_active":false,"is_restricted":false}]}`)
	runFail(t, env, out, errb, 4, "ambiguous_device", "transfer", "Speaker")
	if len(f.callsTo("PUT /me/player")) != 0 {
		t.Error("nothing may be transferred")
	}
	// The id is still a way out.
	f.reply("PUT /me/player", 204, "")
	runOK(t, env, out, errb, "transfer", "B")
}

func TestTransferMissingArgument(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	runFail(t, env, out, errb, 2, "usage", "transfer")
	runFail(t, env, out, errb, 2, "usage", "transfer", "a", "b")
	if f.total() != 0 {
		t.Error("usage errors must not reach Spotify")
	}
}

func TestTransferPremiumRequired(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player/devices", 200, devicesJSON)
	f.reply("PUT /me/player", 403, `{"error":{"status":403,"message":"Player command failed: Premium required","reason":"PREMIUM_REQUIRED"}}`)
	runFail(t, env, out, errb, 6, "premium_required", "transfer", "D1")
}

func TestHelpListsEveryCommand(t *testing.T) {
	env, out, _ := testEnv(t)
	if code := Run([]string{"--help"}, env); code != 0 {
		t.Fatal(code)
	}
	for _, name := range []string{"login", "logout", "state", "play", "pause", "toggle", "next", "prev", "volume", "seek",
		"shuffle", "repeat", "play-uri", "search", "queue", "queue-add", "devices", "transfer", "version"} {
		if !strings.Contains(out.String(), "  "+name+" ") {
			t.Errorf("help does not list %s", name)
		}
	}
}
