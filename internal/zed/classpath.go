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
	SourceEntries int
	GradleJars    int
}

// Options controls Regenerate's behavior. The zero value is sensible:
// IncludeGradleCache=false keeps the operation to a pure source-folder
// rewrite (slice 1 behavior). Set IncludeGradleCache=true to also add
// Gradle dependency cache jars as <classpathentry kind="lib"> entries.
type Options struct {
	IncludeGradleCache bool
	GradleHome         string // defaults to $HOME/.gradle
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

	srcEntries, err := collectModuleSourceEntries(portalRoot)
	if err != nil {
		return Stats{}, fmt.Errorf("collect module sources: %w", err)
	}

	merged := mergeSrcEntries(parsed.srcEntries, srcEntries)

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
		jars, err := CollectGradleCacheJars(home)
		if err != nil {
			return Stats{}, fmt.Errorf("collect gradle cache jars: %w", err)
		}
		cacheLines = renderGradleCacheLibs(jars)
	}

	rebuilt := rebuildClasspath(parsed, merged, cacheLines)

	stats := Stats{
		SourceEntries: len(merged),
		GradleJars:    len(cacheLines) - countMarkers(cacheLines),
	}
	if bytes.Equal(rebuilt, original) {
		return stats, nil
	}
	if err := os.WriteFile(classpathPath, rebuilt, 0644); err != nil {
		return stats, fmt.Errorf("write .classpath: %w", err)
	}
	return stats, nil
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
// existing source subdirectory.
func collectModuleSourceEntries(portalRoot string) ([]srcEntry, error) {
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
