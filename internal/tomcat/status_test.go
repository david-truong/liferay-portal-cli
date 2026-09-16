package tomcat

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
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

// When the tracked pidfile points at a dead pid but a Tomcat for this exact
// bundle version is still alive under a different pid (started outside this
// CLI, or left over after CATALINA_PID diverged from the real JVM pid),
// Status finds it via orphanPID, adopts it into the pidfile, and reports
// running.
func TestStatus_AdoptsOrphanedProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no ps on windows")
	}

	tomcatDir := t.TempDir()

	// A real Tomcat's -Dcatalina.base flag is a distinct argv token, not
	// text a shell might exec away. "& wait" keeps this shell process (not
	// just its sleep child) alive and un-exec'd, so ps still reports this
	// exact command line, marker included; a trailing "#" comment alone
	// would be dropped once the shell exec's its single remaining command.
	script := fmt.Sprintf("sleep 300 & wait $! # -Dcatalina.base=%s", tomcatDir)
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	pgid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })

	pidFile := filepath.Join(t.TempDir(), "tomcat.pid")
	if err := os.WriteFile(pidFile, []byte("999999"), 0644); err != nil {
		t.Fatal(err)
	}

	paths := Paths{Bundle: tomcatDir, Tomcat: tomcatDir, PidFile: pidFile}
	pid, alive := Status(paths)
	if !alive {
		t.Error("Status did not adopt the orphaned process")
	}
	if pid != cmd.Process.Pid {
		t.Errorf("Status pid = %d, want %d", pid, cmd.Process.Pid)
	}

	adopted, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	if want := strconv.Itoa(cmd.Process.Pid); string(adopted) != want {
		t.Errorf("pidfile = %q, want %q (adopted orphan pid)", adopted, want)
	}
}

// When no tracked pid and no matching orphan exist, Status reports not
// running without adopting anything.
func TestStatus_NoOrphanFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no ps on windows")
	}

	pidFile := filepath.Join(t.TempDir(), "tomcat.pid")
	paths := Paths{Bundle: "/bundle", Tomcat: "/bundle/tomcat-1.2.3", PidFile: pidFile}
	pid, alive := Status(paths)
	if alive {
		t.Error("Status reported running with no tracked pid and no orphan")
	}
	if pid != 0 {
		t.Errorf("Status pid = %d, want 0", pid)
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
