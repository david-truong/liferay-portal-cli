package state

import "time"

// Lock acquires an exclusive advisory file lock at path, creating the file
// (and any parent directories) if necessary. The returned unlock function
// must be called to release the lock; failing to call it leaks an open file
// descriptor until the process exits.
//
// Behavior is platform-specific:
//   - Unix (Linux, macOS, BSD): uses flock(2) LOCK_EX. Two processes calling
//     Lock on the same path serialize against each other.
//   - Windows: best-effort. The lock file is created but no OS-level lock is
//     held — cross-process serialization is not enforced. This is a known
//     limitation tracked for a future release; in practice the Windows
//     workflow is single-user and single-shell.
//
// If timeout elapses before the lock is acquired, Lock returns a non-nil
// error and the unlock function is nil. timeout=0 means "try once and fail
// immediately if the lock is held."
func Lock(path string, timeout time.Duration) (func() error, error) {
	return lockImpl(path, timeout)
}
