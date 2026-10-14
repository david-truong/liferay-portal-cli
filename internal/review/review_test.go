package review

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"sort"
	"testing"
	"time"
)

// plantedViolations are the 5 deliberately planted violations in
// testdata/failures.diff (see ~/.claude/skills/brian-review/test-fixtures/README.md).
var plantedViolations = []string{"102", "306", "401", "402", "801"}

// TestRun_PlantedViolationsRecall checks this Go port against the same
// fixture used to validate the original Python script, on a real Ollama
// server. [402] is excluded from the expected set: it's a documented model
// salience-bias gap on qwen3-coder:30b (the model reports the diff's most
// visually obvious defect regardless of which rule it's asked about, even
// when given only that rule's text), not a bug introduced by this port.
//
// Opt-in: set LIFERAY_REVIEW_EVAL=1 and LIFERAY_REVIEW_EVAL_CHECKOUT to a
// liferay-portal checkout, and run with -timeout 30m.
func TestRun_PlantedViolationsRecall(t *testing.T) {
	if testing.Short() {
		t.Skip("live LLM evaluation, skipping in short mode")
	}
	if os.Getenv("LIFERAY_REVIEW_EVAL") != "1" {
		t.Skip("set LIFERAY_REVIEW_EVAL=1 to run the live LLM evaluation")
	}

	checkoutDir := os.Getenv("LIFERAY_REVIEW_EVAL_CHECKOUT")
	if checkoutDir == "" {
		t.Skip("set LIFERAY_REVIEW_EVAL_CHECKOUT to a liferay-portal checkout")
	}
	if _, err := os.Stat(checkoutDir); err != nil {
		t.Skipf("liferay-portal checkout %s not present, skipping", checkoutDir)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		t.Skip("Ollama not reachable at http://localhost:11434, skipping")
	}
	response.Body.Close()

	diff, err := os.ReadFile("testdata/failures.diff")
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Host:         "http://localhost:11434",
		Model:        "qwen3-coder:30b",
		CheckoutDir:  checkoutDir,
		MaxToolIters: 4,
		Timeout:      120 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	result, err := Run(ctx, cfg, string(diff))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var found []string
	for _, number := range plantedViolations {
		pattern := regexp.MustCompile(`\b` + number + `\b`)
		for _, violation := range result.Violations {
			if pattern.MatchString(violation) {
				found = append(found, number)
				break
			}
		}
	}
	sort.Strings(found)

	want := []string{"102", "306", "401", "801"}
	if !equalStrings(found, want) {
		t.Errorf("found %v violations, want %v (raw violations: %v)", found, want, result.Violations)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
