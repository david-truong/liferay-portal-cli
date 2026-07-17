package cli

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/state"
)

// mustGitInitOnBranch inits a repo in dir with its initial (unborn) branch
// named branchName. Tests avoid ever committing on a branch literally named
// "master"/"main" — some developer machines run a global pre-commit hook
// that blocks that regardless of which repo it is, so a "master" ref here is
// created via mustGitBranchFrom/mustGitForceBranch instead of checkout+commit.
func mustGitInitOnBranch(t *testing.T, dir, branchName string) {
	t.Helper()
	mustGitInit(t, dir)
	if out, err := exec.Command("git", "-C", dir, "symbolic-ref", "HEAD", "refs/heads/"+branchName).CombinedOutput(); err != nil {
		t.Fatalf("git symbolic-ref: %v\n%s", err, out)
	}
}

// mustGitCommit stages every file in dir and commits with message, using a
// fixed test identity so the commit doesn't depend on host git config.
func mustGitCommit(t *testing.T, dir, message string) string {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd := exec.Command("git", "-C", dir, "commit", "-m", message)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	return mustGitRevParse(t, dir, "HEAD")
}

func mustGitRevParse(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", rev).Output()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v", rev, err)
	}
	return string(bytes.TrimSpace(out))
}

func mustGitCheckout(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", dir, "checkout"}, args...)
	if out, err := exec.Command("git", cmdArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git checkout %v: %v\n%s", args, err, out)
	}
}

// mustGitBranchFrom creates newBranch pointing at ref, without checking it
// out (so it never runs afoul of a "no direct commits to master" hook).
func mustGitBranchFrom(t *testing.T, dir, newBranch, ref string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "branch", newBranch, ref).CombinedOutput(); err != nil {
		t.Fatalf("git branch %s %s: %v\n%s", newBranch, ref, err, out)
	}
}

// mustGitForceBranch moves branch's tip to ref without checking it out.
func mustGitForceBranch(t *testing.T, dir, branch, ref string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "branch", "-f", branch, ref).CombinedOutput(); err != nil {
		t.Fatalf("git branch -f %s %s: %v\n%s", branch, ref, err, out)
	}
}

// mustGitRebase rebases the current branch onto ref. Needs a committer
// identity to replay commits, which CI runners don't have configured
// globally, so it sets one explicitly rather than relying on host git config.
func mustGitRebase(t *testing.T, dir, ref string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rebase", ref)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rebase %s: %v\n%s", ref, err, out)
	}
}

// captureStderr redirects os.Stderr for the duration of fn and returns
// whatever was written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestMergeBaseSHA_ReturnsCommonAncestorWithMaster(t *testing.T) {
	root := t.TempDir()
	mustGitInitOnBranch(t, root, "trunk")

	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	base := mustGitCommit(t, root, "initial")
	mustGitBranchFrom(t, root, "master", "trunk")

	mustGitCheckout(t, root, "-b", "feature", "trunk")
	if err := os.WriteFile(filepath.Join(root, "b.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	mustGitCommit(t, root, "feature work")

	if got := mergeBaseSHA(root); got != base {
		t.Errorf("mergeBaseSHA() = %q, want %q (the commit feature branched from)", got, base)
	}
}

func TestMergeBaseSHA_NoMasterRefReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	mustGitInitOnBranch(t, root, "trunk")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	mustGitCommit(t, root, "initial")
	// No "master" branch was ever created, so mergeBaseSHA's hardcoded ref
	// can't resolve.

	if got := mergeBaseSHA(root); got != "" {
		t.Errorf("mergeBaseSHA() = %q, want \"\" when no master ref exists", got)
	}
}

func TestWarnIfRebased_NoWarningWhenBaseUnchanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	mustGitInitOnBranch(t, root, "trunk")

	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	mustGitCommit(t, root, "initial")
	mustGitBranchFrom(t, root, "master", "trunk")

	mustGitCheckout(t, root, "-b", "feature", "trunk")
	if err := os.WriteFile(filepath.Join(root, "b.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	mustGitCommit(t, root, "feature work")

	recordBuildBase(root)

	out := captureStderr(t, func() { warnIfRebased(root) })
	if out != "" {
		t.Errorf("expected no warning when the merge-base hasn't moved, got: %q", out)
	}
}

func TestWarnIfRebased_WarnsAfterRebaseOntoNewMaster(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	mustGitInitOnBranch(t, root, "trunk")

	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	mustGitCommit(t, root, "initial")
	mustGitBranchFrom(t, root, "master", "trunk")

	mustGitCheckout(t, root, "-b", "feature", "trunk")
	if err := os.WriteFile(filepath.Join(root, "b.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	mustGitCommit(t, root, "feature work")

	// Simulate the last "ant all" having run here, against the original base.
	recordBuildBase(root)

	// master gains a new commit (built on a scratch branch so the commit
	// itself never runs with HEAD on "master"), then feature is rebased onto
	// it — the merge-base moves forward even though feature's own content is
	// unchanged.
	mustGitCheckout(t, root, "-b", "temp", "master")
	if err := os.WriteFile(filepath.Join(root, "c.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	mustGitCommit(t, root, "master moves on")
	mustGitCheckout(t, root, "feature")
	mustGitForceBranch(t, root, "master", "temp")
	if out, err := exec.Command("git", "-C", root, "branch", "-D", "temp").CombinedOutput(); err != nil {
		t.Fatalf("git branch -D temp: %v\n%s", err, out)
	}

	mustGitRebase(t, root, "master")

	out := captureStderr(t, func() { warnIfRebased(root) })
	if !bytes.Contains([]byte(out), []byte(`branch has moved onto a new base`)) {
		t.Errorf("expected a rebase warning, got: %q", out)
	}
}

// TestRunWorkspaceBuildAll_RecordsBuildBase proves a Workspace's "ant all"
// equivalent records a build base too, not just the monorepo's runAntAll —
// otherwise warnIfRebased could never fire for Workspace projects.
func TestRunWorkspaceBuildAll_RecordsBuildBase(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	mustGitInitOnBranch(t, root, "trunk")

	if err := os.WriteFile(filepath.Join(root, "settings.gradle"), []byte(`apply plugin: "com.liferay.workspace"`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gradlewSh := filepath.Join(root, "gradlew")
	if err := os.WriteFile(gradlewSh, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	gradlewBat := filepath.Join(root, "gradlew.bat")
	if err := os.WriteFile(gradlewBat, []byte("@echo off\r\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "modules", "my-module"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules", "my-module", "bnd.bnd"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	base := mustGitCommit(t, root, "initial")
	mustGitBranchFrom(t, root, "master", "trunk")

	if err := runWorkspaceBuildAll(root); err != nil {
		t.Fatalf("runWorkspaceBuildAll: %v", err)
	}

	rec, ok, err := state.LoadBuildBase(root)
	if err != nil {
		t.Fatalf("LoadBuildBase: %v", err)
	}
	if !ok {
		t.Fatal("expected runWorkspaceBuildAll to record a build base, found none")
	}
	if rec.SHA != base {
		t.Errorf("recorded base SHA = %q, want %q", rec.SHA, base)
	}
}

func TestWarnIfRebased_NoRecordYet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	mustGitInitOnBranch(t, root, "trunk")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	mustGitCommit(t, root, "initial")

	out := captureStderr(t, func() { warnIfRebased(root) })
	if out != "" {
		t.Errorf("expected no warning when \"ant all\" has never recorded a build base, got: %q", out)
	}
}
