package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
)

func TestExitError_CodeAndMessage(t *testing.T) {
	err := ExitErr(ExitBundleOutside, "bundle %q is outside worktree", "/some/path")
	if err.Code != ExitBundleOutside {
		t.Errorf("Code = %d, want %d", err.Code, ExitBundleOutside)
	}
	if err.Error() != `bundle "/some/path" is outside worktree` {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestExitError_ErrorsAs(t *testing.T) {
	err := fmt.Errorf("wrapping: %w", ExitErr(ExitConfirmationDeclined, "user said no"))
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatal("errors.As failed to unwrap ExitError")
	}
	if ee.Code != ExitConfirmationDeclined {
		t.Errorf("Code = %d, want %d", ee.Code, ExitConfirmationDeclined)
	}
}

func TestResolveExitCode_PlainError(t *testing.T) {
	if code := resolveExitCode(errors.New("oops")); code != ExitGeneric {
		t.Errorf("plain error should map to ExitGeneric, got %d", code)
	}
}

func TestResolveExitCode_TypedError(t *testing.T) {
	cases := []int{
		ExitGeneric,
		ExitNotInPortal,
		ExitDockerUnavailable,
		ExitPortCollision,
		ExitModuleNotFound,
		ExitBundleOutside,
		ExitConfirmationDeclined,
	}
	for _, want := range cases {
		got := resolveExitCode(ExitErr(want, "boom"))
		if got != want {
			t.Errorf("resolveExitCode(ExitErr(%d)) = %d", want, got)
		}
	}
}

func TestResolveExitCode_NilError(t *testing.T) {
	if code := resolveExitCode(nil); code != 0 {
		t.Errorf("nil error should produce exit 0, got %d", code)
	}
}

func TestResolveExitCode_WrappedNotInPortal(t *testing.T) {
	err := fmt.Errorf("x: %w", portal.ErrNotInPortal)
	if code := resolveExitCode(err); code != ExitNotInPortal {
		t.Errorf("resolveExitCode(wrapped ErrNotInPortal) = %d, want %d", code, ExitNotInPortal)
	}
}

func TestResolveExitCode_WrappedDockerUnavailable(t *testing.T) {
	err := fmt.Errorf("x: %w", docker.ErrUnavailable)
	if code := resolveExitCode(err); code != ExitDockerUnavailable {
		t.Errorf("resolveExitCode(wrapped ErrUnavailable) = %d, want %d", code, ExitDockerUnavailable)
	}
}

func TestResolveExitCode_WrappedModuleNotFound(t *testing.T) {
	err := fmt.Errorf("x: %w", portal.ErrModuleNotFound)
	if code := resolveExitCode(err); code != ExitModuleNotFound {
		t.Errorf("resolveExitCode(wrapped ErrModuleNotFound) = %d, want %d", code, ExitModuleNotFound)
	}
}

// TestResolveExitCode_ExplicitExitErrorWinsOverSentinel confirms an *ExitError
// takes precedence even when its wrapped chain also carries one of the
// sentinels above — an explicit code from a command author is authoritative.
func TestResolveExitCode_ExplicitExitErrorWinsOverSentinel(t *testing.T) {
	err := ExitErr(ExitBundleOutside, "x: %w", portal.ErrNotInPortal)
	if code := resolveExitCode(err); code != ExitBundleOutside {
		t.Errorf("resolveExitCode = %d, want %d (explicit ExitError should win)", code, ExitBundleOutside)
	}
}
