package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
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
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	portalRoot, err := portal.FindRoot(cwd)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return runAntFormatSource(portalRoot)
	}

	idx, err := portal.BuildModuleIndex(portalRoot)
	if err != nil {
		return fmt.Errorf("building module index: %w", err)
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

func runAntFormatSource(portalRoot string) error {
	antName := "ant"
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("ant"); err != nil {
			antName = "ant.bat"
		}
	}
	path, err := exec.LookPath(antName)
	if err != nil {
		return fmt.Errorf("ant not found on PATH — install Apache Ant (https://ant.apache.org/)")
	}

	cmd := exec.Command(path, "format-source-current-branch")
	cmd.Dir = filepath.Join(portalRoot, "portal-impl")
	return logrun.Run(cmd, logrun.Options{Label: "format-source", Verbose: verbose, WorktreeRoot: portalRoot})
}

func runGwFormatSource(portalRoot, moduleDir string) error {
	cmd, err := gradle.Command(moduleDir, "formatSource")
	if err != nil {
		return err
	}
	return logrun.Run(cmd, logrun.Options{Label: "format-source-" + filepath.Base(moduleDir), Verbose: verbose, WorktreeRoot: portalRoot})
}
