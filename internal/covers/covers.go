// Package covers keeps album, playlist and artist images in a bounded local
// cache, because the Noctalia plugin UI can only load images from disk.
package covers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/fileutil"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

// Defaults of the limits in the covers spec; Cache fields override them.
const (
	MaxFileBytes    = 2 << 20
	MaxAge          = 30 * 24 * time.Hour
	MaxTotalBytes   = 100 << 20
	CleanupInterval = 24 * time.Hour
	ApplyTimeout    = 8 * time.Second
	Workers         = 6

	stampName = ".cleanup"
	// Orphaned temporary files (from a crash mid-download) are removed after
	// this long.
	staleTemp = time.Hour
)

// Cache is a directory of downloaded images named after their URL.
type Cache struct {
	Dir string
	// HTTP downloads images. It must be a client of its own, never the one
	// that carries Spotify credentials.
	HTTP *http.Client
	Now  func() time.Time

	// Overrides for tests; zero means the default above.
	MaxFile    int64
	MaxAge     time.Duration
	MaxTotal   int64
	ApplyLimit time.Duration
}

func or[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}

// PathFor returns where the image for rawURL lives (whether or not it has
// been downloaded yet).
func (c *Cache) PathFor(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	p := filepath.Join(c.Dir, hex.EncodeToString(sum[:8])+".jpg")
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// Fetch makes sure the image is cached and returns its path. cached reports
// that no download was needed. No credentials are ever sent.
func (c *Cache) Fetch(ctx context.Context, rawURL string) (path string, cached bool, err error) {
	u, perr := url.Parse(rawURL)
	if perr != nil || u.Scheme != "https" || u.Host == "" {
		return "", false, output.Usage("cover URL must be an https:// URL, got %q", rawURL)
	}
	path = c.PathFor(rawURL)
	if fi, serr := os.Stat(path); serr == nil && fi.Mode().IsRegular() {
		now := c.Now()
		_ = os.Chtimes(path, now, now) // "last used" drives eviction
		return path, true, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", false, output.Usage("bad cover URL: %v", err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", false, output.Network(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false, output.Errorf(output.CodeAPIError, "image server answered HTTP %d", resp.StatusCode)
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); !strings.HasPrefix(ct, "image/") {
		return "", false, output.Errorf(output.CodeAPIError, "image server sent %q, not an image", ct)
	}
	limit := or(c.MaxFile, int64(MaxFileBytes))
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", false, output.Network(err)
	}
	if int64(len(body)) > limit {
		return "", false, output.Errorf(output.CodeAPIError, "image is larger than %d bytes", limit)
	}
	if len(body) == 0 {
		return "", false, output.Errorf(output.CodeAPIError, "image server sent an empty body")
	}
	if err := fileutil.WriteAtomic(path, body, 0o600, 0o700); err != nil {
		return "", false, output.Errorf(output.CodeInternal, "cannot store the cover: %v", err)
	}
	return path, false, nil
}

// Apply fills cover_path of every cover: the local path, or null when there is
// no cover or it could not be fetched in time. It never fails. Downloads run
// concurrently under one deadline; images already cached cost no network.
func (c *Cache) Apply(ctx context.Context, covers []*output.Cover) {
	var urls []string
	seen := map[string]bool{}
	for _, cv := range covers {
		if cv.URL != nil && !seen[*cv.URL] {
			seen[*cv.URL] = true
			urls = append(urls, *cv.URL)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, or(c.ApplyLimit, ApplyTimeout))
	defer cancel()

	var (
		mu         sync.Mutex
		paths      = map[string]string{}
		downloaded bool
		wg         sync.WaitGroup
		jobs       = make(chan string)
	)
	for range min(Workers, len(urls)) {
		wg.Go(func() {
			for u := range jobs {
				p, cached, err := c.Fetch(ctx, u)
				if err != nil {
					continue
				}
				mu.Lock()
				paths[u] = p
				downloaded = downloaded || !cached
				mu.Unlock()
			}
		})
	}
	for _, u := range urls {
		jobs <- u
	}
	close(jobs)
	wg.Wait()

	for _, cv := range covers {
		if cv.URL == nil {
			cv.Path.Set("")
			continue
		}
		cv.Path.Set(paths[*cv.URL])
	}
	if downloaded {
		c.MaybeCleanup()
	}
}

// MaybeCleanup runs Cleanup unless it already ran within CleanupInterval.
func (c *Cache) MaybeCleanup() {
	stamp := filepath.Join(c.Dir, stampName)
	if fi, err := os.Stat(stamp); err == nil && c.Now().Sub(fi.ModTime()) < CleanupInterval {
		return
	}
	_ = c.Cleanup()
	_ = os.WriteFile(stamp, nil, 0o600)
	now := c.Now()
	_ = os.Chtimes(stamp, now, now)
}

// Cleanup removes images unused for MaxAge, then the least recently used ones
// until the cache fits MaxTotal. Files that vanish meanwhile (another process
// cleaning up) are ignored.
func (c *Cache) Cleanup() error {
	entries, err := os.ReadDir(c.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	now := c.Now()
	maxAge := or(c.MaxAge, MaxAge)

	type file struct {
		path string
		size int64
		used time.Time
	}
	var kept []file
	var total int64
	// A failed or already-done removal (ENOENT) is fine either way.
	remove := func(p string) { _ = os.Remove(p) }
	for _, e := range entries {
		if e.IsDir() || e.Name() == stampName {
			continue
		}
		p := filepath.Join(c.Dir, e.Name())
		fi, err := e.Info()
		if err != nil {
			continue // gone already
		}
		age := now.Sub(fi.ModTime())
		if strings.HasPrefix(e.Name(), ".") { // temporary file of a download
			if age > staleTemp {
				remove(p)
			}
			continue
		}
		if age > maxAge {
			remove(p)
			continue
		}
		kept = append(kept, file{p, fi.Size(), fi.ModTime()})
		total += fi.Size()
	}

	sort.Slice(kept, func(i, j int) bool { return kept[i].used.Before(kept[j].used) })
	limit := or(c.MaxTotal, int64(MaxTotalBytes))
	for _, f := range kept {
		if total <= limit {
			break
		}
		remove(f.path)
		total -= f.size
	}
	return nil
}
