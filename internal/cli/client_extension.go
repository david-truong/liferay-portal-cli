package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/fsutil"
	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

// defaultClientExtensionPort is the port Liferay's microservice client
// extensions listen on (see the jar-runner / LCP.json convention). Used when
// LCP.json carries no loadBalancer.targetPort.
const defaultClientExtensionPort = 58081

var clientExtensionCmd = &cobra.Command{
	Use:     "client-extension <name> [-- <docker run args>]",
	Aliases: []string{"ce"},
	Short:   "Build, deploy, and run a workspace client extension",
	Long: `Resolves a client extension by name under workspaces/<workspace>/client-extensions/,
builds it with "gw deploy", and copies the resulting zip into the bundle's
osgi/client-extensions/ directory.

If the client extension is a containerized (microservice) extension — i.e. it
has a Dockerfile — the command then builds its image and starts the container:

  docker build -t <name>:latest .          (from build/liferay-client-extension-build)
  docker run -d --rm --name <name> -p <port>:<port> \
    --add-host host.docker.internal:host-gateway <name>:latest

Pass any extension-specific "docker run" flags after a "--" separator; they are
forwarded verbatim (network, environment variables, etc.):

  liferay client-extension liferay-seostudio-crawler -- \
    --network crawler_elastic \
    -e LIFERAY_SEO_STUDIO_CRAWLER_ELASTICSEARCH_HOST=elasticsearch \
    -e COM_LIFERAY_LXC_DXP_MAINDOMAIN=host.docker.internal:8080

Frontend client extensions (no Dockerfile) stop after the zip is deployed.

Examples:
  liferay client-extension liferay-sample-custom-element-1
  liferay ce liferay-sample-etc-spring-boot
  liferay ce liferay-sample-workspace/liferay-sample-custom-element-1`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE:               runClientExtension,
}

func init() {
	rootCmd.AddCommand(clientExtensionCmd)
}

func runClientExtension(cmd *cobra.Command, args []string) error {
	gf, args, err := parseGlobalFlags(args)
	if err != nil {
		return err
	}
	if err := applyGlobalFlags(gf); err != nil {
		return err
	}

	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return cmd.Help()
	}

	// Split the client extension name from the pass-through docker run args.
	name := args[0]
	var dockerArgs []string
	for i, a := range args {
		if a == "--" {
			if i != 1 {
				return fmt.Errorf("expected a single client extension name before \"--\"")
			}
			dockerArgs = args[i+1:]
			args = args[:i]
			break
		}
	}
	if len(args) != 1 {
		return fmt.Errorf("expected a single client extension name, got %d arguments", len(args))
	}

	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	idx, err := portal.BuildClientExtensionIndex(portalRoot)
	if err != nil {
		return fmt.Errorf("building client extension index: %w", err)
	}
	cePath, err := idx.Resolve(name)
	if err != nil {
		return err
	}
	ceName := filepath.Base(cePath)

	bundleDir, err := portal.BundleDir(portalRoot)
	if err != nil {
		return fmt.Errorf("resolving bundle dir: %w", err)
	}

	if err := deployClientExtensionZip(portalRoot, cePath, ceName, bundleDir); err != nil {
		return err
	}

	if !fsutil.Exists(filepath.Join(cePath, "Dockerfile")) {
		fmt.Printf("Deployed %s.zip to %s (no Dockerfile — skipping container)\n",
			ceName, filepath.Join(bundleDir, "osgi", "client-extensions"))
		return nil
	}

	return runClientExtensionContainer(portalRoot, cePath, ceName, dockerArgs)
}

// deployClientExtensionZip runs "gw deploy" in the client extension directory
// and copies the produced zip into the bundle's osgi/client-extensions/.
func deployClientExtensionZip(portalRoot, cePath, ceName, bundleDir string) error {
	gwCmd, err := gradle.Command(cePath, "deploy")
	if err != nil {
		return err
	}
	if err := logrun.Run(gwCmd, logrun.Options{Label: "ce-deploy-" + ceName, Verbose: verbose, WorktreeRoot: portalRoot}); err != nil {
		return fmt.Errorf("deploying %s: %w", ceName, err)
	}

	// "gw deploy" drops the zip into the workspace's own bundle, which is
	// liferay.workspace.home.dir (default <workspace>/bundles). The workspace
	// root is two levels up from the client extension directory.
	workspaceRoot := filepath.Dir(filepath.Dir(cePath))
	builtZip := filepath.Join(workspaceRoot, "bundles", "osgi", "client-extensions", ceName+".zip")
	if !fsutil.Exists(builtZip) {
		return fmt.Errorf("expected built zip not found at %s — check the \"gw deploy\" output", builtZip)
	}

	destDir := filepath.Join(bundleDir, "osgi", "client-extensions")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", destDir, err)
	}
	destZip := filepath.Join(destDir, ceName+".zip")
	if err := fsutil.CopyFile(builtZip, destZip); err != nil {
		return err
	}
	fmt.Printf("Deployed %s\n", destZip)
	return nil
}

// runClientExtensionContainer builds the client extension's Docker image from
// its staged build directory and starts it detached.
func runClientExtensionContainer(portalRoot, cePath, ceName string, dockerArgs []string) error {
	if err := docker.CheckAvailable(); err != nil {
		return err
	}

	buildDir := filepath.Join(cePath, "build", "liferay-client-extension-build")
	if !fsutil.Exists(filepath.Join(buildDir, "Dockerfile")) {
		return fmt.Errorf("staged Docker build directory not found at %s — \"gw deploy\" should have produced it", buildDir)
	}

	image := ceName + ":latest"
	buildCmd := exec.Command("docker", "build", "-t", image, ".")
	buildCmd.Dir = buildDir
	if err := logrun.Run(buildCmd, logrun.Options{Label: "ce-docker-build-" + ceName, Verbose: verbose, WorktreeRoot: portalRoot}); err != nil {
		return fmt.Errorf("building image %s: %w", image, err)
	}

	// Replace any container left over from a previous run so the command is
	// safe to re-run after editing the extension.
	_ = exec.Command("docker", "rm", "-f", ceName).Run()

	port := clientExtensionPort(cePath)
	runArgs := []string{
		"run", "-d", "--rm",
		"--name", ceName,
		"-p", fmt.Sprintf("%d:%d", port, port),
		"--add-host", "host.docker.internal:host-gateway",
	}
	runArgs = append(runArgs, dockerArgs...)
	runArgs = append(runArgs, image)

	out, err := exec.Command("docker", runArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("starting container %s: %w\n%s", ceName, err, strings.TrimSpace(string(out)))
	}

	fmt.Printf("Started container %s (image %s, port %d)\n", ceName, image, port)
	fmt.Printf("  Follow logs:  docker logs -f %s\n", ceName)
	fmt.Printf("  Stop:         docker stop %s\n", ceName)
	return nil
}

// clientExtensionPort reads loadBalancer.targetPort from the extension's
// LCP.json, falling back to the jar-runner default.
func clientExtensionPort(cePath string) int {
	data, err := os.ReadFile(filepath.Join(cePath, "LCP.json"))
	if err != nil {
		return defaultClientExtensionPort
	}
	var lcp struct {
		LoadBalancer struct {
			TargetPort int `json:"targetPort"`
		} `json:"loadBalancer"`
	}
	if err := json.Unmarshal(data, &lcp); err != nil || lcp.LoadBalancer.TargetPort == 0 {
		return defaultClientExtensionPort
	}
	return lcp.LoadBalancer.TargetPort
}
