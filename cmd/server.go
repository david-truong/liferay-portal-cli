package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage the Liferay Tomcat + MySQL Docker stack",
	Long: `Runs a per-worktree Tomcat + MySQL stack in Docker.

Each worktree gets its own:
  - Bundle directory (via app.server.<user>.properties set by "liferay worktree add")
  - MySQL data volume
  - Port set (derived from the worktree path, spaced by 10)

Default ports (for the primary checkout):
  Tomcat  http://localhost:8080
  Debug   localhost:8000
  Gogo    localhost:13331
  MySQL   localhost:3306
  Adminer http://localhost:8081

Note: run "ant all" or "liferay build" first to populate the bundle.`,
}

var serverUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start Tomcat + MySQL containers",
	RunE:  runServerUp,
}

var serverDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop containers (preserves MySQL data)",
	RunE:  runServerDown,
}

var serverRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Stop then start containers",
	RunE:  runServerRestart,
}

var serverLogsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Follow container logs (default: tomcat)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runServerLogs,
}

var serverPsCmd = &cobra.Command{
	Use:   "ps",
	Short: "List containers in this worktree's stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		worktreeRoot, err := findWorktreeRoot()
		if err != nil {
			return err
		}
		return docker.Run(worktreeRoot, "ps")
	},
}

var wipeOnDown bool

func init() {
	serverDownCmd.Flags().BoolVar(&wipeOnDown, "wipe", false, "Also delete the MySQL data volume")
	serverCmd.AddCommand(serverUpCmd, serverDownCmd, serverRestartCmd, serverLogsCmd, serverPsCmd)
	rootCmd.AddCommand(serverCmd)
}

func runServerUp(_ *cobra.Command, _ []string) error {
	worktreeRoot, portalRoot, err := findRoots()
	if err != nil {
		return err
	}

	bundleDir, err := portal.BundleDir(portalRoot)
	if err != nil {
		return fmt.Errorf("resolving bundle dir: %w", err)
	}

	tomcatDir, err := portal.FindTomcatDir(bundleDir)
	if err != nil {
		return fmt.Errorf(
			"%w\n\nThe bundle hasn't been populated yet.\n"+
				"Run: ant all   (from %s)\n"+
				"Or:  liferay build", err, portalRoot)
	}

	tomcatDirName := filepath.Base(tomcatDir)
	ports, err := docker.Setup(worktreeRoot, bundleDir, tomcatDirName)
	if err != nil {
		return fmt.Errorf("setting up docker compose: %w", err)
	}

	fmt.Printf("Starting Liferay stack (slot %d, port base +%d)\n", ports.Slot, ports.Slot*10)
	if err := docker.Run(worktreeRoot, "up", "-d"); err != nil {
		return err
	}

	fmt.Printf("\nListening at:\n")
	fmt.Printf("  Liferay  http://localhost:%d\n", ports.Tomcat)
	fmt.Printf("  Adminer  http://localhost:%d\n", ports.Adminer)
	fmt.Printf("  JPDA     localhost:%d\n", ports.Debug)
	fmt.Printf("  Gogo     localhost:%d\n", ports.Gogo)
	fmt.Printf("  MySQL    localhost:%d\n", ports.MySQL)
	fmt.Printf("\nTip: liferay server logs tomcat\n")
	return nil
}

func runServerDown(_ *cobra.Command, _ []string) error {
	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	args := []string{"down"}
	if wipeOnDown {
		args = append(args, "-v")
		defer wipeDBVolume(worktreeRoot)
	}
	return docker.Run(worktreeRoot, args...)
}

func runServerRestart(cmd *cobra.Command, args []string) error {
	if err := runServerDown(cmd, args); err != nil {
		return err
	}
	return runServerUp(cmd, args)
}

func runServerLogs(_ *cobra.Command, args []string) error {
	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	service := "tomcat"
	if len(args) > 0 {
		service = args[0]
	}
	return docker.Run(worktreeRoot, "logs", "-f", service)
}

func wipeDBVolume(worktreeRoot string) {
	dbDir := filepath.Join(docker.StateDir(worktreeRoot), "db", "mysql")
	if err := os.RemoveAll(dbDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", dbDir, err)
	} else {
		fmt.Printf("Removed MySQL data at %s\n", dbDir)
	}
}

// findWorktreeRoot finds the worktree root (portal root) from cwd.
func findWorktreeRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return portal.FindRoot(cwd)
}

// findRoots returns (worktreeRoot, portalRoot, error).
// For simple (non-worktree) checkouts these are the same.
// For worktrees, worktreeRoot is the portal root of the current worktree,
// and portalRoot is the same (since each worktree is its own portal root).
func findRoots() (worktreeRoot string, portalRoot string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	root, err := portal.FindRoot(cwd)
	if err != nil {
		return "", "", err
	}
	return root, root, nil
}

// printPortalRoot extracts the leading comment on the portal root path for display.
func printPortalRoot(root string) string {
	parts := strings.Split(filepath.ToSlash(root), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return root
}

var _ = printPortalRoot // suppress unused warning; used in future subcommands
