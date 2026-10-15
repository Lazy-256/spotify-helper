// Package fileutil holds the small filesystem helpers shared by the token
// store, the rate-limit cooldown and the cover cache.
package fileutil

import (
	"os"
	"path/filepath"
	"syscall"
)

// WriteAtomic replaces path with data. The bytes go to a temporary file in the
// same directory (created with mode perm) which is then renamed over path, so
// a reader never sees a partial file and a crash leaves the old content.
// Missing parent directories are created with dirPerm.
func WriteAtomic(path string, data []byte, perm, dirPerm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// Lock takes an exclusive advisory lock on the file at path (created if
// needed) and returns the function that releases it. Locks are per open file,
// so they also exclude other goroutines in the same process.
func Lock(path string, dirPerm os.FileMode) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
