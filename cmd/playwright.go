package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var playwrightTests string

var playwrightCmd = &cobra.Command{
	Use:   "playwright",
	Short: "Run Playwright end-to-end tests",
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
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	portalRoot, err := portal.FindRoot(cwd)
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
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Stdin = os.Stdin
	return execCmd.Run()
}
