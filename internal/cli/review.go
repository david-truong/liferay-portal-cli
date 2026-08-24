package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review [base-branch]",
	Short: "Review the current branch's diff against Brian Chan's code-review rules",
	Long: `Diffs HEAD against base-branch (default "master") and reviews it with the
brian-review skill, printing the same {"chance", "violations"} verdict
poll-prs.sh posts to PRs.

By default the review runs on Sonnet via "claude --agent pr-review-monitor".
Set BRIAN_REVIEW_ENGINE=local to review with a local Ollama model instead
(LOCAL_REVIEW_MODEL, LOCAL_REVIEW_OLLAMA_HOST choose which one and where) —
see ~/.claude/skills/brian-review/test-fixtures/README.md before trusting a
candidate model's recall.

Examples:
  liferay review
  liferay review origin/master
  BRIAN_REVIEW_ENGINE=local LOCAL_REVIEW_MODEL=qwen3-coder:30b liferay review`,
	Args: cobra.MaximumNArgs(1),
	RunE: runReview,
}

func init() {
	rootCmd.AddCommand(reviewCmd)
}

func runReview(cmd *cobra.Command, args []string) error {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	baseBranch := "master"
	if len(args) > 0 {
		baseBranch = args[0]
	}

	diff, err := gitOutput("-C", portalRoot, "diff", baseBranch+"...HEAD")
	if err != nil {
		return fmt.Errorf("diffing against %s: %w", baseBranch, err)
	}
	if strings.TrimSpace(diff) == "" {
		fmt.Println("No changes against " + baseBranch + ".")
		return nil
	}

	diffFile, err := os.CreateTemp("", "liferay-review-*.diff")
	if err != nil {
		return err
	}
	defer os.Remove(diffFile.Name())
	if _, err := diffFile.WriteString(diff); err != nil {
		diffFile.Close()
		return err
	}
	if err := diffFile.Close(); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	skillDir := filepath.Join(home, ".claude", "skills", "brian-review")

	verdict, err := runReviewEngine(diffFile.Name(), portalRoot, skillDir)
	if err != nil {
		return err
	}

	fmt.Println(verdict)
	return nil
}

func runReviewEngine(diffPath, portalRoot, skillDir string) (string, error) {
	if os.Getenv("BRIAN_REVIEW_ENGINE") == "local" {
		return runLocalReview(diffPath, portalRoot, skillDir)
	}
	return runClaudeReview(diffPath, portalRoot, skillDir)
}

func runClaudeReview(diffPath, portalRoot, skillDir string) (string, error) {
	prompt := fmt.Sprintf(
		"Review the filtered PR diff at %s. Read the brian-review skill references, review the diff "+
			"against every rule. Your entire final message must be ONLY the raw JSON verdict object — "+
			"no aggregation notes, no prose, no code fence.",
		diffPath,
	)

	cmd := exec.Command("claude",
		"--add-dir", skillDir,
		"--add-dir", filepath.Dir(diffPath),
		"--agent", "pr-review-monitor",
		"--dangerously-skip-permissions",
		"--output-format", "json",
		"--print", prompt,
	)
	cmd.Dir = portalRoot
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running claude --agent pr-review-monitor: %w", err)
	}

	var response struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return "", fmt.Errorf("parsing claude's JSON output: %w", err)
	}
	if strings.TrimSpace(response.Result) == "" {
		return "", fmt.Errorf("claude returned no result")
	}
	return strings.TrimSpace(response.Result), nil
}

func runLocalReview(diffPath, portalRoot, skillDir string) (string, error) {
	script := filepath.Join(skillDir, "scripts", "local-review.py")

	cmd := exec.Command("python3", script, diffPath, portalRoot)
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running local-review.py: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
