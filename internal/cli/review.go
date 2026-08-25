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

var (
	reviewModel   string
	reviewHost    string
	reviewTimeout time.Duration
)

var reviewCmd = &cobra.Command{
	Use:   "review [base-branch]",
	Short: "Review the current branch's diff against Brian Chan's code-review rules",
	Long: `Diffs HEAD against base-branch (default "master") and reviews it against a
vendored copy of Brian Chan's code-review rules using a local Ollama model,
printing the same {"chance", "violations"} verdict poll-prs.sh posts to PRs.

Standalone: no Claude Code, no Python, nothing beyond a running local Ollama
server with a model pulled.

--model and --host choose which model and where to reach it, for testing a
candidate model's recall before adopting it (LOCAL_REVIEW_MODEL,
LOCAL_REVIEW_OLLAMA_HOST set the same via environment instead). --timeout
bounds each individual Ollama call — a large bundled dimension can
legitimately take a couple of minutes to prompt-eval on slower hardware or
a bigger model; raise it if you see "context deadline exceeded".

Examples:
  liferay review
  liferay review origin/master
  liferay review --model qwen2.5-coder:32b
  liferay review --timeout 10m`,
	Args: cobra.MaximumNArgs(1),
	RunE: runReview,
}

func init() {
	reviewCmd.Flags().StringVar(&reviewModel, "model", reviewEnv("LOCAL_REVIEW_MODEL", "qwen3-coder:30b"), "Ollama model to review with")
	reviewCmd.Flags().StringVar(&reviewHost, "host", reviewEnv("LOCAL_REVIEW_OLLAMA_HOST", "http://localhost:11434"), "Ollama server URL")
	reviewCmd.Flags().DurationVar(&reviewTimeout, "timeout", 30*time.Minute, "Timeout for each individual Ollama call")
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
		Host:         reviewHost,
		Model:        reviewModel,
		CheckoutDir:  portalRoot,
		MaxToolIters: 4,
		Timeout:      reviewTimeout,
	}

	start := time.Now()
	stopTicker := startElapsedTicker(start)
	result, err := review.Run(context.Background(), cfg, diff)
	stopTicker()
	fmt.Fprintf(os.Stderr, "[timer] review finished in %s\n", time.Since(start).Round(time.Second))

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

// startElapsedTicker prints an elapsed-time line to stderr every 30s so a
// long-running review (they've taken anywhere from a few minutes to over an
// hour) doesn't read as stalled. Returns a func to stop it.
func startElapsedTicker(start time.Time) func() {
	done := make(chan struct{})
	ticker := time.NewTicker(30 * time.Second)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "[timer] %s elapsed\n", time.Since(start).Round(time.Second))
			}
		}
	}()

	return func() { close(done) }
}

func reviewEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
