package cli

import (
	"fmt"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:     "clean [module ...]",
	Aliases: []string{"c"},
	Short:   "Clean the portal or specific modules",
	Long: `With no arguments: runs "ant clean" from the portal root.
With module names: resolves each to its directory and runs "gw clean".

All invocations work from the portal root — no cd required.

Examples:
  liferay clean
  liferay clean change-tracking-web
  liferay clean change-tracking-web blogs-web`,
	RunE: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return runAnt(portalRoot, portalRoot, "clean", "clean-all")
	}

	idx, err := buildModuleIndex(portalRoot)
	if err != nil {
		return err
	}

	for _, name := range args {
		modulePath, err := idx.Resolve(name)
		if err != nil {
			return err
		}
		if err := runGwClean(portalRoot, modulePath); err != nil {
			return fmt.Errorf("cleaning %s: %w", name, err)
		}
	}
	return nil
}

func runGwClean(portalRoot, moduleDir string) error {
	cmd, err := gradle.Command(moduleDir, "clean")
	if err != nil {
		return err
	}
	return logrun.Run(cmd, logrun.Options{Label: "clean-" + filepath.Base(moduleDir), Verbose: verbose, WorktreeRoot: portalRoot})
}
