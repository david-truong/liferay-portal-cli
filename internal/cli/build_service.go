package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/spf13/cobra"
)

var buildServiceCmd = &cobra.Command{
	Use:     "build-service <module>",
	Aliases: []string{"bs"},
	Short:   "Run Service Builder for a Liferay module",
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
	return runBuilder(args[0], builderSpec{
		requiredFile: "service.xml",
		moduleKind:   "service module",
		gradleTask:   "buildService",
		labelPrefix:  "build-service-",
	})
}

type builderSpec struct {
	requiredFile string
	moduleKind   string
	gradleTask   string
	labelPrefix  string
}

func runBuilder(moduleName string, spec builderSpec) error {
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

	if _, err := os.Stat(filepath.Join(modulePath, spec.requiredFile)); os.IsNotExist(err) {
		return fmt.Errorf("module %q has no %s — is this a %s?", moduleName, spec.requiredFile, spec.moduleKind)
	}

	gwCmd, err := gradle.Command(modulePath, spec.gradleTask)
	if err != nil {
		return err
	}
	return logrun.Run(gwCmd, logrun.Options{Label: spec.labelPrefix + filepath.Base(modulePath), Verbose: verbose, WorktreeRoot: portalRoot})
}
