package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/spf13/cobra"
)

var playwrightTests string

var playwrightCmd = &cobra.Command{
	Use:     "playwright",
	Aliases: []string{"pw"},
	Short:   "Run Playwright end-to-end tests",
	Long: `Runs Playwright tests from modules/test/playwright using npx.

--tests is passed as --grep to the Playwright test runner.
All invocations work from the portal root — no cd required.

Examples:
  liferay playwright --tests myTestName
  liferay playwright --tests "my test description"`,
	RunE: runPlaywright,
}

func init() {
	playwrightCmd.Flags().StringVar(&playwrightTests, "tests", "", "Playwright test name filter (passed as --grep)")
	playwrightCmd.MarkFlagRequired("tests")
	rootCmd.AddCommand(playwrightCmd)
}

func runPlaywright(cmd *cobra.Command, args []string) error {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	npxPath, err := exec.LookPath("npx")
	if err != nil {
		return fmt.Errorf("npx not found on PATH — install Node.js (https://nodejs.org/)")
	}

	playwrightDir := filepath.Join(portalRoot, "modules", "test", "playwright")

	execCmd := exec.Command(npxPath, "playwright", "test", "--grep", playwrightTests)
	execCmd.Dir = playwrightDir
	if portalURL := playwrightPortalURL(portalRoot); portalURL != "" {
		execCmd.Env = append(os.Environ(), "PORTAL_URL="+portalURL)
	}
	return logrun.Run(execCmd, logrun.Options{Label: "playwright", Verbose: verbose, WorktreeRoot: portalRoot})
}

// playwrightPortalURL returns the PORTAL_URL to hand Playwright so it targets
// this worktree's Tomcat instead of the slot 0 default. It returns "" when the
// caller already set PORTAL_URL or the worktree has no stored slot yet.
func playwrightPortalURL(worktreeRoot string) string {
	if _, ok := os.LookupEnv("PORTAL_URL"); ok {
		return ""
	}

	st, ok := docker.LoadState(worktreeRoot)
	if !ok {
		return ""
	}

	return fmt.Sprintf("http://localhost:%d", docker.PortsFromSlot(st.Slot).TomcatHTTP)
}
