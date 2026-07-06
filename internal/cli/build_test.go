package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWorkspaceBuildAll_InitsBundleWhenMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "settings.gradle"), []byte(`apply plugin: "com.liferay.workspace"`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// A fake gradlew that just records its own invocation and does nothing —
	// good enough to prove initBundle and deploy both get invoked.
	gradlew := filepath.Join(root, "gradlew")
	script := "#!/bin/sh\necho \"$@\" >> " + filepath.Join(root, "gw.log") + "\n"
	if err := os.WriteFile(gradlew, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "modules", "my-module"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "modules", "my-module", "bnd.bnd"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	if err := runWorkspaceBuildAll(root); err != nil {
		t.Fatalf("runWorkspaceBuildAll: %v", err)
	}

	log, err := os.ReadFile(filepath.Join(root, "gw.log"))
	if err != nil {
		t.Fatalf("reading gw.log: %v", err)
	}
	got := string(log)
	if !contains(got, "initBundle") {
		t.Errorf("expected initBundle to run, log:\n%s", got)
	}
	if !contains(got, "deploy") {
		t.Errorf("expected deploy to run, log:\n%s", got)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
