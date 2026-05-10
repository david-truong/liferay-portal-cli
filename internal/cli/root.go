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
	Short: "Agent-oriented CLI for Liferay portal workflows",
	Long: `liferay is built for AI agents, not human developers.

Every command works from a single working directory (the portal root) with no cd,
no interactive prompts, and no arcane flags. Human developers should keep using
gw, blade, and their IDE.`,
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
