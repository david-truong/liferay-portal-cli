package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/tomcat"
)

func TestServerStatusJSON_Schema(t *testing.T) {
	var buf bytes.Buffer
	paths := tomcat.Paths{Bundle: "/path/to/bundle"}
	st := docker.State{Slot: 5, Engine: "mysql"}

	if err := serverStatusJSONOutput(paths, st, 12345, true, &buf); err != nil {
		t.Fatalf("serverStatusJSONOutput: %v", err)
	}

	var got struct {
		Slot      int    `json:"slot"`
		Pid       int    `json:"pid"`
		Alive     bool   `json:"alive"`
		Port      int    `json:"port"`
		JPDAPort  int    `json:"jpda_port"`
		BundleDir string `json:"bundle_dir"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	if got.Slot != 5 {
		t.Errorf("slot = %d, want 5", got.Slot)
	}
	if got.Pid != 12345 {
		t.Errorf("pid = %d, want 12345", got.Pid)
	}
	if !got.Alive {
		t.Error("alive = false, want true")
	}
	// Slot 5: TomcatHTTP = 8080 + 5*10 = 8130; JPDA = 8000 + 5*10 = 8050.
	if got.Port != 8130 {
		t.Errorf("port = %d, want 8130", got.Port)
	}
	if got.JPDAPort != 8050 {
		t.Errorf("jpda_port = %d, want 8050", got.JPDAPort)
	}
	if got.BundleDir != "/path/to/bundle" {
		t.Errorf("bundle_dir = %q, want /path/to/bundle", got.BundleDir)
	}
}

func TestServerStatusJSON_NotRunning(t *testing.T) {
	var buf bytes.Buffer
	if err := serverStatusJSONOutput(tomcat.Paths{}, docker.State{Slot: 0}, 0, false, &buf); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Pid   int  `json:"pid"`
		Alive bool `json:"alive"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Alive {
		t.Error("alive should be false when pid=0")
	}
	if got.Pid != 0 {
		t.Errorf("pid = %d, want 0", got.Pid)
	}
}

func TestDBPsJSON_Mysql(t *testing.T) {
	var buf bytes.Buffer
	st := docker.State{Engine: "mysql", Slot: 2}

	if err := dbPsJSONOutput(st, &buf); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Engine  string `json:"engine"`
		Slot    int    `json:"slot"`
		Port    int    `json:"port"`
		Managed bool   `json:"managed"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if got.Engine != "mysql" || got.Slot != 2 || got.Port != 3326 || !got.Managed {
		t.Errorf("got %+v", got)
	}
}

func TestDBPsJSON_Hypersonic(t *testing.T) {
	var buf bytes.Buffer
	st := docker.State{Engine: "hypersonic", Slot: 0}

	if err := dbPsJSONOutput(st, &buf); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Engine  string `json:"engine"`
		Slot    int    `json:"slot"`
		Port    int    `json:"port"`
		Managed bool   `json:"managed"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Engine != "hypersonic" {
		t.Errorf("engine = %q, want hypersonic", got.Engine)
	}
	if got.Managed {
		t.Error("hypersonic should report managed=false")
	}
}

func TestWorktreeListJSON_ParsesPorcelain(t *testing.T) {
	porcelain := `worktree /Users/dtruong/portal
HEAD abc123
branch refs/heads/main

worktree /Users/dtruong/LPD-12345
HEAD def456
branch refs/heads/LPD-12345

worktree /Users/dtruong/LPD-detached
HEAD ghi789
detached

`
	primary := "/Users/dtruong/portal"
	got := parseWorktreePorcelain(porcelain, primary)

	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d:\n%+v", len(got), got)
	}

	if got[0].Path != "/Users/dtruong/portal" || got[0].Branch != "main" || !got[0].Primary {
		t.Errorf("primary entry malformed: %+v", got[0])
	}
	if got[1].Path != "/Users/dtruong/LPD-12345" || got[1].Branch != "LPD-12345" || got[1].Primary {
		t.Errorf("LPD-12345 entry malformed: %+v", got[1])
	}
	if got[2].Path != "/Users/dtruong/LPD-detached" || got[2].Branch != "" || got[2].Primary {
		t.Errorf("detached entry malformed: %+v", got[2])
	}
}

func TestWorktreeListJSON_EmitsArray(t *testing.T) {
	var buf bytes.Buffer
	entries := []worktreeEntry{
		{Path: "/a", Branch: "main", Slot: 0, Primary: true},
		{Path: "/b", Branch: "feature", Slot: 1, Primary: false},
	}
	if err := emitWorktreeListJSON(entries, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(buf.String()), "[") {
		t.Errorf("expected JSON array, got %q", buf.String())
	}
	var got []worktreeEntry
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 2 || got[0].Path != "/a" || got[1].Slot != 1 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
