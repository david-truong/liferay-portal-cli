package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunClean_NoArgsRejectedOnWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "settings.gradle"), []byte(`apply plugin: "com.liferay.workspace"`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	err = runClean(&cobra.Command{}, nil)
	if err == nil {
		t.Fatal("expected an error for no-arg clean on a Workspace")
	}
	if !contains(err.Error(), "Workspace") {
		t.Errorf("error should mention Workspace, got: %v", err)
	}
}
