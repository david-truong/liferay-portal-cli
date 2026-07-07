package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteHostsFileAt_RewritesContentAndPreservesMode guards MED-6b: /etc/hosts
// is a file the OS shares with every process on the machine, so a rewrite
// must preserve the existing file's mode and land the exact new content —
// a torn write there breaks name resolution system-wide.
func TestWriteHostsFileAt_RewritesContentAndPreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	if err := os.WriteFile(path, []byte("127.0.0.1 localhost\n"), 0644); err != nil {
		t.Fatal(err)
	}

	want := "127.0.0.1 localhost\n127.0.0.1 lpd-12345 # liferay-cli abc123\n"
	if err := writeHostsFileAt(path, want); err != nil {
		t.Fatalf("writeHostsFileAt: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("content = %q, want %q", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("mode = %v, want 0644 preserved", info.Mode().Perm())
	}
}

// TestWriteHostsFileAt_NoStrayTempFile guards against a rewrite leaving a
// temp file next to /etc/hosts (e.g. the old fixed ".tmp" suffix, which two
// concurrent invocations — or a previous crash — could collide on or leak).
func TestWriteHostsFileAt_NoStrayTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")

	if err := writeHostsFileAt(path, "127.0.0.1 localhost\n"); err != nil {
		t.Fatalf("writeHostsFileAt: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "hosts" {
		t.Errorf("directory should contain only hosts, got %v", entries)
	}
}

// TestWriteHostsFileAt_PermissionErrorSurfacesForSudoHint guards the caller
// contract in runHostsAdd/runHostsRemove: when /etc/hosts can't be written
// without root, they branch on errors.Is(err, fs.ErrPermission) to print a
// sudo one-liner instead of failing opaquely. Creating the temp file in a
// read-only directory must still surface as a permission error even though
// the failure now happens in os.CreateTemp rather than os.WriteFile.
func TestWriteHostsFileAt_PermissionErrorSurfacesForSudoHint(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root — directory permissions don't block writes")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755) // let t.TempDir() clean up afterward

	path := filepath.Join(dir, "hosts")
	err := writeHostsFileAt(path, "127.0.0.1 localhost\n")
	if err == nil {
		t.Fatal("expected a permission error writing into a read-only directory")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("err = %v, want errors.Is(err, fs.ErrPermission)", err)
	}
}
