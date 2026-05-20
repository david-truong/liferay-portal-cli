package zed

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
)

// gradleDepRE matches the three groovy attributes we care about on a single
// dependency declaration line — group, name, and version — in any order.
// Liferay's build.gradle files never wrap a single dep across lines, so a
// per-line regex is enough.
var (
	gradleGroupRE   = regexp.MustCompile(`\bgroup:\s*"([^"]+)"`)
	gradleNameRE    = regexp.MustCompile(`\bname:\s*"([^"]+)"`)
	gradleVersionRE = regexp.MustCompile(`\bversion:\s*"([^"]+)"`)
	gradleConfigRE  = regexp.MustCompile(`^\s*(\w+)\s+`)
)

// includedConfigurations are the Gradle configuration names whose
// declarations contribute jars that jdtls needs to resolve symbols in
// production code. Everything else (test*, jspC*, themeBuilder,
// targetPlatformBoms, ajc, etc.) bloats the workspace without helping
// cmd+click navigate from regular Java source.
var includedConfigurations = map[string]bool{
	"api":            true,
	"compile":        true,
	"compileInclude": true,
	"compileOnly":    true,
	"implementation": true,
	"provided":       true,
}

// DeclaredDep represents one external dependency declared by a module.
// Version may be "default" or a Gradle variable interpolation like
// "${someVersion}" — those need fallback resolution against whatever
// versions are present in the Gradle cache.
type DeclaredDep struct {
	Group    string
	Artifact string
	Version  string
}

// CollectDeclaredDeps walks every module under portalRoot (subject to the
// same exclude prefixes used for source folders), reads its build.gradle,
// and returns the deduplicated set of (group, artifact, version) tuples
// declared across all of them.
func CollectDeclaredDeps(portalRoot string, excludePrefixes []string) ([]DeclaredDep, error) {
	idx, err := portal.BuildModuleIndex(portalRoot)
	if err != nil {
		return nil, err
	}

	seen := make(map[DeclaredDep]bool)
	for _, modulePath := range idx.AllPaths() {
		rel, err := filepath.Rel(portalRoot, modulePath)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if hasAnyPrefix(rel+"/", excludePrefixes) {
			continue
		}
		gradlePath := filepath.Join(modulePath, "build.gradle")
		f, err := os.Open(gradlePath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024) // some build.gradle lines are long
		for scanner.Scan() {
			line := scanner.Text()
			if dep, ok := parseDepLine(line); ok {
				seen[dep] = true
			}
		}
		f.Close()
	}

	out := make([]DeclaredDep, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		if out[i].Artifact != out[j].Artifact {
			return out[i].Artifact < out[j].Artifact
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// parseDepLine extracts group/name/version from a single line. Returns
// (zero, false) when any of the three is missing, when the configuration
// name is not in includedConfigurations, or for non-dep lines (comments,
// project() refs, partial declarations).
func parseDepLine(line string) (DeclaredDep, bool) {
	// Cheap early-out: every real dep line has all three attribute names.
	if !strings.Contains(line, "group:") || !strings.Contains(line, "name:") || !strings.Contains(line, "version:") {
		return DeclaredDep{}, false
	}
	cfg := gradleConfigRE.FindStringSubmatch(line)
	if len(cfg) < 2 || !includedConfigurations[cfg[1]] {
		return DeclaredDep{}, false
	}
	g := gradleGroupRE.FindStringSubmatch(line)
	n := gradleNameRE.FindStringSubmatch(line)
	v := gradleVersionRE.FindStringSubmatch(line)
	if len(g) < 2 || len(n) < 2 || len(v) < 2 {
		return DeclaredDep{}, false
	}
	return DeclaredDep{Group: g[1], Artifact: n[1], Version: v[1]}, true
}

// ResolveDepsToJars maps each DeclaredDep to a jar path. Liferay's build
// resolves dependencies across multiple stores, so we walk them in order
// and accept the first hit per (group, artifact, version):
//
//  1. <portalRoot>/.gradle/caches/modules-2/files-2.1 — project-local
//     Gradle cache. This is where modules' `compileOnly group:...` deps
//     actually land; the global ~/.gradle/ cache only contains artifacts
//     pulled by non-portal Gradle projects on this machine.
//  2. <portalRoot>/.m2 — project-local Maven cache. Holds
//     com.liferay.portal.kernel-*-SNAPSHOT.jar after a portal build.
//  3. <portalRoot>/tools/sdk/dist — Liferay's own built dist jars for
//     sibling-module deps (populated by `ant all`).
//  4. <userHome>/.gradle/caches/modules-2/files-2.1 — global Gradle
//     cache, a fallback for anything not in (1).
//
// When the exact declared version is unavailable (e.g. "default" or a
// Gradle variable like ${someVar}), the highest cached version is used
// as a fallback. Deps with no cached version at all are silently
// dropped.
//
// skipArtifacts is a set of artifact names already present in the
// committed classpath (typically extracted from lib/*/<name>.jar). Any
// DeclaredDep whose Artifact appears in the set is dropped before
// resolution to prevent jdtls from seeing two versions of the same
// artifact.
//
// Returns deduplicated, sorted absolute jar paths.
func ResolveDepsToJars(deps []DeclaredDep, portalRoot, gradleHome string, skipArtifacts map[string]bool) ([]string, error) {
	gradleRoots := []string{
		filepath.Join(portalRoot, ".gradle", "caches", "modules-2", "files-2.1"),
		filepath.Join(gradleHome, "caches", "modules-2", "files-2.1"),
	}
	m2Root := filepath.Join(portalRoot, ".m2")
	sdkDist := filepath.Join(portalRoot, "tools", "sdk", "dist")

	seen := make(map[string]bool)
	for _, d := range deps {
		if isExcludedGroup(d.Group) {
			continue
		}
		if skipArtifacts[d.Artifact] {
			continue
		}
		jarPath := ""
		for _, root := range gradleRoots {
			if jar := resolveDepJar(filepath.Join(root, d.Group, d.Artifact), d.Artifact, d.Version); jar != "" {
				jarPath = jar
				break
			}
		}
		if jarPath == "" {
			jarPath = resolveMavenJar(m2Root, d.Group, d.Artifact, d.Version)
		}
		if jarPath == "" {
			jarPath = resolveSdkDistJar(sdkDist, d.Artifact)
		}
		if jarPath != "" {
			seen[jarPath] = true
		}
	}

	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// resolveMavenJar looks up an artifact in a Maven local-repo layout:
//
//	<root>/<group-with-slashes>/<artifact>/<version>/<artifact>-<version>.jar
//
// Liferay's project-local .m2 directory holds SNAPSHOT builds of
// portal-kernel and other portal-level artifacts that aren't in the
// Gradle cache.
func resolveMavenJar(m2Root, group, artifact, requestedVersion string) string {
	groupPath := strings.ReplaceAll(group, ".", string(filepath.Separator))
	artifactDir := filepath.Join(m2Root, groupPath, artifact)
	useFallback := requestedVersion == "" || requestedVersion == "default" || strings.Contains(requestedVersion, "${")
	if !useFallback {
		jar := filepath.Join(artifactDir, requestedVersion, artifact+"-"+requestedVersion+".jar")
		if info, err := os.Stat(jar); err == nil && !info.IsDir() {
			return jar
		}
	}
	versions, err := os.ReadDir(artifactDir)
	if err != nil {
		return ""
	}
	best := ""
	bestVersion := ""
	for _, vd := range versions {
		if !vd.IsDir() {
			continue
		}
		v := vd.Name()
		jar := filepath.Join(artifactDir, v, artifact+"-"+v+".jar")
		if info, err := os.Stat(jar); err != nil || info.IsDir() {
			continue
		}
		if best == "" || compareVersions(v, bestVersion) > 0 {
			best = jar
			bestVersion = v
		}
	}
	return best
}

// resolveSdkDistJar searches tools/sdk/dist for a jar whose name starts
// with the artifact name. Liferay's `ant all` populates this directory
// with versioned sibling-module jars like
// `com.liferay.osgi.util-8.1.5.jar`. We match by artifact prefix and
// pick lexically last (proxy for highest version).
func resolveSdkDistJar(distDir, artifact string) string {
	if artifact == "" {
		return ""
	}
	entries, err := os.ReadDir(distDir)
	if err != nil {
		return ""
	}
	prefix := artifact + "-"
	var best string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".jar") {
			continue
		}
		// Skip -sources/-javadoc.
		if strings.HasSuffix(name, "-sources.jar") || strings.HasSuffix(name, "-javadoc.jar") {
			continue
		}
		if name > best {
			best = name
		}
	}
	if best == "" {
		return ""
	}
	return filepath.Join(distDir, best)
}

// resolveDepJar tries the requested version first; if unavailable (or if
// the request is a Gradle variable like ${...} or the literal "default"),
// falls back to the highest cached version.
func resolveDepJar(artifactDir, artifact, requestedVersion string) string {
	useFallback := requestedVersion == "" || requestedVersion == "default" || strings.Contains(requestedVersion, "${")
	if !useFallback {
		if jar := findJarInVersionDir(filepath.Join(artifactDir, requestedVersion), artifact, requestedVersion); jar != "" {
			return jar
		}
	}
	versions, err := os.ReadDir(artifactDir)
	if err != nil {
		return ""
	}
	best := ""
	bestVersion := ""
	for _, vd := range versions {
		if !vd.IsDir() {
			continue
		}
		v := vd.Name()
		jar := findJarInVersionDir(filepath.Join(artifactDir, v), artifact, v)
		if jar == "" {
			continue
		}
		if best == "" || compareVersions(v, bestVersion) > 0 {
			best = jar
			bestVersion = v
		}
	}
	return best
}
