package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean [module ...]",
	Short: "Clean the portal or specific modules",
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
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	portalRoot, err := portal.FindRoot(cwd)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return runAntClean(portalRoot)
	}

	idx, err := portal.BuildModuleIndex(portalRoot)
	if err != nil {
		return fmt.Errorf("building module index: %w", err)
	}

	for _, name := range args {
		modulePath, err := idx.Resolve(name)
		if err != nil {
			return err
		}
		if err := runGwClean(modulePath); err != nil {
			return fmt.Errorf("cleaning %s: %w", name, err)
		}
	}
	return nil
}

func runAntClean(portalRoot string) error {
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

	cmd := exec.Command(path, "clean")
	cmd.Dir = portalRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runGwClean(moduleDir string) error {
	cmd, err := gradle.Command(moduleDir, "clean")
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
