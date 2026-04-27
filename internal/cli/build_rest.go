package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var buildRESTCmd = &cobra.Command{
	Use:     "build-rest <module>",
	Aliases: []string{"br"},
	Short:   "Run REST Builder for a Liferay module",
	Long: `Resolves the module by name and runs "gw buildREST" in its directory.
The module must contain a rest-config.yaml file.

All invocations work from the portal root — no cd required.

Examples:
  liferay build-rest headless-delivery-impl
  liferay build-rest headless-delivery/headless-delivery-impl`,
	Args: cobra.ExactArgs(1),
	RunE: runBuildREST,
}

func init() {
	rootCmd.AddCommand(buildRESTCmd)
}

func runBuildREST(cmd *cobra.Command, args []string) error {
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

	if _, err := os.Stat(filepath.Join(modulePath, "rest-config.yaml")); os.IsNotExist(err) {
		return fmt.Errorf("module %q has no rest-config.yaml — is this a REST impl module?", args[0])
	}

	gwCmd, err := gradle.Command(modulePath, "buildREST")
	if err != nil {
		return err
	}
	return logrun.Run(gwCmd, logrun.Options{Label: "build-rest-" + filepath.Base(modulePath), Verbose: verbose, WorktreeRoot: portalRoot})
}
