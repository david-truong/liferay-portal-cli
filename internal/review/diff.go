package review

import (
	"regexp"
	"strings"
)

type diffChunk struct {
	Path  string
	Chunk string
}

var diffGitLinePattern = regexp.MustCompile(`(?m)^diff --git `)
var diffFileHeaderPattern = regexp.MustCompile(`^diff --git a/(\S+) b/(\S+)`)

// splitDiffByFile splits a unified diff into one chunk per "diff --git"
// section, keyed by the file's post-change ("b/...") path.
func splitDiffByFile(diffText string) []diffChunk {
	locs := diffGitLinePattern.FindAllStringIndex(diffText, -1)

	var result []diffChunk
	for i, loc := range locs {
		start := loc[0]
		end := len(diffText)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		chunk := diffText[start:end]

		m := diffFileHeaderPattern.FindStringSubmatch(chunk)
		if m == nil {
			continue
		}
		result = append(result, diffChunk{Path: m[2], Chunk: chunk})
	}
	return result
}

// filterDiff drops generated files (marked "@generated" or "@Generated(")
// and, when extensions is non-empty, any file not ending in one of them.
func filterDiff(diffText string, extensions []string) string {
	files := splitDiffByFile(diffText)

	generated := map[string]bool{}
	for _, f := range files {
		if strings.Contains(f.Chunk, "@generated") || strings.Contains(f.Chunk, "@Generated(") {
			generated[f.Path] = true
		}
	}

	var sb strings.Builder
	for _, f := range files {
		if generated[f.Path] {
			continue
		}
		if len(extensions) > 0 && !hasAnySuffix(f.Path, extensions) {
			continue
		}
		sb.WriteString(f.Chunk)
	}
	return sb.String()
}

func hasAnySuffix(s string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}
