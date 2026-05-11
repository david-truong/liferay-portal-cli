package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDir_DeterministicForSameRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := "/some/abs/path/portal"
	d1 := Dir(root)
	d2 := Dir(root)
	if d1 != d2 {
		t.Errorf("Dir is not deterministic: %q vs %q", d1, d2)
	}
}

func TestDir_DistinctForDifferentRoots(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := Dir("/portal-a")
	b := Dir("/portal-b")
	if a == b {
		t.Errorf("Dir(a)==Dir(b): %q", a)
	}
}

func TestDir_DistinctForSameBasenameDifferentPaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := Dir("/work/portal")
	b := Dir("/other/portal")
	if a == b {
		t.Errorf("two different parents share the same basename 'portal' but produced the same Dir: %q", a)
	}
}

func TestDir_RelativePathResolvesToAbsolute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Relative paths should be made absolute before hashing — otherwise the
	// state dir would shift with the caller's cwd.
	abs, err := filepath.Abs("./portal")
	if err != nil {
		t.Fatal(err)
	}
	rel := Dir("./portal")
	want := Dir(abs)
	if rel != want {
		t.Errorf("relative path produced different Dir than absolute equivalent\nrel: %s\nabs: %s", rel, want)
	}
}

func TestRoot_PanicsWhenHomeMissing(t *testing.T) {
	// Force os.UserHomeDir to fail by unsetting every variable it
	// consults.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Root to panic when HOME is unset")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "HOME") {
			t.Errorf("panic message should mention HOME, got %v", r)
		}
	}()
	_ = Root()
}

func TestDir_LivesUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := Dir("/foo/portal")
	if !strings.HasPrefix(got, filepath.Join(home, ".liferay-cli")) {
		t.Errorf("Dir should live under $HOME/.liferay-cli, got %q (home=%q)", got, home)
	}
}

func TestWriteFileAtomic_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	want := []byte("hello\n")

	if err := WriteFileAtomic(path, want, 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("content mismatch: got %q, want %q", got, want)
	}

	// Tempfile should not be left behind after the rename.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected %s.tmp to be removed, got err=%v", path, err)
	}
}

func TestWriteFileAtomic_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	if err := os.WriteFile(path, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	want := []byte("new content")
	if err := WriteFileAtomic(path, want, 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("content mismatch: got %q, want %q", got, want)
	}
}

func TestDisplayHome_PathUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	p := filepath.Join(home, "Projects", "liferay")
	got := DisplayHome(p)
	want := filepath.Join("~", "Projects", "liferay")
	if got != want {
		t.Errorf("DisplayHome(%q) = %q, want %q", p, got, want)
	}
}

func TestDisplayHome_PathOutsideHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p := "/var/log/something"
	got := DisplayHome(p)
	if got != p {
		t.Errorf("DisplayHome(%q) should return path unchanged when not under HOME, got %q", p, got)
	}
}
