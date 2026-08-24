package review

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"
)

func readReference(p string) string {
	data, err := fs.ReadFile(References, p)
	if err != nil {
		return ""
	}
	return string(data)
}

func relToReferences(p string) string {
	return strings.TrimPrefix(p, "references/")
}

func runReviewLoop(ctx context.Context, host, model, checkoutDir, systemPrompt, userPrompt string, maxToolIters int, timeout time.Duration) ([]map[string]interface{}, error) {
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	for i := 0; i < maxToolIters; i++ {
		message, err := chat(ctx, host, model, messages, toolSchemas, timeout)
		if err != nil {
			return nil, err
		}
		if len(message.ToolCalls) == 0 {
			return extractJSONArray(message.Content), nil
		}

		messages = append(messages, message)
		for _, call := range message.ToolCalls {
			start := time.Now()
			result := runTool(checkoutDir, call.Function.Name, call.Function.Arguments)
			elapsed := time.Since(start)
			fmt.Fprintf(os.Stderr, "[tool] %s(%v) took %.1fs\n", call.Function.Name, call.Function.Arguments, elapsed.Seconds())
			messages = append(messages, ChatMessage{Role: "tool", Content: result})
		}
	}

	finalMessage, err := chat(ctx, host, model, messages, []toolSchema{}, timeout)
	if err != nil {
		return nil, err
	}
	return extractJSONArray(finalMessage.Content), nil
}

func reviewRuleFile(ctx context.Context, host, model, checkoutDir, dimensionTitle, focus, ruleFile, styleText, scopedDiff string, maxToolIters int, timeout time.Duration) ([]map[string]interface{}, error) {
	ruleText := readReference(ruleFile)
	systemPrompt := fmt.Sprintf(
		"You are one focused rule-check of a Liferay Portal PR code review. Judge the diff strictly "+
			"against the rule below and nothing else. The overriding rule (style.md) is: match the file's "+
			"existing convention over any abstract preference.\n\n# style.md\n\n%s\n\n# %s\n\n%s",
		styleText, path.Base(ruleFile), ruleText,
	)
	userPrompt := fmt.Sprintf(
		"## Dimension: %s\n%s\n\n"+
			"The diff below may contain several unrelated code-review issues. Ignore all of them — report "+
			"only a violation of the one rule given above, nothing else.\n\n"+
			"Use the git_grep / git_ls_files / read_file tools to check precedent before citing this rule, "+
			"if it depends on one. Return `high` confidence only when the rule's text matches exactly or a "+
			"named precedent contradicts the diff; everything else is `partial`.\n\n"+
			"Diff to review:\n```diff\n%s\n```\n\n"+
			"The array entry must describe something to FIX, never something the diff already does "+
			"correctly. Do not narrate compliance — if you catch yourself writing 'correctly follows', "+
			"'accurately reflects', 'clearly indicates', 'is followed', or any other confirmation that the "+
			"code already satisfies the rule, omit that entry entirely instead of adding it. If the rule "+
			"does not apply to this diff at all, output [] — never a finding that says so.\n\n"+
			"When done, reply with ONLY a JSON array (no prose, no code fence) in this shape: "+
			`[{"file": "<path>", "anchor": "<line text or @@ hunk>", "rule": "<citation>", `+
			`"confidence": "high|partial", "description": "<one line, backticks not double quotes>"}]. `+
			"Return [] if you find nothing.",
		dimensionTitle, focus, scopedDiff,
	)
	return runReviewLoop(ctx, host, model, checkoutDir, systemPrompt, userPrompt, maxToolIters, timeout)
}

func reviewDimensionBundled(ctx context.Context, host, model, checkoutDir, title, sectionText string, referenceFiles []string, scopedDiff string, maxToolIters int, timeout time.Duration) ([]map[string]interface{}, error) {
	refTextParts := make([]string, 0, len(referenceFiles))
	for _, p := range referenceFiles {
		refTextParts = append(refTextParts, fmt.Sprintf("# %s\n\n%s", relToReferences(p), readReference(p)))
	}
	referenceText := strings.Join(refTextParts, "\n\n")

	systemPrompt := "You are one dimension of a Liferay Portal PR code review. Judge the diff strictly against the " +
		"rules below and nothing else. The overriding rule (style.md) is: match the file's existing " +
		"convention over any abstract preference.\n\n" + referenceText

	userPrompt := fmt.Sprintf(
		"## Dimension: %s\n\n%s\n\n"+
			"Use the git_grep / git_ls_files / read_file tools to check precedent before citing a rule that "+
			"depends on it. Return `high` confidence only when the rule's text matches exactly or a named "+
			"precedent contradicts the diff; everything else is `partial`.\n\n"+
			"Diff to review:\n```diff\n%s\n```\n\n"+
			"Every array entry must describe something to FIX, never something the diff already does "+
			"correctly. Do not narrate compliance — if you catch yourself writing 'correctly follows', "+
			"'accurately reflects', 'clearly indicates', 'is followed', or any other confirmation that the "+
			"code already satisfies a rule, omit that entry entirely instead of adding it. Walking every rule "+
			"and reporting on each one, violated or not, is wrong; report only actual violations.\n\n"+
			"When done, reply with ONLY a JSON array (no prose, no code fence) in this shape: "+
			`[{"file": "<path>", "anchor": "<line text or @@ hunk>", "rule": "<citation>", `+
			`"confidence": "high|partial", "description": "<one line, backticks not double quotes>"}]. `+
			"Return [] if you find nothing.",
		title, sectionText, scopedDiff,
	)
	return runReviewLoop(ctx, host, model, checkoutDir, systemPrompt, userPrompt, maxToolIters, timeout)
}

func reviewDimensionNarrow(ctx context.Context, host, model, checkoutDir, title, focus string, ruleFiles []string, styleText, scopedDiff string, maxToolIters int, timeout time.Duration) ([]map[string]interface{}, error) {
	var findings []map[string]interface{}
	for _, ruleFile := range ruleFiles {
		result, err := reviewRuleFile(ctx, host, model, checkoutDir, title, focus, ruleFile, styleText, scopedDiff, maxToolIters, timeout)
		if err != nil {
			return nil, err
		}
		findings = append(findings, result...)
	}
	return findings, nil
}

// narrowSplitThreshold: dimensions bundling more rule files than this dilute
// attention across too many rules at once (measured: a 17-file bundle
// consistently missed rules it owns across 3 identical runs) — split those
// into one call per rule file instead. Below the threshold, bundling is
// faster with no measured recall cost.
const narrowSplitThreshold = 10

const styleReferencePath = "references/style.md"

func reviewDimension(ctx context.Context, host, model, checkoutDir string, dim Dimension, diffText string, maxToolIters int, timeout time.Duration) ([]map[string]interface{}, error) {
	scopedDiff := filterDiff(diffText, DimensionScope(dim.Section))
	if strings.TrimSpace(scopedDiff) == "" {
		return nil, nil
	}

	referenceFiles, err := ResolveReferenceFiles(dim.Section)
	if err != nil {
		return nil, err
	}

	var ruleFiles []string
	for _, f := range referenceFiles {
		if f != styleReferencePath {
			ruleFiles = append(ruleFiles, f)
		}
	}
	if len(ruleFiles) == 0 {
		return nil, nil
	}

	if len(ruleFiles) > narrowSplitThreshold {
		styleText := readReference(styleReferencePath)
		focus := DimensionFocus(dim.Section)
		return reviewDimensionNarrow(ctx, host, model, checkoutDir, dim.Title, focus, ruleFiles, styleText, scopedDiff, maxToolIters, timeout)
	}

	return reviewDimensionBundled(ctx, host, model, checkoutDir, dim.Title, dim.Section, referenceFiles, scopedDiff, maxToolIters, timeout)
}

// Config holds the parameters for a Run.
type Config struct {
	Host         string
	Model        string
	CheckoutDir  string
	MaxToolIters int
	Timeout      time.Duration
}

// Run reviews diffText against every dimension, verifies each surviving
// candidate, and returns the {"chance", "violations"} result.
func Run(ctx context.Context, cfg Config, diffText string) (Result, error) {
	if strings.TrimSpace(diffText) == "" {
		return Result{Chance: 0, Violations: nil}, nil
	}

	dimensions := ParseDimensions(readReference("references/dimensions.md"))
	if len(dimensions) == 0 {
		return Result{}, fmt.Errorf("unable to parse dimensions.md")
	}

	findingsByDimension := make([][]map[string]interface{}, 0, len(dimensions))
	for _, dim := range dimensions {
		findings, err := reviewDimension(ctx, cfg.Host, cfg.Model, cfg.CheckoutDir, dim, diffText, cfg.MaxToolIters, cfg.Timeout)
		if err != nil {
			return Result{}, fmt.Errorf("reviewing dimension %q: %w", dim.Title, err)
		}
		findingsByDimension = append(findingsByDimension, findings)
	}

	candidates, err := dedupeCandidates(findingsByDimension, diffText)
	if err != nil {
		return Result{}, err
	}

	var verified []Finding
	for _, candidate := range candidates {
		ok, err := verifyFinding(ctx, cfg.Host, cfg.Model, candidate, diffText, cfg.Timeout)
		if err != nil {
			return Result{}, err
		}
		if ok {
			verified = append(verified, candidate)
		}
	}

	return finalize(verified), nil
}
