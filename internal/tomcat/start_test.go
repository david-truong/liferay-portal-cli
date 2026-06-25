package tomcat

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// runForeground must write the child's pid while it runs (so "server status"
// and the dashboard see "running") and clear it once the child exits.
func TestRunForegroundTracksPidFile(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "tomcat.pid")

	cmd := exec.Command("sleep", "1")

	observed := make(chan int, 1)
	go func() {
		for i := 0; i < 100; i++ {
			data, err := os.ReadFile(pidFile)
			if err == nil {
				pid, _ := strconv.Atoi(string(data))
				observed <- pid
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		observed <- 0
	}()

	if err := runForeground(cmd, pidFile); err != nil {
		t.Fatalf("runForeground: %v", err)
	}

	pid := <-observed
	if pid != cmd.Process.Pid {
		t.Errorf("pid file held %d while running, want %d", pid, cmd.Process.Pid)
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Errorf("pid file still present after exit (err=%v), want removed", err)
	}
}
