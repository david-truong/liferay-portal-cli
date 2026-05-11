package portal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePortalRoot stages the markers FindRoot looks for: a build.xml file
// and a modules/ directory. Returns the portal root path.
func fakePortalRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.xml"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "modules"), 0755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestIsPortalRepo_TrueWhenMarkersPresent(t *testing.T) {
	root := fakePortalRoot(t)
	if !IsPortalRepo(root) {
		t.Error("expected IsPortalRepo to be true when build.xml + modules/ present")
	}
}

func TestIsPortalRepo_FalseWhenMarkersMissing(t *testing.T) {
	if IsPortalRepo(t.TempDir()) {
		t.Error("expected IsPortalRepo to be false for empty dir")
	}
}

func TestIsPortalRepo_FalseWhenModulesIsFile(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "build.xml"), nil, 0644)
	_ = os.WriteFile(filepath.Join(root, "modules"), nil, 0644) // file, not dir
	if IsPortalRepo(root) {
		t.Error("modules as a file should not satisfy IsPortalRepo")
	}
}

func TestFindRoot_AtRoot(t *testing.T) {
	root := fakePortalRoot(t)
	got, err := FindRoot(root)
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	if got != root {
		t.Errorf("FindRoot(root) = %q, want %q", got, root)
	}
}

func TestFindRoot_WalksUpFromNested(t *testing.T) {
	root := fakePortalRoot(t)
	nested := filepath.Join(root, "modules", "apps", "foo", "src")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	got, err := FindRoot(nested)
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	if got != root {
		t.Errorf("FindRoot(%q) = %q, want %q", nested, got, root)
	}
}

func TestFindRoot_NotInsideRepo(t *testing.T) {
	_, err := FindRoot(t.TempDir())
	if err == nil {
		t.Error("expected error when not inside a portal repo")
	}
}

func TestBundleDir_DefaultsToSiblingBundles(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "portal")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	// No app.server.properties — BundleDir falls back to <parent>/bundles.

	got, err := BundleDir(root)
	if err != nil {
		t.Fatalf("BundleDir: %v", err)
	}
	want := filepath.Join(parent, "bundles")
	if got != want {
		t.Errorf("BundleDir = %q, want %q", got, want)
	}
}

func TestBundleDir_RespectsAppServerParentDir(t *testing.T) {
	root := t.TempDir()
	customBundles := filepath.Join(t.TempDir(), "custom-bundles")

	props := "app.server.parent.dir=" + customBundles + "\n"
	if err := os.WriteFile(filepath.Join(root, "app.server.properties"), []byte(props), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := BundleDir(root)
	if err != nil {
		t.Fatalf("BundleDir: %v", err)
	}
	if got != customBundles {
		t.Errorf("BundleDir = %q, want %q", got, customBundles)
	}
}

func TestBundleDir_ResolvesProjectDirInterpolation(t *testing.T) {
	root := t.TempDir()
	props := "app.server.parent.dir=${project.dir}/build-output/bundles\n"
	if err := os.WriteFile(filepath.Join(root, "app.server.properties"), []byte(props), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := BundleDir(root)
	if err != nil {
		t.Fatalf("BundleDir: %v", err)
	}
	want := filepath.Join(root, "build-output", "bundles")
	if got != want {
		t.Errorf("BundleDir = %q, want %q", got, want)
	}
}

func TestFindTomcatDir_VersionOnly(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "portal")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	props := "app.server.tomcat.version=9.0.78\n"
	if err := os.WriteFile(filepath.Join(root, "app.server.properties"), []byte(props), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := FindTomcatDir(root)
	if err != nil {
		t.Fatalf("FindTomcatDir: %v", err)
	}
	want := filepath.Join(parent, "bundles", "tomcat-9.0.78")
	if got != want {
		t.Errorf("FindTomcatDir = %q, want %q", got, want)
	}
}

func TestFindTomcatDir_ExplicitDir(t *testing.T) {
	root := t.TempDir()
	props := "app.server.tomcat.dir=/opt/tomcat-explicit\n"
	if err := os.WriteFile(filepath.Join(root, "app.server.properties"), []byte(props), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := FindTomcatDir(root)
	if err != nil {
		t.Fatalf("FindTomcatDir: %v", err)
	}
	want := filepath.FromSlash("/opt/tomcat-explicit")
	if got != want {
		t.Errorf("FindTomcatDir = %q, want %q", got, want)
	}
}

func TestFindTomcatDir_MissingVersionAndDir(t *testing.T) {
	root := t.TempDir()
	// No app.server.properties at all.

	_, err := FindTomcatDir(root)
	if err == nil {
		t.Error("expected error when neither app.server.tomcat.version nor .dir is set")
	}
	if err != nil && !strings.Contains(err.Error(), "tomcat.version") {
		t.Errorf("error should mention 'tomcat.version', got: %v", err)
	}
}
