package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoveWorktree_RefusesUnregisteredPath is the regression test for
// audit finding HIGH-1: removeWorktree must never os.RemoveAll a path that
// isn't a registered linked worktree of the current repository, even with
// assumeYes — a typo or an arbitrary directory (e.g. ~/Documents) must
// survive.
func TestRemoveWorktree_RefusesUnregisteredPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	target := t.TempDir()
	canary := filepath.Join(target, "canary")
	if err := os.WriteFile(canary, []byte("survive me"), 0644); err != nil {
		t.Fatal(err)
	}

	err := removeWorktree(target, true /* assumeYes */, strings.NewReader(""), &bytes.Buffer{}, false)

	if err == nil {
		t.Fatal("expected an error for a path that is not a registered worktree")
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Errorf("target directory should still exist, got: %v", statErr)
	}
	if _, statErr := os.Stat(canary); statErr != nil {
		t.Errorf("canary file should still exist, got: %v", statErr)
	}
}

// TestRemoveWorktree_RemovesRegisteredLinkedWorktree builds a real git repo
// with a real linked worktree (via `git worktree add`) and confirms
// removeWorktree deletes it — the guard added for HIGH-1 must not reject
// legitimate targets.
func TestRemoveWorktree_RemovesRegisteredLinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("HOME", t.TempDir())

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "initial")

	worktreePath := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, repoDir, "worktree", "add", worktreePath, "-b", "tmp-branch")

	// removeWorktree's git invocations run in the process's working
	// directory, so point the process at the repo for the duration of the
	// test.
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origWD); err != nil {
			t.Fatal(err)
		}
	})

	err = removeWorktree(worktreePath, true /* assumeYes */, strings.NewReader(""), &bytes.Buffer{}, false)

	if err != nil {
		t.Fatalf("removeWorktree returned an error for a real linked worktree: %v", err)
	}
	if _, statErr := os.Stat(worktreePath); !os.IsNotExist(statErr) {
		t.Errorf("worktree directory should have been removed, stat err: %v", statErr)
	}
}

// runGit runs git with args in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
