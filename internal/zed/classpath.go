// Package zed regenerates Eclipse project descriptors so jdtls (the language
// server backing Zed's Java support) can resolve cross-module symbols in the
// liferay-portal source tree.
//
// The portal root already commits a .classpath / .project pair recognized by
// Eclipse / jdtls, but only ~28 of the 1000+ modules are listed as source
// folders. Files under modules/apps/** fall through to jdtls's "invisible
// project" mode, which has only the JRE on the classpath — that's why
// cmd+click fails on most Liferay code.
//
// `liferay zed regen` reads the existing .classpath, preserves every
// non-source entry (lib, con, the legacy src entries already in the file),
// and adds `<classpathentry kind="src" ...>` lines for every module source
// folder it discovers under modules/{apps,dxp/apps,core,util,...}.
package zed

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
)

// moduleSourceSubdirs are the conventional source-folder paths inside a
// Liferay module that hold Java code or resources jdtls needs to index.
var moduleSourceSubdirs = []string{
	"src/main/java",
	"src/main/resources",
	"src/test/java",
	"src/test/resources",
	"src/testIntegration/java",
	"src/testIntegration/resources",
}

// classpathEntryRE matches a single self-closing <classpathentry .../> line,
// capturing the kind attribute and the path attribute. The file's existing
// entries put kind and path in varying orders, sometimes with excluding= in
// front; the regex tolerates that.
var (
	kindRE = regexp.MustCompile(`\bkind="([^"]+)"`)
	pathRE = regexp.MustCompile(`\bpath="([^"]+)"`)
)

// Stats is what Regenerate returns to the caller for logging.
type Stats struct {
	SourceEntries     int
	GradleJars        int
	SkipWorktreeAdded bool // true if we set skip-worktree on this run
}

// Options controls Regenerate's behavior. The zero value is sensible:
// IncludeGradleCache=false keeps the operation to a pure source-folder
// rewrite (slice 1 behavior). Set IncludeGradleCache=true to also add
// Gradle dependency cache jars as <classpathentry kind="lib"> entries.
type Options struct {
	IncludeGradleCache bool
	GradleHome         string // defaults to $HOME/.gradle

	// SkipWorktree, when true, runs `git update-index --skip-worktree
	// .classpath` after writing so git stops surfacing the file as
	// modified. The committed copy is unaffected; the local edit just
	// becomes invisible to status/diff. Off by default for callers that
	// don't want git side effects (e.g. tests).
	SkipWorktree bool

	// ExcludeModulePrefixes drops any module whose portal-relative path
	// starts with one of these prefixes (forward-slash). Used to keep the
	// jdtls workspace lean by skipping low-value module groups like
	// modules/third-party/ and modules/sdk/. Paths must use forward
	// slashes regardless of host OS.
	ExcludeModulePrefixes []string
}

// DefaultExcludeModulePrefixes are skipped by default because the modules
// under them are almost never useful to navigate from production code:
//
//   - modules/third-party/ holds vendored forks of upstream libraries;
//     real users almost always want the cache jar, not these sources.
//   - modules/sdk/ holds Liferay's own Gradle plugin sources used to
//     build the portal — build infra rather than runtime code.
//   - modules/util/ holds standalone utilities (data migration, dist
//     packaging, schema dumpers) that ship outside the OSGi container.
//   - modules/test/ holds portal-level test harnesses and fixtures, not
//     production code paths.
var DefaultExcludeModulePrefixes = []string{
	"modules/third-party/",
	"modules/sdk/",
	"modules/util/",
	"modules/test/",
}

// Regenerate rewrites portalRoot/.classpath in place, replacing the source
// folder section with one source entry per discovered module source folder
// while leaving the committed lib/con/output entries untouched. When
// opts.IncludeGradleCache is set, jars resolved from the Gradle dependency
// cache are appended as additional lib entries, marked with a sentinel
// comment so they can be cleaned up later.
func Regenerate(portalRoot string, opts Options) (Stats, error) {
	classpathPath := filepath.Join(portalRoot, ".classpath")

	original, err := os.ReadFile(classpathPath)
	if err != nil {
		return Stats{}, fmt.Errorf("read .classpath: %w", err)
	}

	parsed, err := parseClasspath(original)
	if err != nil {
		return Stats{}, fmt.Errorf("parse .classpath: %w", err)
	}

	// Drop previously-generated cache lib entries so re-runs don't accumulate.
	parsed.otherLines = stripGeneratedLibs(parsed.otherLines)

	srcEntries, err := collectModuleSourceEntries(portalRoot, opts.ExcludeModulePrefixes)
	if err != nil {
		return Stats{}, fmt.Errorf("collect module sources: %w", err)
	}

	// Also evict any existing entries (left over from a prior regen) that
	// fall under the current exclude list — otherwise excluded prefixes
	// would persist across runs forever.
	keptExisting := filterEntries(parsed.srcEntries, opts.ExcludeModulePrefixes)
	merged := mergeSrcEntries(keptExisting, srcEntries)

	var cacheLines []string
	if opts.IncludeGradleCache {
		home := opts.GradleHome
		if home == "" {
			h, err := os.UserHomeDir()
			if err != nil {
				return Stats{}, fmt.Errorf("resolve gradle home: %w", err)
			}
			home = filepath.Join(h, ".gradle")
		}
		deps, err := CollectDeclaredDeps(portalRoot, opts.ExcludeModulePrefixes)
		if err != nil {
			return Stats{}, fmt.Errorf("collect declared deps: %w", err)
		}
		jars, err := ResolveDepsToJars(deps, home)
		if err != nil {
			return Stats{}, fmt.Errorf("resolve declared deps: %w", err)
		}
		cacheLines = renderGradleCacheLibs(jars)
	}

	rebuilt := rebuildClasspath(parsed, merged, cacheLines)

	stats := Stats{
		SourceEntries: len(merged),
		GradleJars:    len(cacheLines) - countMarkers(cacheLines),
	}
	if !bytes.Equal(rebuilt, original) {
		if err := os.WriteFile(classpathPath, rebuilt, 0644); err != nil {
			return stats, fmt.Errorf("write .classpath: %w", err)
		}
	}

	if opts.SkipWorktree {
		added, err := ensureSkipWorktree(portalRoot, ".classpath")
		if err != nil {
			return stats, fmt.Errorf("set skip-worktree: %w", err)
		}
		stats.SkipWorktreeAdded = added
	}

	return stats, nil
}

// ensureSkipWorktree marks the file skip-worktree in git iff it isn't
// already. Returns true if this call did the marking, false if it was
// already set (or the file isn't tracked). Errors are bubbled up so the
// caller can decide whether to ignore them (e.g. in non-git directories).
func ensureSkipWorktree(repoDir, relPath string) (bool, error) {
	// `ls-files -v -- <path>` prints flags as the first character.
	// "S" means skip-worktree is set; "H" means tracked normally.
	out, err := runGit(repoDir, "ls-files", "-v", "--", relPath)
	if err != nil {
		return false, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		// Not tracked — nothing to do.
		return false, nil
	}
	if strings.HasPrefix(out, "S") {
		return false, nil
	}
	if _, err := runGit(repoDir, "update-index", "--skip-worktree", "--", relPath); err != nil {
		return false, err
	}
	return true, nil
}

// ClearSkipWorktree clears the skip-worktree bit on .classpath and
// restores it from HEAD. Used by `liferay zed reset` to give users a
// clean exit path back to the committed state.
func ClearSkipWorktree(portalRoot string) error {
	if _, err := runGit(portalRoot, "update-index", "--no-skip-worktree", "--", ".classpath"); err != nil {
		return fmt.Errorf("clear skip-worktree: %w", err)
	}
	if _, err := runGit(portalRoot, "checkout", "HEAD", "--", ".classpath"); err != nil {
		return fmt.Errorf("restore .classpath from HEAD: %w", err)
	}
	return nil
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// parsedClasspath holds the line-segmented decomposition of an existing
// .classpath file. Preserving entries as raw strings means the regenerated
// file diffs minimally against the committed copy.
type parsedClasspath struct {
	header     []string // lines before the first <classpathentry>
	srcEntries []srcEntry
	otherLines []string // lib, con, output, and anything else, in original order
	footer     []string // </classpath> + trailing
}

type srcEntry struct {
	path string // path attribute, used as the dedupe key and sort key
	line string // full raw line, indentation included
}

func parseClasspath(data []byte) (*parsedClasspath, error) {
	lines := strings.Split(string(data), "\n")
	p := &parsedClasspath{}

	state := "header"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "<classpathentry"):
			state = "entries"
			kindMatch := kindRE.FindStringSubmatch(line)
			pathMatch := pathRE.FindStringSubmatch(line)
			if len(kindMatch) < 2 || len(pathMatch) < 2 {
				// Unexpected shape; treat as non-src so we don't drop it.
				p.otherLines = append(p.otherLines, line)
				continue
			}
			if kindMatch[1] == "src" {
				p.srcEntries = append(p.srcEntries, srcEntry{
					path: pathMatch[1],
					line: line,
				})
			} else {
				p.otherLines = append(p.otherLines, line)
			}
		case strings.HasPrefix(trimmed, "</classpath>"):
			state = "footer"
			p.footer = append(p.footer, line)
		default:
			switch state {
			case "header":
				p.header = append(p.header, line)
			case "entries":
				// Blank line or comment between entries — keep with other.
				p.otherLines = append(p.otherLines, line)
			case "footer":
				p.footer = append(p.footer, line)
			}
		}
	}

	if len(p.footer) == 0 {
		return nil, fmt.Errorf(".classpath has no </classpath> closing tag")
	}
	return p, nil
}

// collectModuleSourceEntries walks every moduleRoot under portalRoot,
// identifies modules (folders with bnd.bnd), and returns one srcEntry per
// existing source subdirectory. Modules whose portal-relative path starts
// with any excludePrefix are skipped entirely.
func collectModuleSourceEntries(portalRoot string, excludePrefixes []string) ([]srcEntry, error) {
	idx, err := portal.BuildModuleIndex(portalRoot)
	if err != nil {
		return nil, err
	}

	var entries []srcEntry
	for _, modulePath := range idx.AllPaths() {
		rel, err := filepath.Rel(portalRoot, modulePath)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if hasAnyPrefix(rel+"/", excludePrefixes) {
			continue
		}
		for _, sub := range moduleSourceSubdirs {
			abs := filepath.Join(modulePath, filepath.FromSlash(sub))
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				path := rel + "/" + sub
				entries = append(entries, srcEntry{
					path: path,
					line: formatSrcEntry(path),
				})
			}
		}
	}
	return entries, nil
}

// formatSrcEntry produces a canonical <classpathentry kind="src"> line that
// matches the indentation and attribute style already used in the committed
// .classpath (tab indent, `excluding` before `kind` before `path`).
func formatSrcEntry(path string) string {
	return fmt.Sprintf(
		`	<classpathentry excluding="**/.svn/**|.svn/" kind="src" path="%s"/>`,
		path,
	)
}

// mergeSrcEntries returns the union of existing and discovered src entries,
// keyed by path. Existing entries win on conflict so the committed file's
// custom excludes are preserved verbatim.
func mergeSrcEntries(existing, discovered []srcEntry) []srcEntry {
	seen := make(map[string]srcEntry, len(existing)+len(discovered))
	for _, e := range existing {
		seen[e.path] = e
	}
	for _, e := range discovered {
		if _, ok := seen[e.path]; !ok {
			seen[e.path] = e
		}
	}
	merged := make([]srcEntry, 0, len(seen))
	for _, e := range seen {
		merged = append(merged, e)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].path < merged[j].path
	})
	return merged
}

// rebuildClasspath serializes the parsedClasspath with merged src entries
// substituted for the original src section, then appends any extra lines
// (e.g. Gradle cache lib entries) just before the closing </classpath>.
// Header / other / footer lines are written back verbatim.
func rebuildClasspath(p *parsedClasspath, srcs []srcEntry, extras []string) []byte {
	var buf bytes.Buffer
	for _, line := range p.header {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	for _, e := range srcs {
		buf.WriteString(e.line)
		buf.WriteByte('\n')
	}
	for _, line := range p.otherLines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	for _, line := range extras {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	for i, line := range p.footer {
		buf.WriteString(line)
		if i < len(p.footer)-1 {
			buf.WriteByte('\n')
		}
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out
}

// generatedLibMarker brackets the Gradle-cache lib entries so re-runs can
// identify and replace the previously-generated block without touching the
// hand-maintained lib entries committed to the .classpath.
const (
	generatedLibStart = `	<!-- BEGIN liferay-zed-cache (do not edit; regenerated by `+ "`liferay zed regen`" +`) -->`
	generatedLibEnd   = `	<!-- END liferay-zed-cache -->`
)

// renderGradleCacheLibs wraps the jar paths in a <classpathentry kind="lib">
// line plus the sentinel start/end comments.
func renderGradleCacheLibs(jars []string) []string {
	if len(jars) == 0 {
		return nil
	}
	lines := make([]string, 0, len(jars)+2)
	lines = append(lines, generatedLibStart)
	for _, jar := range jars {
		lines = append(lines, fmt.Sprintf(`	<classpathentry kind="lib" path="%s"/>`, jar))
	}
	lines = append(lines, generatedLibEnd)
	return lines
}

// stripGeneratedLibs removes any block previously emitted by renderGradleCacheLibs.
// Used to keep re-runs idempotent — we replace the whole block rather than
// merging line-by-line.
func stripGeneratedLibs(lines []string) []string {
	out := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.Contains(trimmed, "BEGIN liferay-zed-cache"):
			skipping = true
		case strings.Contains(trimmed, "END liferay-zed-cache"):
			skipping = false
		case !skipping:
			out = append(out, line)
		}
	}
	return out
}

// filterEntries returns the subset of entries whose path doesn't fall
// under any of the given exclude prefixes.
func filterEntries(entries []srcEntry, excludePrefixes []string) []srcEntry {
	if len(excludePrefixes) == 0 {
		return entries
	}
	out := make([]srcEntry, 0, len(entries))
	for _, e := range entries {
		if hasAnyPrefix(e.path+"/", excludePrefixes) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// hasAnyPrefix reports whether path starts with any of the given prefixes.
// Used to filter excluded module trees by their portal-relative path.
func hasAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func countMarkers(lines []string) int {
	n := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!--") {
			n++
		}
	}
	return n
}
