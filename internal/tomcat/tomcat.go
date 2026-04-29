package tomcat

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
)

// Paths groups the filesystem locations liferay-cli cares about for a given bundle.
type Paths struct {
	Bundle    string // bundleDir
	Tomcat    string // bundleDir/tomcat-*
	Bin       string // tomcat/bin
	CatalinaS string // tomcat/bin/catalina.sh (or catalina.bat on Windows)
	PidFile   string // bundleDir/.liferay-cli/tomcat.pid
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

	pidDir := filepath.Join(bundleDir, ".liferay-cli")
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
	return cmd.Run()
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
// is live and belongs to a java process.
func Status(paths Paths) (int, bool) {
	data, err := os.ReadFile(paths.PidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, processAlive(pid)
}

// Wipe removes the bundle subdirectories that hold derived state (data,
// logs, osgi/state, work) plus portal-setup-wizard.properties, so the next
// boot starts clean against a fresh DB. Matches upstream StartTestableTomcatTask.
func Wipe(paths Paths) []string {
	targets := []string{
		filepath.Join(paths.Bundle, "data"),
		filepath.Join(paths.Bundle, "logs"),
		filepath.Join(paths.Bundle, "osgi", "state"),
		filepath.Join(paths.Tomcat, "work"),
		filepath.Join(paths.Bundle, "portal-setup-wizard.properties"),
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

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil
}
