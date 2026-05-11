package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// TestLoadOrInitState_RaceFreshWorktrees fires 8 goroutines, each
// representing a different worktree with its own fresh stateDir, calling
// loadOrInitState concurrently. The fix for the slot-allocation race
// (audit blocker #1) requires that each goroutine receives a distinct slot.
//
// Run with `go test -race` to catch data races on the shared lock/state.
func TestLoadOrInitState_RaceFreshWorktrees(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("state.Lock is a documented best-effort no-op on Windows; serialization not enforced")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	const n = 8
	var wg sync.WaitGroup
	slots := make([]int, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			stateDir := filepath.Join(home, ".liferay-cli", "worktrees", fmt.Sprintf("wt-%d", i), "docker")
			if err := os.MkdirAll(stateDir, 0755); err != nil {
				errs[i] = fmt.Errorf("mkdir: %w", err)
				return
			}
			s, err := loadOrInitState(stateDir, "mysql")
			if err != nil {
				errs[i] = err
				return
			}
			slots[i] = s.Slot
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	seen := map[int]int{}
	for i, s := range slots {
		if prev, ok := seen[s]; ok {
			t.Errorf("goroutines %d and %d both received slot %d", prev, i, s)
		}
		seen[s] = i
	}
}
