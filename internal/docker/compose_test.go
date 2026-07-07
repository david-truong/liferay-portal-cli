package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrInitStateCreatesFile(t *testing.T) {
	home := t.TempDir(); t.Setenv("HOME", home); t.Setenv("USERPROFILE", home)
	dir := t.TempDir()
	state, err := loadOrInitState(dir, "", dir, true)
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
	home := t.TempDir(); t.Setenv("HOME", home); t.Setenv("USERPROFILE", home)
	dir := t.TempDir()
	existing := State{Slot: 5, Engine: EngineMariaDB}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(dir, "ports.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	state, err := loadOrInitState(dir, "", dir, true)
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
	home := t.TempDir(); t.Setenv("HOME", home); t.Setenv("USERPROFILE", home)
	dir := t.TempDir()
	existing := State{Slot: 3, Engine: EngineMySQL}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(dir, "ports.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	state, err := loadOrInitState(dir, EnginePostgres, dir, true)
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
	home := t.TempDir(); t.Setenv("HOME", home); t.Setenv("USERPROFILE", home)
	dir := t.TempDir()
	_, err := loadOrInitState(dir, "oracle", dir, true)
	if err == nil {
		t.Error("expected error for unsupported engine")
	}
}

// TestLoadOrInitStateFailsLoudlyOnCorruptFile guards MED-5: a truncated or
// otherwise unparseable ports.json used to be silently treated as "no state
// yet", which allocates slot 0 — the slot reserved for the primary
// checkout — for what may be a linked worktree with containers already
// running under a different slot. Corruption must surface as an error
// naming the file, never as a fabricated State{Slot: 0}.
func TestLoadOrInitStateFailsLoudlyOnCorruptFile(t *testing.T) {
	home := t.TempDir(); t.Setenv("HOME", home); t.Setenv("USERPROFILE", home)
	dir := t.TempDir()
	portsFile := filepath.Join(dir, "ports.json")
	if err := os.WriteFile(portsFile, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := loadOrInitState(dir, "", dir, true)
	if err == nil {
		t.Fatalf("expected error for corrupt %s, got state %+v", portsFile, got)
	}
	if !strings.Contains(err.Error(), portsFile) {
		t.Errorf("error %q should name the corrupt file %q", err.Error(), portsFile)
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
		"object.encryption.enabled=true",
		"object.encryption.algorithm=AES",
		"object.encryption.key=0H5WCxHcGAHsVv0OcGktBQ==",
		`configuration.override.com.liferay.change.tracking.web.internal.configuration.CTConfiguration_showAllData=B"true"`,
		`configuration.override.com.liferay.change.tracking.configuration.CTSettingsConfiguration_enabled=B"true"`,
		`configuration.override.com.liferay.portal.search.elasticsearch7.configuration.ElasticsearchConfiguration_productionModeEnabled=B"false"`,
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

func TestWritePortalExtWhitelistsSlotHost(t *testing.T) {
	dir := t.TempDir()
	if err := writePortalExt(dir, EngineMySQL, PortsFromSlot(3)); err != nil {
		t.Fatalf("writePortalExt: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "portal-ext.properties"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "virtual.hosts.valid.hosts=localhost,127.0.0.1,[::1],[0:0:0:0:0:0:0:1],*.liferay.test"
	if !strings.Contains(string(got), want) {
		t.Errorf("missing %q in:\n%s", want, got)
	}
}

// TestWritePortalExtPreservesModeAndIsAtomic guards MED-6a: portal-ext.properties
// is owned by the user (it's their bundle config, only a managed block inside
// it belongs to liferay-cli), so a rewrite must preserve the file's existing
// mode and never leave a stray temp file behind — a torn write here would
// destroy the user's own lines that writePortalExt re-emits from its read.
func TestWritePortalExtPreservesModeAndIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portal-ext.properties")
	if err := os.WriteFile(path, []byte("company.default.locale=en_US\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writePortalExt(dir, EngineMySQL, PortsFromSlot(0)); err != nil {
		t.Fatalf("writePortalExt: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %v, want existing file's mode 0600 preserved", info.Mode().Perm())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "portal-ext.properties" {
		t.Errorf("directory should contain only portal-ext.properties, got %v", entries)
	}
}

func TestWritePortalExtStockOmitsSlotHost(t *testing.T) {
	dir := t.TempDir()
	if err := writePortalExt(dir, EngineMySQL, PortsFromSlot(0)); err != nil {
		t.Fatalf("writePortalExt: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "portal-ext.properties"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(got), "virtual.hosts.valid.hosts") {
		t.Errorf("stock slot must not override virtual.hosts.valid.hosts:\n%s", got)
	}
}
