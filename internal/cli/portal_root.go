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
// already occupied.
func checkStockPorts(worktreeRoot string) error {
	if isLinkedWorktree(worktreeRoot) || portal.DetectProjectType(worktreeRoot) == portal.Workspace {
		return nil
	}
	ports := docker.PortsFromSlot(0)
	if docker.AnyPortInUse(docker.ProbePorts(ports)...) {
		return fmt.Errorf("stock ports are already in use — run from a worktree to use alternate ports")
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
