package cli

import (
	"errors"
	"fmt"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
)

// Stable exit codes the CLI emits for failure modes agents can reasonably
// want to branch on. Codes not enumerated here collapse into ExitGeneric.
//
// Changing a code is a breaking change to the public CLI surface.
const (
	ExitOK      = 0
	ExitGeneric = 1
	// ExitNotInPortal is emitted whenever an error wraps portal.ErrNotInPortal.
	ExitNotInPortal = 2
	// ExitDockerUnavailable is emitted whenever an error wraps docker.ErrUnavailable.
	ExitDockerUnavailable = 3
	ExitPortCollision     = 4
	// ExitModuleNotFound is emitted whenever an error wraps portal.ErrModuleNotFound.
	ExitModuleNotFound       = 5
	ExitBundleOutside        = 6
	ExitConfirmationDeclined = 7
)

// ExitError pairs an error with a process exit code. Returning an
// ExitError from a cobra RunE causes Execute() to exit with the wrapped
// code instead of the default 1.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// ExitErr constructs an *ExitError. Mirrors fmt.Errorf's format-arg
// signature so call sites read naturally.
func ExitErr(code int, format string, args ...any) *ExitError {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}

// resolveExitCode walks err's chain looking for an *ExitError first — an
// explicit ExitError always wins, even over a chain that also carries one of
// the sentinels below. Failing that, it checks for the sentinels
// internal/portal and internal/docker export, covering failure modes command
// authors haven't wrapped in an ExitError themselves. Anything else maps to
// ExitGeneric; nil maps to 0.
func resolveExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	switch {
	case errors.Is(err, portal.ErrNotInPortal):
		return ExitNotInPortal
	case errors.Is(err, docker.ErrUnavailable):
		return ExitDockerUnavailable
	case errors.Is(err, portal.ErrModuleNotFound):
		return ExitModuleNotFound
	}
	return ExitGeneric
}
