package ratelimit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lazy-256/spotify-helper/internal/output"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newGate(t *testing.T) (*Gate, *clock) {
	t.Helper()
	c := &clock{t: time.Unix(1_700_000_000, 0)}
	return &Gate{Path: filepath.Join(t.TempDir(), "cache", "rate-limit.json"), Now: c.now}, c
}

func TestParseRetryAfter(t *testing.T) {
	tests := map[string]int{
		"12":                            12,
		" 7 ":                           7,
		"":                              DefaultWait,
		"soon":                          DefaultWait,
		"-5":                            DefaultWait,
		"1.5":                           DefaultWait,
		"Wed, 21 Oct 2026 07:28:00 GMT": DefaultWait,
		"0":                             1,
		"999999999":                     MaxWait,
	}
	for in, want := range tests {
		if got := ParseRetryAfter(in); got != want {
			t.Errorf("ParseRetryAfter(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestRecordReportsRetryAfter(t *testing.T) {
	g, _ := newGate(t)
	err := g.Record("12")
	if err.Code != output.CodeRateLimited || err.RetryAfter != 12 {
		t.Errorf("got %+v", err)
	}
}

func TestNoCooldownByDefault(t *testing.T) {
	g, _ := newGate(t)
	if err := g.Check(); err != nil {
		t.Errorf("Check with no file: %v", err)
	}
}

func TestCooldownActiveThenOver(t *testing.T) {
	g, c := newGate(t)
	g.Record("12")

	c.advance(5 * time.Second)
	err := g.Check()
	var oe *output.Error
	if !errors.As(err, &oe) || oe.Code != output.CodeRateLimited {
		t.Fatalf("want rate_limited, got %v", err)
	}
	if oe.RetryAfter < 1 || oe.RetryAfter > 7 {
		t.Errorf("retry_after %d, want 1..7", oe.RetryAfter)
	}

	c.advance(8 * time.Second)
	if err := g.Check(); err != nil {
		t.Errorf("cooldown should be over: %v", err)
	}
}

func TestLaterDeadlineIsKept(t *testing.T) {
	g, c := newGate(t)
	g.Record("100")
	c.advance(time.Second)
	g.Record("5") // would end earlier than the recorded one
	if left := g.Remaining(); left < 90 {
		t.Errorf("remaining %d, the longer cooldown must be kept", left)
	}
	g.Record("500")
	if left := g.Remaining(); left < 400 {
		t.Errorf("remaining %d, the longer cooldown must replace", left)
	}
}

func TestCorruptFileMeansNoCooldown(t *testing.T) {
	g, _ := newGate(t)
	if err := os.MkdirAll(filepath.Dir(g.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(g.Path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := g.Check(); err != nil {
		t.Errorf("corrupt state must not block calls: %v", err)
	}
}
