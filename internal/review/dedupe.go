package review

import (
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
)

var ruleNumberPattern = regexp.MustCompile(`\b(\d{1,3})\b`)

func ruleCitationIsValid(rule string) (bool, error) {
	m := ruleNumberPattern.FindStringSubmatch(rule)
	if m == nil {
		return true, nil
	}
	number, _ := strconv.Atoi(m[1])

	matches, err := fs.Glob(References, fmt.Sprintf("references/rules/%03d-*.md", number))
	if err != nil {
		return false, err
	}
	if len(matches) > 0 {
		return true, nil
	}

	// format-rules.md has its own independent 20-56 numbering ("Format Rule N"), separate
	// from the rules/NNN-*.md files.
	formatRulesText := readReference("references/format-rules.md")
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)^### Rule %d:`, number))
	return pattern.MatchString(formatRulesText), nil
}

var selfNegatingPhrases = []string{
	"without violating",
	"does not violate",
	"not a violation",
	"no violation",
}

// ruleNotApplicablePattern is scoped narrower than a blanket "does not apply" substring,
// which would also match a legitimate object-level finding like "the code does not apply
// StringUtil" — this only matches a meta-judgment about the cited rule's own applicability.
var ruleNotApplicablePattern = regexp.MustCompile(`rule[^.]*does not apply|does not apply to this`)

func dedupeKeyRule(rule string) string {
	m := ruleNumberPattern.FindStringSubmatch(rule)
	if m == nil {
		return rule
	}
	number, _ := strconv.Atoi(m[1])
	return fmt.Sprintf("%03d", number)
}

var methodNamePattern = regexp.MustCompile("`(\\w+)`")

func hasConfirmedSingleCaller(description, diffText string) bool {
	m := methodNamePattern.FindStringSubmatch(description)
	if m == nil {
		return true
	}
	occurrencePattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(m[1]) + `\s*\(`)

	// The declaration itself is one occurrence; rule 801 requires exactly one caller beyond
	// that. The checkout stays pinned to master (poll-prs.sh never checks out the PR branch),
	// so the diff is the only reliable caller evidence — "if a single caller cannot be
	// confirmed, do not flag."
	return len(occurrencePattern.FindAllString(diffText, -1)) == 2
}

type dedupeKey struct {
	File   string
	Rule   string
	Anchor string
}

func dedupeCandidates(findingsByDimension [][]map[string]interface{}, diffText string) ([]Finding, error) {
	seen := map[dedupeKey]bool{}
	var candidates []Finding

	for _, findings := range findingsByDimension {
		for _, raw := range findings {
			f := findingFromMap(raw)
			rule := strings.TrimSpace(f.Rule)

			valid, err := ruleCitationIsValid(rule)
			if err != nil {
				return nil, err
			}
			if !valid {
				continue
			}

			description := strings.TrimSpace(f.Description)
			descriptionLower := strings.ToLower(description)

			if containsAny(descriptionLower, selfNegatingPhrases) {
				continue
			}
			if ruleNotApplicablePattern.MatchString(descriptionLower) {
				continue
			}
			if dedupeKeyRule(rule) == "801" && !hasConfirmedSingleCaller(description, diffText) {
				continue
			}

			key := dedupeKey{File: f.File, Rule: dedupeKeyRule(rule), Anchor: f.Anchor}
			if seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, f)
		}
	}
	return candidates, nil
}

func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
