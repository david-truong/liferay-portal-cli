package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// moduleNameCompletions builds the module index from the current worktree and
// returns the names matching toComplete. Extra names (e.g. the Ant root
// projects that "build" accepts) are folded in and the combined list is
// prefix-filtered and sorted.
func moduleNameCompletions(toComplete string, extra ...string) ([]string, cobra.ShellCompDirective) {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	idx, err := buildModuleIndex(portalRoot)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names := append(idx.Names(), extra...)

	matches := make([]string, 0, len(names))
	for _, name := range names {
		if strings.HasPrefix(name, toComplete) {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)

	return matches, cobra.ShellCompDirectiveNoFileComp
}

// completeModuleArgs completes module names for every positional argument, for
// commands that take a variable-length list of modules (build, clean,
// source-format).
func completeModuleArgs(extra ...string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return moduleNameCompletions(toComplete, extra...)
	}
}

// completeModuleFirstArg completes a module name only for the first positional
// argument, for commands that take a single module followed by other arguments
// (test, test-integration, build-service, build-rest, gradle-wrapper).
func completeModuleFirstArg(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return moduleNameCompletions(toComplete)
}
