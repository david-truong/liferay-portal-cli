package cli

import (
	"fmt"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/tomcat"
	"github.com/spf13/cobra"
)

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Inspect or roll back the per-slot bundle patches",
	Long: `Subcommands for managing the per-slot edits liferay-cli applies to the
active bundle on "liferay server start". The only command today is
"bundle unpatch", which restores the bundle to the state captured by
the most-recent automatic snapshot.`,
}

var bundleUnpatchCmd = &cobra.Command{
	Use:   "unpatch",
	Short: "Restore the bundle from the most-recent pre-patch snapshot",
	Long: `Reads the newest snapshot under ~/.liferay-cli/worktrees/<id>/bundle-snapshot/
and copies every snapshotted file back to its original location. Files
that were absent at snapshot time are deleted if they exist now.

Refuses to run while Tomcat is up — stop it with "liferay server stop"
first.

If no snapshot exists, prints a message and exits 0.`,
	RunE: runBundleUnpatch,
}

func init() {
	bundleCmd.AddCommand(bundleUnpatchCmd)
	rootCmd.AddCommand(bundleCmd)
}

func runBundleUnpatch(_ *cobra.Command, _ []string) error {
	paths, err := resolvePaths()
	if err != nil {
		return err
	}
	return unpatchBundle(paths, filepath.Dir(paths.PidFile))
}

// unpatchBundle is the testable core of "liferay bundle unpatch". It
// refuses to run while Tomcat is alive, then walks the most-recent
// snapshot under stateDir and restores it.
func unpatchBundle(paths tomcat.Paths, stateDir string) error {
	if pid, alive := tomcat.Status(paths); alive {
		return fmt.Errorf("tomcat is running (pid %d) — stop it first with `liferay server stop`", pid)
	}

	snapshotDir, ok, err := tomcat.MostRecentSnapshot(stateDir)
	if err != nil {
		return fmt.Errorf("locating snapshot: %w", err)
	}
	if !ok {
		fmt.Println("No snapshot to restore; bundle is unchanged.")
		return nil
	}

	if err := tomcat.RestoreFromSnapshot(snapshotDir, paths); err != nil {
		return fmt.Errorf("restoring snapshot %s: %w", snapshotDir, err)
	}
	fmt.Printf("Restored bundle from %s\n", snapshotDir)
	return nil
}

