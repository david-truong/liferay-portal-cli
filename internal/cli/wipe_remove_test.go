package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWipeServer_DeclinedNonTTY(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")

	err := wipeServer(false /* assumeYes */, strings.NewReader(""), &bytes.Buffer{}, false /* isTTY */)

	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitConfirmationDeclined {
		t.Errorf("expected ExitConfirmationDeclined (7), got %v", err)
	}
}

func TestWipeServer_DeclinedMessageMentionsConsent(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	var out bytes.Buffer

	_ = wipeServer(false, strings.NewReader(""), &out, false)

	if !strings.Contains(out.String(), "--yes") {
		t.Errorf("refusal output should mention --yes, got %q", out.String())
	}
}

func TestRemoveWorktree_DeclinedNonTTY(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	target := t.TempDir()
	canary := filepath.Join(target, "canary")
	if err := os.WriteFile(canary, []byte("survive me"), 0644); err != nil {
		t.Fatal(err)
	}

	err := removeWorktree(target, false /* assumeYes */, strings.NewReader(""), &bytes.Buffer{}, false)

	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitConfirmationDeclined {
		t.Errorf("expected ExitConfirmationDeclined (7), got %v", err)
	}
	if _, err := os.Stat(canary); err != nil {
		t.Errorf("canary file should still exist after declined removal, got: %v", err)
	}
}

func TestRemoveWorktree_DeclinedPromptIncludesTarget(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	var out bytes.Buffer
	target := t.TempDir()

	_ = removeWorktree(target, false, strings.NewReader(""), &out, false)

	if !strings.Contains(out.String(), "--yes") {
		t.Errorf("refusal output should mention --yes, got %q", out.String())
	}
}
