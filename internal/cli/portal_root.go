package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
)

func findWorktreeRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return portal.FindRoot(cwd)
}

func buildModuleIndex(portalRoot string) (*portal.ModuleIndex, error) {
	idx, err := portal.BuildModuleIndex(portalRoot)
	if err != nil {
		return nil, fmt.Errorf("building module index: %w", err)
	}
	return idx, nil
}

func isLinkedWorktree(portalRoot string) bool {
	info, err := os.Stat(filepath.Join(portalRoot, ".git"))
	return err == nil && !info.IsDir()
}

// checkStockPorts returns an error if the caller is in the main repo (not a
// linked worktree, and not a Liferay Workspace) and stock ports (slot 0) are
// already occupied. A checkout that has already claimed slot 0 itself is
// exempt from the probe: its own DB/Tomcat being up is expected, not a
// collision with someone else's stock instance. Without this exemption, a
// repeat "db start"/"server restart" on the primary checkout would trip over
// the CLI's own stack — the port probe can see the CLI's own docker-proxy/
// Tomcat binds (this is most visible on Linux; see internal/docker/ports.go).
func checkStockPorts(worktreeRoot string) error {
	if isLinkedWorktree(worktreeRoot) || portal.DetectProjectType(worktreeRoot) == portal.Workspace {
		return nil
	}
	if s, ok := docker.LoadState(worktreeRoot); ok && s.Slot == 0 {
		return nil
	}
	ports := docker.PortsFromSlot(0)
	if docker.AnyPortInUse(docker.ProbePorts(ports)...) {
		return ExitErr(ExitPortCollision, "stock ports are already in use — run from a worktree to use alternate ports")
	}
	return nil
}

// isPrimarySlot reports whether worktreeRoot should reserve/use slot 0 (the
// stock-port "primary" checkout) rather than always claiming its own
// non-zero slot. A Liferay Workspace project's bundle lives inside the
// project itself, so it never shares a "primary" instance with anything
// else — it always claims its own slot, just like a linked git worktree
// does, even though it is its own repo's primary (only) checkout.
func isPrimarySlot(worktreeRoot string) bool {
	return isPrimaryWorktree(worktreeRoot) && portal.DetectProjectType(worktreeRoot) != portal.Workspace
}
