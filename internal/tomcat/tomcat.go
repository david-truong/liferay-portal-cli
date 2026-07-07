package tomcat

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/david-truong/liferay-portal-cli/internal/state"
)

// Paths groups the filesystem locations liferay-cli cares about for a given bundle.
type Paths struct {
	Bundle    string // bundleDir
	Tomcat    string // bundleDir/tomcat-*
	Bin       string // tomcat/bin
	CatalinaS string // tomcat/bin/catalina.sh (or catalina.bat on Windows)
	PidFile   string // <state-dir>/tomcat.pid (under ~/.liferay-cli/)
	CatOut    string // tomcat/logs/catalina.out
}

// Resolve locates the Tomcat install under bundleDir and returns a Paths struct.
// portalRoot is used to read the tomcat version from app.server.properties.
func Resolve(portalRoot, bundleDir string) (Paths, error) {
	tomcatDir, err := portal.FindTomcatDir(portalRoot)
	if err != nil {
		return Paths{}, err
	}
	bin := filepath.Join(tomcatDir, "bin")

	script := "catalina.sh"
	if runtime.GOOS == "windows" {
		script = "catalina.bat"
	}

	pidDir := state.Dir(portalRoot)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return Paths{}, fmt.Errorf("creating %s: %w", pidDir, err)
	}

	return Paths{
		Bundle:    bundleDir,
		Tomcat:    tomcatDir,
		Bin:       bin,
		CatalinaS: filepath.Join(bin, script),
		PidFile:   filepath.Join(pidDir, "tomcat.pid"),
		CatOut:    filepath.Join(tomcatDir, "logs", "catalina.out"),
	}, nil
}

// StartOptions controls how catalina.sh is invoked.
type StartOptions struct {
	// Foreground runs "catalina.sh run" (streams output) instead of "start".
	Foreground bool
	// Debug enables the JDWP agent by prefixing the catalina command with
	// "jpda" — the bundle's setenv.sh controls JPDA_ADDRESS (8000 by default;
	// per-slot when the bundle patcher has run).
	Debug bool
}

// Start runs catalina.sh {start|run}, optionally prefixed with "jpda" when
// opts.Debug is true.
func Start(paths Paths, opts StartOptions) error {
	action := "start"
	if opts.Foreground {
		action = "run"
	}

	if !opts.Foreground {
		if pid, alive := Status(paths); alive {
			return fmt.Errorf("tomcat already running (pid %d, %s)", pid, paths.PidFile)
		}
	}

	args := []string{action}
	if opts.Debug {
		args = []string{"jpda", action}
	}

	cmd := exec.Command(paths.CatalinaS, args...)
	cmd.Dir = paths.Bin
	cmd.Env = append(os.Environ(), "CATALINA_PID="+paths.PidFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if !opts.Foreground {
		return cmd.Run()
	}
	return runForeground(cmd, paths.PidFile)
}

// runForeground runs catalina.sh "run" and keeps the PID file in sync with the
// foreground process. Unlike "start", catalina.sh's "run" action execs the JVM
// in place without ever writing $CATALINA_PID, so "liferay server status" and
// the dashboard would report the server as down while it is running. The script
// execs the JVM, so the child we launch is the JVM itself; record its pid and
// clear the file once it exits. Ctrl+C reaches the foreground JVM through the
// terminal — capture it here so this process outlives the JVM long enough to
// remove the PID file instead of being torn down first.
func runForeground(cmd *exec.Cmd, pidFile string) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	if err := os.WriteFile(
		pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {

		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", pidFile, err)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	err := cmd.Wait()
	_ = os.Remove(pidFile)
	return err
}

// Stop runs catalina.sh stop, using the same CATALINA_PID file written by Start.
// -force is passed so catalina.sh kills the PID after its own timeout.
func Stop(paths Paths) error {
	if _, alive := Status(paths); !alive {
		return errors.New("tomcat is not running")
	}
	cmd := exec.Command(paths.CatalinaS, "stop", "-force")
	cmd.Dir = paths.Bin
	cmd.Env = append(os.Environ(), "CATALINA_PID="+paths.PidFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Status reads the PID file and returns (pid, running). Running means the pid
// is live and, when ps is available to check, its command line references
// paths.Bundle — the same PID-reuse guard ForceStop applies, since a live pid
// alone doesn't prove it's still this Tomcat and not an unrelated process
// that inherited the recycled PID. When ps is unavailable (e.g. Windows),
// liveness alone is trusted.
func Status(paths Paths) (int, bool) {
	data, err := os.ReadFile(paths.PidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	if !processAlive(pid) {
		return pid, false
	}
	if cmdline, ok := processCommandLine(pid); ok && !strings.Contains(cmdline, paths.Bundle) {
		return pid, false
	}
	return pid, true
}

// Wipe removes the bundle subdirectories that hold derived state (data,
// logs, osgi/state, work) plus portal-setup-wizard.properties, so the next
// boot starts clean against a fresh DB. Matches upstream StartTestableTomcatTask.
//
// When keepSetupWizard is true the wizard file is preserved — slot 0 is the
// untouched stock checkout whose wizard is the user's to manage, not the CLI's.
func Wipe(paths Paths, keepSetupWizard bool) []string {
	targets := []string{
		filepath.Join(paths.Bundle, "data"),
		filepath.Join(paths.Bundle, "logs"),
		filepath.Join(paths.Bundle, "osgi", "state"),
		filepath.Join(paths.Tomcat, "work"),
	}
	if !keepSetupWizard {
		targets = append(
			targets, filepath.Join(paths.Bundle, "portal-setup-wizard.properties"))
	}

	removed := make([]string, 0, len(targets))
	for _, t := range targets {
		if _, err := os.Stat(t); os.IsNotExist(err) {
			continue
		}
		if err := os.RemoveAll(t); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", t, err)
			continue
		}
		removed = append(removed, t)
	}
	return removed
}

// ForceStop kills the Tomcat recorded in pidFile when catalina.sh is no
// longer available (the bundle was deleted with its worktree). It reads the
// PID, confirms the process is alive AND that its command line references
// expectedCatalinaBase — this guards against killing an unrelated process
// that inherited a recycled PID — then sends SIGTERM, escalating to SIGKILL
// if the process does not exit. Returns (false, nil) when nothing is running.
// Returns (false, err) when the PID is alive but does not look like this
// slot's Tomcat, so the caller can warn and leave it untouched.
func ForceStop(pidFile, expectedCatalinaBase string) (bool, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false, nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 || !processAlive(pid) {
		return false, nil
	}

	if cmdline, ok := processCommandLine(pid); ok &&
		!strings.Contains(cmdline, expectedCatalinaBase) {

		return false, fmt.Errorf(
			"pid %d is alive but does not reference %s (possible PID reuse); leaving it running",
			pid, expectedCatalinaBase)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, nil
	}
	_ = proc.Signal(syscall.SIGTERM)

	for i := 0; i < 100; i++ {
		if !processAlive(pid) {
			return true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		return false, fmt.Errorf("SIGKILL pid %d: %w", pid, err)
	}
	return true, nil
}

// processCommandLine returns the full command line of pid via ps. ok is false
// when ps is unavailable (e.g. Windows) so callers fall back to killing
// without the PID-reuse guard rather than refusing to act.
func processCommandLine(pid int) (string, bool) {
	if runtime.GOOS == "windows" {
		return "", false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil
}
