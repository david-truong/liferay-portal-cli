package zed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDepLine(t *testing.T) {
	cases := []struct {
		line string
		want DeclaredDep
		ok   bool
	}{
		{
			line: `	compileOnly group: "com.liferay.portal", name: "com.liferay.portal.kernel", version: "default"`,
			want: DeclaredDep{Group: "com.liferay.portal", Artifact: "com.liferay.portal.kernel", Version: "default"},
			ok:   true,
		},
		{
			// compileJspClasspathTransform isn't an included configuration.
			line: `	compileJspClasspathTransform group: "com.liferay", name: "com.liferay.gradle.plugins.jasper.jspc", transitive: false, version: "2.0.20"`,
			ok:   false,
		},
		{
			line: `	compileOnly project(":apps:foo:foo-api")`,
			ok:   false,
		},
		{
			line: `// comment with group: "x", name: "y"`,
			ok:   false, // no version, so fails
		},
		{
			// jspCClasspath isn't an included configuration anymore — skip.
			line: `	jspCClasspath group: "org.apache.tomcat", name: "tomcat-jasper", version: "10.1.55"`,
			ok:   false,
		},
		{
			// testImplementation isn't included either.
			line: `	testImplementation group: "junit", name: "junit", version: "4.13.1"`,
			ok:   false,
		},
		{
			line: `	compileInclude group: "com.liferay", name: "com.liferay.osgi.util", version: "8.1.5"`,
			want: DeclaredDep{Group: "com.liferay", Artifact: "com.liferay.osgi.util", Version: "8.1.5"},
			ok:   true,
		},
	}
	for _, c := range cases {
		got, ok := parseDepLine(c.line)
		if ok != c.ok {
			t.Errorf("parseDepLine(%q) ok=%v, want %v", c.line, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if got != c.want {
			t.Errorf("parseDepLine(%q) = %+v, want %+v", c.line, got, c.want)
		}
	}
}

func TestResolveDepsToJars_ExactVersionPreferred(t *testing.T) {
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
	mkJar("g", "a", "1.0", "s1", "a-1.0.jar")
	mkJar("g", "a", "2.0", "s2", "a-2.0.jar")

	// Exact requested version.
	jars, err := ResolveDepsToJars([]DeclaredDep{{Group: "g", Artifact: "a", Version: "1.0"}}, "", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jars) != 1 || !strings.HasSuffix(jars[0], "a-1.0.jar") {
		t.Errorf("exact version resolution wrong: %v", jars)
	}

	// "default" falls back to highest.
	jars, _ = ResolveDepsToJars([]DeclaredDep{{Group: "g", Artifact: "a", Version: "default"}}, "", dir, nil)
	if len(jars) != 1 || !strings.HasSuffix(jars[0], "a-2.0.jar") {
		t.Errorf("default resolution wrong: %v", jars)
	}

	// Variable interpolation falls back to highest.
	jars, _ = ResolveDepsToJars([]DeclaredDep{{Group: "g", Artifact: "a", Version: "${someVar}"}}, "", dir, nil)
	if len(jars) != 1 || !strings.HasSuffix(jars[0], "a-2.0.jar") {
		t.Errorf("variable resolution wrong: %v", jars)
	}

	// Missing artifact silently dropped, not an error.
	jars, _ = ResolveDepsToJars([]DeclaredDep{{Group: "g", Artifact: "missing", Version: "1.0"}}, "", dir, nil)
	if len(jars) != 0 {
		t.Errorf("expected missing artifact dropped, got: %v", jars)
	}

	// Skip set drops matching artifacts entirely.
	jars, _ = ResolveDepsToJars(
		[]DeclaredDep{{Group: "g", Artifact: "a", Version: "1.0"}},
		"",
		dir,
		map[string]bool{"a": true},
	)
	if len(jars) != 0 {
		t.Errorf("skipArtifacts ignored, got: %v", jars)
	}
}

func TestCollectDeclaredDeps_AcrossModules(t *testing.T) {
	dir := t.TempDir()
	mkModule := func(rel, gradle string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(full, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "bnd.bnd"), []byte("Bundle-SymbolicName: x\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "build.gradle"), []byte(gradle), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mkModule("modules/apps/foo/foo-api", `dependencies {
	compileOnly group: "org.osgi", name: "osgi.annotation", version: "8.0.1"
	compileOnly project(":apps:bar")
}`)
	mkModule("modules/apps/bar/bar-api", `dependencies {
	compileOnly group: "org.osgi", name: "osgi.annotation", version: "8.0.1"
	compileOnly group: "org.springframework", name: "spring-core", version: "6.2.18"
}`)
	mkModule("modules/util/excluded-util", `dependencies {
	compileOnly group: "should.not", name: "appear", version: "1.0"
}`)

	deps, err := CollectDeclaredDeps(dir, []string{"modules/util/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 {
		t.Errorf("got %d deps, want 2: %+v", len(deps), deps)
	}
	for _, d := range deps {
		if d.Group == "should.not" {
			t.Errorf("excluded module's dep leaked through: %+v", d)
		}
	}
}
