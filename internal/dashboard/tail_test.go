package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailFileFromZeroIsUnfiltered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := tailFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "line1\nline2\n" {
		t.Errorf("content = %q, want both lines", got)
	}
}

func TestTailFileFromOffsetHidesEarlierContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	offset := info.Size() // "clear" at the current end of file

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("line3\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, err := tailFile(path, offset)
	if err != nil {
		t.Fatal(err)
	}
	if got != "line3\n" {
		t.Errorf("content = %q, want only the line written after the offset", got)
	}
}

func TestTailFileFromOffsetPastEndOfFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte("line1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// A stale offset from a previous, larger file (e.g. a fresh command's
	// log) must clamp rather than error.
	got, err := tailFile(path, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("content = %q, want empty", got)
	}
}

func TestTailFileOffsetNeverBeatsTailBytesCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	content := strings.Repeat("x", tailBytes+100) + "\ntail\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := tailFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "tail\n" {
		t.Errorf("content = %q, want the tailBytes-capped tail", got)
	}
}
