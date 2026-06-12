package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
)

// addedFlagRE matches a feature-flag declaration added by a diff, e.g.
// "+	feature.flag.LPD-12345=false" in portal.properties.
var addedFlagRE = regexp.MustCompile(`(?m)^\+\s*feature\.flag\.([A-Za-z0-9]+-[0-9]+)=`)

// BranchFlags returns the feature flags the worktree's branch declares on
// top of master, by diffing portal.properties files against the merge base.
// Errors (no master ref, detached state, ...) yield nil — the dashboard
// then simply has no flags to manage for that worktree.
func BranchFlags(path string) []string {
	base, err := gitOutput(path, "merge-base", "master", "HEAD")
	if err != nil {
		return nil
	}

	diff, err := gitOutput(path, "diff", "--unified=0", strings.TrimSpace(base),
		"--", "*portal.properties")
	if err != nil {
		return nil
	}

	return parseAddedFlags(diff)
}

// parseAddedFlags extracts unique flag keys from added diff lines, in order
// of first appearance.
func parseAddedFlags(diff string) []string {
	var flags []string

	seen := map[string]bool{}
	for _, match := range addedFlagRE.FindAllStringSubmatch(diff, -1) {
		if seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		flags = append(flags, match[1])
	}
	return flags
}

func gitOutput(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return string(out), err
}

// flagLineRE matches one feature.flag assignment line; group 1 is the key,
// group 2 the value.
var flagLineRE = regexp.MustCompile(`^\s*feature\.flag\.([A-Za-z0-9]+-[0-9]+)\s*=\s*(.*?)\s*$`)

// flagsMarker introduces the lines this dashboard appends, so a reader of
// portal-ext.properties knows where they came from.
const flagsMarker = "## liferay-cli dashboard: feature flags added by this branch"

// EnsureFlagLines returns content with every flag set to true: an existing
// assignment is rewritten in place, missing ones are appended under a
// marker comment. The second result reports whether content changed.
func EnsureFlagLines(content string, flags []string) (string, bool) {
	missing := map[string]bool{}
	for _, flag := range flags {
		missing[flag] = true
	}

	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")

	changed := false
	for i, line := range lines {
		match := flagLineRE.FindStringSubmatch(line)
		if match == nil || !missing[match[1]] {
			continue
		}
		delete(missing, match[1])
		if match[2] != "true" {
			lines[i] = fmt.Sprintf("feature.flag.%s=true", match[1])
			changed = true
		}
	}

	if len(missing) > 0 {
		if content != "" {
			lines = append(lines, "")
		}
		lines = append(lines, flagsMarker)
		for _, flag := range flags {
			if missing[flag] {
				lines = append(lines, fmt.Sprintf("feature.flag.%s=true", flag))
			}
		}
		changed = true
	}

	if content == "" && len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	if !changed {
		return content, false
	}
	return strings.Join(lines, "\n") + "\n", true
}

// flagStates reports, for each flag, whether content currently enables it.
// The last assignment wins, matching how Liferay loads properties.
func flagStates(content string, flags []string) map[string]bool {
	states := make(map[string]bool, len(flags))
	for _, flag := range flags {
		states[flag] = false
	}

	for _, line := range strings.Split(content, "\n") {
		match := flagLineRE.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if _, tracked := states[match[1]]; tracked {
			states[match[1]] = match[2] == "true"
		}
	}
	return states
}

// portalExtPath resolves the bundle's portal-ext.properties for a worktree.
func portalExtPath(worktreePath string) (string, error) {
	bundleDir, err := portal.BundleDir(worktreePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(bundleDir, "portal-ext.properties"), nil
}

// enableBranchFlags rewrites portal-ext.properties so every branch flag is
// enabled. Returns the flags it had to flip, or nil when nothing changed.
func enableBranchFlags(w Worktree) ([]string, error) {
	if len(w.Flags) == 0 {
		return nil, nil
	}

	path, err := portalExtPath(w.Path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	content := string(data)

	var flipped []string
	for flag, enabled := range flagStates(content, w.Flags) {
		if !enabled {
			flipped = append(flipped, flag)
		}
	}

	updated, changed := EnsureFlagLines(content, w.Flags)
	if !changed {
		return nil, nil
	}
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return nil, err
	}
	return flipped, nil
}
