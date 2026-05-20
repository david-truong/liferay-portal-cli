package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	verbose    bool
	workingDir string
)

var rootCmd = &cobra.Command{
	Use:   "liferay",
	Short: "CLI for Liferay portal workflows, built for agents and humans alike",
	Long: `liferay drives every common Liferay workflow from a single working directory
(the portal root) with no cd, no interactive prompts, and no arcane flags.

Designed first for AI agents, but useful to any developer who'd rather not
juggle gw, blade, catalina.sh, and docker compose by hand.`,
	SilenceUsage:      true,
	PersistentPreRunE: rootPreSetup,
}

// rootPreSetup performs the workspace bootstrapping every liferay invocation
// expects: honor -C/--directory, then auto-fix any missing per-worktree files.
func rootPreSetup(_ *cobra.Command, _ []string) error {
	if workingDir != "" {
		if err := os.Chdir(workingDir); err != nil {
			return fmt.Errorf("change directory to %s: %w", workingDir, err)
		}
	}
	autofixCurrentWorktree()
	return nil
}

func autofixCurrentWorktree() {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return
	}
	autofixWorktree(portalRoot)
}

// globalFlags captures the root persistent flag values parsed manually by
// subcommands that set DisableFlagParsing: true. In that mode cobra also
// disables parsing of the parent's persistent flags, leaving their bound
// variables at zero values when the subcommand runs.
type globalFlags struct {
	dir     string
	verbose bool
}

// parseGlobalFlags consumes root-level persistent flags (-C/--directory and
// -v/--verbose) from the front of args. Parsing stops at the first arg that
// is not one of those flags; everything from that point on is returned
// untouched so subcommand-specific flags (e.g. gradle's --tests) pass through.
//
// Recognized forms:
//
//	-C <path>, -C<path>, --directory <path>, --directory=<path>
//	-v, --verbose
func parseGlobalFlags(args []string) (globalFlags, []string, error) {
	var gf globalFlags
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "-C", a == "--directory":
			if i+1 >= len(args) {
				return gf, nil, fmt.Errorf("flag needs an argument: %s", a)
			}
			gf.dir = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--directory="):
			gf.dir = strings.TrimPrefix(a, "--directory=")
			i++
		case strings.HasPrefix(a, "-C") && len(a) > 2:
			gf.dir = a[2:]
			i++
		case a == "-v", a == "--verbose":
			gf.verbose = true
			i++
		default:
			return gf, args[i:], nil
		}
	}
	return gf, args[i:], nil
}

// applyGlobalFlags performs the side effects (chdir + autofix, verbose) that
// rootPreSetup would have performed if the flags had been parsed normally.
func applyGlobalFlags(gf globalFlags) error {
	if gf.verbose {
		verbose = true
	}
	if gf.dir != "" {
		if err := os.Chdir(gf.dir); err != nil {
			return fmt.Errorf("change directory to %s: %w", gf.dir, err)
		}
		workingDir = gf.dir
		autofixCurrentWorktree()
	}
	return nil
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"Stream full build/test output to the terminal (default: log to temp file, show tail on failure)")
	rootCmd.PersistentFlags().StringVarP(&workingDir, "directory", "C", "",
		"Run as if liferay was started in <path> instead of the current working directory")
}

func Execute(version string) {
	if home, err := os.UserHomeDir(); err != nil || home == "" {
		fmt.Fprintln(os.Stderr,
			"Error: HOME (USERPROFILE on Windows) is not set. liferay-cli stores per-worktree state under ~/.liferay-cli/ and cannot run without a writable user home directory.")
		os.Exit(ExitGeneric)
	}
	rootCmd.Version = version
	err := rootCmd.Execute()
	if code := resolveExitCode(err); code != ExitOK {
		os.Exit(code)
	}
}
