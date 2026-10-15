// Package ratelimit persists Spotify's 429 cooldown on disk so that separate
// helper processes fail fast instead of hammering a rate-limited app.
package ratelimit

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/fileutil"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

const (
	// DefaultWait is used when Retry-After is missing, malformed or negative.
	DefaultWait = 30
	// MaxWait caps absurd Retry-After values.
	MaxWait = 24 * 60 * 60
)

// Gate reads and writes the cooldown file.
type Gate struct {
	Path string
	Now  func() time.Time
}

type stateFile struct {
	UntilUnix int64 `json:"until_unix"`
}

// ParseRetryAfter turns a Retry-After header value into seconds: at least 1,
// DefaultWait when the value is absent or not a non-negative integer.
func ParseRetryAfter(h string) int {
	n, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || n < 0 {
		return DefaultWait
	}
	return min(max(n, 1), MaxWait)
}

// Remaining returns the whole seconds left of an active cooldown, or 0.
func (g *Gate) Remaining() int {
	st, err := g.read()
	if err != nil {
		return 0
	}
	left := st.UntilUnix - g.Now().Unix()
	if left <= 0 {
		return 0
	}
	return int(left)
}

// Check fails with rate_limited while a recorded cooldown is active.
func (g *Gate) Check() error {
	if left := g.Remaining(); left > 0 {
		return output.RateLimited(left)
	}
	return nil
}

// Record stores the cooldown announced by a 429 response and returns the
// rate_limited error to report. An existing, later deadline is kept.
func (g *Gate) Record(retryAfterHeader string) *output.Error {
	wait := ParseRetryAfter(retryAfterHeader)
	until := g.Now().Unix() + int64(wait)

	unlock, err := fileutil.Lock(g.Path+".lock", 0o700)
	if err == nil {
		defer unlock()
		if cur, rerr := g.read(); rerr == nil && cur.UntilUnix > until {
			until = cur.UntilUnix
		}
		if data, merr := json.Marshal(stateFile{UntilUnix: until}); merr == nil {
			// A failed write only loses the fail-fast optimisation.
			_ = fileutil.WriteAtomic(g.Path, data, 0o600, 0o700)
		}
	}
	return output.RateLimited(wait)
}

func (g *Gate) read() (stateFile, error) {
	var st stateFile
	data, err := os.ReadFile(g.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return st, nil
		}
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}
