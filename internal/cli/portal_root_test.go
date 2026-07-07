package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
)

// mustGitInit runs a real "git init" in dir so gitPrimaryRoot/isPrimaryWorktree
// have a real repository to inspect.
func mustGitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

// writeWorkspaceMarker stages the marker isWorkspaceRoot looks for.
func writeWorkspaceMarker(t *testing.T, dir string) {
	t.Helper()
	content := []byte(`apply plugin: "com.liferay.workspace"` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "settings.gradle"), content, 0644); err != nil {
		t.Fatal(err)
	}
}

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

func TestIsPrimarySlot_TrueForPrimaryMonorepoCheckout(t *testing.T) {
	root := t.TempDir()
	mustGitInit(t, root)
	if err := os.WriteFile(filepath.Join(root, "build.xml"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "modules"), 0755); err != nil {
		t.Fatal(err)
	}

	if !isPrimarySlot(root) {
		t.Error("expected isPrimarySlot to be true for a primary monorepo checkout")
	}
}

func TestIsPrimarySlot_FalseForWorkspace(t *testing.T) {
	root := t.TempDir()
	mustGitInit(t, root)
	writeWorkspaceMarker(t, root)

	if isPrimarySlot(root) {
		t.Error("expected isPrimarySlot to be false for a Workspace project, even though it is its own repo's primary checkout")
	}
}

func TestCheckStockPorts_WorkspaceBypassesRefusal(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceMarker(t, root)

	// Occupy one of slot 0's ports so a non-Workspace primary checkout would
	// be refused; a Workspace must bypass the check entirely regardless.
	ports := docker.PortsFromSlot(0)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", ports.Arquillian))
	if err != nil {
		t.Skipf("port %d unavailable for this test: %v", ports.Arquillian, err)
	}
	defer ln.Close()

	if err := checkStockPorts(root); err != nil {
		t.Errorf("checkStockPorts should bypass the refusal for a Workspace project, got: %v", err)
	}
}

// mustWritePrimaryMonorepoMarkers stages build.xml + modules/ (Monorepo) and
// a directory .git (not a linked worktree) so DetectProjectType/isLinkedWorktree
// see dir as a primary monorepo checkout.
func mustWritePrimaryMonorepoMarkers(t *testing.T, dir string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build.xml"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "modules"), 0755); err != nil {
		t.Fatal(err)
	}
}

// mustPersistSlotZeroState writes the docker state file a real "db start"
// would leave behind after claiming slot 0, so checkStockPorts can recognize
// this checkout as the stock slot's owner.
func mustPersistSlotZeroState(t *testing.T, worktreeRoot string) {
	t.Helper()
	stateDir := docker.StateDir(worktreeRoot)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"slot":0}`)
	if err := os.WriteFile(filepath.Join(stateDir, "ports.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// TestCheckStockPorts_SelfOwnedSlotZeroShortCircuits reproduces HIGH-2's
// inverse failure on Linux: there, the probe DOES see the CLI's own
// Tomcat/docker-proxy binds, so a repeat "db start"/"server restart" on the
// primary checkout would otherwise be refused for colliding with itself. Once
// this checkout's own state says it owns slot 0, checkStockPorts must not
// probe at all — its own stack being up is expected, not a collision.
func TestCheckStockPorts_SelfOwnedSlotZeroShortCircuits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	mustWritePrimaryMonorepoMarkers(t, root)
	mustPersistSlotZeroState(t, root)

	if err := checkStockPorts(root); err != nil {
		t.Errorf("checkStockPorts should short-circuit for a self-owned slot 0, got: %v", err)
	}
}

// TestCheckStockPorts_NoStateProbesNormally confirms the short-circuit only
// fires when this checkout's own state claims slot 0 — with no persisted
// state at all (never ran "db start" here) and nothing occupying stock ports,
// checkStockPorts still passes via the normal probe path. Stock ports are
// fixed host ports the dev machine may genuinely have busy for unrelated
// reasons, so skip rather than fail if that's the case here.
func TestCheckStockPorts_NoStateProbesNormally(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stockPorts := docker.PortsFromSlot(0)
	if docker.AnyPortInUse(docker.ProbePorts(stockPorts)...) {
		t.Skip("a stock port is already occupied on this machine for unrelated reasons")
	}

	root := t.TempDir()
	mustWritePrimaryMonorepoMarkers(t, root)

	if err := checkStockPorts(root); err != nil {
		t.Errorf("checkStockPorts should pass when stock ports are free, got: %v", err)
	}
}

func TestCheckStockPorts_RefusesWhenPortsOccupiedForNonWorktree(t *testing.T) {
	root := t.TempDir() // a primary monorepo checkout: not linked, not Workspace
	if err := os.WriteFile(filepath.Join(root, "build.xml"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "modules"), 0755); err != nil {
		t.Fatal(err)
	}

	ports := docker.PortsFromSlot(0)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", ports.Arquillian))
	if err != nil {
		t.Skipf("port %d unavailable for this test: %v", ports.Arquillian, err)
	}
	defer ln.Close()

	if err := checkStockPorts(root); err == nil {
		t.Error("expected checkStockPorts to refuse when a slot-0 port is occupied and this is neither a linked worktree nor a Workspace")
	}
}
