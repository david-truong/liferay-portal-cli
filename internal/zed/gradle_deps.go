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
//
// We deliberately accept any leading configuration name (compileOnly,
// testImplementation, jspCClasspath, etc.) because the goal is to surface
// every jar jdtls might need on its classpath, not to model Gradle scopes.
var (
	gradleGroupRE   = regexp.MustCompile(`\bgroup:\s*"([^"]+)"`)
	gradleNameRE    = regexp.MustCompile(`\bname:\s*"([^"]+)"`)
	gradleVersionRE = regexp.MustCompile(`\bversion:\s*"([^"]+)"`)
)

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
// (zero, false) when any of the three is missing — including comment
// lines, project() refs, and partial declarations.
func parseDepLine(line string) (DeclaredDep, bool) {
	// Cheap early-out: every real dep line has all three attribute names.
	if !strings.Contains(line, "group:") || !strings.Contains(line, "name:") || !strings.Contains(line, "version:") {
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

// ResolveDepsToJars maps each DeclaredDep to a jar path in the Gradle
// cache. When the exact declared version is unavailable (e.g., it's
// "default" or a Gradle variable, or simply not yet cached), the highest
// cached version of that (group, artifact) is used as a fallback. Deps
// with no cached version at all are silently dropped — the caller can
// log the count if useful.
//
// Returns deduplicated, sorted absolute jar paths.
func ResolveDepsToJars(deps []DeclaredDep, gradleHome string) ([]string, error) {
	root := filepath.Join(gradleHome, "caches", "modules-2", "files-2.1")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	seen := make(map[string]bool)
	for _, d := range deps {
		if isExcludedGroup(d.Group) {
			continue
		}
		artifactDir := filepath.Join(root, d.Group, d.Artifact)
		jarPath := resolveDepJar(artifactDir, d.Artifact, d.Version)
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
