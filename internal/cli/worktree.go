package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/fsutil"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
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

The worktree's bundle directory is left empty (apart from the setup-wizard
properties); run "ant all" or "liferay build" to populate it before using
"liferay server up".`,
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

var worktreeBuild bool

func init() {
	worktreeAddCmd.Flags().BoolVar(&worktreeBuild, "build", false, "Run 'liferay build' (ant all) after creating the worktree")
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
	primaryRoot, err := gitPrimaryRoot()
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

	if worktreeBuild {
		fmt.Println("\nRunning liferay build (ant all) ...")
		return runAntAll(absTarget)
	}
	return nil
}

func propagatePortalFiles(primaryRoot, worktreeRoot string) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	type result struct {
		name   string
		action string
		note   string
	}
	var results []result

	// --- Symlink candidates ---
	symlinkTargets := collectSymlinkTargets(primaryRoot)
	for _, target := range symlinkTargets {
		src := filepath.Join(primaryRoot, target)
		if !fsutil.Exists(src) {
			continue
		}
		dst := filepath.Join(worktreeRoot, target)
		if fsutil.Exists(dst) {
			results = append(results, result{target, "skipped", "already exists"})
			continue
		}
		action, note, err := fsutil.SymlinkOrCopy(src, dst)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not propagate %s: %v\n", target, err)
			continue
		}
		results = append(results, result{target, action, note})
	}

	// --- Copy candidates ---
	copyPatterns := []struct {
		glob    string
		tracked string // filename of the tracked version to exclude
	}{
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
			dst := filepath.Join(worktreeRoot, base)
			if fsutil.Exists(dst) {
				results = append(results, result{base, "skipped", "already exists"})
				continue
			}
			if err := fsutil.CopyFile(src, dst); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not copy %s: %v\n", base, err)
				continue
			}
			results = append(results, result{base, "copied", ""})
		}
	}

	// .env at root
	if envSrc := filepath.Join(primaryRoot, ".env"); fsutil.Exists(envSrc) {
		envDst := filepath.Join(worktreeRoot, ".env")
		if fsutil.Exists(envDst) {
			results = append(results, result{".env", "skipped", "already exists"})
		} else if err := fsutil.CopyFile(envSrc, envDst); err == nil {
			results = append(results, result{".env", "copied", ""})
		}
	}

	// --- Generate app.server.<user>.properties ---
	appServerFile := fmt.Sprintf("app.server.%s.properties", u.Username)
	appServerDst := filepath.Join(worktreeRoot, appServerFile)
	if fsutil.Exists(appServerDst) {
		results = append(results, result{appServerFile, "skipped", "already exists — worktree will use existing server config"})
	} else {
		content := "app.server.parent.dir=${project.dir}/bundles\n"
		if err := os.WriteFile(appServerDst, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", appServerFile, err)
		} else {
			results = append(results, result{appServerFile, "generated", "bundles/ will be inside this worktree"})
		}
	}

	// --- Generate bundles/portal-setup-wizard.properties ---
	setupWizardRel := filepath.Join("bundles", "portal-setup-wizard.properties")
	setupWizardDst := filepath.Join(worktreeRoot, setupWizardRel)
	if fsutil.Exists(setupWizardDst) {
		results = append(results, result{setupWizardRel, "skipped", "already exists"})
	} else if err := os.MkdirAll(filepath.Dir(setupWizardDst), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create bundles/: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", setupWizardRel, err)
		} else {
			results = append(results, result{setupWizardRel, "generated", "skips setup wizard on first boot"})
		}
	}

	// Print summary
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
	fmt.Printf("\nNext: cd %s && ant all   (populates the bundle)\n", worktreeRoot)
	fmt.Println("Then: liferay server up   (starts Tomcat + MySQL)")
	return nil
}

func runWorktreeRemove(cmd *cobra.Command, args []string) error {
	absTarget, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}

	stateDir := filepath.Join(absTarget, ".liferay-cli")
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

func gitPrimaryRoot() (string, error) {
	// Find the common git dir (works inside worktrees too)
	out, err := gitOutput("rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	commonDir := strings.TrimSpace(out)
	// --git-common-dir returns the .git dir of the primary worktree
	// The primary root is its parent (unless it's a bare repo)
	primaryRoot := filepath.Dir(commonDir)
	// Canonicalise (resolve any symlinks)
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
