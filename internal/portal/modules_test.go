package portal

import (
	"os"
	"path/filepath"
	"testing"
)

// buildFakePortal creates a minimal fake portal filesystem for testing.
func buildFakePortal(t *testing.T) string {
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

	// Real deployable module: has bnd.bnd
	touch("modules", "apps", "change-tracking", "change-tracking-web", "bnd.bnd")
	touch("modules", "apps", "change-tracking", "change-tracking-web", "build.gradle")

	// Group container: has build.gradle + app.bnd but NO bnd.bnd — must not be indexed
	touch("modules", "apps", "change-tracking", "app.bnd")
	touch("modules", "apps", "change-tracking", "build.gradle")

	// Fake .releng duplicate — must be filtered (starts with ".")
	touch("modules", ".releng", "apps", "change-tracking", "change-tracking-web", "bnd.bnd")

	// Playwright duplicate — must be filtered
	touch("modules", "test", "playwright", "change-tracking-web", "bnd.bnd")

	// Another real module for suggestion tests
	touch("modules", "apps", "blogs", "blogs-web", "bnd.bnd")

	// build.xml to satisfy FindRoot
	touch("build.xml")

	return root
}

func TestResolveExactMatch(t *testing.T) {
	root := buildFakePortal(t)
	idx, err := BuildModuleIndex(root)
	if err != nil {
		t.Fatalf("BuildModuleIndex: %v", err)
	}

	got, err := idx.Resolve("change-tracking-web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, "modules", "apps", "change-tracking", "change-tracking-web")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGroupContainerNotIndexed(t *testing.T) {
	root := buildFakePortal(t)
	idx, err := BuildModuleIndex(root)
	if err != nil {
		t.Fatalf("BuildModuleIndex: %v", err)
	}

	// "change-tracking" is a group container (app.bnd, no bnd.bnd): must not resolve
	_, err = idx.Resolve("change-tracking")
	if err == nil {
		t.Error("expected error for group container name, got nil")
	}
}

func TestRelengFiltered(t *testing.T) {
	root := buildFakePortal(t)
	idx, err := BuildModuleIndex(root)
	if err != nil {
		t.Fatalf("BuildModuleIndex: %v", err)
	}

	// Only the real module should be indexed; the .releng one is filtered
	got, err := idx.Resolve("change-tracking-web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, "modules", "apps", "change-tracking", "change-tracking-web")
	if got != want {
		t.Errorf("got %q (expected only real path), want %q", got, want)
	}
}

func TestPlaywrightFiltered(t *testing.T) {
	root := buildFakePortal(t)
	idx, err := BuildModuleIndex(root)
	if err != nil {
		t.Fatalf("BuildModuleIndex: %v", err)
	}

	// Playwright "change-tracking-web" must be filtered; only the real one remains
	got, err := idx.Resolve("change-tracking-web")
	if err != nil {
		t.Fatalf("unexpected ambiguity or miss: %v", err)
	}
	if filepath.Base(filepath.Dir(filepath.Dir(got))) == "test" {
		t.Errorf("playwright path leaked into index: %s", got)
	}
}

func TestMissingModuleSuggestion(t *testing.T) {
	root := buildFakePortal(t)
	idx, err := BuildModuleIndex(root)
	if err != nil {
		t.Fatalf("BuildModuleIndex: %v", err)
	}

	_, err = idx.Resolve("change-tracking-webbbb")
	if err == nil {
		t.Fatal("expected error for missing module")
	}
	// The suggestion should mention the real module
	msg := err.Error()
	if !contains(msg, "change-tracking-web") {
		t.Errorf("suggestion missing expected module name; got: %s", msg)
	}
}

func TestGroupSuffixDisambiguate(t *testing.T) {
	root := buildFakePortal(t)
	idx, err := BuildModuleIndex(root)
	if err != nil {
		t.Fatalf("BuildModuleIndex: %v", err)
	}

	got, err := idx.Resolve("change-tracking/change-tracking-web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, "modules", "apps", "change-tracking", "change-tracking-web")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
