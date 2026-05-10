//go:build !windows

package state

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func lockImpl(path string, timeout time.Duration) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() error {
				flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				closeErr := f.Close()
				if flockErr != nil {
					return flockErr
				}
				return closeErr
			}, nil
		}
		if err != syscall.EWOULDBLOCK {
			_ = f.Close()
			return nil, fmt.Errorf("flock %s: %w", path, err)
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("timeout waiting for lock %s after %s", path, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
