package zed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseClasspath_RoundTrip(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<classpath>
	<classpathentry kind="src" path="portal-kernel/src"/>
	<classpathentry kind="src" path="util-java/src"/>
	<classpathentry kind="lib" path="lib/development/foo.jar"/>
	<classpathentry kind="con" path="org.eclipse.jdt.launching.JRE_CONTAINER"/>
</classpath>
`
	parsed, err := parseClasspath([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := len(parsed.srcEntries); got != 2 {
		t.Errorf("srcEntries=%d, want 2", got)
	}
	if got := len(parsed.otherLines); got != 2 {
		t.Errorf("otherLines=%d, want 2", got)
	}

	rebuilt := rebuildClasspath(parsed, parsed.srcEntries, nil)
	if string(rebuilt) != input {
		t.Errorf("round-trip mismatch.\ngot:\n%s\nwant:\n%s", rebuilt, input)
	}
}

func TestParseClasspath_PreservesAttributes(t *testing.T) {
	// Real-world entry with excluding= before kind=.
	input := `<?xml version="1.0" encoding="UTF-8"?>
<classpath>
	<classpathentry excluding="**/.svn/**|.svn/" kind="src" path="portal-impl/src"/>
</classpath>
`
	parsed, err := parseClasspath([]byte(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.srcEntries) != 1 {
		t.Fatalf("srcEntries=%d, want 1", len(parsed.srcEntries))
	}
	if parsed.srcEntries[0].path != "portal-impl/src" {
		t.Errorf("path=%q, want portal-impl/src", parsed.srcEntries[0].path)
	}
	// The raw line should be preserved so excluding= survives unchanged.
	if !strings.Contains(parsed.srcEntries[0].line, `excluding="**/.svn/**|.svn/"`) {
		t.Errorf("excluding attribute lost: %s", parsed.srcEntries[0].line)
	}
}

func TestMergeSrcEntries_DiscoveredAdditive(t *testing.T) {
	existing := []srcEntry{{path: "portal-kernel/src", line: "existing-portal-kernel-line"}}
	discovered := []srcEntry{
		{path: "portal-kernel/src", line: "should-not-overwrite"},
		{path: "modules/apps/foo/src/main/java", line: "new-foo-line"},
	}
	merged := mergeSrcEntries(existing, discovered)
	if len(merged) != 2 {
		t.Fatalf("merged=%d, want 2", len(merged))
	}
	// Existing entry must win on path conflict.
	for _, e := range merged {
		if e.path == "portal-kernel/src" && e.line != "existing-portal-kernel-line" {
			t.Errorf("existing entry overwritten: %s", e.line)
		}
	}
	// Sorted output.
	if merged[0].path > merged[1].path {
		t.Errorf("merged not sorted: %v", merged)
	}
}

func TestRegenerate_AddsModuleSources(t *testing.T) {
	dir := t.TempDir()

	// Minimal portal layout: a root .classpath, one module with src/main/java.
	classpathOrig := `<?xml version="1.0" encoding="UTF-8"?>
<classpath>
	<classpathentry excluding="**/.svn/**|.svn/" kind="src" path="portal-kernel/src"/>
	<classpathentry kind="lib" path="lib/development/foo.jar"/>
	<classpathentry kind="con" path="org.eclipse.jdt.launching.JRE_CONTAINER"/>
</classpath>
`
	if err := os.WriteFile(filepath.Join(dir, ".classpath"), []byte(classpathOrig), 0644); err != nil {
		t.Fatal(err)
	}
	// Pretend-module under modules/apps/foo/foo-api with bnd.bnd + src/main/java.
	modulePath := filepath.Join(dir, "modules", "apps", "foo", "foo-api")
	if err := os.MkdirAll(filepath.Join(modulePath, "src", "main", "java"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(modulePath, "src", "main", "resources"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modulePath, "bnd.bnd"), []byte("Bundle-SymbolicName: foo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	stats, err := Regenerate(dir, Options{})
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if stats.SourceEntries != 3 {
		t.Errorf("SourceEntries=%d, want 3", stats.SourceEntries)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".classpath"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	for _, want := range []string{
		`path="portal-kernel/src"`,
		`path="modules/apps/foo/foo-api/src/main/java"`,
		`path="modules/apps/foo/foo-api/src/main/resources"`,
		`path="lib/development/foo.jar"`,
		`path="org.eclipse.jdt.launching.JRE_CONTAINER"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRegenerate_ExcludesPrefixes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".classpath"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<classpath>
	<classpathentry kind="src" path="modules/third-party/legacy-fork/src/main/java"/>
	<classpathentry kind="con" path="org.eclipse.jdt.launching.JRE_CONTAINER"/>
</classpath>
`), 0644); err != nil {
		t.Fatal(err)
	}
	// Create one keeper and one excluded module.
	for _, p := range []string{
		"modules/apps/foo/foo-api",
		"modules/third-party/some-vendor/vendor-impl",
		"modules/sdk/gradle-plugins-util",
	} {
		base := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Join(base, "src", "main", "java"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, "bnd.bnd"), []byte("Bundle-SymbolicName: x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := Regenerate(dir, Options{
		ExcludeModulePrefixes: []string{"modules/third-party/", "modules/sdk/"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, ".classpath"))
	out := string(got)
	if !strings.Contains(out, `modules/apps/foo/foo-api/src/main/java`) {
		t.Error("keeper module dropped")
	}
	for _, banned := range []string{
		`modules/third-party/legacy-fork`,        // pre-existing entry should be evicted
		`modules/third-party/some-vendor`,        // new discovery should be skipped
		`modules/sdk/gradle-plugins-util`,        // new discovery under sdk should be skipped
	} {
		if strings.Contains(out, banned) {
			t.Errorf("excluded path leaked through: %s", banned)
		}
	}
}

func TestRegenerate_NoChangeWhenAlreadyComplete(t *testing.T) {
	dir := t.TempDir()
	classpathOrig := `<?xml version="1.0" encoding="UTF-8"?>
<classpath>
	<classpathentry kind="con" path="org.eclipse.jdt.launching.JRE_CONTAINER"/>
</classpath>
`
	path := filepath.Join(dir, ".classpath")
	if err := os.WriteFile(path, []byte(classpathOrig), 0644); err != nil {
		t.Fatal(err)
	}
	infoBefore, _ := os.Stat(path)

	_, err := Regenerate(dir, Options{})
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	infoAfter, _ := os.Stat(path)
	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Errorf("file rewritten despite no changes (mtime before=%v after=%v)",
			infoBefore.ModTime(), infoAfter.ModTime())
	}
}
