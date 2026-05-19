package zed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"5.3.9", "5.3.31", -1},   // numeric, not lexical
		{"5.3.31", "5.3.9", 1},    // mirror
		{"6.0.0", "5.99.99", 1},   // major dominates
		{"1.0", "1.0.0", -1}, // shorter compares as if missing segments are < anything
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		if got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCollectGradleCacheJars_PicksHighestVersion(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "caches", "modules-2", "files-2.1")

	mkJar := func(group, artifact, version, sha, fname string) {
		p := filepath.Join(root, group, artifact, version, sha)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, fname), []byte("jar"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mkJar("org.springframework", "spring-core", "5.3.9", "sha1", "spring-core-5.3.9.jar")
	mkJar("org.springframework", "spring-core", "5.3.31", "sha2", "spring-core-5.3.31.jar")
	mkJar("org.springframework", "spring-core", "6.0.0", "sha3", "spring-core-6.0.0.jar")
	// A version dir with only -sources should be ignored.
	mkJar("foo", "bar", "1.0", "sha4", "bar-1.0-sources.jar")
	// A pom-only version should be ignored.
	mkJar("baz", "qux", "2.0", "sha5", "qux-2.0.pom")

	jars, err := CollectGradleCacheJars(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(jars) != 1 {
		t.Fatalf("got %d jars, want 1: %v", len(jars), jars)
	}
	if !strings.HasSuffix(jars[0], "/spring-core/6.0.0/sha3/spring-core-6.0.0.jar") {
		t.Errorf("picked wrong jar: %s", jars[0])
	}
}

func TestCollectGradleCacheJars_MissingCache(t *testing.T) {
	dir := t.TempDir()
	jars, err := CollectGradleCacheJars(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing cache, got %v", err)
	}
	if len(jars) != 0 {
		t.Errorf("expected empty result, got %d jars", len(jars))
	}
}
