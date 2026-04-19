package portal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// moduleRoots are paths relative to the portal root that are walked for deployable modules.
var moduleRoots = []string{
	"modules/apps",
	"modules/dxp/apps",
	"modules/core",
	"modules/util",
	"modules/sdk",
	"modules/frontend-sdk",
	"modules/integrations",
	"modules/aspectj",
	"modules/suites",
	"modules/third-party",
	"modules/test",
}

var skipSegments = map[string]bool{
	"node_modules":       true,
	"node_modules_cache": true,
	".gradle":            true,
	".releng":            true,
	"_node-scripts":      true,
}

// ModuleIndex holds resolved module name → absolute path mappings.
type ModuleIndex struct {
	byName   map[string][]string // basename → paths
	bySuffix map[string][]string // "group/name" → paths
}

// BuildModuleIndex walks all module roots under portalRoot and returns an index.
func BuildModuleIndex(portalRoot string) (*ModuleIndex, error) {
	idx := &ModuleIndex{
		byName:   make(map[string][]string),
		bySuffix: make(map[string][]string),
	}
	for _, rel := range moduleRoots {
		root := filepath.Join(portalRoot, filepath.FromSlash(rel))
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		if err := walkModules(root, idx); err != nil {
			return nil, err
		}
	}
	return idx, nil
}

func walkModules(dir string, idx *ModuleIndex) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // skip unreadable dirs non-fatally
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if shouldSkip(name) {
			continue
		}
		absPath := filepath.Join(dir, name)
		if fileExists(filepath.Join(absPath, "bnd.bnd")) {
			addModule(idx, absPath)
		} else {
			if err := walkModules(absPath, idx); err != nil {
				return err
			}
		}
	}
	return nil
}

func addModule(idx *ModuleIndex, absPath string) {
	base := filepath.Base(absPath)
	idx.byName[base] = appendUnique(idx.byName[base], absPath)

	parent := filepath.Base(filepath.Dir(absPath))
	suffix := parent + "/" + base
	idx.bySuffix[suffix] = appendUnique(idx.bySuffix[suffix], absPath)
}

// Resolve returns the absolute path for the named module.
func (idx *ModuleIndex) Resolve(name string) (string, error) {
	if strings.Contains(name, "/") {
		return resolveFromMap(name, idx.bySuffix, name)
	}
	return resolveFromMap(name, idx.byName, idx.suggest(name, 3))
}

func resolveFromMap(name string, m map[string][]string, suggestionMsg string) (string, error) {
	paths := m[name]
	switch len(paths) {
	case 0:
		return "", fmt.Errorf("no module named %q\n\nDid you mean:\n%s", name, suggestionMsg)
	case 1:
		return paths[0], nil
	default:
		return "", fmt.Errorf("ambiguous module %q — use a group/name qualifier:\n%s",
			name, formatPaths(paths))
	}
}

func (idx *ModuleIndex) suggest(name string, n int) string {
	type entry struct {
		path string
		dist int
	}
	var candidates []entry
	for k, paths := range idx.byName {
		d := levenshtein(name, k)
		for _, p := range paths {
			candidates = append(candidates, entry{p, d})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].dist != candidates[j].dist {
			return candidates[i].dist < candidates[j].dist
		}
		return candidates[i].path < candidates[j].path
	})
	if len(candidates) > n {
		candidates = candidates[:n]
	}
	var sb strings.Builder
	for _, c := range candidates {
		fmt.Fprintf(&sb, "  %s\n", c.path)
	}
	return sb.String()
}

func shouldSkip(name string) bool {
	if skipSegments[name] {
		return true
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	if strings.HasPrefix(name, "playwright") {
		return true
	}
	return false
}

func formatPaths(paths []string) string {
	var sb strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&sb, "  %s\n", p)
	}
	return sb.String()
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1]
			} else {
				m := prev[j-1]
				if prev[j] < m {
					m = prev[j]
				}
				if curr[j-1] < m {
					m = curr[j-1]
				}
				curr[j] = 1 + m
			}
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
