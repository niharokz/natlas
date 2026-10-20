//go:build unix

package store

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockPath takes an exclusive flock on <dir>/<hash of path>.lock (waits for the other process).
func lockPath(dir, path string) (func(), error) {
	sum := sha1.Sum([]byte(path))
	name := filepath.Join(dir, hex.EncodeToString(sum[:8])+".lock")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock file: %w", err)
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
