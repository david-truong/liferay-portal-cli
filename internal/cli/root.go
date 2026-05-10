package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose    bool
	workingDir string
)

var rootCmd = &cobra.Command{
	Use:   "liferay",
	Short: "CLI for Liferay portal workflows, built for agents and humans alike",
	Long: `liferay drives every common Liferay workflow from a single working directory
(the portal root) with no cd, no interactive prompts, and no arcane flags.

Designed first for AI agents, but useful to any developer who'd rather not
juggle gw, blade, catalina.sh, and docker compose by hand.`,
	SilenceUsage:      true,
	PersistentPreRunE: rootPreSetup,
}

// rootPreSetup performs the workspace bootstrapping every liferay invocation
// expects: honor -C/--directory, then auto-fix any missing per-worktree files.
func rootPreSetup(_ *cobra.Command, _ []string) error {
	if workingDir != "" {
		if err := os.Chdir(workingDir); err != nil {
			return fmt.Errorf("change directory to %s: %w", workingDir, err)
		}
	}
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return nil
	}
	autofixWorktree(portalRoot)
	return nil
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"Stream full build/test output to the terminal (default: log to temp file, show tail on failure)")
	rootCmd.PersistentFlags().StringVarP(&workingDir, "directory", "C", "",
		"Run as if liferay was started in <path> instead of the current working directory")
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
