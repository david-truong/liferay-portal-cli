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

var zedRegenCmd = &cobra.Command{
	Use:   "regen",
	Short: "Regenerate the portal root .classpath to include every module source folder",
	Long: `Reads the existing .classpath at the portal root, preserves every
non-source entry (lib, con, output) verbatim, and adds a <classpathentry
kind="src" ...> line for every src/main/java, src/main/resources,
src/test/java, and src/testIntegration/java folder under each discovered
module.

Run this once per worktree (or after creating/removing modules). The result
is a single .classpath that jdtls reads on Zed startup; no per-module .iml
files, no Gradle import dance.`,
	Args: cobra.NoArgs,
	RunE: runZedRegen,
}

func init() {
	zedCmd.AddCommand(zedRegenCmd)
	rootCmd.AddCommand(zedCmd)
}

func runZedRegen(cmd *cobra.Command, args []string) error {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}
	n, err := zed.Regenerate(portalRoot)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "[zed] wrote %d source entries to %s/.classpath\n", n, portalRoot)
	return nil
}
