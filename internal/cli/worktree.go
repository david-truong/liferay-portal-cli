package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/fsutil"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/david-truong/liferay-portal-cli/internal/state"
	"github.com/spf13/cobra"
)

var worktreeCmd = &cobra.Command{
	Use:     "worktree",
	Aliases: []string{"wt"},
	Short:   "Manage git worktrees for liferay-portal",
}

var worktreeAddCmd = &cobra.Command{
	Use:   "add <branch> <path>",
	Short: "Create a new worktree and propagate user-local files",
	Long: `Creates a git worktree and propagates files that git ignores but are
required for Liferay development:

  Symlinked (edits sync across worktrees):
    CLAUDE.md, GEMINI.md, .claude/, .gemini/, .idea/

  Copied (branch-specific, safe to diverge):
    build.*.properties, test.*.properties, release.*.properties, .env

  Generated:
    app.server.<user>.properties       — points bundles/ inside the worktree
    bundles/portal-setup-wizard.properties — skips setup wizard on first boot

By default, runs "ant all" after creating the worktree to populate the bundle
directory. Pass --skip-build to skip this step (you'll need to run
"liferay build" manually before "liferay server up").`,
	Args: cobra.ExactArgs(2),
	RunE: runWorktreeAdd,
}

var worktreeRemoveCmd = &cobra.Command{
	Use:   "remove <path>",
	Short: "Remove a worktree and its CLI state directory",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorktreeRemove,
}

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List git worktrees",
	RunE: func(cmd *cobra.Command, args []string) error {
		return gitRun("worktree", "list")
	},
}

var (
	worktreeSkipBuild bool
	worktreeRemoveYes bool
)

func init() {
	worktreeAddCmd.Flags().BoolVar(&worktreeSkipBuild, "skip-build", false, "Skip running 'liferay build' (ant all) after creating the worktree")
	worktreeRemoveCmd.Flags().BoolVar(&worktreeRemoveYes, "yes", false, "Skip the confirmation prompt. Required when stdin is not a TTY (or set LIFERAY_CLI_ASSUME_YES=1).")
	worktreeCmd.AddCommand(worktreeAddCmd, worktreeRemoveCmd, worktreeListCmd)
	rootCmd.AddCommand(worktreeCmd)
}

func runWorktreeAdd(cmd *cobra.Command, args []string) error {
	branch, targetPath := args[0], args[1]

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolving path %q: %w", targetPath, err)
	}

	// Find the primary checkout (the common git dir's worktree)
	primaryRoot, err := gitPrimaryRoot("")
	if err != nil {
		return err
	}

	// Create the worktree
	if err := gitRun("worktree", "add", absTarget, branch); err != nil {
		return err
	}

	// Only do portal-specific steps if this is a liferay-portal repo
	if !portal.IsPortalRepo(primaryRoot) {
		fmt.Printf("Worktree created at %s (non-portal repo: skipping file propagation)\n", absTarget)
		return nil
	}

	if err := propagatePortalFiles(primaryRoot, absTarget); err != nil {
		return err
	}

	if worktreeSkipBuild {
		fmt.Printf("\nNext: cd %s && ant all   (populates the bundle)\n", absTarget)
		fmt.Println("Then: liferay server up   (starts Tomcat + MySQL)")
		return nil
	}
	fmt.Println("\nRunning liferay build (ant all) ...")
	return runAntAll(absTarget)
}

// fixAction records what ensureWorktreeFiles did (or didn't do) for one file.
type fixAction struct {
	name   string
	action string // "linked", "copied", "generated", "skipped", "failed"
	note   string
}

// ensureWorktreeFiles is the shared engine behind both "liferay worktree add"
// and the auto-fix that runs on every command. It propagates user-local files
// from primaryRoot into worktreeRoot and generates per-worktree config that
// upstream git ignores. Idempotent — files that already exist get an action
// of "skipped".
func ensureWorktreeFiles(primaryRoot, worktreeRoot string) []fixAction {
	u, err := user.Current()
	if err != nil {
		return []fixAction{{name: "current-user", action: "failed", note: err.Error()}}
	}

	var results []fixAction

	// Symlink candidates (CLAUDE.md, .claude/, .idea/, etc.)
	for _, target := range collectSymlinkTargets(primaryRoot) {
		src := filepath.Join(primaryRoot, target)
		dst := filepath.Join(worktreeRoot, target)
		if fsutil.Exists(dst) {
			results = append(results, fixAction{target, "skipped", "already exists"})
			continue
		}
		action, note, err := fsutil.SymlinkOrCopy(src, dst)
		if err != nil {
			results = append(results, fixAction{target, "failed", err.Error()})
			continue
		}
		results = append(results, fixAction{target, action, note})
	}

	// Copy candidates: build.*.properties / test.*.properties / release.*.properties
	copyPatterns := []struct{ glob, tracked string }{
		{"build.*.properties", "build.properties"},
		{"test.*.properties", "test.properties"},
		{"release.*.properties", "release.properties"},
	}
	for _, pat := range copyPatterns {
		matches, _ := filepath.Glob(filepath.Join(primaryRoot, pat.glob))
		for _, src := range matches {
			base := filepath.Base(src)
			if base == pat.tracked {
				continue
			}
			results = append(results, copyIfMissing(base, src, filepath.Join(worktreeRoot, base)))
		}
	}

	// .env
	if envSrc := filepath.Join(primaryRoot, ".env"); fsutil.Exists(envSrc) {
		results = append(results, copyIfMissing(".env", envSrc, filepath.Join(worktreeRoot, ".env")))
	}

	// app.server.<user>.properties
	appServerFile := fmt.Sprintf("app.server.%s.properties", u.Username)
	appServerDst := filepath.Join(worktreeRoot, appServerFile)
	if fsutil.Exists(appServerDst) {
		results = append(results, fixAction{appServerFile, "skipped", "already exists — worktree will use existing server config"})
	} else {
		content := "app.server.parent.dir=${project.dir}/bundles\n"
		if err := os.WriteFile(appServerDst, []byte(content), 0644); err != nil {
			results = append(results, fixAction{appServerFile, "failed", err.Error()})
		} else {
			results = append(results, fixAction{appServerFile, "generated", "bundles/ will be inside this worktree"})
		}
	}

	// bundles/portal-setup-wizard.properties
	setupWizardRel := filepath.Join("bundles", "portal-setup-wizard.properties")
	setupWizardDst := filepath.Join(worktreeRoot, setupWizardRel)
	if fsutil.Exists(setupWizardDst) {
		results = append(results, fixAction{setupWizardRel, "skipped", "already exists"})
	} else if err := os.MkdirAll(filepath.Dir(setupWizardDst), 0755); err != nil {
		results = append(results, fixAction{setupWizardRel, "failed", err.Error()})
	} else {
		content := "admin.email.from.address=test@liferay.com\n" +
			"admin.email.from.name=Test Test\n" +
			"company.default.locale=en_US\n" +
			"company.default.time.zone=UTC\n" +
			"company.default.web.id=liferay.com\n" +
			"default.admin.email.address.prefix=test\n" +
			"liferay.home=" + filepath.Join(worktreeRoot, "bundles") + "\n" +
			"setup.wizard.enabled=false\n"
		if err := os.WriteFile(setupWizardDst, []byte(content), 0644); err != nil {
			results = append(results, fixAction{setupWizardRel, "failed", err.Error()})
		} else {
			results = append(results, fixAction{setupWizardRel, "generated", "skips setup wizard on first boot"})
		}
	}

	return results
}

func copyIfMissing(name, src, dst string) fixAction {
	if fsutil.Exists(dst) {
		return fixAction{name, "skipped", "already exists"}
	}
	if err := fsutil.CopyFile(src, dst); err != nil {
		return fixAction{name, "failed", err.Error()}
	}
	return fixAction{name, "copied", ""}
}

func propagatePortalFiles(primaryRoot, worktreeRoot string) error {
	results := ensureWorktreeFiles(primaryRoot, worktreeRoot)

	for _, r := range results {
		if r.action == "failed" {
			fmt.Fprintf(os.Stderr, "warning: could not propagate %s: %s\n", r.name, r.note)
		}
	}

	fmt.Printf("\nWorktree created at %s\n\n", worktreeRoot)
	maxName := 0
	for _, r := range results {
		if len(r.name) > maxName {
			maxName = len(r.name)
		}
	}
	for _, r := range results {
		line := fmt.Sprintf("  %-*s  %s", maxName, r.name, r.action)
		if r.note != "" {
			line += fmt.Sprintf("  (%s)", r.note)
		}
		fmt.Println(line)
	}
	return nil
}

func runWorktreeRemove(cmd *cobra.Command, args []string) error {
	absTarget, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	return removeWorktree(absTarget, worktreeRemoveYes, os.Stdin, os.Stdout, isStdinTTY())
}

// removeWorktree is the testable core of "liferay worktree remove". The
// confirmation gate runs first so a declined remove never invokes git or
// touches the bundle directory.
func removeWorktree(absTarget string, assumeYes bool, in io.Reader, out io.Writer, isTTY bool) error {
	if !confirmWithIO(
		fmt.Sprintf("This will remove the git worktree at %s along with its bundles/ and CLI state directories.", absTarget),
		assumeYes, in, out, isTTY,
	) {
		return ExitErr(ExitConfirmationDeclined,
			"worktree remove declined — pass --yes or set %s=1 to skip the prompt", AssumeYesEnvVar)
	}

	stateDir := state.Dir(absTarget)
	bundleDir := filepath.Join(absTarget, "bundles")

	if err := gitRun("worktree", "remove", absTarget); err != nil {
		return err
	}

	for _, dir := range []string{stateDir, bundleDir} {
		if fsutil.Exists(dir) {
			fmt.Printf("Removing %s ... ", dir)
			if err := os.RemoveAll(dir); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				fmt.Println("done")
			}
		}
	}
	return nil
}

// gitPrimaryRoot returns the primary worktree root (the directory whose
// .git is the common dir, not a "gitdir:" file). dir scopes the git
// invocation; pass "" to inherit the current process working directory.
func gitPrimaryRoot(dir string) (string, error) {
	args := []string{}
	if dir != "" {
		args = append(args, "-C", dir)
	}
	args = append(args, "rev-parse", "--git-common-dir")
	out, err := gitOutput(args...)
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	commonDir := strings.TrimSpace(out)
	if !filepath.IsAbs(commonDir) && dir != "" {
		commonDir = filepath.Join(dir, commonDir)
	}
	primaryRoot := filepath.Dir(commonDir)
	if abs, err := filepath.Abs(primaryRoot); err == nil {
		primaryRoot = abs
	}
	return primaryRoot, nil
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func gitRun(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func collectSymlinkTargets(root string) []string {
	candidates := []string{"CLAUDE.md", "GEMINI.md", ".claude", ".gemini", ".idea"}
	var result []string
	for _, c := range candidates {
		if fsutil.Exists(filepath.Join(root, c)) {
			result = append(result, c)
		}
	}
	return result
}
