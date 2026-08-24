package review

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Dimension is one "## N. Title" section of dimensions.md, holding the raw
// markdown body that follows the heading.
type Dimension struct {
	Title   string
	Section string
}

var dimensionHeadingPattern = regexp.MustCompile(`(?m)^## \d+\. (.+)$`)

// ParseDimensions splits dimensions.md into its numbered sections.
func ParseDimensions(dimensionsMD string) []Dimension {
	titleMatches := dimensionHeadingPattern.FindAllStringSubmatch(dimensionsMD, -1)
	sections := dimensionHeadingPattern.Split(dimensionsMD, -1)
	if len(sections) > 0 {
		sections = sections[1:]
	}

	dimensions := make([]Dimension, 0, len(titleMatches))
	for i, m := range titleMatches {
		if i >= len(sections) {
			break
		}
		dimensions = append(dimensions, Dimension{Title: m[1], Section: sections[i]})
	}
	return dimensions
}

var (
	knownReferenceFiles = []string{"style.md", "mandates.md", "liferay-conventions.md", "format-rules.md"}
	rangePattern        = regexp.MustCompile("`rules/(\\d{3})-[\\w.*-]+`\\s+through\\s+`rules/(\\d{3})-[\\w.*-]+`")
	rulesSuffixPattern  = regexp.MustCompile("`rules/([\\w.*-]+)`")
)

// ResolveReferenceFiles resolves the backtick-quoted file references and
// "through" ranges in a dimension's "**Read:**" bullet into embedded-FS
// paths (e.g. "references/rules/401-avoid-switch-and-case-statements.md").
func ResolveReferenceFiles(sectionText string) ([]string, error) {
	files := map[string]struct{}{}

	for _, name := range knownReferenceFiles {
		if strings.Contains(sectionText, "`"+name+"`") {
			files["references/"+name] = struct{}{}
		}
	}

	for _, m := range rangePattern.FindAllStringSubmatch(sectionText, -1) {
		start, _ := strconv.Atoi(m[1])
		end, _ := strconv.Atoi(m[2])
		for num := start; num <= end; num++ {
			matches, err := fs.Glob(References, fmt.Sprintf("references/rules/%03d-*.md", num))
			if err != nil {
				return nil, err
			}
			for _, match := range matches {
				files[match] = struct{}{}
			}
		}
	}

	remaining := rangePattern.ReplaceAllString(sectionText, "")

	for _, m := range rulesSuffixPattern.FindAllStringSubmatch(remaining, -1) {
		matches, err := fs.Glob(References, "references/rules/"+m[1])
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			files[match] = struct{}{}
		}
	}

	result := make([]string, 0, len(files))
	for f := range files {
		result = append(result, f)
	}
	sort.Strings(result)
	return result, nil
}

var scopePattern = regexp.MustCompile(`\*\*Scope:\*\*\s*(.+)`)

// DimensionScope extracts the "**Scope:**" bullet's file extensions, or nil
// when the dimension isn't scoped to a particular language.
func DimensionScope(sectionText string) []string {
	m := scopePattern.FindStringSubmatch(sectionText)
	if m == nil {
		return nil
	}
	scopeText := m[1]
	if strings.Contains(scopeText, ".java") {
		return []string{".java"}
	}
	if strings.Contains(scopeText, ".ts") {
		return []string{".ts", ".tsx", ".js", ".jsx"}
	}
	return nil
}

var focusPattern = regexp.MustCompile(`\*\*Focus:\*\*\s*(.+)`)

// DimensionFocus extracts the "**Focus:**" bullet's text.
func DimensionFocus(sectionText string) string {
	m := focusPattern.FindStringSubmatch(sectionText)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}
