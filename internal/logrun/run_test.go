package logrun

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/state"
)

// setFakeHome points os.UserHomeDir at dir so state.Dir resolves into the
// test sandbox instead of the real ~/.liferay-cli.
func setFakeHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestRun_FailingCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execs sh")
	}
	home := t.TempDir()
	setFakeHome(t, home)
	worktreeRoot := t.TempDir()

	cmd := exec.Command("sh", "-c", "echo out; echo err 1>&2; exit 3")
	err := Run(cmd, Options{Label: "x", WorktreeRoot: worktreeRoot})
	if err == nil {
		t.Fatal("expected an error from a failing command")
	}

	logDir := filepath.Join(state.Dir(worktreeRoot), "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("reading log dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one log file, got %d", len(entries))
	}
	logPath := filepath.Join(logDir, entries[0].Name())

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "out") || !strings.Contains(content, "err") {
		t.Errorf("expected log to contain both stdout and stderr, got:\n%s", content)
	}

	rec, ok, err := state.LoadLastCmd(worktreeRoot)
	if err != nil {
		t.Fatalf("LoadLastCmd: %v", err)
	}
	if !ok {
		t.Fatal("expected a last-command record to be saved")
	}
	if rec.Kind != state.LastCmdArchive {
		t.Errorf("expected Kind %q, got %q", state.LastCmdArchive, rec.Kind)
	}
	if rec.LogPath != logPath {
		t.Errorf("expected LogPath %q, got %q", logPath, rec.LogPath)
	}
}

func TestRun_SucceedingCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execs sh")
	}
	home := t.TempDir()
	setFakeHome(t, home)
	worktreeRoot := t.TempDir()

	cmd := exec.Command("sh", "-c", "echo hello")
	if err := Run(cmd, Options{Label: "x", WorktreeRoot: worktreeRoot}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rec, ok, err := state.LoadLastCmd(worktreeRoot)
	if err != nil {
		t.Fatalf("LoadLastCmd: %v", err)
	}
	if !ok {
		t.Fatal("expected a last-command record to be saved")
	}

	data, err := os.ReadFile(rec.LogPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("expected log to contain command output, got:\n%s", string(data))
	}
}

func TestNewLogPath_SanitizesLabel(t *testing.T) {
	home := t.TempDir()
	setFakeHome(t, home)
	worktreeRoot := t.TempDir()

	path, err := newLogPath("my/weird label", worktreeRoot)
	if err != nil {
		t.Fatalf("newLogPath: %v", err)
	}
	name := filepath.Base(path)
	if strings.ContainsAny(name, "/ ") {
		t.Errorf("expected label runes '/' and ' ' to be replaced, got name %q", name)
	}
	if !strings.HasPrefix(name, "my-weird-label-") {
		t.Errorf("expected sanitized label prefix, got name %q", name)
	}
}
