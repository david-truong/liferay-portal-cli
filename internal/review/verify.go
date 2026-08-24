package review

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func resolveRuleText(rule string) (string, error) {
	m := ruleNumberPattern.FindStringSubmatch(rule)
	if m != nil {
		number, _ := strconv.Atoi(m[1])
		matches, err := fs.Glob(References, fmt.Sprintf("references/rules/%03d-*.md", number))
		if err != nil {
			return "", err
		}
		if len(matches) > 0 {
			return readReference(matches[0]), nil
		}
	}

	for _, name := range knownReferenceFiles {
		if strings.Contains(rule, name) {
			return readReference("references/" + name), nil
		}
	}
	return "", nil
}

func fileDiffChunk(diffText, path string) string {
	for _, f := range splitDiffByFile(diffText) {
		if f.Path == path {
			return f.Chunk
		}
	}
	return ""
}

var validPattern = regexp.MustCompile(`(?i)"valid"\s*:\s*(true|false)`)

func verifyFinding(ctx context.Context, host, model string, finding Finding, diffText string, timeout time.Duration) (bool, error) {
	ruleText, err := resolveRuleText(finding.Rule)
	if err != nil {
		return false, err
	}
	if ruleText == "" {
		ruleText = "(no rule file resolved — judge on the citation text alone)"
	}
	chunk := fileDiffChunk(diffText, finding.File)

	prompt := fmt.Sprintf(
		"Verify a single code-review finding for a Liferay Portal PR. Reject it unless it is a genuine "+
			"violation of the rule as written, against the actual diff below — reject a confirmation of "+
			"compliance, a rule misapplied to content it doesn't govern, or an inverted or illogical "+
			"conclusion.\n\n"+
			"Rule text:\n%s\n\n"+
			"Diff for %s:\n```diff\n%s\n```\n\n"+
			"Finding to verify: %s (cites: %s)\n\n"+
			"First explain your reasoning in one or two sentences, then on a final line write ONLY "+
			`{"valid": true|false}.`,
		ruleText, finding.File, chunk, finding.Description, finding.Rule,
	)

	messages := []ChatMessage{{Role: "user", Content: prompt}}
	message, err := chat(ctx, host, model, messages, []toolSchema{}, timeout)
	if err != nil {
		// Fail open: an unreachable/erroring Ollama shouldn't silently drop a real finding.
		return true, nil
	}

	matches := validPattern.FindAllStringSubmatch(message.Content, -1)
	if len(matches) == 0 {
		return true, nil
	}
	last := strings.ToLower(matches[len(matches)-1][1])
	return last == "true", nil
}
