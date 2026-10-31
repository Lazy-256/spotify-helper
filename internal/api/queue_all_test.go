package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

// container is a fake album or playlist: "" is a null entry, "local" a local
// file of a playlist, "bad:<id>" a track marked is_playable false, "ok:<id>" a
// track marked true, anything else a track id without the mark. Like Spotify,
// it sends the mark only when the request carries a market.
type container struct {
	playlist bool
	entries  []string
	noItems  bool // playlist served without items

	failPost int // 1-based index of the queue POST answered with 429; 0 = never

	mu       sync.Mutex
	posted   []string
	pages    []string // "path?offset" of every page read
	markets  []string // market parameter of every page read
	inflight atomic.Int32
	overlap  atomic.Bool
}

func (f *container) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/me/player/queue":
			if f.inflight.Add(1) > 1 {
				f.overlap.Store(true)
			}
			defer f.inflight.Add(-1)
			time.Sleep(time.Millisecond)
			f.mu.Lock()
			n := len(f.posted) + 1
			if n == f.failPost {
				f.mu.Unlock()
				w.Header().Set("Retry-After", "12")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			f.posted = append(f.posted, r.URL.Query().Get("uri"))
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/v1/albums/alb":
			jsonReply(w, 200, albumHead)
		case r.URL.Path == "/v1/playlists/pl":
			if f.noItems {
				jsonReply(w, 200, playlistHead+`}`)
				return
			}
			jsonReply(w, 200, playlistHead+fmt.Sprintf(`,"items":{"total":%d}}`, len(f.entries)))
		case r.URL.Path == "/v1/albums/alb/tracks" || r.URL.Path == "/v1/playlists/pl/items":
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			f.mu.Lock()
			f.pages = append(f.pages, r.URL.Path+"?"+strconv.Itoa(offset))
			f.markets = append(f.markets, r.URL.Query().Get("market"))
			f.mu.Unlock()
			end := min(offset+limit, len(f.entries))
			var items []string
			for i := min(offset, end); i < end; i++ {
				items = append(items, f.wire(f.entries[i], r.URL.Query().Get("market") != ""))
			}
			jsonReply(w, 200, fmt.Sprintf(`{"total":%d,"items":[%s]}`, len(f.entries), strings.Join(items, ",")))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(599)
		}
	}
}

func (f *container) wire(e string, market bool) string {
	track := func(id string) string {
		mark := ""
		if m, ok := strings.CutPrefix(id, "bad:"); ok {
			id, mark = m, `,"is_playable":false`
		} else if m, ok := strings.CutPrefix(id, "ok:"); ok {
			id, mark = m, `,"is_playable":true`
		}
		if !market {
			mark = ""
		}
		return fmt.Sprintf(`{"id":%q,"uri":"spotify:track:%s","name":"N","type":"track","artists":[]%s}`, id, id, mark)
	}
	switch {
	case e == "" && !f.playlist:
		return `null`
	case e == "":
		return `{"is_local":false,"item":null}`
	case e == "local":
		return `{"is_local":true,"item":{"id":"","uri":"spotify:local:a:b:c:1","name":"L","type":"track"}}`
	case f.playlist:
		return `{"is_local":false,"item":` + track(e) + `}`
	}
	return track(e)
}

func tracks(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "t" + strconv.Itoa(i)
	}
	return out
}

func uris(ids ...string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "spotify:track:" + id
	}
	return out
}

func sameStrings(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

func TestQueueAddAllAlbum(t *testing.T) {
	f := &container{entries: tracks(12)}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:album:alb"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	want := output.QueueAddAllResult{OK: true, URI: "spotify:album:alb", Added: 12, Total: 12}
	if *res != want {
		t.Errorf("got %+v, want %+v", *res, want)
	}
	if !sameStrings(f.posted, uris(tracks(12)...)) {
		t.Errorf("posted %v", f.posted)
	}
}

func TestQueueAddAllOffset(t *testing.T) {
	f := &container{entries: tracks(12)}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:album:alb"), 4, "DEV")
	if err != nil {
		t.Fatal(err)
	}
	want := output.QueueAddAllResult{OK: true, URI: "spotify:album:alb", Added: 8, Total: 12, Offset: 4}
	if *res != want {
		t.Errorf("got %+v, want %+v", *res, want)
	}
	if len(f.posted) != 8 || f.posted[0] != "spotify:track:t4" {
		t.Errorf("posted %v", f.posted)
	}
	if !sameStrings(f.pages, []string{"/v1/albums/alb/tracks?4"}) {
		t.Errorf("pages %v", f.pages)
	}
}

func TestQueueAddAllPlaylistOverCap(t *testing.T) {
	f := &container{playlist: true, entries: tracks(342)}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:playlist:pl"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	want := output.QueueAddAllResult{OK: true, URI: "spotify:playlist:pl", Added: 100, Total: 342, Truncated: true}
	if *res != want {
		t.Errorf("got %+v, want %+v", *res, want)
	}
	if !sameStrings(f.posted, uris(tracks(100)...)) {
		t.Errorf("exactly the first 100 items must be posted, got %d", len(f.posted))
	}
	if !sameStrings(f.pages, []string{"/v1/playlists/pl/items?0", "/v1/playlists/pl/items?50"}) {
		t.Errorf("the third page must not be read: %v", f.pages)
	}
}

func TestQueueAddAllExactlyAtCapIsNotTruncated(t *testing.T) {
	f := &container{playlist: true, entries: tracks(100)}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:playlist:pl"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 100 || res.Truncated || len(f.pages) != 2 {
		t.Errorf("got %+v pages %v", *res, f.pages)
	}
}

func TestQueueAddAllCapInsidePage(t *testing.T) {
	// Skipped entries do not eat the cap: 60 tracks, a local file, 60 tracks.
	entries := append(append(tracks(60), "local"), tracks(60)...)
	f := &container{playlist: true, entries: entries}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:playlist:pl"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 100 || res.Skipped != 1 || res.Total != 121 || !res.Truncated {
		t.Errorf("got %+v", *res)
	}
}

func TestQueueAddAllSkipsLocalFilesAndGaps(t *testing.T) {
	f := &container{playlist: true, entries: []string{"a", "local", "b", "", "local", "c"}}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:playlist:pl"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 3 || res.Skipped != 3 || res.Total != 6 || res.Truncated {
		t.Errorf("got %+v", *res)
	}
	if !sameStrings(f.posted, uris("a", "b", "c")) {
		t.Errorf("posted %v", f.posted)
	}
}

func TestQueueAddAllAlbumGapsAreSkipped(t *testing.T) {
	f := &container{entries: []string{"a", "", "b"}}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:album:alb"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 || res.Skipped != 1 {
		t.Errorf("got %+v", *res)
	}
}

func TestQueueAddAllOffsetPastEnd(t *testing.T) {
	f := &container{entries: tracks(12)}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:album:alb"), 40, "")
	if err != nil {
		t.Fatal(err)
	}
	want := output.QueueAddAllResult{OK: true, URI: "spotify:album:alb", Total: 12, Offset: 40}
	if *res != want || len(f.posted) != 0 {
		t.Errorf("got %+v posted %v", *res, f.posted)
	}
}

func TestQueueAddAllUnreadablePlaylist(t *testing.T) {
	f := &container{playlist: true, noItems: true}
	c, _, _ := newClient(t, f.handler(t))
	_, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:playlist:pl"), 0, "")
	oe := asErr(t, err)
	if oe.Code != output.CodeTracksUnavailable || oe.Added != 0 || oe.Total != 0 || len(f.posted) != 0 {
		t.Errorf("err %+v posted %v", oe, f.posted)
	}
}

func TestQueueAddAllRateLimitedMidway(t *testing.T) {
	f := &container{entries: tracks(30), failPost: 9}
	c, _, _ := newClient(t, f.handler(t))
	_, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:album:alb"), 0, "")
	oe := asErr(t, err)
	if oe.Code != output.CodeRateLimited || oe.RetryAfter != 12 || oe.Added != 8 || oe.Total != 30 {
		t.Errorf("err %+v", oe)
	}
	if len(f.posted) != 8 {
		t.Errorf("eight accepted and no tenth request, posted %d", len(f.posted))
	}
}

func TestQueueAddAllFailureBeforeFirstItemHasNoCounts(t *testing.T) {
	f := &container{entries: tracks(30), failPost: 1}
	c, _, _ := newClient(t, f.handler(t))
	_, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:album:alb"), 0, "")
	oe := asErr(t, err)
	if oe.Code != output.CodeRateLimited || oe.Added != 0 || oe.Total != 0 {
		t.Errorf("err %+v", oe)
	}
}

func TestQueueAddAllOrderIsSequential(t *testing.T) {
	f := &container{playlist: true, entries: tracks(70)}
	c, _, _ := newClient(t, f.handler(t))
	if _, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:playlist:pl"), 0, ""); err != nil {
		t.Fatal(err)
	}
	if !sameStrings(f.posted, uris(tracks(70)...)) {
		t.Errorf("posts are not in container order: %v", f.posted)
	}
	if f.overlap.Load() {
		t.Error("two queue requests were in flight at the same time")
	}
}

func TestQueueAddAllRejectsOtherTypes(t *testing.T) {
	f := &container{}
	c, n, _ := newClient(t, f.handler(t))
	_, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:artist:x"), 0, "")
	if oe := asErr(t, err); oe.Code != output.CodeUsage || n.Load() != 0 {
		t.Errorf("err %+v requests %d", oe, n.Load())
	}
}

func TestQueueAddAllSkipsUnplayableTracks(t *testing.T) {
	f := &container{entries: []string{"a", "b", "bad:c", "ok:d", "e", "f", "g"}}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:album:alb"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	want := output.QueueAddAllResult{OK: true, URI: "spotify:album:alb", Added: 6, Skipped: 1, Total: 7}
	if *res != want {
		t.Errorf("got %+v, want %+v", *res, want)
	}
	if !sameStrings(f.posted, uris("a", "b", "d", "e", "f", "g")) {
		t.Errorf("posted %v", f.posted)
	}
	if !sameStrings(f.markets, []string{"from_token"}) {
		t.Errorf("markets %v", f.markets)
	}
}

func TestQueueAddAllPlaylistCountsUnplayableAndLocal(t *testing.T) {
	f := &container{playlist: true, entries: []string{"a", "bad:b", "local", "c"}}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:playlist:pl"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 || res.Skipped != 2 || res.Total != 4 || res.Truncated {
		t.Errorf("got %+v", *res)
	}
	if !sameStrings(f.posted, uris("a", "c")) {
		t.Errorf("posted %v", f.posted)
	}
}

func TestQueueAddAllUnplayableDoNotUseUpTheCap(t *testing.T) {
	e := tracks(130)
	for _, i := range []int{3, 20, 44, 71, 90} {
		e[i] = "bad:" + e[i]
	}
	f := &container{playlist: true, entries: e}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:playlist:pl"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 100 || !res.Truncated || res.Skipped != 5 {
		t.Errorf("got %+v", *res)
	}
	if len(f.posted) != 100 || f.posted[3] != "spotify:track:t4" || f.posted[99] != "spotify:track:t104" {
		t.Errorf("posted %d, first %v", len(f.posted), f.posted[:5])
	}
}

func TestQueueAddAllPageOfOnlyUnplayableContinues(t *testing.T) {
	e := make([]string, 50, 53)
	for i := range e {
		e[i] = "bad:u" + strconv.Itoa(i)
	}
	f := &container{entries: append(e, "a", "b", "c")}
	c, _, _ := newClient(t, f.handler(t))
	res, err := c.QueueAddAll(context.Background(), mustURI(t, "spotify:album:alb"), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 3 || res.Skipped != 50 || res.Total != 53 || res.Truncated {
		t.Errorf("got %+v", *res)
	}
	if len(f.pages) != 2 {
		t.Errorf("pages %v", f.pages)
	}
}
