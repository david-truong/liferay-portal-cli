package cli

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// stageWorktreePair stages a primary + worktree pair of temp dirs and
// returns their paths.
func stageWorktreePair(t *testing.T) (primary, worktree string) {
	t.Helper()
	return t.TempDir(), t.TempDir()
}

// resultByName builds a name → fixAction lookup for the test assertions.
func resultByName(results []fixAction) map[string]fixAction {
	m := make(map[string]fixAction, len(results))
	for _, r := range results {
		m[r.name] = r
	}
	return m
}

func TestEnsureWorktreeFiles_LinksSymlinkCandidates(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	mustWriteCLI(t, filepath.Join(primary, "CLAUDE.md"), []byte("# claude\n"))

	results := ensureWorktreeFiles(primary, worktree)

	r, ok := resultByName(results)["CLAUDE.md"]
	if !ok {
		t.Fatalf("CLAUDE.md not in results: %+v", results)
	}
	if r.action != "linked" && r.action != "copied" {
		t.Errorf("CLAUDE.md action = %q, want linked|copied", r.action)
	}
	dst := filepath.Join(worktree, "CLAUDE.md")
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("CLAUDE.md not propagated to worktree: %v", err)
	}
}

func TestEnsureWorktreeFiles_CopiesUserSpecificProperties(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	mustWriteCLI(t, filepath.Join(primary, "build.dtruong.properties"), []byte("user.local=true\n"))

	results := ensureWorktreeFiles(primary, worktree)

	r, ok := resultByName(results)["build.dtruong.properties"]
	if !ok {
		t.Fatalf("build.dtruong.properties not in results: %+v", results)
	}
	if r.action != "copied" {
		t.Errorf("action = %q, want copied", r.action)
	}
	if _, err := os.Stat(filepath.Join(worktree, "build.dtruong.properties")); err != nil {
		t.Errorf("file not propagated: %v", err)
	}
}

func TestEnsureWorktreeFiles_DoesNotCopyTrackedProperties(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	// build.properties is the canonical (tracked) version — should NOT be
	// propagated (git already handles it).
	mustWriteCLI(t, filepath.Join(primary, "build.properties"), []byte("tracked=true\n"))

	results := ensureWorktreeFiles(primary, worktree)

	if _, ok := resultByName(results)["build.properties"]; ok {
		t.Error("build.properties should not appear in results — git tracks it")
	}
	if _, err := os.Stat(filepath.Join(worktree, "build.properties")); err == nil {
		t.Error("build.properties should not be propagated by ensureWorktreeFiles")
	}
}

func TestEnsureWorktreeFiles_GeneratesAppServerProperties(t *testing.T) {
	primary, worktree := stageWorktreePair(t)

	results := ensureWorktreeFiles(primary, worktree)

	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	want := "app.server." + u.Username + ".properties"
	r, ok := resultByName(results)[want]
	if !ok {
		t.Fatalf("%s not in results: %+v", want, results)
	}
	if r.action != "generated" {
		t.Errorf("%s action = %q, want generated", want, r.action)
	}
	content, err := os.ReadFile(filepath.Join(worktree, want))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "app.server.parent.dir=${project.dir}/bundles") {
		t.Errorf("expected app.server.parent.dir override, got:\n%s", content)
	}
}

func TestEnsureWorktreeFiles_GeneratesSetupWizard(t *testing.T) {
	primary, worktree := stageWorktreePair(t)

	results := ensureWorktreeFiles(primary, worktree)

	want := filepath.Join("bundles", "portal-setup-wizard.properties")
	r, ok := resultByName(results)[want]
	if !ok {
		t.Fatalf("%s not in results: %+v", want, results)
	}
	if r.action != "generated" {
		t.Errorf("action = %q, want generated", r.action)
	}
	content, err := os.ReadFile(filepath.Join(worktree, want))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"setup.wizard.enabled=false",
		"liferay.home=",
		"company.default.web.id=liferay.com",
	} {
		if !strings.Contains(string(content), want) {
			t.Errorf("expected %q in setup-wizard properties, got:\n%s", want, content)
		}
	}
}

func TestEnsureWorktreeFiles_SkipsExistingFiles(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	mustWriteCLI(t, filepath.Join(primary, "CLAUDE.md"), []byte("# primary claude\n"))
	// Pre-place a different CLAUDE.md in the worktree.
	mustWriteCLI(t, filepath.Join(worktree, "CLAUDE.md"), []byte("# worktree-local claude\n"))

	results := ensureWorktreeFiles(primary, worktree)

	r := resultByName(results)["CLAUDE.md"]
	if r.action != "skipped" {
		t.Errorf("expected skipped for existing file, got %q (note=%q)", r.action, r.note)
	}
	// Worktree-local content must survive untouched.
	got, err := os.ReadFile(filepath.Join(worktree, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "worktree-local") {
		t.Errorf("worktree-local CLAUDE.md was overwritten")
	}
}

func TestEnsureWorktreeFiles_Idempotent(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	mustWriteCLI(t, filepath.Join(primary, "CLAUDE.md"), []byte("# claude\n"))

	// First pass: linked/copied/generated.
	ensureWorktreeFiles(primary, worktree)

	// Second pass: every result should be "skipped" — except autogenerated
	// files which already exist after pass 1 so they're also "skipped".
	results := ensureWorktreeFiles(primary, worktree)

	for _, r := range results {
		if r.action != "skipped" {
			t.Errorf("idempotent run produced non-skipped action for %s: %q", r.name, r.action)
		}
	}
}

func TestEnsureWorktreeFiles_PropagatesEnvFile(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	mustWriteCLI(t, filepath.Join(primary, ".env"), []byte("FOO=bar\n"))

	results := ensureWorktreeFiles(primary, worktree)

	r, ok := resultByName(results)[".env"]
	if !ok {
		t.Fatalf(".env not in results: %+v", results)
	}
	if r.action != "copied" {
		t.Errorf(".env action = %q, want copied", r.action)
	}
}

func TestAutofixWorktree_NoOpForPrimary(t *testing.T) {
	// When the portalRoot is the primary checkout (i.e. .git is a
	// directory, not a "gitdir:" file), autofixWorktree returns early.
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Should not panic and not write anything despite primary having
	// no propagatable content.
	autofixWorktree(root)
	// Just assert no app.server.<user>.properties was generated, which
	// would be the visible side-effect if the function ran ensureWorktreeFiles.
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "app.server."+u.Username+".properties")); err == nil {
		t.Error("autofixWorktree should not generate files in a primary checkout")
	}
}
