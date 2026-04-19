package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "liferay",
	Short:        "CLI for Liferay portal development workflows",
	Long:         `liferay wraps common liferay-portal / liferay-portal-ee tasks: building modules, managing git worktrees, and running containerized servers.`,
	SilenceUsage: true,
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
