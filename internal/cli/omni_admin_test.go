package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOmniAdminGuard_BundleOutside_NoOverride(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	workRoot := t.TempDir()
	bundleDir := t.TempDir() // different temp dir, not under workRoot

	err := omniAdminGuard(workRoot, bundleDir,
		false /* allowExternal */, true, /* assumeYes */
		strings.NewReader(""), &bytes.Buffer{}, false)

	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitBundleOutside {
		t.Errorf("expected ExitBundleOutside (6), got %v", err)
	}
}

func TestOmniAdminGuard_BundleOutside_WithOverride_Consent(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	workRoot := t.TempDir()
	bundleDir := t.TempDir()

	err := omniAdminGuard(workRoot, bundleDir,
		true /* allowExternal */, true, /* assumeYes */
		strings.NewReader(""), &bytes.Buffer{}, false)

	if err != nil {
		t.Errorf("override + consent should pass, got: %v", err)
	}
}

func TestOmniAdminGuard_BundleInside_Consent(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	workRoot := t.TempDir()
	bundleDir := filepath.Join(workRoot, "bundles")
	if err := os.Mkdir(bundleDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := omniAdminGuard(workRoot, bundleDir,
		false /* allowExternal */, true, /* assumeYes */
		strings.NewReader(""), &bytes.Buffer{}, false)

	if err != nil {
		t.Errorf("inside-worktree + consent should pass, got: %v", err)
	}
}

func TestOmniAdminGuard_BundleInside_NoConsent_NonTTY(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	workRoot := t.TempDir()
	bundleDir := filepath.Join(workRoot, "bundles")
	if err := os.Mkdir(bundleDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := omniAdminGuard(workRoot, bundleDir,
		false /* allowExternal */, false, /* assumeYes */
		strings.NewReader(""), &bytes.Buffer{}, false /* isTTY */)

	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitConfirmationDeclined {
		t.Errorf("expected ExitConfirmationDeclined (7), got %v", err)
	}
}

func TestOmniAdminGuard_BundleInside_EnvVarConsent(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "1")
	workRoot := t.TempDir()
	bundleDir := filepath.Join(workRoot, "bundles")
	if err := os.Mkdir(bundleDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := omniAdminGuard(workRoot, bundleDir,
		false /* allowExternal */, false, /* assumeYes flag not set */
		strings.NewReader(""), &bytes.Buffer{}, false)

	if err != nil {
		t.Errorf("env-var consent should pass, got: %v", err)
	}
}

func TestOmniAdminGuard_BundleErrorMessageIncludesOverrideHint(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	workRoot := t.TempDir()
	bundleDir := t.TempDir()

	err := omniAdminGuard(workRoot, bundleDir,
		false, true,
		strings.NewReader(""), &bytes.Buffer{}, false)

	if err == nil || !strings.Contains(err.Error(), "--allow-external-bundle") {
		t.Errorf("error should hint at --allow-external-bundle override, got: %v", err)
	}
}
