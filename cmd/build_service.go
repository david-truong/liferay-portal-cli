package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var buildServiceCmd = &cobra.Command{
	Use:   "build-service <module>",
	Short: "Run Service Builder for a Liferay module",
	Long: `Resolves the module by name and runs "gw buildService" in its directory.
The module must contain a service.xml file.

All invocations work from the portal root — no cd required.

Examples:
  liferay build-service change-tracking-service
  liferay build-service change-tracking/change-tracking-service`,
	Args: cobra.ExactArgs(1),
	RunE: runBuildService,
}

func init() {
	rootCmd.AddCommand(buildServiceCmd)
}

func runBuildService(cmd *cobra.Command, args []string) error {
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

	modulePath, err := idx.Resolve(args[0])
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(modulePath, "service.xml")); os.IsNotExist(err) {
		return fmt.Errorf("module %q has no service.xml — is this a service module?", args[0])
	}

	gwCmd, err := gradle.Command(modulePath, "buildService")
	if err != nil {
		return err
	}
	gwCmd.Stdout = os.Stdout
	gwCmd.Stderr = os.Stderr
	gwCmd.Stdin = os.Stdin
	return gwCmd.Run()
}
