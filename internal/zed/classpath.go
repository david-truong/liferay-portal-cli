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

// Regenerate rewrites portalRoot/.classpath in place, replacing the source
// folder section with one source entry per discovered module source folder
// while leaving lib/con/output entries untouched.
//
// Returns the number of source entries written and any error.
func Regenerate(portalRoot string) (int, error) {
	classpathPath := filepath.Join(portalRoot, ".classpath")

	original, err := os.ReadFile(classpathPath)
	if err != nil {
		return 0, fmt.Errorf("read .classpath: %w", err)
	}

	parsed, err := parseClasspath(original)
	if err != nil {
		return 0, fmt.Errorf("parse .classpath: %w", err)
	}

	srcEntries, err := collectModuleSourceEntries(portalRoot)
	if err != nil {
		return 0, fmt.Errorf("collect module sources: %w", err)
	}

	// Merge: legacy src entries from the existing file (anything not under
	// modules/) keep their full original line so excludes/attrs are
	// preserved verbatim; newly discovered module src entries get a
	// canonical line. De-duplicate by path.
	merged := mergeSrcEntries(parsed.srcEntries, srcEntries)

	rebuilt := rebuildClasspath(parsed, merged)

	if bytes.Equal(rebuilt, original) {
		return len(merged), nil
	}
	if err := os.WriteFile(classpathPath, rebuilt, 0644); err != nil {
		return 0, fmt.Errorf("write .classpath: %w", err)
	}
	return len(merged), nil
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
// substituted for the original src section. Header / other / footer lines
// are written back verbatim.
func rebuildClasspath(p *parsedClasspath, srcs []srcEntry) []byte {
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
	for i, line := range p.footer {
		buf.WriteString(line)
		if i < len(p.footer)-1 {
			buf.WriteByte('\n')
		}
	}
	// Preserve trailing newline if the original had one.
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out
}
