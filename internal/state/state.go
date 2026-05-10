// Package state computes the per-worktree CLI state directory. State lives
// under ~/.liferay-cli/worktrees/<basename>-<hash> rather than inside the
// portal source tree so that "ant all" / "ant clean" cannot delete it.
package state

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
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
