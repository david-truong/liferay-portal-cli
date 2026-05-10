package portal

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempProps(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.properties")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadProperties_EqualsSeparator(t *testing.T) {
	path := writeTempProps(t, "key=value\n")
	got, err := ReadProperties(path)
	if err != nil {
		t.Fatalf("ReadProperties: %v", err)
	}
	if got["key"] != "value" {
		t.Errorf("expected key=value, got %v", got)
	}
}

func TestReadProperties_ColonSeparator(t *testing.T) {
	path := writeTempProps(t, "key : value\n")
	got, err := ReadProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["key"] != "value" {
		t.Errorf("expected key=value, got %v", got)
	}
}

func TestReadProperties_LineContinuation(t *testing.T) {
	// A trailing backslash continues the value onto the next line.
	path := writeTempProps(t, "key=foo\\\nbar\n")
	got, err := ReadProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["key"] != "foobar" {
		t.Errorf("expected key=foobar, got %v", got)
	}
}

func TestReadProperties_HashComment(t *testing.T) {
	path := writeTempProps(t, "# this is a comment\nkey=value\n")
	got, err := ReadProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["key"] != "value" {
		t.Errorf("expected only key=value, got %v", got)
	}
}

func TestReadProperties_BangComment(t *testing.T) {
	path := writeTempProps(t, "! also a comment\nkey=value\n")
	got, err := ReadProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["key"] != "value" {
		t.Errorf("expected only key=value, got %v", got)
	}
}

func TestReadProperties_BlankLines(t *testing.T) {
	path := writeTempProps(t, "\n\nkey=value\n\n\n")
	got, err := ReadProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["key"] != "value" {
		t.Errorf("expected only key=value, got %v", got)
	}
}

func TestReadProperties_NoSeparatorIsSkipped(t *testing.T) {
	path := writeTempProps(t, "this-line-has-no-separator\nkey=value\n")
	got, err := ReadProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["key"] != "value" {
		t.Errorf("malformed line should be ignored, got %v", got)
	}
}

func TestReadProperties_TrimsSpace(t *testing.T) {
	path := writeTempProps(t, "  key  =  value  \n")
	got, err := ReadProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["key"] != "value" {
		t.Errorf("expected key=value (whitespace trimmed), got %v", got)
	}
}

func TestReadProperties_MissingFile(t *testing.T) {
	_, err := ReadProperties(filepath.Join(t.TempDir(), "does-not-exist.properties"))
	if err == nil {
		t.Error("expected error on missing file")
	}
}
