package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// AssumeYesEnvVar is the environment variable that bypasses confirmation
// prompts the same way as a --yes flag. Honored by every destructive
// command.
const AssumeYesEnvVar = "LIFERAY_CLI_ASSUME_YES"

// Confirm asks the user to consent to a destructive operation. Returns
// true when consent is given via any of:
//   - the assumeYes flag is set (e.g. --yes on the command)
//   - the LIFERAY_CLI_ASSUME_YES=1 env var is set
//   - stdin is a TTY and the user typed y/Y/yes
//
// In all other cases (non-TTY without flag or env, user typed n / garbage /
// nothing) returns false and the caller should refuse the operation.
func Confirm(prompt string, assumeYes bool) bool {
	return confirmWithIO(prompt, assumeYes, os.Stdin, os.Stdout, isStdinTTY())
}

// confirmWithIO is the testable core. Pass an explicit stdin reader, stdout
// writer, and isTTY flag.
func confirmWithIO(prompt string, assumeYes bool, in io.Reader, out io.Writer, isTTY bool) bool {
	if assumeYes {
		return true
	}
	if os.Getenv(AssumeYesEnvVar) == "1" {
		return true
	}
	if !isTTY {
		fmt.Fprintf(out,
			"Refusing destructive operation: stdin is not a terminal. Pass --yes or set %s=1 to proceed.\n",
			AssumeYesEnvVar)
		return false
	}

	fmt.Fprintf(out, "%s [y/N]: ", prompt)
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return false
	}
	resp := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return resp == "y" || resp == "yes"
}

func isStdinTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
