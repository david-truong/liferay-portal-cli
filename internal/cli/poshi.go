package cli

import (
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var poshiTests string

var poshiCmd = &cobra.Command{
	Use:   "poshi",
	Short: "Run Poshi functional tests",
	Long: `Runs Poshi tests from portal-web using the Poshi Runner Gradle plugin.

--tests accepts the Poshi test name in TestCaseFile#TestCaseName format.
All invocations work from the portal root — no cd required.

Examples:
  liferay poshi --tests Login#viewWelcomePage
  liferay poshi --tests Foo#testBar`,
	RunE: runPoshi,
}

func init() {
	poshiCmd.Flags().StringVar(&poshiTests, "tests", "", "Poshi test name (TestCaseFile#TestCaseName)")
	poshiCmd.MarkFlagRequired("tests")
	rootCmd.AddCommand(poshiCmd)
}

func runPoshi(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	portalRoot, err := portal.FindRoot(cwd)
	if err != nil {
		return err
	}

	portalWebDir := filepath.Join(portalRoot, "portal-web")

	gwCmd, err := gradle.Command(portalWebDir, "-b", "build-test.gradle", "runPoshi", "-Dtest.name="+poshiTests)
	if err != nil {
		return err
	}
	return logrun.Run(gwCmd, logrun.Options{Label: "poshi", Verbose: verbose, WorktreeRoot: portalRoot})
}
