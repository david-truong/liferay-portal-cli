package zed

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// excludedGroupPrefixes lists Gradle group names whose jars are never used
// by liferay-portal. Including them just bloats the jdtls workspace index
// and slows everything down. The user's Gradle cache typically holds these
// because other Gradle projects (Android, Kotlin tooling) share the same
// cache directory.
var excludedGroupPrefixes = []string{
	"androidx.",
	"com.android.",
	"com.android",
	"org.jetbrains.kotlin",
	"org.jetbrains.kotlinx",
	"com.google.testing.platform",
}

// CollectGradleCacheJars walks the Gradle dependency cache and returns one
// absolute jar path per (group, artifact) pair, picking the highest version
// when multiple are present. Source and javadoc jars are excluded, as are
// groups in excludedGroupPrefixes.
//
// The cache layout is:
//
//	<root>/<group>/<artifact>/<version>/<sha>/<artifact>-<version>.jar
//
// gradleHome is typically ~/.gradle; we read from <gradleHome>/caches/modules-2/files-2.1.
func CollectGradleCacheJars(gradleHome string) ([]string, error) {
	root := filepath.Join(gradleHome, "caches", "modules-2", "files-2.1")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	groupDirs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	// Keyed by "<group>/<artifact>" so we can keep just the highest version.
	type best struct {
		version string
		jarPath string
	}
	picks := make(map[string]best)

	for _, gd := range groupDirs {
		if !gd.IsDir() {
			continue
		}
		groupName := gd.Name()
		if isExcludedGroup(groupName) {
			continue
		}
		artifactDirs, err := os.ReadDir(filepath.Join(root, groupName))
		if err != nil {
			continue
		}
		for _, ad := range artifactDirs {
			if !ad.IsDir() {
				continue
			}
			artifactName := ad.Name()
			versionDirs, err := os.ReadDir(filepath.Join(root, groupName, artifactName))
			if err != nil {
				continue
			}
			for _, vd := range versionDirs {
				if !vd.IsDir() {
					continue
				}
				version := vd.Name()
				jarPath := findJarInVersionDir(filepath.Join(root, groupName, artifactName, version), artifactName, version)
				if jarPath == "" {
					continue
				}
				key := groupName + "/" + artifactName
				cur, ok := picks[key]
				if !ok || compareVersions(version, cur.version) > 0 {
					picks[key] = best{version: version, jarPath: jarPath}
				}
			}
		}
	}

	jars := make([]string, 0, len(picks))
	for _, b := range picks {
		jars = append(jars, b.jarPath)
	}
	sort.Strings(jars)
	return jars, nil
}

func isExcludedGroup(group string) bool {
	for _, prefix := range excludedGroupPrefixes {
		if group == prefix || strings.HasPrefix(group, prefix) {
			return true
		}
	}
	return false
}

// findJarInVersionDir scans the per-version directory (which contains one or
// more sha-named subdirs, each holding a single artifact). Returns the
// absolute path of the main jar — skipping sources / javadoc / non-jar
// artifacts. Returns "" if no suitable jar is present.
func findJarInVersionDir(versionDir, artifact, version string) string {
	shaDirs, err := os.ReadDir(versionDir)
	if err != nil {
		return ""
	}
	want := artifact + "-" + version + ".jar"
	for _, sd := range shaDirs {
		if !sd.IsDir() {
			continue
		}
		full := filepath.Join(versionDir, sd.Name(), want)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full
		}
	}
	return ""
}

// compareVersions returns -1, 0, or 1 like strings.Compare but treats
// numeric segments numerically so 5.3.9 < 5.3.31 (which lexical compare
// gets wrong). Non-numeric suffixes are compared lexically; a numeric
// segment sorts above a non-numeric one of equal prefix.
func compareVersions(a, b string) int {
	as := splitVersion(a)
	bs := splitVersion(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv string
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if c := compareSegment(av, bv); c != 0 {
			return c
		}
	}
	return 0
}

func splitVersion(v string) []string {
	out := []string{}
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range v {
		switch r {
		case '.', '-', '+', '_':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func compareSegment(a, b string) int {
	ai, aerr := strconv.Atoi(a)
	bi, berr := strconv.Atoi(b)
	aok := aerr == nil
	bok := berr == nil
	switch {
	case aok && bok:
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		default:
			return 0
		}
	case aok && !bok:
		return 1 // numeric > non-numeric (so 1.0 > 1.0-rc, matches semver)
	case !aok && bok:
		return -1
	default:
		return strings.Compare(a, b)
	}
}
