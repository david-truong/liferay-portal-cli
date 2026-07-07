package portal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrModuleNotFound is the sentinel wrapped by resolveFromMap's "no match"
// case so callers can distinguish it via errors.Is regardless of message
// text. The same index type serves modules and client extensions, so the
// sentinel name reflects the more common noun rather than one per index.
var ErrModuleNotFound = errors.New("module not found")

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

	// noun and qualifier customize error messages so the same index type can
	// serve both modules ("module" / "group/name") and client extensions
	// ("client extension" / "workspace/name").
	noun      string
	qualifier string
}

// BuildModuleIndex walks all module roots under portalRoot and returns an
// index. For a Monorepo root this is the fixed moduleRoots list; for a
// Workspace root it's the single configured modules directory
// (liferay.workspace.modules.dir in gradle.properties, default "modules").
func BuildModuleIndex(portalRoot string) (*ModuleIndex, error) {
	idx := &ModuleIndex{
		byName:    make(map[string][]string),
		bySuffix:  make(map[string][]string),
		noun:      "module",
		qualifier: "group/name",
	}

	roots := moduleRoots
	if DetectProjectType(portalRoot) == Workspace {
		roots = []string{workspaceModulesDir(portalRoot)}
	}

	for _, rel := range roots {
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

// workspaceModulesDir reads liferay.workspace.modules.dir from portalRoot's
// gradle.properties, defaulting to "modules" when unset.
func workspaceModulesDir(portalRoot string) string {
	props, _ := ReadProperties(filepath.Join(portalRoot, "gradle.properties"))
	dir := props["liferay.workspace.modules.dir"]
	if dir == "" {
		dir = "modules"
	}
	return dir
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

// AllPaths returns every discovered module's absolute path in sorted order.
// Useful for callers (e.g. classpath generation) that need to iterate the
// full set rather than look up a single name.
func (idx *ModuleIndex) AllPaths() []string {
	seen := make(map[string]bool)
	for _, paths := range idx.byName {
		for _, p := range paths {
			seen[p] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Names returns every resolvable module name (basename) in sorted order.
// Useful for shell completion, where a name maps to one or more paths.
func (idx *ModuleIndex) Names() []string {
	out := make([]string, 0, len(idx.byName))
	for name := range idx.byName {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Resolve returns the absolute path for the named module.
func (idx *ModuleIndex) Resolve(name string) (string, error) {
	if strings.Contains(name, "/") {
		return idx.resolveFromMap(name, idx.bySuffix, name)
	}
	return idx.resolveFromMap(name, idx.byName, idx.suggest(name, 3))
}

func (idx *ModuleIndex) resolveFromMap(name string, m map[string][]string, suggestionMsg string) (string, error) {
	paths := m[name]
	switch len(paths) {
	case 0:
		msg := fmt.Sprintf("no %s named %q\n\nDid you mean:\n%s", idx.noun, name, suggestionMsg)
		return "", fmt.Errorf("%s\n%w", strings.TrimRight(msg, "\n"), ErrModuleNotFound)
	case 1:
		return paths[0], nil
	default:
		return "", fmt.Errorf("ambiguous %s %q — use a %s qualifier:\n%s",
			idx.noun, name, idx.qualifier, formatPaths(paths))
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
