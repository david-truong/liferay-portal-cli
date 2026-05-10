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

// Dir returns the persistent state directory for the given worktree root.
// The path is deterministic for a given absolute worktreeRoot; the hash
// suffix disambiguates worktrees that share a basename.
func Dir(worktreeRoot string) string {
	abs, err := filepath.Abs(worktreeRoot)
	if err != nil {
		abs = worktreeRoot
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	sum := sha1.Sum([]byte(abs))
	id := filepath.Base(abs) + "-" + hex.EncodeToString(sum[:4])
	return filepath.Join(home, ".liferay-cli", "worktrees", id)
}

// WriteFileAtomic writes data to path via a temp file + rename so concurrent
// readers always see either the old or new content, never a torn write.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
