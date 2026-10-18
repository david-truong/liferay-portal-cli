package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
)

func writePlaywrightSlotState(t *testing.T, worktreeRoot, content string) {
	t.Helper()

	stateDir := docker.StateDir(worktreeRoot)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "ports.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPlaywrightPortalURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	worktreeRoot := t.TempDir()

	t.Run("no stored state", func(t *testing.T) {
		os.Unsetenv("PORTAL_URL")
		if got := playwrightPortalURL(worktreeRoot); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("slot 0 keeps 8080", func(t *testing.T) {
		os.Unsetenv("PORTAL_URL")
		writePlaywrightSlotState(t, worktreeRoot, `{"slot":0}`)
		if got, want := playwrightPortalURL(worktreeRoot), "http://localhost:8080"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("slot 14 uses its offset port", func(t *testing.T) {
		os.Unsetenv("PORTAL_URL")
		writePlaywrightSlotState(t, worktreeRoot, `{"slot":14}`)
		want := "http://localhost:" + strconv.Itoa(docker.PortsFromSlot(14).TomcatHTTP)
		if got := playwrightPortalURL(worktreeRoot); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("explicit PORTAL_URL wins", func(t *testing.T) {
		t.Setenv("PORTAL_URL", "http://example.test:1")
		if got := playwrightPortalURL(worktreeRoot); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}
