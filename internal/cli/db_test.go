package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/spf13/cobra"
)

// hypersonicWorktree fakes a portal root whose persisted docker state says the
// engine is hypersonic (i.e. embedded, no container). Returns the portal root.
// HOME and cwd are restored via t.Cleanup.
func hypersonicWorktree(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	portalRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(portalRoot, "build.xml"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(portalRoot, "modules"), 0755); err != nil {
		t.Fatal(err)
	}

	stateDir := docker.StateDir(portalRoot)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"engine": "hypersonic", "slot": 0})
	if err := os.WriteFile(filepath.Join(stateDir, "ports.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(portalRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	return portalRoot
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestRunDBLogsHypersonicNoOp(t *testing.T) {
	_ = hypersonicWorktree(t)
	cmd := &cobra.Command{}

	var err error
	out := captureStdout(t, func() {
		err = runDBLogs(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runDBLogs returned error: %v", err)
	}
	if !strings.Contains(out, "No Docker-managed") {
		t.Errorf("expected 'No Docker-managed' in stdout, got: %q", out)
	}
}

func TestRunDBPsHypersonicNoOp(t *testing.T) {
	_ = hypersonicWorktree(t)
	cmd := &cobra.Command{}

	var err error
	out := captureStdout(t, func() {
		err = runDBPs(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runDBPs returned error: %v", err)
	}
	if !strings.Contains(out, "No Docker-managed") {
		t.Errorf("expected 'No Docker-managed' in stdout, got: %q", out)
	}
}

func TestRunDBDownHypersonicNoOp(t *testing.T) {
	_ = hypersonicWorktree(t)
	cmd := &cobra.Command{}

	var err error
	out := captureStdout(t, func() {
		err = runDBDown(cmd, nil)
	})
	if err != nil {
		t.Fatalf("runDBDown returned error: %v", err)
	}
	if !strings.Contains(out, "No Docker-managed") {
		t.Errorf("expected 'No Docker-managed' in stdout, got: %q", out)
	}
}
