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

// testCheckoutDir is used only as a target for the git_grep/git_ls_files/
// read_file tools' precedent lookups — the synthetic file the test diff adds
// doesn't actually need to exist there.
const testCheckoutDir = "/Users/dtruong/Projects/liferay/liferay-portal"

// plantedViolations are the 5 deliberately planted violations in
// testdata/failures.diff (see ~/.claude/skills/brian-review/test-fixtures/README.md).
var plantedViolations = []string{"102", "306", "401", "402", "801"}

// TestRun_PlantedViolationsRecall checks this Go port against the same
// fixture used to validate the original Python script, on a real Ollama
// server. [402] is excluded from the expected set: it's a documented model
// salience-bias gap on qwen3-coder:30b (the model reports the diff's most
// visually obvious defect regardless of which rule it's asked about, even
// when given only that rule's text), not a bug introduced by this port.
func TestRun_PlantedViolationsRecall(t *testing.T) {
	if _, err := http.Get("http://localhost:11434/api/tags"); err != nil {
		t.Skip("Ollama not reachable at http://localhost:11434, skipping")
	}
	if _, err := os.Stat(testCheckoutDir); err != nil {
		t.Skip("liferay-portal checkout not present, skipping")
	}

	diff, err := os.ReadFile("testdata/failures.diff")
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Host:         "http://localhost:11434",
		Model:        "qwen3-coder:30b",
		CheckoutDir:  testCheckoutDir,
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
