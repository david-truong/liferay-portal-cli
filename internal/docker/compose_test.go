package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrInitStateCreatesFile(t *testing.T) {
	dir := t.TempDir()
	state, err := loadOrInitState(dir, "")
	if err != nil {
		t.Fatalf("loadOrInitState: %v", err)
	}
	if state.Engine != DefaultEngine {
		t.Errorf("engine = %q, want %q", state.Engine, DefaultEngine)
	}

	data, err := os.ReadFile(filepath.Join(dir, "ports.json"))
	if err != nil {
		t.Fatalf("ports.json not created: %v", err)
	}
	var persisted State
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if persisted.Slot != state.Slot {
		t.Errorf("persisted slot = %d, want %d", persisted.Slot, state.Slot)
	}
}

func TestLoadOrInitStateReusesExisting(t *testing.T) {
	dir := t.TempDir()
	existing := State{Slot: 5, Engine: EngineMariaDB}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(dir, "ports.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	state, err := loadOrInitState(dir, "")
	if err != nil {
		t.Fatalf("loadOrInitState: %v", err)
	}
	if state.Slot != 5 {
		t.Errorf("slot = %d, want 5", state.Slot)
	}
	if state.Engine != EngineMariaDB {
		t.Errorf("engine = %q, want %q", state.Engine, EngineMariaDB)
	}
}

func TestLoadOrInitStateOverridesEngine(t *testing.T) {
	dir := t.TempDir()
	existing := State{Slot: 3, Engine: EngineMySQL}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(dir, "ports.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	state, err := loadOrInitState(dir, EnginePostgres)
	if err != nil {
		t.Fatalf("loadOrInitState: %v", err)
	}
	if state.Engine != EnginePostgres {
		t.Errorf("engine = %q, want %q", state.Engine, EnginePostgres)
	}
	if state.Slot != 3 {
		t.Errorf("slot = %d, want 3 (should preserve slot)", state.Slot)
	}
}

func TestLoadOrInitStateRejectsUnsupportedEngine(t *testing.T) {
	dir := t.TempDir()
	_, err := loadOrInitState(dir, "oracle")
	if err == nil {
		t.Error("expected error for unsupported engine")
	}
}

func TestLoadStateNonExistent(t *testing.T) {
	dir := t.TempDir()
	_, ok := LoadState(dir)
	if ok {
		t.Error("LoadState should return false for missing state")
	}
}

func TestWritePortalExtFreshFile(t *testing.T) {
	dir := t.TempDir()
	if err := writePortalExt(dir, EngineMySQL, PortsFromSlot(0)); err != nil {
		t.Fatalf("writePortalExt: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "portal-ext.properties"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{
		"include-and-override=portal-developer.properties",
		"users.reminder.queries.enabled=false",
		"terms.of.use.required=false",
		"passwords.default.policy.change.required=false",
		"jdbc.default.driverClassName=com.mysql.cj.jdbc.Driver",
		"browser.launcher.url=",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestWritePortalExtIdempotent(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		if err := writePortalExt(dir, EngineMySQL, PortsFromSlot(0)); err != nil {
			t.Fatalf("writePortalExt iter %d: %v", i, err)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, "portal-ext.properties"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n := strings.Count(string(got), managedBlockBegin); n != 1 {
		t.Errorf("managed block begin count = %d, want 1 (no stacking)", n)
	}
	if n := strings.Count(string(got), "include-and-override=portal-developer.properties"); n != 1 {
		t.Errorf("include-and-override count = %d, want 1", n)
	}
}

func TestWritePortalExtPreservesUserContentAndStripsManagedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal-ext.properties")
	existing := `# user comment
feature.flag.LPD-12345=true
include-and-override=portal-developer.properties
jdbc.default.password=stale
company.default.locale=en_US
`
	if err := os.WriteFile(path, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePortalExt(dir, EngineMySQL, PortsFromSlot(0)); err != nil {
		t.Fatalf("writePortalExt: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"# user comment", "feature.flag.LPD-12345=true", "company.default.locale=en_US"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("dropped user line %q from:\n%s", want, got)
		}
	}
	if n := strings.Count(string(got), "include-and-override=portal-developer.properties"); n != 1 {
		t.Errorf("include-and-override count = %d, want exactly 1 (user copy must be stripped)", n)
	}
	if strings.Contains(string(got), "jdbc.default.password=stale") {
		t.Errorf("stale managed key not stripped:\n%s", got)
	}
}
