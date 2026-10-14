package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/state"
)

func TestDashboardProjects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	parent := t.TempDir()

	portalRoot := filepath.Join(parent, "liferay-portal")
	writeTestFile(t, filepath.Join(portalRoot, "build.xml"), "")
	if err := os.MkdirAll(filepath.Join(portalRoot, "modules"), 0755); err != nil {
		t.Fatal(err)
	}
	usedWorkspace := writeTestWorkspace(t, portalRoot, "liferay-used-workspace")
	writeTestWorkspace(t, portalRoot, "liferay-unused-workspace")

	sparseRoot := filepath.Join(parent, "liferay-portal-ee-LPD-1")
	launchWorkspace := writeTestWorkspace(t, sparseRoot, "liferay-launch-workspace")

	if err := os.MkdirAll(state.Dir(usedWorkspace), 0755); err != nil {
		t.Fatal(err)
	}

	worktrees := dashboardProjects([]worktreeEntry{
		{Path: portalRoot, Branch: "master", Primary: true},
		{Path: sparseRoot, Branch: "LPD-1"},
	}, launchWorkspace)

	want := []struct {
		path      string
		branch    string
		primary   bool
		workspace bool
	}{
		{portalRoot, "master", true, false},
		{usedWorkspace, "master", false, true},
		{launchWorkspace, "LPD-1", false, true},
	}
	if len(worktrees) != len(want) {
		t.Fatalf("dashboardProjects returned %d tabs, want %d: %+v", len(worktrees), len(want), worktrees)
	}
	for i, w := range want {
		got := worktrees[i]
		if got.Path != w.path || got.Branch != w.branch || got.Primary != w.primary || got.Workspace != w.workspace {
			t.Errorf("tab %d = %+v, want %+v", i, got, w)
		}
	}
}
