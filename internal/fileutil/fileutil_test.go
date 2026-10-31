package fileutil

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriteAtomic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	path := filepath.Join(dir, "f.json")

	if err := WriteAtomic(path, []byte("one"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("two"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "two" {
		t.Fatalf("read %q %v", got, err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, want 0600", fi.Mode().Perm())
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v, want 0700", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

func TestWriteAtomicLeavesOldFileWhenDirIsReadOnlyTarget(t *testing.T) {
	// Writing to a path whose parent is a regular file must fail without
	// touching anything.
	base := t.TempDir()
	blocker := filepath.Join(base, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(filepath.Join(blocker, "child"), []byte("y"), 0o600, 0o700); err == nil {
		t.Fatal("expected an error")
	}
	if got, _ := os.ReadFile(blocker); string(got) != "x" {
		t.Errorf("blocker changed: %q", got)
	}
}

func TestLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	var inside, maxInside int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			unlock, err := Lock(path, 0o700)
			if err != nil {
				t.Error(err)
				return
			}
			n := atomic.AddInt32(&inside, 1)
			for {
				m := atomic.LoadInt32(&maxInside)
				if n <= m || atomic.CompareAndSwapInt32(&maxInside, m, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&inside, -1)
			unlock()
		})
	}
	wg.Wait()
	if maxInside != 1 {
		t.Errorf("%d holders at once, want 1", maxInside)
	}
}

func TestWriteAtomicRemovesStaleTemps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	stale := filepath.Join(dir, ".token.json.tmp-old")
	fresh := filepath.Join(dir, ".token.json.tmp-new")
	other := filepath.Join(dir, ".other.json.tmp-old")
	for _, f := range []string{stale, fresh, other} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, f := range []string{stale, other} {
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteAtomic(path, []byte("one"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("the stale temporary file of this target was kept")
	}
	for _, f := range []string{fresh, other} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s must stay: %v", filepath.Base(f), err)
		}
	}
}
