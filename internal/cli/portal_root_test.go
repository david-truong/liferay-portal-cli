package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsLinkedWorktreeWithDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if isLinkedWorktree(root) {
		t.Error("directory .git should not be a linked worktree")
	}
}

func TestIsLinkedWorktreeWithFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /some/path"), 0644); err != nil {
		t.Fatal(err)
	}
	if !isLinkedWorktree(root) {
		t.Error("file .git should be detected as a linked worktree")
	}
}

func TestIsLinkedWorktreeWithNoGit(t *testing.T) {
	root := t.TempDir()
	if isLinkedWorktree(root) {
		t.Error("missing .git should not be a linked worktree")
	}
}
