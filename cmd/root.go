package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "liferay",
	Short: "Agent-oriented CLI for Liferay portal workflows",
	Long: `liferay is built for AI agents, not human developers.

Every command works from a single working directory (the portal root) with no cd,
no interactive prompts, and no arcane flags. Human developers should keep using
gw, blade, and their IDE.`,
	SilenceUsage: true,
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
