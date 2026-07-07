package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A label sorting after another label's ("zzz-" > "aaa-") must not win over a
// genuinely newer log — newestLog compares modification time, not filename.
func TestNewestLog_PicksLatestModTime(t *testing.T) {
	logDir := t.TempDir()

	older := filepath.Join(logDir, "zzz-20260101-000000.000000000.log")
	newer := filepath.Join(logDir, "aaa-20260102-000000.000000000.log")
	if err := os.WriteFile(older, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().Add(-time.Hour)
	newTime := time.Now()
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	got, err := newestLog(logDir)
	if err != nil {
		t.Fatalf("newestLog: %v", err)
	}
	if got != newer {
		t.Errorf("newestLog = %q, want %q", got, newer)
	}
}
