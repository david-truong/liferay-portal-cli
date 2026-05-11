package state

import (
	"os"
	"testing"
	"time"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func TestLastCmd_RoundTrip(t *testing.T) {
	setFakeHome(t, t.TempDir())
	root := t.TempDir()

	want := LastCmd{
		Kind:    LastCmdDB,
		Service: "db",
	}
	if err := SaveLastCmd(root, want); err != nil {
		t.Fatalf("SaveLastCmd: %v", err)
	}

	got, ok, err := LoadLastCmd(root)
	if err != nil {
		t.Fatalf("LoadLastCmd: %v", err)
	}
	if !ok {
		t.Fatal("LoadLastCmd returned ok=false after a successful Save")
	}
	if got.Kind != want.Kind || got.Service != want.Service {
		t.Errorf("round-trip mismatch:\ngot:  %+v\nwant: %+v", got, want)
	}
	if got.When.IsZero() {
		t.Error("expected When to be populated by Save")
	}
	if time.Since(got.When) > 10*time.Second {
		t.Errorf("When should be recent, got %v", got.When)
	}
}

func TestLastCmd_LoadCorrupt(t *testing.T) {
	setFakeHome(t, t.TempDir())
	root := t.TempDir()

	// Stage a malformed record at the expected path.
	if err := SaveLastCmd(root, LastCmd{Kind: LastCmdDB}); err != nil {
		t.Fatal(err)
	}
	path := lastCmdPath(root)
	if err := writeFile(path, "not json"); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadLastCmd(root)
	if err == nil {
		t.Error("expected unmarshal error for corrupt last_command.json")
	}
}

func TestLastCmd_LoadMissing(t *testing.T) {
	setFakeHome(t, t.TempDir())
	root := t.TempDir()

	_, ok, err := LoadLastCmd(root)
	if err != nil {
		t.Fatalf("LoadLastCmd should not error on missing file, got: %v", err)
	}
	if ok {
		t.Error("LoadLastCmd should return ok=false when no record exists")
	}
}

func TestLastCmd_OverwritePrevious(t *testing.T) {
	setFakeHome(t, t.TempDir())
	root := t.TempDir()

	first := LastCmd{Kind: LastCmdServer, LogPath: "/var/log/a.log"}
	if err := SaveLastCmd(root, first); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	second := LastCmd{Kind: LastCmdArchive, LogPath: "/var/log/b.log"}
	if err := SaveLastCmd(root, second); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, _, err := LoadLastCmd(root)
	if err != nil {
		t.Fatalf("LoadLastCmd: %v", err)
	}
	if got.Kind != LastCmdArchive || got.LogPath != "/var/log/b.log" {
		t.Errorf("expected second record to win, got %+v", got)
	}
}
