package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var verbose bool

var rootCmd = &cobra.Command{
	Use:   "liferay",
	Short: "Agent-oriented CLI for Liferay portal workflows",
	Long: `liferay is built for AI agents, not human developers.

Every command works from a single working directory (the portal root) with no cd,
no interactive prompts, and no arcane flags. Human developers should keep using
gw, blade, and their IDE.`,
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"Stream full build/test output to the terminal (default: log to temp file, show tail on failure)")
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
