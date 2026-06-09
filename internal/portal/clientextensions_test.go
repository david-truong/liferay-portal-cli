package portal

import (
	"os"
	"path/filepath"
	"testing"
)

// buildFakeWorkspaces creates a fake portal with two workspaces, each holding
// client extensions, for index tests.
func buildFakeWorkspaces(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	touch := func(parts ...string) {
		t.Helper()
		p := filepath.Join(append([]string{root}, parts...)...)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Unique client extension in one workspace.
	touch("workspaces", "sample-workspace", "client-extensions", "liferay-sample-spring-boot", "client-extension.yaml")

	// Same name present in two workspaces — must require a workspace qualifier.
	touch("workspaces", "sample-workspace", "client-extensions", "shared-element", "client-extension.yaml")
	touch("workspaces", "other-workspace", "client-extensions", "shared-element", "client-extension.yaml")

	// Directory without client-extension.yaml — must not be indexed.
	touch("workspaces", "sample-workspace", "client-extensions", "not-a-ce", "build.gradle")

	touch("build.xml")

	return root
}

func TestClientExtensionResolveExact(t *testing.T) {
	root := buildFakeWorkspaces(t)
	idx, err := BuildClientExtensionIndex(root)
	if err != nil {
		t.Fatalf("BuildClientExtensionIndex: %v", err)
	}

	got, err := idx.Resolve("liferay-sample-spring-boot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, "workspaces", "sample-workspace", "client-extensions", "liferay-sample-spring-boot")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestClientExtensionWithoutYamlNotIndexed(t *testing.T) {
	root := buildFakeWorkspaces(t)
	idx, err := BuildClientExtensionIndex(root)
	if err != nil {
		t.Fatalf("BuildClientExtensionIndex: %v", err)
	}

	if _, err := idx.Resolve("not-a-ce"); err == nil {
		t.Error("expected error for directory without client-extension.yaml, got nil")
	}
}

func TestClientExtensionAmbiguousAcrossWorkspaces(t *testing.T) {
	root := buildFakeWorkspaces(t)
	idx, err := BuildClientExtensionIndex(root)
	if err != nil {
		t.Fatalf("BuildClientExtensionIndex: %v", err)
	}

	_, err = idx.Resolve("shared-element")
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	if !contains(err.Error(), "workspace/name") {
		t.Errorf("ambiguity error should mention the workspace/name qualifier; got: %s", err.Error())
	}

	got, err := idx.Resolve("other-workspace/shared-element")
	if err != nil {
		t.Fatalf("unexpected error resolving qualified name: %v", err)
	}
	want := filepath.Join(root, "workspaces", "other-workspace", "client-extensions", "shared-element")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestClientExtensionNoWorkspacesDir(t *testing.T) {
	root := t.TempDir()
	idx, err := BuildClientExtensionIndex(root)
	if err != nil {
		t.Fatalf("BuildClientExtensionIndex: %v", err)
	}
	if _, err := idx.Resolve("anything"); err == nil {
		t.Error("expected error resolving against empty index, got nil")
	}
}
