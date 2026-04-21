package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var testIntegrationTests string

var testIntegrationCmd = &cobra.Command{
	Use:   "test-integration <module>",
	Short: "Run integration tests for a Liferay module",
	Long: `Resolves the module by name and runs "gw testIntegration --tests <filter>" in its directory.

--tests accepts any pattern supported by Gradle's Test task.
A running Liferay server is required for most integration tests.

If a liferay server stack is running (liferay server up), the Tomcat port is
detected automatically from .liferay-cli/docker/ports.json and injected via a
Gradle init script written to .gradle/ (already gitignored).

All invocations work from the portal root — no cd required.

Examples:
  liferay test-integration change-tracking-web --tests "*FooTest"
  liferay test-integration change-tracking-web --tests "com.liferay.foo.FooTest"`,
	Args: cobra.ExactArgs(1),
	RunE: runTestIntegration,
}

func init() {
	testIntegrationCmd.Flags().StringVar(&testIntegrationTests, "tests", "", "Gradle test filter (class name, wildcard, or class.method)")
	testIntegrationCmd.MarkFlagRequired("tests")
	rootCmd.AddCommand(testIntegrationCmd)
}

func runTestIntegration(cmd *cobra.Command, args []string) error {
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

	gwArgs := []string{"testIntegration", "--tests", testIntegrationTests}

	if initScript, err := writePortInitScript(portalRoot); err == nil && initScript != "" {
		gwArgs = append([]string{"-I", initScript}, gwArgs...)
	}

	gwCmd, err := gradle.Command(modulePath, gwArgs...)
	if err != nil {
		return err
	}
	gwCmd.Stdout = os.Stdout
	gwCmd.Stderr = os.Stderr
	gwCmd.Stdin = os.Stdin
	return gwCmd.Run()
}

// writePortInitScript reads the Docker port slot from ports.json and writes a Gradle
// init script to <portalRoot>/.gradle/ that configures testIntegrationTomcat.portNumber.
// Returns the init script path, or ("", nil) if no ports.json exists (stack not running).
func writePortInitScript(portalRoot string) (string, error) {
	portsFile := filepath.Join(docker.StateDir(portalRoot), "ports.json")
	data, err := os.ReadFile(portsFile)
	if err != nil {
		return "", nil // stack not running; fall back to default port
	}

	var pj struct {
		Slot int `json:"slot"`
	}
	if err := json.Unmarshal(data, &pj); err != nil {
		return "", nil
	}

	ports := docker.PortsFromSlot(pj.Slot)

	gradleDir := filepath.Join(portalRoot, ".gradle")
	if err := os.MkdirAll(gradleDir, 0755); err != nil {
		return "", err
	}

	initPath := filepath.Join(gradleDir, "liferay-test-integration-init.gradle")
	content := fmt.Sprintf(`allprojects {
	afterEvaluate { project ->
		if (project.extensions.findByName('testIntegrationTomcat')) {
			project.testIntegrationTomcat.portNumber = %d
		}
	}
}
`, ports.Tomcat)

	if err := os.WriteFile(initPath, []byte(content), 0644); err != nil {
		return "", err
	}

	return initPath, nil
}
