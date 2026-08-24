package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/david-truong/liferay-portal-cli/internal/review"
	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review [base-branch]",
	Short: "Review the current branch's diff against Brian Chan's code-review rules",
	Long: `Diffs HEAD against base-branch (default "master") and reviews it against a
vendored copy of Brian Chan's code-review rules using a local Ollama model,
printing the same {"chance", "violations"} verdict poll-prs.sh posts to PRs.

Standalone: no Claude Code, no Python, nothing beyond a running local Ollama
server with a model pulled.

LOCAL_REVIEW_MODEL and LOCAL_REVIEW_OLLAMA_HOST choose which model and where
to reach it, for testing a candidate model's recall before adopting it.

Examples:
  liferay review
  liferay review origin/master
  LOCAL_REVIEW_MODEL=qwen2.5-coder:32b liferay review`,
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

	cfg := review.Config{
		Host:         reviewEnv("LOCAL_REVIEW_OLLAMA_HOST", "http://localhost:11434"),
		Model:        reviewEnv("LOCAL_REVIEW_MODEL", "qwen3-coder:30b"),
		CheckoutDir:  portalRoot,
		MaxToolIters: 4,
		Timeout:      120 * time.Second,
	}

	result, err := review.Run(context.Background(), cfg, diff)
	if err != nil {
		return err
	}

	out, err := json.Marshal(result)
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func reviewEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
