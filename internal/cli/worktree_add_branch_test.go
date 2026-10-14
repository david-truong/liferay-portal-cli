package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRunWorktreeAdd_CreatesBranchFromMasterWhenMissing confirms that
// "worktree add <branch>" creates <branch> from master instead of failing
// when no such branch exists yet, locally or on origin.
func TestRunWorktreeAdd_CreatesBranchFromMasterWhenMissing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("HOME", t.TempDir())

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "master")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "initial")

	chdir(t, repoDir)

	worktreePath := filepath.Join(t.TempDir(), "new-worktree")
	if err := runWorktreeAdd(nil, []string{"LPD-99999", worktreePath}); err != nil {
		t.Fatalf("runWorktreeAdd returned an error: %v", err)
	}

	branch := gitOutputInDir(t, worktreePath, "branch", "--show-current")
	if branch != "LPD-99999" {
		t.Errorf("expected new worktree to be on branch LPD-99999, got %q", branch)
	}

	masterHead := gitOutputInDir(t, repoDir, "rev-parse", "master")
	branchHead := gitOutputInDir(t, worktreePath, "rev-parse", "HEAD")
	if branchHead != masterHead {
		t.Errorf("expected LPD-99999 to start at master's HEAD (%s), got %s", masterHead, branchHead)
	}
}

// TestRunWorktreeAdd_UsesExistingBranch confirms that an already-existing
// branch is attached to the new worktree as-is, not recreated from master.
func TestRunWorktreeAdd_UsesExistingBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("HOME", t.TempDir())

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "master")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "initial")
	runGit(t, repoDir, "branch", "existing-branch")

	chdir(t, repoDir)

	worktreePath := filepath.Join(t.TempDir(), "new-worktree")
	if err := runWorktreeAdd(nil, []string{"existing-branch", worktreePath}); err != nil {
		t.Fatalf("runWorktreeAdd returned an error: %v", err)
	}

	branch := gitOutputInDir(t, worktreePath, "branch", "--show-current")
	if branch != "existing-branch" {
		t.Errorf("expected new worktree to be on branch existing-branch, got %q", branch)
	}
}

// TestRunWorktreeAdd_CreatesBranchFromPrimaryBranchWhenPrimaryIsNotAProject
// confirms that a missing branch is created from the primary checkout's
// current branch when the primary is not a Liferay project root, as with
// liferay-portal-ee's sparse master-private checkout, and that user-local
// files are still propagated.
func TestRunWorktreeAdd_CreatesBranchFromPrimaryBranchWhenPrimaryIsNotAProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("HOME", t.TempDir())

	repoDir := newSparsePrimary(t)
	if err := os.WriteFile(filepath.Join(repoDir, "CLAUDE.local.md"), []byte("local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, repoDir)

	worktreePath := filepath.Join(t.TempDir(), "new-worktree")
	if err := runWorktreeAdd(nil, []string{"LPD-99999", worktreePath}); err != nil {
		t.Fatalf("runWorktreeAdd returned an error: %v", err)
	}

	masterPrivateHead := gitOutputInDir(t, repoDir, "rev-parse", "master-private")
	branchHead := gitOutputInDir(t, worktreePath, "rev-parse", "HEAD")
	if branchHead != masterPrivateHead {
		t.Errorf("expected LPD-99999 to start at master-private's HEAD (%s), got %s", masterPrivateHead, branchHead)
	}

	info, err := os.Lstat(filepath.Join(worktreePath, "CLAUDE.local.md"))
	if err != nil {
		t.Fatalf("expected CLAUDE.local.md to be propagated: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected CLAUDE.local.md to be a symlink, got mode %v", info.Mode())
	}
}

// TestRunWorktreeAdd_DetectsProjectTypeOnWorktree confirms that a monorepo
// worktree created from a primary that is not a Liferay project root still
// gets the monorepo's generated config.
func TestRunWorktreeAdd_DetectsProjectTypeOnWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("HOME", t.TempDir())

	origSkipBuild := worktreeSkipBuild
	worktreeSkipBuild = true
	t.Cleanup(func() { worktreeSkipBuild = origSkipBuild })

	fakeAnt(t)

	repoDir := newSparsePrimary(t)

	chdir(t, repoDir)

	worktreePath := filepath.Join(t.TempDir(), "new-worktree")
	if err := runWorktreeAdd(nil, []string{"master", worktreePath}); err != nil {
		t.Fatalf("runWorktreeAdd returned an error: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(worktreePath, "app.server.*.properties"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Errorf("expected one generated app.server.<user>.properties, got %v", matches)
	}
}

// TestRunWorktreeAdd_SkipBuildRunsSetupSDK confirms that --skip-build still
// runs "ant setup-sdk" in a monorepo worktree, so Gradle commands work
// without the full "ant all" build.
func TestRunWorktreeAdd_SkipBuildRunsSetupSDK(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("HOME", t.TempDir())

	origSkipBuild := worktreeSkipBuild
	worktreeSkipBuild = true
	t.Cleanup(func() { worktreeSkipBuild = origSkipBuild })

	antArgsFile := fakeAnt(t)

	repoDir := newSparsePrimary(t)

	chdir(t, repoDir)

	worktreePath := filepath.Join(t.TempDir(), "new-worktree")
	if err := runWorktreeAdd(nil, []string{"master", worktreePath}); err != nil {
		t.Fatalf("runWorktreeAdd returned an error: %v", err)
	}

	antArgs, err := os.ReadFile(antArgsFile)
	if err != nil {
		t.Fatalf("ant was not run: %v", err)
	}
	if got := strings.TrimSpace(string(antArgs)); got != "setup-sdk" {
		t.Errorf("ant ran with %q, want setup-sdk", got)
	}
}

// fakeAnt puts an ant script first on PATH that records its arguments, and
// returns the file it records them to.
func fakeAnt(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stubs ant with a shell script, which doesn't run on Windows")
	}

	binDir := t.TempDir()
	antArgsFile := filepath.Join(binDir, "ant-args")
	script := "#!/bin/sh\necho \"$@\" > " + antArgsFile + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "ant"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return antArgsFile
}

// newSparsePrimary creates a repository whose master branch is a monorepo
// (build.xml + modules/) and whose checked-out branch is an unrelated
// master-private branch with neither, mirroring liferay-portal-ee.
func newSparsePrimary(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "master")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test")
	if err := os.MkdirAll(filepath.Join(repoDir, "modules"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"build.xml", filepath.Join("modules", "build.gradle")} {
		if err := os.WriteFile(filepath.Join(repoDir, name), []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "monorepo")

	runGit(t, repoDir, "checkout", "--orphan", "master-private")
	runGit(t, repoDir, "rm", "-rf", "--quiet", ".")
	if err := os.WriteFile(filepath.Join(repoDir, "working.dir.properties"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "working.dir.properties")
	runGit(t, repoDir, "commit", "-m", "sparse")

	return repoDir
}

// chdir switches the process working directory to dir for the duration of
// the test, restoring it on cleanup. worktree add's git invocations run in
// the process's working directory.
func chdir(t *testing.T, dir string) {
	t.Helper()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origWD); err != nil {
			t.Fatal(err)
		}
	})
}

func gitOutputInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}
