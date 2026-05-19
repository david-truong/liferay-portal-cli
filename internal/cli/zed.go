package cli

import (
	"fmt"

	"github.com/david-truong/liferay-portal-cli/internal/zed"
	"github.com/spf13/cobra"
)

var zedCmd = &cobra.Command{
	Use:   "zed",
	Short: "Configure liferay-portal for Zed / jdtls",
	Long: `Tools for making the Zed editor's Java language server (jdtls)
resolve symbols across the full liferay-portal source tree.

The portal root ships an Eclipse-style .classpath that only lists ~28 of the
1000+ modules. Files under modules/apps/** fall through to jdtls's "invisible
project" mode and cmd+click fails on most Liferay code. The subcommands here
regenerate that .classpath so every module's sources are visible to jdtls.`,
}

var (
	zedRegenIncludeCache bool
	zedRegenSkipWorktree bool
	zedRegenExcludes     []string
)

var zedRegenCmd = &cobra.Command{
	Use:   "regen",
	Short: "Regenerate the portal root .classpath to include every module source folder",
	Long: `Reads the existing .classpath at the portal root, preserves every
non-source entry (lib, con, output) verbatim, and adds a <classpathentry
kind="src" ...> line for every src/main/java, src/main/resources,
src/test/java, and src/testIntegration/java folder under each discovered
module.

With --include-gradle-cache, additionally appends every jar found under
~/.gradle/caches/modules-2/files-2.1 as a <classpathentry kind="lib">
entry (highest version per artifact). This lets jdtls resolve OSGi
annotations, Spring, SLF4J, and other dependencies that aren't shipped in
the committed lib/development/ folder. The generated block is bracketed
with sentinel comments so it can be regenerated cleanly on the next run.

Run once per worktree (or after creating/removing modules, or refreshing
the Gradle cache). The result is a single .classpath that jdtls reads on
Zed startup; no per-module .iml files, no Gradle import dance.`,
	Args: cobra.NoArgs,
	RunE: runZedRegen,
}

var zedResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Restore the committed .classpath and undo skip-worktree",
	Long: `Clears the skip-worktree bit on .classpath, then runs
git checkout HEAD -- .classpath to restore the committed version. Use
this when you want to share a clean tree, or to recover after a botched
regen.`,
	Args: cobra.NoArgs,
	RunE: runZedReset,
}

func init() {
	zedRegenCmd.Flags().BoolVar(&zedRegenIncludeCache, "include-gradle-cache", true,
		"Append jars from ~/.gradle/caches/modules-2/files-2.1 as lib entries")
	zedRegenCmd.Flags().BoolVar(&zedRegenSkipWorktree, "skip-worktree", true,
		"Mark .classpath skip-worktree so git stops surfacing local edits (committed copy unaffected)")
	zedRegenCmd.Flags().StringSliceVar(&zedRegenExcludes, "exclude",
		zed.DefaultExcludeModulePrefixes,
		"Portal-relative path prefixes whose modules should be skipped (repeatable)")
	zedCmd.AddCommand(zedRegenCmd)
	zedCmd.AddCommand(zedResetCmd)
	rootCmd.AddCommand(zedCmd)
}

func runZedRegen(cmd *cobra.Command, args []string) error {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}
	stats, err := zed.Regenerate(portalRoot, zed.Options{
		IncludeGradleCache:    zedRegenIncludeCache,
		SkipWorktree:          zedRegenSkipWorktree,
		ExcludeModulePrefixes: zedRegenExcludes,
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if zedRegenIncludeCache {
		fmt.Fprintf(out,
			"[zed] wrote %d source entries + %d Gradle cache jars to %s/.classpath\n",
			stats.SourceEntries, stats.GradleJars, portalRoot)
	} else {
		fmt.Fprintf(out,
			"[zed] wrote %d source entries to %s/.classpath\n",
			stats.SourceEntries, portalRoot)
	}
	if stats.SkipWorktreeAdded {
		fmt.Fprintln(out, "[zed] marked .classpath skip-worktree — git will ignore further local edits (run `liferay zed reset` to undo)")
	}
	return nil
}

func runZedReset(cmd *cobra.Command, args []string) error {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}
	if err := zed.ClearSkipWorktree(portalRoot); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "[zed] restored %s/.classpath from HEAD and cleared skip-worktree\n", portalRoot)
	return nil
}
