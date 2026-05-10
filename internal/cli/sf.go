package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/spf13/cobra"
)

var sfCmd = &cobra.Command{
	Use:     "source-format [module ...]",
	Aliases: []string{"sf"},
	Short:   "Run source formatter for Liferay modules",
	Long: `With no arguments: runs "ant format-source-current-branch" from portal-impl.
With module names: resolves each to its directory and runs "gw formatSource".

Examples:
  liferay sf
  liferay sf change-tracking-web
  liferay sf change-tracking-web blogs-web`,
	RunE: runSf,
}

func init() {
	rootCmd.AddCommand(sfCmd)
}

func runSf(cmd *cobra.Command, args []string) error {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return runAnt(portalRoot, filepath.Join(portalRoot, "portal-impl"), "format-source-current-branch", "format-source")
	}

	idx, err := buildModuleIndex(portalRoot)
	if err != nil {
		return err
	}

	for _, name := range args {
		if strings.Contains(name, ".") {
			return fmt.Errorf("%q looks like a file path — liferay sf only accepts module names (e.g. \"change-tracking-web\")", name)
		}
		modulePath, err := idx.Resolve(name)
		if err != nil {
			return err
		}
		if err := runGwFormatSource(portalRoot, modulePath); err != nil {
			return fmt.Errorf("formatting %s: %w", name, err)
		}
	}
	return nil
}

func runGwFormatSource(portalRoot, moduleDir string) error {
	cmd, err := gradle.Command(moduleDir, "formatSource")
	if err != nil {
		return err
	}
	return logrun.Run(cmd, logrun.Options{Label: "format-source-" + filepath.Base(moduleDir), Verbose: verbose, WorktreeRoot: portalRoot})
}
