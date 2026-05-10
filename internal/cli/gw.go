package cli

import (
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/spf13/cobra"
)

var gwCmd = &cobra.Command{
	Use:                "gradle-wrapper <module> [gradle-args...]",
	Aliases:            []string{"gw"},
	Short:              "Run a Gradle task in a Liferay module",
	Long: `Resolves the module by name and runs gradlew with the given arguments.

Examples:
  liferay gw change-tracking-web deploy
  liferay gw change-tracking-web clean deploy
  liferay gw change-tracking/change-tracking-web deploy --info`,
	DisableFlagParsing: true,
	RunE:               runGw,
}

func init() {
	rootCmd.AddCommand(gwCmd)
}

func runGw(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return cmd.Help()
	}

	moduleName := args[0]
	gradleArgs := args[1:]

	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	idx, err := buildModuleIndex(portalRoot)
	if err != nil {
		return err
	}

	modulePath, err := idx.Resolve(moduleName)
	if err != nil {
		return err
	}

	gwCmd, err := gradle.Command(modulePath, gradleArgs...)
	if err != nil {
		return err
	}
	return logrun.Run(gwCmd, logrun.Options{Label: "gw-" + filepath.Base(modulePath), Verbose: verbose, WorktreeRoot: portalRoot})
}
