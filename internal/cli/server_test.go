package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/tomcat"
)

func TestServerStatusText_Running(t *testing.T) {
	var buf bytes.Buffer
	paths := tomcat.Paths{Bundle: "/path/to/bundle"}
	st := docker.State{Slot: 5, Engine: "mysql"}

	if err := serverStatusTextOutput(paths, st, 12345, true, &buf); err != nil {
		t.Fatalf("serverStatusTextOutput: %v", err)
	}

	got := buf.String()
	// Slot 5: TomcatHTTP = 8080 + 5*10 = 8130; JPDA = 8000 + 5*10 = 8050.
	wants := []string{
		"running\n",
		"  pid:    12345\n",
		"  slot:   5\n",
		"  port:   8130\n",
		"  jpda:   8050\n",
		"  bundle: /path/to/bundle\n",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, got)
		}
	}
}

func TestServerStatusText_StalePid(t *testing.T) {
	var buf bytes.Buffer
	paths := tomcat.Paths{Bundle: "/path/to/bundle"}
	st := docker.State{Slot: 2}

	if err := serverStatusTextOutput(paths, st, 9999, false, &buf); err != nil {
		t.Fatalf("serverStatusTextOutput: %v", err)
	}

	got := buf.String()
	if !strings.HasPrefix(got, "stale pid file\n") {
		t.Errorf("headline should be 'stale pid file', got:\n%s", got)
	}
	if !strings.Contains(got, "9999 (no longer alive)") {
		t.Errorf("stale pid annotation missing, got:\n%s", got)
	}
	if !strings.Contains(got, "  slot:   2\n") {
		t.Errorf("slot line missing, got:\n%s", got)
	}
}

func TestServerStatusText_NotRunning(t *testing.T) {
	var buf bytes.Buffer
	if err := serverStatusTextOutput(tomcat.Paths{}, docker.State{Slot: 0}, 0, false, &buf); err != nil {
		t.Fatalf("serverStatusTextOutput: %v", err)
	}

	got := buf.String()
	if !strings.HasPrefix(got, "not running\n") {
		t.Errorf("headline should be 'not running', got:\n%s", got)
	}
	if strings.Contains(got, "pid:") {
		t.Errorf("pid line should be absent when pid=0, got:\n%s", got)
	}
	// Slot 0 still reports its configured ports.
	if !strings.Contains(got, "  port:   8080\n") {
		t.Errorf("slot-0 port line missing, got:\n%s", got)
	}
	// Empty bundle path is suppressed.
	if strings.Contains(got, "bundle:") {
		t.Errorf("empty bundle path should be omitted, got:\n%s", got)
	}
}
