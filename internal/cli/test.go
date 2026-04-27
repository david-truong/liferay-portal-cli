package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var testTests string

var testCmd = &cobra.Command{
	Use:     "test <module>",
	Aliases: []string{"t"},
	Short:   "Run unit tests for a Liferay module",
	Long: `Resolves the module by name and runs "gw test --tests <filter>" in its directory.

--tests accepts any pattern supported by Gradle's Test task:
a fully qualified class name, a wildcard like *FooTest, or ClassName.methodName.

All invocations work from the portal root — no cd required.

Examples:
  liferay test change-tracking-web --tests "*FooTest"
  liferay test change-tracking-web --tests "com.liferay.foo.FooTest.testBar"`,
	Args: cobra.ExactArgs(1),
	RunE: runTest,
}

func init() {
	testCmd.Flags().StringVar(&testTests, "tests", "", "Gradle test filter (class name, wildcard, or class.method)")
	testCmd.MarkFlagRequired("tests")
	rootCmd.AddCommand(testCmd)
}

func runTest(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	portalRoot, err := portal.FindRoot(cwd)
	if err != nil {
		return err
	}

	idx, err := portal.BuildModuleIndex(portalRoot)
	if err != nil {
		return fmt.Errorf("building module index: %w", err)
	}

	modulePath, err := idx.Resolve(args[0])
	if err != nil {
		return err
	}

	gwCmd, err := gradle.Command(modulePath, "test", "--tests", testTests)
	if err != nil {
		return err
	}
	return logrun.Run(gwCmd, logrun.Options{Label: "test-" + filepath.Base(modulePath), Verbose: verbose, WorktreeRoot: portalRoot})
}
