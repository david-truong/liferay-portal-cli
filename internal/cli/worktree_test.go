package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
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

	// CLAUDE.local.md is git-ignored, so only this propagation carries it into
	// a worktree — it must be in the candidate list alongside CLAUDE.md.
	for _, name := range []string{"CLAUDE.md", "CLAUDE.local.md"} {
		mustWriteCLI(t, filepath.Join(primary, name), []byte("# "+name+"\n"))
	}

	results := ensureWorktreeFiles(primary, worktree, portal.Monorepo)

	for _, name := range []string{"CLAUDE.md", "CLAUDE.local.md"} {
		r, ok := resultByName(results)[name]
		if !ok {
			t.Fatalf("%s not in results: %+v", name, results)
		}
		if r.action != "linked" && r.action != "copied" {
			t.Errorf("%s action = %q, want linked|copied", name, r.action)
		}
		if _, err := os.Stat(filepath.Join(worktree, name)); err != nil {
			t.Errorf("%s not propagated to worktree: %v", name, err)
		}
	}
}

func TestEnsureWorktreeFiles_CopiesUserSpecificProperties(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	mustWriteCLI(t, filepath.Join(primary, "build.dtruong.properties"), []byte("user.local=true\n"))

	results := ensureWorktreeFiles(primary, worktree, portal.Monorepo)

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

	results := ensureWorktreeFiles(primary, worktree, portal.Monorepo)

	if _, ok := resultByName(results)["build.properties"]; ok {
		t.Error("build.properties should not appear in results — git tracks it")
	}
	if _, err := os.Stat(filepath.Join(worktree, "build.properties")); err == nil {
		t.Error("build.properties should not be propagated by ensureWorktreeFiles")
	}
}

func TestEnsureWorktreeFiles_GeneratesAppServerProperties(t *testing.T) {
	primary, worktree := stageWorktreePair(t)

	results := ensureWorktreeFiles(primary, worktree, portal.Monorepo)

	username, err := portal.SafeUsername()
	if err != nil {
		t.Fatal(err)
	}
	want := "app.server." + username + ".properties"
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
	if !strings.Contains(string(content), "app.server.parent.dir=${project.dir}/.bundles") {
		t.Errorf("expected app.server.parent.dir override, got:\n%s", content)
	}
}

func TestEnsureWorktreeFiles_GeneratesSetupWizard(t *testing.T) {
	primary, worktree := stageWorktreePair(t)

	results := ensureWorktreeFiles(primary, worktree, portal.Monorepo)

	want := filepath.Join(".bundles", "portal-setup-wizard.properties")
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

	results := ensureWorktreeFiles(primary, worktree, portal.Monorepo)

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
	ensureWorktreeFiles(primary, worktree, portal.Monorepo)

	// Second pass: every result should be "skipped" — except autogenerated
	// files which already exist after pass 1 so they're also "skipped".
	results := ensureWorktreeFiles(primary, worktree, portal.Monorepo)

	for _, r := range results {
		if r.action != "skipped" {
			t.Errorf("idempotent run produced non-skipped action for %s: %q", r.name, r.action)
		}
	}
}

func TestEnsureWorktreeFiles_PropagatesEnvFile(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	mustWriteCLI(t, filepath.Join(primary, ".env"), []byte("FOO=bar\n"))

	results := ensureWorktreeFiles(primary, worktree, portal.Monorepo)

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
	username, err := portal.SafeUsername()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "app.server."+username+".properties")); err == nil {
		t.Error("autofixWorktree should not generate files in a primary checkout")
	}
}

func TestEnsureWorktreeFiles_WorkspaceSkipsAppServerProperties(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	mustWriteCLI(t, filepath.Join(primary, "CLAUDE.md"), []byte("# claude\n"))

	results := ensureWorktreeFiles(primary, worktree, portal.Workspace)

	username, err := portal.SafeUsername()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resultByName(results)["app.server."+username+".properties"]; ok {
		t.Error("app.server.<user>.properties should not be generated for a Workspace")
	}
	if _, ok := resultByName(results)["CLAUDE.md"]; !ok {
		t.Error("CLAUDE.md should still be propagated for a Workspace")
	}
}

// TestEnsureWorktreeFiles_GeneratesSetupWizardForWorkspace proves the setup
// wizard file lands at a Workspace's actual bundle dir ("bundles", per
// portal.BundleDir) rather than the Monorepo-only ".bundles" — writing to the
// wrong path would make the file invisible to both Tomcat and "server wipe".
func TestEnsureWorktreeFiles_GeneratesSetupWizardForWorkspace(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	writeWorkspaceMarker(t, worktree)

	results := ensureWorktreeFiles(primary, worktree, portal.Workspace)

	want := filepath.Join("bundles", "portal-setup-wizard.properties")
	r, ok := resultByName(results)[want]
	if !ok {
		t.Fatalf("%s not in results: %+v", want, results)
	}
	if r.action != "generated" {
		t.Errorf("action = %q, want generated", r.action)
	}
	if _, err := os.Stat(filepath.Join(worktree, want)); err != nil {
		t.Errorf("setup wizard not written to workspace bundle dir: %v", err)
	}
}

// TestEnsureWorktreeFiles_RegeneratesSetupWizardForWorkspaceAfterWipe proves
// the autofix pass restores a Workspace's setup wizard file after "server
// wipe" deletes it — the bug this test guards against: autofix used to skip
// setup-wizard generation for every non-Monorepo project, so a Workspace
// worktree's file never came back on the next liferay invocation.
func TestEnsureWorktreeFiles_RegeneratesSetupWizardForWorkspaceAfterWipe(t *testing.T) {
	primary, worktree := stageWorktreePair(t)
	writeWorkspaceMarker(t, worktree)
	ensureWorktreeFiles(primary, worktree, portal.Workspace)

	wizardPath := filepath.Join(worktree, "bundles", "portal-setup-wizard.properties")
	if err := os.Remove(wizardPath); err != nil {
		t.Fatal(err)
	}

	results := ensureWorktreeFiles(primary, worktree, portal.Workspace)

	want := filepath.Join("bundles", "portal-setup-wizard.properties")
	r, ok := resultByName(results)[want]
	if !ok || r.action != "generated" {
		t.Errorf("expected setup wizard regenerated after wipe, got %+v", r)
	}
	if _, err := os.Stat(wizardPath); err != nil {
		t.Errorf("setup wizard missing after autofix pass: %v", err)
	}
}
