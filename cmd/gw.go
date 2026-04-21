package cmd

import (
	"fmt"
	"os"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var gwCmd = &cobra.Command{
	Use:                "gw <module> [gradle-args...]",
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

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	portalRoot, err := portal.FindRoot(cwd)
	if err != nil {
		return err
	}

	idx, err := portal.BuildModuleIndex(portalRoot)
	if err != nil {
		return fmt.Errorf("building module index: %w", err)
	}

	modulePath, err := idx.Resolve(moduleName)
	if err != nil {
		return err
	}

	gwCmd, err := gradle.Command(modulePath, gradleArgs...)
	if err != nil {
		return err
	}
	gwCmd.Stdout = os.Stdout
	gwCmd.Stderr = os.Stderr
	gwCmd.Stdin = os.Stdin
	return gwCmd.Run()
}
