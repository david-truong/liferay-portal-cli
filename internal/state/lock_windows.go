//go:build windows

package state

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Windows fallback: touch the lock file so callers can rely on its existence,
// but do not hold an OS-level advisory lock. Cross-process serialization is
// not enforced. See lock.go.
func lockImpl(path string, _ time.Duration) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return func() error { return f.Close() }, nil
}
