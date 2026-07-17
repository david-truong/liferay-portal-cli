package state

import "testing"

func TestBuildBase_RoundTrip(t *testing.T) {
	setFakeHome(t, t.TempDir())
	root := t.TempDir()

	if err := SaveBuildBase(root, "abc123"); err != nil {
		t.Fatalf("SaveBuildBase: %v", err)
	}

	got, ok, err := LoadBuildBase(root)
	if err != nil {
		t.Fatalf("LoadBuildBase: %v", err)
	}
	if !ok {
		t.Fatal("LoadBuildBase returned ok=false after a successful Save")
	}
	if got.SHA != "abc123" {
		t.Errorf("got SHA %q, want %q", got.SHA, "abc123")
	}
}

func TestBuildBase_LoadCorrupt(t *testing.T) {
	setFakeHome(t, t.TempDir())
	root := t.TempDir()

	if err := SaveBuildBase(root, "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(buildBasePath(root), "not json"); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadBuildBase(root)
	if err == nil {
		t.Error("expected unmarshal error for corrupt build_base.json")
	}
}

func TestBuildBase_LoadMissing(t *testing.T) {
	setFakeHome(t, t.TempDir())
	root := t.TempDir()

	_, ok, err := LoadBuildBase(root)
	if err != nil {
		t.Fatalf("LoadBuildBase should not error on missing file, got: %v", err)
	}
	if ok {
		t.Error("LoadBuildBase should return ok=false when no record exists")
	}
}

func TestBuildBase_OverwritePrevious(t *testing.T) {
	setFakeHome(t, t.TempDir())
	root := t.TempDir()

	if err := SaveBuildBase(root, "first"); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := SaveBuildBase(root, "second"); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, _, err := LoadBuildBase(root)
	if err != nil {
		t.Fatalf("LoadBuildBase: %v", err)
	}
	if got.SHA != "second" {
		t.Errorf("expected second record to win, got %+v", got)
	}
}
