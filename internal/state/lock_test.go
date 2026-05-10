//go:build !windows

package state

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLock_SerializesParallelHolders fires 8 goroutines all trying to take an
// exclusive lock on the same file. Each goroutine holds for ~10ms, then
// releases. The test passes if at no point during the run did two goroutines
// hold the lock simultaneously.
//
// Skipped on Windows where Lock degrades to a best-effort no-op.
func TestLock_SerializesParallelHolders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	var holders, maxHolders int32
	var wg sync.WaitGroup

	const n = 8
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := Lock(path, 5*time.Second)
			if err != nil {
				t.Errorf("Lock: %v", err)
				return
			}
			cur := atomic.AddInt32(&holders, 1)
			for {
				m := atomic.LoadInt32(&maxHolders)
				if cur <= m || atomic.CompareAndSwapInt32(&maxHolders, m, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&holders, -1)
			if err := unlock(); err != nil {
				t.Errorf("unlock: %v", err)
			}
		}()
	}
	wg.Wait()

	if maxHolders > 1 {
		t.Errorf("expected at most 1 simultaneous holder, observed %d", maxHolders)
	}
}

// TestLock_Timeout takes a lock, then in parallel tries to take the same lock
// with a tight timeout. The second attempt must time out with an error.
func TestLock_Timeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timeout.lock")

	unlock, err := Lock(path, time.Second)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	defer unlock()

	start := time.Now()
	_, err = Lock(path, 100*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("expected timeout error when lock is already held")
	}
	if elapsed < 90*time.Millisecond {
		t.Errorf("Lock returned before timeout elapsed: %v", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Lock waited well past timeout: %v", elapsed)
	}
}

// TestLock_AcquireAfterRelease shows the lock is reusable: one holder
// releases, a second holder acquires immediately.
func TestLock_AcquireAfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reuse.lock")

	unlock, err := Lock(path, time.Second)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	unlock2, err := Lock(path, time.Second)
	if err != nil {
		t.Fatalf("second Lock after release: %v", err)
	}
	if err := unlock2(); err != nil {
		t.Fatalf("second unlock: %v", err)
	}
}
