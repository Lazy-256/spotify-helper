package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

// player is a fake Spotify player: a current item, a user queue that `next`
// consumes from the head, and a log of every request.
type player struct {
	inactive    bool // state answers 204
	noItem      bool // state has a device but no item
	queued      int  // entries in the queue
	failNext    int  // 1-based `next` answered with 429; 0 = never
	showMax     int  // entries the queue endpoint shows (default 20)
	pauseStatus int  // pause answers this status instead of 204
	lag         bool // the first queue read after a `next` still shows one entry more
	phantom     int  // entries shown beyond the real ones; `next` for them is refused once, then they vanish
	padding     bool // behind the queued entries the queue shows copies of the current item, and next refuses

	ended     bool // a `next` went past the last item: nothing is current any more
	afterNext bool

	mu       sync.Mutex
	log      []string // "METHOD path" in arrival order
	devices  []string // device_id of every request, "" when absent
	playBody string
	nexts    int
	reads    int
	inflight atomic.Int32
	overlap  atomic.Bool
}

func (p *player) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p.inflight.Add(1) > 1 {
			p.overlap.Store(true)
		}
		defer p.inflight.Add(-1)
		time.Sleep(time.Millisecond)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.log = append(p.log, r.Method+" "+strings.TrimPrefix(r.URL.Path, "/v1"))
		p.devices = append(p.devices, r.URL.Query().Get("device_id"))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/me/player":
			if p.inactive {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			item := `,"item":{"id":"cur","uri":"spotify:track:cur","name":"Now","type":"track","artists":[]}`
			if p.noItem {
				item = ""
			}
			jsonReply(w, 200, `{"device":{"id":"D","name":"d","type":"Speaker","is_active":true},"is_playing":true,"progress_ms":5000`+item+`}`)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/me/player/play":
			b, _ := io.ReadAll(r.Body)
			p.playBody = string(b)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/me/player/queue":
			p.reads++
			show := p.showMax
			if show == 0 {
				show = 20
			}
			shown := p.queued + p.phantom
			if p.lag && p.afterNext {
				shown++
			}
			p.afterNext = false
			var items []string
			for i := 0; i < min(shown, show); i++ {
				items = append(items, fmt.Sprintf(`{"id":"q%d","uri":"spotify:track:q%d","name":"Q","type":"track","artists":[]}`, i, i))
			}
			cur := `{"id":"cur","uri":"spotify:track:cur","name":"Now","type":"track","artists":[]}`
			if p.padding {
				for range 10 {
					items = append(items, cur)
				}
			}
			jsonReply(w, 200, `{"currently_playing":`+cur+`,"queue":[`+strings.Join(items, ",")+`]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/me/player/next":
			p.nexts++
			p.afterNext = true
			if p.ended || (p.padding && p.queued == 0) {
				jsonReply(w, 403, `{"error":{"status":403,"message":"Player command failed: Restriction violated"}}`)
				return
			}
			if p.queued == 0 && p.phantom > 0 {
				p.phantom = 0
				jsonReply(w, 403, `{"error":{"status":403,"message":"Player command failed: Restriction violated"}}`)
				return
			}
			if p.nexts == p.failNext {
				w.Header().Set("Retry-After", "12")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			if p.queued > 0 {
				p.queued--
			} else if !p.padding {
				p.ended = true // nothing follows: the player stops
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/me/player/pause":
			if p.ended {
				jsonReply(w, 403, `{"error":{"status":403,"message":"Player command failed: Restriction violated"}}`)
				return
			}
			if p.pauseStatus != 0 {
				jsonReply(w, p.pauseStatus, `{"error":{"status":`+strconv.Itoa(p.pauseStatus)+`,"message":"pause failed"}}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(599)
		}
	}
}

func (p *player) count(prefix string) int {
	n := 0
	for _, l := range p.log {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

func TestQueueClearQueueOfThree(t *testing.T) {
	p := &player{queued: 3}
	c, _, _ := newClient(t, p.handler(t))
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Skipped: 3, Empty: true}) {
		t.Errorf("got %+v", *res)
	}
	if p.playBody != `{"uris":["spotify:track:cur"]}`+"\n" && p.playBody != `{"uris":["spotify:track:cur"]}` {
		t.Errorf("the current item must restart as its own context, body %q", p.playBody)
	}
	if p.count("POST /me/player/next") != 3 {
		t.Errorf("log %v", p.log)
	}
	if last := p.log[len(p.log)-1]; last != "PUT /me/player/pause" {
		t.Errorf("pause must be last, log %v", p.log)
	}
	// state, play, queue read, 3 nexts, two agreeing queue reads, pause
	want := []string{"GET /me/player", "PUT /me/player/play", "GET /me/player/queue",
		"POST /me/player/next", "POST /me/player/next", "POST /me/player/next",
		"GET /me/player/queue", "GET /me/player/queue", "PUT /me/player/pause"}
	if !sameStrings(p.log, want) {
		t.Errorf("log %v, want %v", p.log, want)
	}
	if p.overlap.Load() {
		t.Error("requests must never overlap")
	}
}

func TestQueueClearEmptyQueue(t *testing.T) {
	p := &player{}
	c, _, _ := newClient(t, p.handler(t))
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Empty: true}) {
		t.Errorf("got %+v", *res)
	}
	if p.count("POST") != 0 || p.count("PUT /me/player/play") != 1 || p.count("PUT /me/player/pause") != 1 {
		t.Errorf("log %v", p.log)
	}
}

func TestQueueClearOverTheCap(t *testing.T) {
	p := &player{queued: 250}
	c, _, _ := newClient(t, p.handler(t))
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Skipped: 100, Empty: false}) {
		t.Errorf("got %+v", *res)
	}
	if p.count("POST /me/player/next") != 100 {
		t.Errorf("exactly 100 next requests, log has %d", p.count("POST /me/player/next"))
	}
	// Batches of at most 20, then two agreeing reads after each: 1 + 5*2 reads.
	if p.reads != 11 {
		t.Errorf("queue reads %d, want 11", p.reads)
	}
	if last := p.log[len(p.log)-1]; last != "PUT /me/player/pause" {
		t.Errorf("pause must be last, log tail %v", p.log[len(p.log)-3:])
	}
	longest, run := 0, 0
	for _, l := range p.log {
		if l == "POST /me/player/next" {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	if longest > 20 {
		t.Errorf("a batch of %d next requests exceeds what the queue showed", longest)
	}
}

func TestQueueClearExactlyAtTheCapIsEmpty(t *testing.T) {
	p := &player{queued: 100}
	c, _, _ := newClient(t, p.handler(t))
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Skipped: 100, Empty: true}) {
		t.Errorf("got %+v", *res)
	}
}

func TestQueueClearNothingPlaying(t *testing.T) {
	for name, p := range map[string]*player{"no device": {inactive: true, queued: 2}, "no item": {noItem: true, queued: 2}} {
		t.Run(name, func(t *testing.T) {
			c, _, _ := newClient(t, p.handler(t))
			_, err := c.QueueClear(context.Background(), "")
			oe := asErr(t, err)
			if oe.Code != output.CodeNoActiveDevice || oe.Skipped != 0 {
				t.Errorf("got %+v", *oe)
			}
			if !sameStrings(p.log, []string{"GET /me/player"}) {
				t.Errorf("nothing may be sent after the state read: %v", p.log)
			}
		})
	}
}

func TestQueueClearRateLimitedInTheMiddle(t *testing.T) {
	p := &player{queued: 60, failNext: 31}
	c, _, _ := newClient(t, p.handler(t))
	_, err := c.QueueClear(context.Background(), "")
	oe := asErr(t, err)
	if oe.Code != output.CodeRateLimited || oe.RetryAfter != 12 || oe.Skipped != 30 {
		t.Errorf("got %+v", *oe)
	}
	if p.nexts != 31 {
		t.Errorf("no further next after the 429, got %d", p.nexts)
	}
	if p.count("PUT /me/player/pause") != 0 {
		t.Errorf("no pause after a rate limit: %v", p.log)
	}
}

func TestQueueClearCooldownSendsNothing(t *testing.T) {
	p := &player{queued: 5}
	c, n, _ := newClient(t, p.handler(t))
	c.Gate.Record("30")
	before := n.Load()
	_, err := c.QueueClear(context.Background(), "")
	oe := asErr(t, err)
	if oe.Code != output.CodeRateLimited || oe.Skipped != 0 || n.Load() != before {
		t.Errorf("got %+v, requests %d", *oe, n.Load()-before)
	}
}

func TestQueueClearPauseFailureIsReported(t *testing.T) {
	p := &player{queued: 2, pauseStatus: 500}
	c, _, _ := newClient(t, p.handler(t))
	_, err := c.QueueClear(context.Background(), "")
	oe := asErr(t, err)
	if oe.Code != output.CodeAPIError || !strings.Contains(oe.Message, "500") || oe.Skipped != 2 {
		t.Errorf("got %+v", *oe)
	}
	if p.count("PUT /me/player/pause") != 1 {
		t.Errorf("a failed pause is not retried: %v", p.log)
	}
}

func TestQueueClearPassesTheDevice(t *testing.T) {
	p := &player{queued: 2}
	c, _, _ := newClient(t, p.handler(t))
	if _, err := c.QueueClear(context.Background(), "D1"); err != nil {
		t.Fatal(err)
	}
	for i, l := range p.log {
		takesDevice := strings.HasPrefix(l, "PUT /me/player/play") || strings.HasPrefix(l, "POST /me/player/next") || strings.HasPrefix(l, "PUT /me/player/pause")
		if takesDevice && p.devices[i] != "D1" {
			t.Errorf("%s lacks the device id (%q)", l, p.devices[i])
		}
	}
}

func TestQueueClearPaddingIsNotQueued(t *testing.T) {
	// After the queued entries are used up Spotify shows copies of the current
	// item and refuses `next`; the run must stop on its own with a success.
	p := &player{queued: 3, padding: true}
	c, _, _ := newClient(t, p.handler(t))
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Skipped: 3, Empty: true}) {
		t.Errorf("got %+v", *res)
	}
	if p.nexts != 3 {
		t.Errorf("%d next requests, want 3 (none for the padding)", p.nexts)
	}
	if last := p.log[len(p.log)-1]; last != "PUT /me/player/pause" {
		t.Errorf("pause must be last, log %v", p.log)
	}
}

func TestQueueClearOnlyPadding(t *testing.T) {
	p := &player{padding: true}
	c, _, _ := newClient(t, p.handler(t))
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Empty: true}) || p.nexts != 0 {
		t.Errorf("got %+v after %d next requests", *res, p.nexts)
	}
}

func TestWithoutPadding(t *testing.T) {
	item := func(id string) output.Item { return output.Item{URI: "spotify:track:" + id} }
	cur := item("cur")
	cases := []struct {
		name string
		now  *output.Item
		in   []output.Item
		want int
	}{
		{"copies at the end", &cur, []output.Item{item("a"), item("b"), cur, cur, cur}, 2},
		{"a copy inside stays", &cur, []output.Item{item("a"), cur, item("b"), cur}, 3},
		{"only copies", &cur, []output.Item{cur, cur}, 0},
		{"no padding", &cur, []output.Item{item("a"), item("b")}, 2},
		{"nothing playing", nil, []output.Item{cur, cur}, 0},
		{"empty", &cur, nil, 0},
	}
	for _, tc := range cases {
		q := &output.QueueResult{NowPlaying: tc.now, Queue: tc.in}
		if got := len(withoutPadding(q)); got != tc.want {
			t.Errorf("%s: %d entries left, want %d", tc.name, got, tc.want)
		}
	}
}

func TestQueueClearWaitsForALateDevice(t *testing.T) {
	// The first read after a `next` still shows the entry that was just used up:
	// counting from it would send a `next` past the end and stop the player.
	p := &player{queued: 5, lag: true}
	c, _, _ := newClient(t, p.handler(t))
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Skipped: 5, Empty: true}) || p.nexts != 5 || p.ended {
		t.Errorf("got %+v after %d next requests, player ended: %v", *res, p.nexts, p.ended)
	}
}

func TestQueueClearRefusedNextAtTheEndIsTheEnd(t *testing.T) {
	// Spotify shows two entries beyond the real ones and refuses `next` for them;
	// the queue then holds nothing, so the run ended normally.
	p := &player{queued: 3, phantom: 2}
	c, _, _ := newClient(t, p.handler(t))
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Skipped: 3, Empty: true}) {
		t.Errorf("got %+v", *res)
	}
}

func TestQueueClearRefusedNextWithEntriesLeftIsAnError(t *testing.T) {
	// A refusal that does not go away while the queue still holds entries is a real
	// failure, after the retries.
	p := &player{queued: 5}
	refused := 0
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/me/player/next" && p.nexts >= 2 {
			p.mu.Lock()
			p.nexts++
			refused++
			p.mu.Unlock()
			jsonReply(w, 403, `{"error":{"status":403,"message":"Player command failed: Restriction violated"}}`)
			return
		}
		p.handler(t)(w, r)
	})
	_, err := c.QueueClear(context.Background(), "")
	oe := asErr(t, err)
	if oe.Code != output.CodeAPIError || oe.Skipped != 2 {
		t.Errorf("got %+v", *oe)
	}
	if refused != 1+nextRetries {
		t.Errorf("%d refused attempts, want %d", refused, 1+nextRetries)
	}
}

func TestQueueClearRetriesATransientRefusal(t *testing.T) {
	// The device refuses one `next` while it changes track; the retry succeeds and
	// the run goes on.
	p := &player{queued: 5}
	refused := 0
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/me/player/next" && p.nexts == 2 && refused == 0 {
			p.mu.Lock()
			refused++
			p.mu.Unlock()
			jsonReply(w, 403, `{"error":{"status":403,"message":"Player command failed: Restriction violated"}}`)
			return
		}
		p.handler(t)(w, r)
	})
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Skipped: 5, Empty: true}) || refused != 1 {
		t.Errorf("got %+v, refused %d", *res, refused)
	}
}

func TestQueueClearPauseRefusedWhenNothingPlays(t *testing.T) {
	// A refused final pause (no current item) is silence, not a failure.
	p := &player{queued: 2, pauseStatus: 403}
	c, _, _ := newClient(t, p.handler(t))
	res, err := c.QueueClear(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if *res != (output.QueueClearResult{OK: true, Skipped: 2, Empty: true}) {
		t.Errorf("got %+v", *res)
	}
}
