package covers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

var fixedNow = time.Unix(1_700_000_000, 0)

type imageServer struct {
	*httptest.Server
	hits    atomic.Int32
	headers []http.Header
	mu      sync.Mutex
	handler func(w http.ResponseWriter, r *http.Request)
}

func newServer(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *imageServer {
	t.Helper()
	s := &imageServer{handler: h}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.mu.Lock()
		s.headers = append(s.headers, r.Header.Clone())
		s.mu.Unlock()
		s.handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func png(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write([]byte("\xff\xd8\xff-image-bytes-for-" + r.URL.Path))
}

func newCache(t *testing.T, s *imageServer) *Cache {
	t.Helper()
	return &Cache{Dir: filepath.Join(t.TempDir(), "covers"), HTTP: s.Client(), Now: func() time.Time { return fixedNow }}
}

func code(err error) output.Code {
	var oe *output.Error
	if errors.As(err, &oe) {
		return oe.Code
	}
	return ""
}

func cover(url string) *output.Cover {
	c := &output.Cover{}
	if url != "" {
		c.URL = &url
	}
	return c
}

// ---- Fetch ----

func TestFetchFirstThenCached(t *testing.T) {
	s := newServer(t, png)
	c := newCache(t, s)
	url := s.URL + "/image/abc"

	path, cached, err := c.Fetch(context.Background(), url)
	if err != nil || cached {
		t.Fatalf("first fetch: %q cached=%v err=%v", path, cached, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "/image/abc") {
		t.Fatalf("stored file: %q %v", data, err)
	}
	if !filepath.IsAbs(path) || filepath.Dir(path) != c.Dir {
		t.Errorf("path %q should be absolute inside %q", path, c.Dir)
	}

	path2, cached, err := c.Fetch(context.Background(), url)
	if err != nil || !cached || path2 != path {
		t.Errorf("second fetch: %q cached=%v err=%v", path2, cached, err)
	}
	if s.hits.Load() != 1 {
		t.Errorf("%d requests, want 1", s.hits.Load())
	}
	other, _, _ := c.Fetch(context.Background(), s.URL+"/image/other")
	if other == path {
		t.Error("different URLs must map to different files")
	}
}

func TestFetchSendsNoCredentials(t *testing.T) {
	s := newServer(t, png)
	c := newCache(t, s)
	if _, _, err := c.Fetch(context.Background(), s.URL+"/image/x"); err != nil {
		t.Fatal(err)
	}
	h := s.headers[0]
	for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
		if h.Get(name) != "" {
			t.Errorf("%s header sent: %q", name, h.Get(name))
		}
	}
}

func TestFetchRejectsNonHTTPS(t *testing.T) {
	c := &Cache{Dir: t.TempDir(), Now: func() time.Time { return fixedNow }}
	for _, u := range []string{"http://example.com/a.jpg", "", "ftp://x/y", "https://", "not a url", "file:///etc/passwd"} {
		if _, _, err := c.Fetch(context.Background(), u); code(err) != output.CodeUsage {
			t.Errorf("%q: want usage, got %v", u, err)
		}
	}
}

func TestFetchRejectsBadResponses(t *testing.T) {
	tests := map[string]func(w http.ResponseWriter, r *http.Request){
		"html": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		},
		"no content type": func(w http.ResponseWriter, r *http.Request) {
			w.Header()["Content-Type"] = nil
			_, _ = w.Write([]byte("xx"))
		},
		"404": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			w.WriteHeader(404)
		},
		"empty": func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Content-Type", "image/jpeg") },
		"oversized": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte(strings.Repeat("x", 100)))
		},
	}
	for name, h := range tests {
		s := newServer(t, h)
		c := newCache(t, s)
		c.MaxFile = 50
		_, _, err := c.Fetch(context.Background(), s.URL+"/i")
		if code(err) != output.CodeAPIError {
			t.Errorf("%s: want api_error, got %v", name, err)
		}
		if entries, _ := os.ReadDir(c.Dir); len(entries) != 0 {
			t.Errorf("%s: nothing may be cached, found %v", name, entries)
		}
	}
}

func TestFetchExactlyAtLimitIsAccepted(t *testing.T) {
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(strings.Repeat("x", 50)))
	})
	c := newCache(t, s)
	c.MaxFile = 50
	if _, _, err := c.Fetch(context.Background(), s.URL+"/i"); err != nil {
		t.Errorf("a file of exactly the limit is fine: %v", err)
	}
}

func TestFetchNetworkFailure(t *testing.T) {
	s := newServer(t, png)
	url := s.URL + "/x"
	s.Close()
	c := newCache(t, s)
	if _, _, err := c.Fetch(context.Background(), url); code(err) != output.CodeNetworkError {
		t.Errorf("want network_error, got %v", err)
	}
}

func TestFetchTouchesFileOnHit(t *testing.T) {
	s := newServer(t, png)
	c := newCache(t, s)
	p, _, _ := c.Fetch(context.Background(), s.URL+"/x")
	old := fixedNow.Add(-48 * time.Hour)
	_ = os.Chtimes(p, old, old)
	_, _, _ = c.Fetch(context.Background(), s.URL+"/x")
	if fi, _ := os.Stat(p); !fi.ModTime().Equal(fixedNow) {
		t.Errorf("mtime %v, a cache hit must record the use", fi.ModTime())
	}
}

// ---- Apply ----

func TestApplyWarmCacheNeedsNoRequest(t *testing.T) {
	s := newServer(t, png)
	c := newCache(t, s)
	url := s.URL + "/warm"
	_, _, _ = c.Fetch(context.Background(), url)
	s.hits.Store(0)

	cv := cover(url)
	c.Apply(context.Background(), []*output.Cover{cv})
	if cv.Path.IsZero() || cv.Path.Path() != c.PathFor(url) {
		t.Errorf("cover path %q", cv.Path.Path())
	}
	if s.hits.Load() != 0 {
		t.Errorf("%d requests for a cached image", s.hits.Load())
	}
}

func TestApplyOneFailureDoesNotFailTheRest(t *testing.T) {
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/bad") {
			w.WriteHeader(500)
			return
		}
		png(w, r)
	})
	c := newCache(t, s)
	good1, bad, good2, none := cover(s.URL+"/a"), cover(s.URL+"/bad"), cover(s.URL+"/b"), cover("")
	c.Apply(context.Background(), []*output.Cover{good1, bad, good2, none})

	if good1.Path.Path() == "" || good2.Path.Path() == "" {
		t.Error("successful covers must get paths")
	}
	for name, cv := range map[string]*output.Cover{"failed": bad, "no url": none} {
		if cv.Path.IsZero() || cv.Path.Path() != "" {
			t.Errorf("%s: cover_path must be requested-but-null, got zero=%v path=%q", name, cv.Path.IsZero(), cv.Path.Path())
		}
	}
}

func TestApplyDeduplicates(t *testing.T) {
	s := newServer(t, png)
	c := newCache(t, s)
	a, b, d := cover(s.URL+"/same"), cover(s.URL+"/same"), cover(s.URL+"/same")
	c.Apply(context.Background(), []*output.Cover{a, b, d})
	if s.hits.Load() != 1 {
		t.Errorf("%d downloads for one URL", s.hits.Load())
	}
	if a.Path.Path() == "" || a.Path.Path() != b.Path.Path() || b.Path.Path() != d.Path.Path() {
		t.Error("all covers with the same URL share one path")
	}
}

func TestApplyDeadline(t *testing.T) {
	release := make(chan struct{})
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/slow") {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		png(w, r)
	})
	defer close(release)
	c := newCache(t, s)
	c.ApplyLimit = 200 * time.Millisecond

	fast, slow := cover(s.URL+"/fast"), cover(s.URL+"/slow")
	start := time.Now()
	c.Apply(context.Background(), []*output.Cover{fast, slow})
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("Apply took %v, the shared deadline was 200ms", d)
	}
	if fast.Path.Path() == "" {
		t.Error("the fast cover should have made it")
	}
	if slow.Path.IsZero() || slow.Path.Path() != "" {
		t.Error("the late cover must be null, not an error")
	}
}

func TestApplyUsesParallelWorkers(t *testing.T) {
	var inflight, peak atomic.Int32
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := inflight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		inflight.Add(-1)
		png(w, r)
	})
	c := newCache(t, s)
	var covers []*output.Cover
	for i := range 12 {
		covers = append(covers, cover(s.URL+"/i"+string(rune('a'+i))))
	}
	c.Apply(context.Background(), covers)
	if p := peak.Load(); p < 2 || p > Workers {
		t.Errorf("peak concurrency %d, want between 2 and %d", p, Workers)
	}
}

// ---- Cleanup ----

func writeAged(t *testing.T, dir, name string, size int, age time.Duration) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(strings.Repeat("x", size)), 0o600); err != nil {
		t.Fatal(err)
	}
	mt := fixedNow.Add(-age)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
	return p
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestCleanupRemovesOldFiles(t *testing.T) {
	c := &Cache{Dir: filepath.Join(t.TempDir(), "covers"), Now: func() time.Time { return fixedNow }}
	old := writeAged(t, c.Dir, "old.jpg", 10, 31*24*time.Hour)
	fresh := writeAged(t, c.Dir, "fresh.jpg", 10, 29*24*time.Hour)
	if err := c.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if exists(old) || !exists(fresh) {
		t.Errorf("old exists=%v fresh exists=%v", exists(old), exists(fresh))
	}
}

func TestCleanupEnforcesSizeCapLeastRecentlyUsedFirst(t *testing.T) {
	c := &Cache{Dir: filepath.Join(t.TempDir(), "covers"), Now: func() time.Time { return fixedNow }, MaxTotal: 250}
	a := writeAged(t, c.Dir, "a.jpg", 100, 3*time.Hour) // oldest use
	b := writeAged(t, c.Dir, "b.jpg", 100, 2*time.Hour)
	d := writeAged(t, c.Dir, "d.jpg", 100, 1*time.Hour)
	if err := c.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if exists(a) || !exists(b) || !exists(d) {
		t.Errorf("only the least recently used file should go: a=%v b=%v d=%v", exists(a), exists(b), exists(d))
	}
}

func TestCleanupTemporaryFiles(t *testing.T) {
	c := &Cache{Dir: filepath.Join(t.TempDir(), "covers"), Now: func() time.Time { return fixedNow }}
	stale := writeAged(t, c.Dir, ".x.jpg.tmp-1", 5, 2*time.Hour)
	live := writeAged(t, c.Dir, ".y.jpg.tmp-2", 5, time.Minute) // a download in progress
	_ = c.Cleanup()
	if exists(stale) || !exists(live) {
		t.Errorf("stale temp exists=%v, live temp exists=%v", exists(stale), exists(live))
	}
}

func TestCleanupMissingDirectory(t *testing.T) {
	c := &Cache{Dir: filepath.Join(t.TempDir(), "nope"), Now: func() time.Time { return fixedNow }}
	if err := c.Cleanup(); err != nil {
		t.Error(err)
	}
}

func TestCleanupToleratesConcurrentRemoval(t *testing.T) {
	c := &Cache{Dir: filepath.Join(t.TempDir(), "covers"), Now: func() time.Time { return fixedNow }, MaxTotal: 1}
	for i := range 200 {
		writeAged(t, c.Dir, "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".jpg", 10, time.Duration(i)*time.Minute)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			if err := c.Cleanup(); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent cleanup failed: %v", err)
	}
}

func TestMaybeCleanupRunsOncePerDay(t *testing.T) {
	now := fixedNow
	c := &Cache{Dir: filepath.Join(t.TempDir(), "covers"), Now: func() time.Time { return now }}
	writeAged(t, c.Dir, "first.jpg", 5, 40*24*time.Hour)
	c.MaybeCleanup() // first run: removes it and stamps
	if exists(filepath.Join(c.Dir, "first.jpg")) {
		t.Fatal("the first run should clean up")
	}

	late := writeAged(t, c.Dir, "late.jpg", 5, 40*24*time.Hour)
	now = now.Add(2 * time.Hour)
	c.MaybeCleanup() // same day: skipped
	if !exists(late) {
		t.Error("a second cleanup within a day must be skipped")
	}

	now = now.Add(25 * time.Hour)
	c.MaybeCleanup()
	if exists(late) {
		t.Error("after a day the cleanup runs again")
	}
	if !exists(filepath.Join(c.Dir, stampName)) {
		t.Error("the stamp file must be kept out of the cleanup")
	}
}
