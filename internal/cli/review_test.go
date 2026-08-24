package cli

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunReview_NoChangesAgainstMaster(t *testing.T) {
	root := t.TempDir()
	mustGitInitOnBranch(t, root, "trunk")
	writeWorkspaceMarker(t, root)
	mustGitCommit(t, root, "initial")
	mustGitBranchFrom(t, root, "master", "trunk")

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	if err := runReview(&cobra.Command{}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
