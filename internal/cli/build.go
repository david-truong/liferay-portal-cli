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

var noFormat bool

// antDeployProjects are root-level Ant projects that have no bnd.bnd and are
// deployed via "ant deploy" from their own directory (see root build.xml's
// "deploy" target).
var antDeployProjects = map[string]bool{
	"portal-impl":   true,
	"portal-kernel": true,
	"util-bridges":  true,
	"util-java":     true,
	"util-slf4j":    true,
	"util-taglib":   true,
}

var buildCmd = &cobra.Command{
	Use:     "build [module ...]",
	Aliases: []string{"b"},
	Short:   "Build and deploy Liferay modules",
	Long: `With no arguments: runs "ant all" from the portal root (full rebuild).
With module names: resolves each to its directory and runs "gw deploy -a".

The root-level Ant projects (portal-impl, portal-kernel, util-bridges,
util-java, util-slf4j, util-taglib) are deployed via "ant deploy" instead.

Examples:
  liferay build
  liferay build change-tracking-web
  liferay build change-tracking-web blogs-web
  liferay build portal-impl`,
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
		if antDeployProjects[name] {
			if err := runAntDeploy(portalRoot, filepath.Join(portalRoot, name)); err != nil {
				return fmt.Errorf("deploying %s: %w", name, err)
			}
			continue
		}
		modulePath, err := idx.Resolve(name)
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, "-test") {
			if err := runGwCompileTest(portalRoot, modulePath); err != nil {
				return fmt.Errorf("compiling test %s: %w", name, err)
			}
		} else if err := runGwDeploy(portalRoot, modulePath); err != nil {
			return fmt.Errorf("deploying %s: %w", name, err)
		}
	}
	return nil
}

func runAntDeploy(portalRoot, projectDir string) error {
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

	cmd := exec.Command(path, "deploy")
	cmd.Dir = projectDir
	return logrun.Run(cmd, logrun.Options{Label: "deploy-" + filepath.Base(projectDir), Verbose: verbose, WorktreeRoot: portalRoot})
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
	return logrun.Run(cmd, logrun.Options{Label: "build-all", Verbose: verbose, WorktreeRoot: portalRoot})
}

func runGwCompileTest(portalRoot, moduleDir string) error {
	cmd, err := gradle.Command(moduleDir, "compileTestIntegrationJava")
	if err != nil {
		return err
	}
	return logrun.Run(cmd, logrun.Options{Label: "compile-test-" + filepath.Base(moduleDir), Verbose: verbose, WorktreeRoot: portalRoot})
}

func runGwDeploy(portalRoot, moduleDir string) error {
	gwArgs := []string{"deploy"}
	if !noFormat {
		gwArgs = append(gwArgs, "-a")
	}

	cmd, err := gradle.Command(moduleDir, gwArgs...)
	if err != nil {
		return err
	}
	return logrun.Run(cmd, logrun.Options{Label: "deploy-" + filepath.Base(moduleDir), Verbose: verbose, WorktreeRoot: portalRoot})
}
