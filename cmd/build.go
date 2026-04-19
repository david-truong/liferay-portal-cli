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

var noFormat bool

var buildCmd = &cobra.Command{
	Use:   "build [module ...]",
	Short: "Build and deploy Liferay modules",
	Long: `With no arguments: runs "ant all" from the portal root (full rebuild).
With module names: resolves each to its directory and runs "gw deploy -a".

Examples:
  liferay build
  liferay build change-tracking-web
  liferay build change-tracking-web blogs-web`,
	RunE: runBuild,
}

func init() {
	buildCmd.Flags().BoolVar(&noFormat, "no-format", false, "Skip formatSource (omit -a from gw deploy)")
	rootCmd.AddCommand(buildCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	portalRoot, err := portal.FindRoot(cwd)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return runAntAll(portalRoot)
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
		if err := runGwDeploy(modulePath); err != nil {
			return fmt.Errorf("deploying %s: %w", name, err)
		}
	}
	return nil
}

func runAntAll(portalRoot string) error {
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

	cmd := exec.Command(path, "all")
	cmd.Dir = portalRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runGwDeploy(moduleDir string) error {
	gwArgs := []string{"deploy"}
	if !noFormat {
		gwArgs = append(gwArgs, "-a")
	}

	cmd, err := gradle.Command(moduleDir, gwArgs...)
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
