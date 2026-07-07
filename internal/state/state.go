// Package state computes the per-worktree CLI state directory. State lives
// under ~/.liferay-cli/worktrees/<basename>-<hash> rather than inside the
// portal source tree so that "ant all" / "ant clean" cannot delete it.
package state

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// Root returns the host-wide ~/.liferay-cli/ directory that contains every
// per-worktree subtree plus host-global state (e.g. the slot allocation
// lock file). Panics when the home directory cannot be resolved — silently
// falling back to os.TempDir() would put persistent state on a path that
// gets wiped on reboot, which is worse than refusing to run. Callers that
// can't recover from this should validate at startup via the same
// os.UserHomeDir() call so the user gets a clean error message instead of
// a stack trace.
func Root() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		panic("state.Root: HOME (USERPROFILE on Windows) is not set — liferay-cli requires a writable user home directory")
	}
	return filepath.Join(home, ".liferay-cli")
}

// Dir returns the persistent state directory for the given worktree root.
// The path is deterministic for a given absolute worktreeRoot; the hash
// suffix disambiguates worktrees that share a basename.
func Dir(worktreeRoot string) string {
	abs, err := filepath.Abs(worktreeRoot)
	if err != nil {
		abs = worktreeRoot
	}
	sum := sha1.Sum([]byte(abs))
	id := filepath.Base(abs) + "-" + hex.EncodeToString(sum[:4])
	return filepath.Join(Root(), "worktrees", id)
}

// ID returns the stable per-worktree identifier (basename + path hash) used as
// the directory name under ~/.liferay-cli/worktrees/. Unlike Dir, the value
// depends only on the absolute worktree path, not on the home directory, so it
// is safe to use as a marker that stays stable across a `sudo` invocation.
func ID(worktreeRoot string) string {
	abs, err := filepath.Abs(worktreeRoot)
	if err != nil {
		abs = worktreeRoot
	}
	sum := sha1.Sum([]byte(abs))
	return filepath.Base(abs) + "-" + hex.EncodeToString(sum[:4])
}

// WriteFileAtomic writes data to path via a temp file + rename so concurrent
// readers always see either the old or new content, never a torn write. The
// temp file is created in path's own directory so the rename can't cross a
// filesystem boundary, and is removed if anything fails before the rename
// commits. If path already exists, its mode is preserved (some callers — the
// bundle's portal-ext.properties, /etc/hosts — share the file with the user
// or the OS and must not have permissions reset out from under them);
// otherwise the new file gets defaultMode.
func WriteFileAtomic(path string, data []byte, defaultMode os.FileMode) error {
	mode := defaultMode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below has succeeded

	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// DisplayHome renders p with the user's home directory replaced by "~". If
// the home dir can't be resolved or p doesn't live under it, returns p
// unchanged.
func DisplayHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	rel, err := filepath.Rel(home, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return filepath.Join("~", rel)
}
