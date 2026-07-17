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
	"github.com/david-truong/liferay-portal-cli/internal/state"
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
On a Liferay Workspace, no arguments deploys every discovered OSGi module
instead; client extensions are not included and must be deployed individually
via "liferay client-extension <name>".
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

	antProjects := make([]string, 0, len(antDeployProjects))
	for name := range antDeployProjects {
		antProjects = append(antProjects, name)
	}
	buildCmd.ValidArgsFunction = completeModuleArgs(antProjects...)

	rootCmd.AddCommand(buildCmd)
}

func runBuild(cmd *cobra.Command, args []string) error {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		if portal.DetectProjectType(portalRoot) == portal.Workspace {
			return runWorkspaceBuildAll(portalRoot)
		}
		return runAntAll(portalRoot)
	}

	warnIfRebased(portalRoot)

	idx, err := buildModuleIndex(portalRoot)
	if err != nil {
		return err
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

// runWorkspaceBuildAll mirrors "ant all" for a Liferay Workspace: assemble
// the bundle if it doesn't exist yet, then deploy every discovered module.
// This covers OSGi modules only — client extensions live under a separate
// directory and are deployed individually via "liferay client-extension".
func runWorkspaceBuildAll(portalRoot string) error {
	bundleDir, err := portal.BundleDir(portalRoot)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(bundleDir); statErr != nil || !info.IsDir() {
		if err := runGwInitBundle(portalRoot); err != nil {
			return err
		}
	}

	idx, err := buildModuleIndex(portalRoot)
	if err != nil {
		return err
	}
	for _, modulePath := range idx.AllPaths() {
		if err := runGwDeploy(portalRoot, modulePath); err != nil {
			return fmt.Errorf("deploying %s: %w", filepath.Base(modulePath), err)
		}
	}
	recordBuildBase(portalRoot)
	return nil
}

func runGwInitBundle(portalRoot string) error {
	cmd, err := gradle.Command(portalRoot, "initBundle")
	if err != nil {
		return err
	}
	return logrun.Run(cmd, logrun.Options{Label: "init-bundle", Verbose: verbose, WorktreeRoot: portalRoot})
}

func runAnt(portalRoot, dir, target, label string) error {
	path, err := lookupAnt()
	if err != nil {
		return err
	}
	cmd := exec.Command(path, target)
	cmd.Dir = dir
	return logrun.Run(cmd, logrun.Options{Label: label, Verbose: verbose, WorktreeRoot: portalRoot})
}

func runAntDeploy(portalRoot, projectDir string) error {
	return runAnt(portalRoot, projectDir, "deploy", "deploy-"+filepath.Base(projectDir))
}

func runAntAll(portalRoot string) error {
	if err := runAnt(portalRoot, portalRoot, "all", "build-all"); err != nil {
		return err
	}
	recordBuildBase(portalRoot)
	return nil
}

// recordBuildBase persists the current merge-base as the base portalRoot was
// last fully built against, so a later rebase can be detected by
// warnIfRebased. A no-op when the merge-base can't be determined.
func recordBuildBase(portalRoot string) {
	if sha := mergeBaseSHA(portalRoot); sha != "" {
		_ = state.SaveBuildBase(portalRoot, sha)
	}
}

// mergeBaseSHA returns the merge-base between HEAD and master, or "" if it
// can't be determined (no master ref, detached HEAD, ...).
func mergeBaseSHA(portalRoot string) string {
	out, err := gitOutput("-C", portalRoot, "merge-base", "master", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// warnIfRebased prints a warning when portalRoot's branch has moved onto a
// new base since its last "ant all", so changes from master picked up by a
// rebase aren't silently missing from modules deployed individually.
func warnIfRebased(portalRoot string) {
	rec, ok, err := state.LoadBuildBase(portalRoot)
	if err != nil || !ok {
		return
	}
	current := mergeBaseSHA(portalRoot)
	if current == "" || current == rec.SHA {
		return
	}
	fmt.Fprintln(os.Stderr,
		"warning: branch has moved onto a new base since the last \"ant all\" — run \"liferay build\" with no arguments to rebuild")
}

func lookupAnt() (string, error) {
	antName := "ant"
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("ant"); err != nil {
			antName = "ant.bat"
		}
	}
	path, err := exec.LookPath(antName)
	if err != nil {
		return "", fmt.Errorf("ant not found on PATH — install Apache Ant (https://ant.apache.org/)")
	}
	return path, nil
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
