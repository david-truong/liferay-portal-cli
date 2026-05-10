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
// linked worktree) and stock ports (slot 0) are already occupied.
func checkStockPorts(worktreeRoot string) error {
	if isLinkedWorktree(worktreeRoot) {
		return nil
	}
	ports := docker.PortsFromSlot(0)
	if docker.AnyPortInUse(docker.ProbePorts(ports)...) {
		return fmt.Errorf("stock ports are already in use — run from a worktree to use alternate ports")
	}
	return nil
}
