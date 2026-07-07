package tomcat

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// When the live PID's command line does not reference the bundle dir,
// Status refuses to trust it (PID-reuse guard) and reports not-running,
// same as a stale pid file.
func TestStatus_PidReuseGuard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no ps on windows")
	}
	cmd, pidFile := spawnSleep(t)

	paths := Paths{Bundle: "/bundle/that/does/not/appear/in/cmdline", PidFile: pidFile}
	pid, alive := Status(paths)
	if alive {
		t.Error("Status reported running for a pid whose command line doesn't reference the bundle")
	}
	if pid != cmd.Process.Pid {
		t.Errorf("Status pid = %d, want %d (stale-pid-file reporting)", pid, cmd.Process.Pid)
	}
}

// When the live PID's command line does reference the bundle dir, Status
// reports running.
func TestStatus_MatchingCommandLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no ps on windows")
	}
	// Spawn a process whose argv contains the bundle path without a shell
	// in between: sh may exec its single command in place, dropping a
	// trailing "# <path>" comment from the ps output. tail's argument
	// survives verbatim.
	bundleDir := t.TempDir()
	watched := filepath.Join(bundleDir, "watched")
	if err := os.WriteFile(watched, nil, 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("tail", "-f", watched)
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	pidFile := filepath.Join(t.TempDir(), "tomcat.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		t.Fatal(err)
	}

	paths := Paths{Bundle: bundleDir, PidFile: pidFile}
	pid, alive := Status(paths)
	if !alive {
		t.Error("Status reported not-running for a pid whose command line references the bundle")
	}
	if pid != cmd.Process.Pid {
		t.Errorf("Status pid = %d, want %d", pid, cmd.Process.Pid)
	}
}
