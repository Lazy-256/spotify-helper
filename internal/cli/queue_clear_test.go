package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/api"
)

// clearFixtures serves a playing track and a queue of n entries that `next`
// consumes from the head; the queue endpoint shows at most 20.
func clearFixtures(f *fakeSpotify, n int) *int {
	return clearFixturesPadded(f, n, false)
}

// clearFixturesPadded is clearFixtures; with padding Spotify shows copies of the
// current item behind the queued entries and refuses `next` once they are used up.
func clearFixturesPadded(f *fakeSpotify, n int, padding bool) *int {
	left := n
	f.reply("GET /me/player", 200, `{"device":{"id":"D","name":"d","type":"Speaker","is_active":true},"is_playing":true,"progress_ms":1000,
	 "item":{"id":"cur","uri":"spotify:track:cur","name":"Now","type":"track","artists":[]}}`)
	f.reply("PUT /me/player/play", 204, "")
	f.on("GET /me/player/queue", func(w http.ResponseWriter, r *http.Request) {
		var items []string
		for i := 0; i < min(left, 20); i++ {
			items = append(items, fmt.Sprintf(`{"id":"q%[1]d","uri":"spotify:track:q%[1]d","name":"Q","type":"track","artists":[]}`, i))
		}
		cur := `{"id":"cur","uri":"spotify:track:cur","name":"Now","type":"track","artists":[]}`
		if padding {
			for range 10 {
				items = append(items, cur)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"currently_playing":%s,"queue":[%s]}`, cur, strings.Join(items, ","))
	})
	f.on("POST /me/player/next", func(w http.ResponseWriter, r *http.Request) {
		if padding && left == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(403)
			_, _ = io.WriteString(w, `{"error":{"status":403,"message":"Player command failed: Restriction violated"}}`)
			return
		}
		if left > 0 {
			left--
		}
		w.WriteHeader(204)
	})
	f.reply("PUT /me/player/pause", 204, "")
	return &left
}

func TestQueueClear(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	clearFixtures(f, 3)
	got := parse(t, runOK(t, env, out, errb, "queue-clear"))
	if got["ok"] != true || got["skipped"] != 3.0 || got["empty"] != true {
		t.Errorf("output %v", got)
	}
	if body := f.callsTo("PUT /me/player/play")[0].json(t); fmt.Sprint(body["uris"]) != "[spotify:track:cur]" || body["context_uri"] != nil {
		t.Errorf("the current track must restart on its own: %v", body)
	}
	if n := len(f.callsTo("POST /me/player/next")); n != 3 {
		t.Errorf("%d next requests, want 3", n)
	}
	if n := len(f.callsTo("PUT /me/player/pause")); n != 1 {
		t.Errorf("%d pauses, want 1", n)
	}
}

func TestQueueClearEmptyQueue(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	clearFixtures(f, 0)
	got := parse(t, runOK(t, env, out, errb, "queue-clear"))
	if got["skipped"] != 0.0 || got["empty"] != true {
		t.Errorf("output %v", got)
	}
	if len(f.callsTo("POST /me/player/next")) != 0 || len(f.callsTo("PUT /me/player/pause")) != 1 {
		t.Error("an empty queue needs no next and ends with a pause")
	}
}

func TestQueueClearOverTheCap(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	clearFixtures(f, 250)
	got := parse(t, runOK(t, env, out, errb, "queue-clear"))
	if got["skipped"] != 100.0 || got["empty"] != false {
		t.Errorf("output %v", got)
	}
}

func TestQueueClearUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"queue-clear", "spotify:track:abc"},
		{"queue-clear", "--offset", "3"},
		{"queue-clear", "--device", "D1"},
	} {
		env, f, out, errb := spotifyEnv(t)
		runFail(t, env, out, errb, 2, "usage", args...)
		if f.total() != 0 {
			t.Errorf("%v: nothing may be sent to Spotify", args)
		}
	}
}

func TestQueueClearNothingPlaying(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	f.reply("GET /me/player", 204, "")
	runFail(t, env, out, errb, 4, "no_active_device", "queue-clear")
	if f.total() != 1 {
		t.Errorf("only the state read may be sent, got %d requests", f.total())
	}
	if strings.Contains(errb.String(), `"skipped"`) {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestQueueClearRateLimitedMidway(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	clearFixtures(f, 60)
	n := 0
	f.on("POST /me/player/next", func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 31 {
			w.Header().Set("Retry-After", "12")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(204)
	})
	runFail(t, env, out, errb, 5, "rate_limited", "queue-clear")
	e := parse(t, errb.String())["error"].(map[string]any)
	if e["retry_after"] != 12.0 || e["skipped"] != 30.0 {
		t.Errorf("error %v", e)
	}
	if got := len(f.callsTo("POST /me/player/next")); got != 31 {
		t.Errorf("%d next requests, want 31 (thirty accepted, the 31st refused, no 32nd)", got)
	}

	// The cooldown is recorded: the next run stops at once, without counts.
	before := f.total()
	runFail(t, env, out, errb, 5, "rate_limited", "queue-clear")
	if f.total() != before {
		t.Error("an active cooldown must not reach Spotify")
	}
	if strings.Contains(errb.String(), `"skipped"`) {
		t.Errorf("stderr %q", errb.String())
	}
}

func TestQueueClearNotLoggedIn(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	_ = os.Remove(tokenPath(env))
	runFail(t, env, out, errb, 3, "not_authenticated", "queue-clear")
	if f.total() != 0 {
		t.Error("nothing may be sent without a token")
	}
}

func TestQueueClearPaddingEndsTheRun(t *testing.T) {
	env, f, out, errb := spotifyEnv(t)
	clearFixturesPadded(f, 4, true)
	got := parse(t, runOK(t, env, out, errb, "queue-clear"))
	if got["skipped"] != 4.0 || got["empty"] != true {
		t.Errorf("output %v", got)
	}
	if n := len(f.callsTo("POST /me/player/next")); n != 4 {
		t.Errorf("%d next requests, want 4", n)
	}
}

func TestBulkCommandsGetALongerBudget(t *testing.T) {
	env, _, _, _ := spotifyEnv(t)
	deadline := func(run func(env *Env, fn func(ctx context.Context, c *api.Client) error) error) time.Duration {
		var left time.Duration
		if err := run(env, func(ctx context.Context, _ *api.Client) error {
			d, ok := ctx.Deadline()
			if !ok {
				t.Fatal("no deadline")
			}
			left = time.Until(d)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return left
	}
	if d := deadline(withClient); d > 31*time.Second || d < 29*time.Second {
		t.Errorf("ordinary commands: budget %v, want about 30 s", d)
	}
	if d := deadline(withBulkClient); d > 121*time.Second || d < 119*time.Second {
		t.Errorf("bulk commands: budget %v, want about 120 s", d)
	}
}
