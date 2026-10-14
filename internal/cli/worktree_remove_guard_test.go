package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/state"
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

// TestRemoveWorktree_RemovesNestedWorkspaceState confirms removeWorktree
// cleans up the CLI state of a Liferay Workspace nested in the worktree, not
// just the worktree root's own.
func TestRemoveWorktree_RemovesNestedWorkspaceState(t *testing.T) {
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

	workspaceStateDir := state.Dir(writeTestWorkspace(t, worktreePath, "liferay-test-workspace"))
	if err := os.MkdirAll(workspaceStateDir, 0755); err != nil {
		t.Fatal(err)
	}

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
		t.Fatalf("removeWorktree returned an error: %v", err)
	}
	if _, statErr := os.Stat(workspaceStateDir); !os.IsNotExist(statErr) {
		t.Errorf("nested workspace state directory should have been removed, stat err: %v", statErr)
	}
}

func TestTomcatGuardDir(t *testing.T) {
	workspace := writeTestWorkspace(t, t.TempDir(), "liferay-test-workspace")
	if got, want := tomcatGuardDir(workspace), filepath.Join(workspace, "bundles"); got != want {
		t.Errorf("tomcatGuardDir(workspace) = %q, want %q", got, want)
	}

	deleted := filepath.Join(t.TempDir(), "LPD-1")
	if got, want := tomcatGuardDir(deleted), deleted+string(filepath.Separator); got != want {
		t.Errorf("tomcatGuardDir(deleted) = %q, want %q", got, want)
	}
}

// TestStopFsmonitorDaemon_StopsRunningDaemon is the regression test for the
// fsmonitor daemon leak: the built-in daemon has no idle timeout, so
// removeWorktree must stop it before deleting the directory it watches, or
// it lingers indefinitely.
func TestStopFsmonitorDaemon_StopsRunningDaemon(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("HOME", t.TempDir())

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "fsmonitor--daemon", "start")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", repoDir, "fsmonitor--daemon", "stop").Run()
	})

	stopFsmonitorDaemon(repoDir)

	if exec.Command("git", "-C", repoDir, "fsmonitor--daemon", "status").Run() == nil {
		t.Error("fsmonitor daemon should have been stopped")
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

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeTestWorkspace(t *testing.T, root, name string) string {
	t.Helper()
	workspace := filepath.Join(root, "workspaces", name)
	writeTestFile(t, filepath.Join(workspace, "settings.gradle"), `apply plugin: "com.liferay.workspace"`+"\n")
	return workspace
}
