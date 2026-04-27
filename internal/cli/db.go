package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:     "database",
	Aliases: []string{"db"},
	Short:   "Manage the per-worktree database stack",
	Long: `Runs a per-worktree database container (+ Adminer) in Docker and
rewrites the bundle's portal-ext.properties with the matching JDBC stanza.

Supported engines:
  mysql       (default) — mysql:8.0
  mariadb                — mariadb:11
  postgres               — postgres:17
  hypersonic             — Liferay's built-in HSQL; no container, no JDBC override

The portal's Tomcat runs natively on the host (see "liferay server"). Each
worktree gets its own data volume and port set so multiple worktrees can run
in parallel.`,
}

var dbEngine string

var dbUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the database stack (or configure hypersonic)",
	RunE:  runDBUp,
}

var dbDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the database stack (preserves data)",
	RunE:  runDBDown,
}

var dbRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Stop then start the database stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runDBDown(cmd, args); err != nil {
			return err
		}
		return runDBUp(cmd, args)
	},
}

var dbLogsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Follow container logs (default: db)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		worktreeRoot, err := findWorktreeRoot()
		if err != nil {
			return err
		}
		if state, ok := docker.LoadState(worktreeRoot); !ok || !docker.IsDockerManagedEngine(state.Engine) {
			return fmt.Errorf("no Docker-managed database for this worktree (engine=%q)", state.Engine)
		}
		service := "db"
		if len(args) > 0 {
			service = args[0]
		}
		return docker.Run(worktreeRoot, "logs", "-f", service)
	},
}

var dbPsCmd = &cobra.Command{
	Use:   "ps",
	Short: "List containers in this worktree's db stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		worktreeRoot, err := findWorktreeRoot()
		if err != nil {
			return err
		}
		if state, ok := docker.LoadState(worktreeRoot); !ok || !docker.IsDockerManagedEngine(state.Engine) {
			return fmt.Errorf("no Docker-managed database for this worktree (engine=%q)", state.Engine)
		}
		return docker.Run(worktreeRoot, "ps")
	},
}

var dbWipeOnDown bool

func init() {
	dbUpCmd.Flags().StringVar(&dbEngine, "engine", "", "Database engine (mysql|mariadb|postgres|hypersonic); reuses the stored engine when omitted")
	dbDownCmd.Flags().BoolVar(&dbWipeOnDown, "wipe", false, "Also delete the database data volume")
	dbCmd.AddCommand(dbUpCmd, dbDownCmd, dbRestartCmd, dbLogsCmd, dbPsCmd)
	rootCmd.AddCommand(dbCmd)
}

func runDBUp(_ *cobra.Command, _ []string) error {
	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	bundleDir, err := portal.BundleDir(worktreeRoot)
	if err != nil {
		return fmt.Errorf("resolving bundle dir: %w", err)
	}

	state, ports, err := docker.Setup(worktreeRoot, bundleDir, dbEngine)
	if err != nil {
		return fmt.Errorf("setting up docker compose: %w", err)
	}

	if !docker.IsDockerManagedEngine(state.Engine) {
		fmt.Printf("Engine: %s (embedded — no container started)\n", state.Engine)
		fmt.Printf("Bundle portal-ext.properties cleared of CLI jdbc overrides; Liferay will use its built-in HSQL.\n")
		return nil
	}

	fmt.Printf("Starting %s stack (slot %d)\n", state.Engine, state.Slot)
	if err := docker.Run(worktreeRoot, "up", "-d"); err != nil {
		return err
	}

	fmt.Printf("\nListening at:\n")
	fmt.Printf("  %-8s localhost:%d\n", state.Engine, ports.MySQL)
	fmt.Printf("  Adminer  http://localhost:%d\n", ports.Adminer)
	fmt.Printf("\nBundle portal-ext.properties updated with jdbc URL.\n")
	fmt.Printf("Start the portal with: liferay server start\n")
	return nil
}

func runDBDown(_ *cobra.Command, _ []string) error {
	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	state, ok := docker.LoadState(worktreeRoot)
	if !ok || !docker.IsDockerManagedEngine(state.Engine) {
		fmt.Printf("No Docker-managed database for this worktree; nothing to stop.\n")
		return nil
	}

	args := []string{"down"}
	if dbWipeOnDown {
		args = append(args, "-v")
		defer wipeDBVolume(worktreeRoot, state.Engine)
	}
	return docker.Run(worktreeRoot, args...)
}

func wipeDBVolume(worktreeRoot, engine string) {
	sub := dbDataSubdir(engine)
	dbDir := filepath.Join(docker.StateDir(worktreeRoot), "db", sub)
	if err := os.RemoveAll(dbDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", dbDir, err)
	} else {
		fmt.Printf("Removed %s data at %s\n", engine, dbDir)
	}
}

func dbDataSubdir(engine string) string {
	switch engine {
	case docker.EngineMariaDB:
		return "mariadb"
	case docker.EnginePostgres:
		return "postgres"
	}
	return "mysql"
}
