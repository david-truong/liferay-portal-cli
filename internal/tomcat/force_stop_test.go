package tomcat

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

func TestForceStopNoPidFile(t *testing.T) {
	stopped, err := ForceStop(filepath.Join(t.TempDir(), "tomcat.pid"), "/any")
	if err != nil || stopped {
		t.Errorf("ForceStop(missing) = (%v, %v), want (false, nil)", stopped, err)
	}
}

func TestForceStopDeadPid(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "tomcat.pid")
	// A PID that is essentially certain not to be running.
	if err := os.WriteFile(pidFile, []byte("999999"), 0644); err != nil {
		t.Fatal(err)
	}
	stopped, err := ForceStop(pidFile, "/any")
	if err != nil || stopped {
		t.Errorf("ForceStop(dead) = (%v, %v), want (false, nil)", stopped, err)
	}
}

// spawnSleep starts a long-lived process and writes its pid to a pidfile.
func spawnSleep(t *testing.T) (*exec.Cmd, string) {
	t.Helper()
	cmd := exec.Command("sleep", "300")
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn sleep: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	pidFile := filepath.Join(t.TempDir(), "tomcat.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		t.Fatal(err)
	}
	return cmd, pidFile
}

// When the live PID's command line does not match the expected catalina.base,
// ForceStop refuses to kill (PID-reuse guard) and reports an error.
func TestForceStopPidReuseGuard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no ps on windows")
	}
	cmd, pidFile := spawnSleep(t)

	stopped, err := ForceStop(pidFile, "/expected/catalina/base/that/does/not/match")
	if stopped {
		t.Error("ForceStop killed a process whose command line did not match")
	}
	if err == nil {
		t.Error("expected an error reporting the mismatch")
	}
	if !processAlive(cmd.Process.Pid) {
		t.Error("process should still be alive after a guarded ForceStop")
	}
}

// When the command line matches, ForceStop terminates the process.
func TestForceStopKillsMatchingProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no ps on windows")
	}
	cmd, pidFile := spawnSleep(t)

	// Reap concurrently so the killed child does not linger as a zombie
	// (which signal-0 liveness checks would still see as alive). A real
	// stranded Tomcat is not the CLI's child, so init reaps it instead.
	go func() { _ = cmd.Wait() }()

	// "sleep" appears in the process command line, satisfying the guard.
	stopped, err := ForceStop(pidFile, "sleep")
	if err != nil {
		t.Fatalf("ForceStop: %v", err)
	}
	if !stopped {
		t.Fatal("ForceStop reported nothing stopped")
	}
}
