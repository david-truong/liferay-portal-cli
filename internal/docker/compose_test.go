package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
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
