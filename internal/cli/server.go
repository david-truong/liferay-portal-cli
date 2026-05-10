package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/david-truong/liferay-portal-cli/internal/state"
	"github.com/david-truong/liferay-portal-cli/internal/tomcat"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:     "server",
	Aliases: []string{"s"},
	Short:   "Manage the host-native Tomcat server",
	Long: `Starts, stops, and inspects the Liferay Tomcat bundle on the host.

Runs catalina.sh directly under the bundle's tomcat-*/bin directory, with
CATALINA_PID set to ~/.liferay-cli/worktrees/<id>/tomcat.pid so start/stop/
status stay consistent across invocations and survive "ant all".

MySQL runs in Docker — see "liferay db". "server start" and "server run"
will bring up the db stack automatically if it isn't already running.`,
}

var serverStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"up"},
	Short:   "Start Tomcat in the background",
	RunE:    runServerStart,
}

var serverStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"down"},
	Short:   "Stop the running Tomcat",
	RunE:    runServerStop,
}

var serverRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Stop then start Tomcat",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, alive := currentStatus(); alive {
			if err := runServerStop(cmd, args); err != nil {
				return err
			}
		}
		return runServerStart(cmd, args)
	},
}

var serverRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Tomcat in the foreground (streams catalina output)",
	RunE:  runServerRun,
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether Tomcat is running",
	RunE:  runServerStatus,
}

var serverLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Follow tomcat-*/logs/catalina.out",
	RunE:  runServerLogs,
}

var serverWipeCmd = &cobra.Command{
	Use:   "wipe",
	Short: "Stop Tomcat and delete data/logs/osgi-state/work and portal-setup-wizard.properties",
	RunE:  runServerWipe,
}

var (
	serverDebug   bool
	serverWipeYes bool
)

func init() {
	serverStartCmd.Flags().BoolVar(&serverDebug, "debug", false, "Start Tomcat with JDWP enabled (catalina.sh jpda start). JPDA_ADDRESS comes from setenv.sh — default 8000, per-slot for slot > 0.")
	serverRunCmd.Flags().BoolVar(&serverDebug, "debug", false, "Run Tomcat in foreground with JDWP enabled (catalina.sh jpda run).")
	serverRestartCmd.Flags().BoolVar(&serverDebug, "debug", false, "Restart with JDWP enabled.")
	serverWipeCmd.Flags().BoolVar(&serverWipeYes, "yes", false, "Skip the confirmation prompt. Required when stdin is not a TTY (or set LIFERAY_CLI_ASSUME_YES=1).")
	serverCmd.AddCommand(
		serverStartCmd, serverStopCmd, serverRestartCmd, serverRunCmd,
		serverStatusCmd, serverLogsCmd, serverWipeCmd,
	)
	rootCmd.AddCommand(serverCmd)
}

func runServerStart(_ *cobra.Command, _ []string) error {
	return startTomcat(false)
}

func runServerRun(_ *cobra.Command, _ []string) error {
	return startTomcat(true)
}

func startTomcat(foreground bool) error {
	paths, err := resolvePaths()
	if err != nil {
		return err
	}
	if pid, alive := tomcat.Status(paths); alive {
		if foreground {
			return fmt.Errorf("tomcat already running (pid %d) — stop it first", pid)
		}
		fmt.Printf("Tomcat already running (pid %d)\n", pid)
		return nil
	}
	ports, err := ensureDB(paths.Bundle)
	if err != nil {
		return err
	}
	if err := tomcat.PatchBundle(paths, ports); err != nil {
		return fmt.Errorf("patching bundle for slot %d: %w", ports.Slot, err)
	}
	if foreground {
		// Save before exec — once tomcat exits the user may want "liferay logs"
		// to surface what happened.
		saveServerLastCmd(paths.CatOut)
		printServerBanner(paths, ports, serverDebug)
		return tomcat.Start(paths, tomcat.StartOptions{Foreground: true, Debug: serverDebug})
	}
	if err := tomcat.Start(paths, tomcat.StartOptions{Debug: serverDebug}); err != nil {
		return err
	}
	saveServerLastCmd(paths.CatOut)
	printServerBanner(paths, ports, serverDebug)
	return nil
}

func saveServerLastCmd(catOut string) {
	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return
	}
	_ = state.SaveLastCmd(worktreeRoot, state.LastCmd{
		Kind:    state.LastCmdServer,
		LogPath: catOut,
	})
}

func printServerBanner(paths tomcat.Paths, ports docker.Ports, debug bool) {
	fmt.Printf("\nTomcat starting (slot %d)\n", ports.Slot)
	fmt.Printf("  HTTP     http://localhost:%d\n", ports.TomcatHTTP)
	fmt.Printf("  Gogo     localhost:%d\n", ports.OSGiConsole)
	if debug {
		fmt.Printf("  JPDA     localhost:%d (--debug)\n", ports.JPDA)
	}
	fmt.Printf("  logs:    %s\n", paths.CatOut)
	fmt.Printf("Tip: liferay server logs\n")
}

func runServerStop(_ *cobra.Command, _ []string) error {
	paths, err := resolvePaths()
	if err != nil {
		return err
	}
	return tomcat.Stop(paths)
}

func runServerStatus(_ *cobra.Command, _ []string) error {
	paths, err := resolvePaths()
	if err != nil {
		return err
	}
	pid, alive := tomcat.Status(paths)
	if alive {
		fmt.Printf("running (pid %d)\n", pid)
		return nil
	}
	if pid > 0 {
		fmt.Printf("stale pid file (pid %d no longer alive)\n", pid)
	} else {
		fmt.Printf("not running\n")
	}
	return nil
}

func runServerLogs(_ *cobra.Command, _ []string) error {
	paths, err := resolvePaths()
	if err != nil {
		return err
	}
	saveServerLastCmd(paths.CatOut)
	return tailServer(paths.CatOut, nil, 0)
}

func runServerWipe(_ *cobra.Command, _ []string) error {
	return wipeServer(serverWipeYes, os.Stdin, os.Stdout, isStdinTTY())
}

// wipeServer is the testable core. The confirmation gate runs first so a
// declined wipe never touches the filesystem.
func wipeServer(assumeYes bool, in io.Reader, out io.Writer, isTTY bool) error {
	if !confirmWithIO(
		"This will delete data/, logs/, osgi/state/, work/, and portal-setup-wizard.properties from the active bundle.",
		assumeYes, in, out, isTTY,
	) {
		return ExitErr(ExitConfirmationDeclined,
			"server wipe declined — pass --yes or set %s=1 to skip the prompt", AssumeYesEnvVar)
	}

	paths, err := resolvePaths()
	if err != nil {
		return err
	}
	if _, alive := tomcat.Status(paths); alive {
		if err := tomcat.Stop(paths); err != nil {
			fmt.Fprintf(os.Stderr, "warning: stop failed: %v\n", err)
		}
	}
	removed := tomcat.Wipe(paths)
	for _, p := range removed {
		fmt.Printf("Removed %s\n", p)
	}
	if len(removed) == 0 {
		fmt.Println("Nothing to remove.")
	}
	return nil
}

func ensureDB(bundleDir string) (docker.Ports, error) {
	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return docker.Ports{}, err
	}
	if err := checkStockPorts(worktreeRoot); err != nil {
		return docker.Ports{}, err
	}
	state, ports, err := docker.Setup(worktreeRoot, bundleDir, "")
	if err != nil {
		return docker.Ports{}, fmt.Errorf("setting up docker compose: %w", err)
	}
	if !docker.IsDockerManagedEngine(state.Engine) {
		fmt.Printf("Engine: %s (embedded — skipping Docker)\n", state.Engine)
		return ports, nil
	}
	fmt.Printf("Ensuring %s (slot %d, localhost:%d)...\n", state.Engine, state.Slot, ports.MySQL)
	if err := docker.Run(worktreeRoot, "up", "-d", "--wait"); err != nil {
		return ports, err
	}
	return ports, nil
}

func resolvePaths() (tomcat.Paths, error) {
	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return tomcat.Paths{}, err
	}
	bundleDir, err := portal.BundleDir(worktreeRoot)
	if err != nil {
		return tomcat.Paths{}, fmt.Errorf("resolving bundle dir: %w", err)
	}
	paths, err := tomcat.Resolve(worktreeRoot, bundleDir)
	if err != nil {
		return tomcat.Paths{}, fmt.Errorf(
			"%w\n\nRun \"ant all\" or \"liferay build\" to populate the bundle first", err)
	}
	return paths, nil
}

func currentStatus() (int, bool) {
	paths, err := resolvePaths()
	if err != nil {
		return 0, false
	}
	return tomcat.Status(paths)
}

