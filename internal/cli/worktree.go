package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/fsutil"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/david-truong/liferay-portal-cli/internal/state"
	"github.com/david-truong/liferay-portal-cli/internal/tomcat"
	"github.com/spf13/cobra"
)

// bundleSubdirName is the per-worktree subdirectory that holds the deployed
// Liferay bundle (Tomcat install, OSGi runtime, data). The dot prefix lets
// users add ".bundles/" to a global gitignore (core.excludesFile) and have
// it disappear from `git status` everywhere without touching the portal
// repo's tracked .gitignore.
const bundleSubdirName = ".bundles"

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
    app.server.<user>.properties              — points .bundles/ inside the worktree
    .bundles/portal-setup-wizard.properties   — skips setup wizard on first boot

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

var (
	worktreePruneYes    bool
	worktreePruneDryRun bool
)

var worktreePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove CLI state for worktrees that were deleted outside the CLI",
	Long: `Scans ~/.liferay-cli/worktrees/ for state directories whose git worktree
no longer exists (e.g. the worktree was "rm -rf"-ed instead of removed with
"liferay worktree remove"). Such orphaned state keeps claiming its slot and
can leave Docker containers and a Tomcat process running.

For each orphan, prune stops the slot's Docker stack and any stranded Tomcat
process, then removes the state directory — freeing the slot.

Detection is conservative: an orphan is only removed when its recorded
worktree path is gone, or (for older state with no recorded path) when the
directory's hash can be reproduced from a deleted path. State that cannot be
verified — for instance a live worktree belonging to another repository — is
left untouched.

Prints what it would do and asks for confirmation. Pass --dry-run to only
report, or --yes to skip the prompt.`,
	Args: cobra.NoArgs,
	RunE: runWorktreePrune,
}

var worktreeListJSON bool

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List git worktrees",
	RunE:  runWorktreeList,
}

func runWorktreeList(_ *cobra.Command, _ []string) error {
	if !worktreeListJSON {
		return gitRun("worktree", "list")
	}
	porcelain, err := gitOutput("worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	primary, _ := gitPrimaryRoot("")
	entries := parseWorktreePorcelain(porcelain, primary)
	return emitWorktreeListJSON(entries, os.Stdout)
}

// worktreeEntry is the stable JSON shape for `liferay worktree list --json`.
// Slot is -1 when the worktree has no persisted CLI state (i.e. liferay-cli
// has never been run there).
type worktreeEntry struct {
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Slot    int    `json:"slot"`
	Primary bool   `json:"primary"`
}

// parseWorktreePorcelain converts `git worktree list --porcelain` output
// into a slice of worktreeEntry. Slot is read from each worktree's
// persisted ports.json under ~/.liferay-cli/worktrees/<id>/docker/.
func parseWorktreePorcelain(porcelain, primaryRoot string) []worktreeEntry {
	blocks := strings.Split(strings.TrimSpace(porcelain), "\n\n")
	entries := make([]worktreeEntry, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		entry := worktreeEntry{Slot: -1}
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				entry.Path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "branch "):
				ref := strings.TrimPrefix(line, "branch ")
				entry.Branch = strings.TrimPrefix(ref, "refs/heads/")
			}
		}
		if entry.Path == "" {
			continue
		}
		entry.Slot = readPersistedSlot(entry.Path)
		entry.Primary = entry.Path == primaryRoot
		entries = append(entries, entry)
	}
	return entries
}

func readPersistedSlot(worktreePath string) int {
	data, err := os.ReadFile(filepath.Join(state.Dir(worktreePath), "docker", "ports.json"))
	if err != nil {
		return -1
	}
	var s struct {
		Slot int `json:"slot"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return -1
	}
	return s.Slot
}

func emitWorktreeListJSON(entries []worktreeEntry, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

var (
	worktreeSkipBuild bool
	worktreeRemoveYes bool
)

func init() {
	worktreeAddCmd.Flags().BoolVar(&worktreeSkipBuild, "skip-build", false, "Skip running 'liferay build' (ant all) after creating the worktree")
	worktreeRemoveCmd.Flags().BoolVar(&worktreeRemoveYes, "yes", false, "Skip the confirmation prompt. Required when stdin is not a TTY (or set LIFERAY_CLI_ASSUME_YES=1).")
	worktreeListCmd.Flags().BoolVar(&worktreeListJSON, "json", false, "Emit machine-readable JSON instead of git porcelain. Schema is stable: [{path, branch, slot, primary}].")
	worktreePruneCmd.Flags().BoolVar(&worktreePruneYes, "yes", false, "Skip the confirmation prompt. Required when stdin is not a TTY (or set LIFERAY_CLI_ASSUME_YES=1).")
	worktreePruneCmd.Flags().BoolVar(&worktreePruneDryRun, "dry-run", false, "Report orphaned state without stopping anything or deleting.")
	worktreeCmd.AddCommand(worktreeAddCmd, worktreeRemoveCmd, worktreeListCmd, worktreePruneCmd)
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

	projectType := portal.DetectProjectType(primaryRoot)
	if projectType == portal.ProjectTypeUnknown {
		fmt.Printf("Worktree created at %s (not a Liferay project: skipping file propagation)\n", absTarget)
		return nil
	}

	if err := propagatePortalFiles(primaryRoot, absTarget, projectType); err != nil {
		return err
	}

	if worktreeSkipBuild {
		fmt.Printf("\nNext: cd %s && liferay build   (populates the bundle)\n", absTarget)
		fmt.Println("Then: liferay server up   (starts Tomcat + MySQL)")
		return nil
	}

	if projectType == portal.Workspace {
		fmt.Println("\nRunning liferay build (initBundle + deploy) ...")
		return runWorkspaceBuildAll(absTarget)
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
func ensureWorktreeFiles(primaryRoot, worktreeRoot string, projectType portal.ProjectType) []fixAction {
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

	if projectType != portal.Monorepo {
		return results
	}

	username, err := portal.SafeUsername()
	if err != nil {
		return []fixAction{{name: "current-user", action: "failed", note: err.Error()}}
	}

	// app.server.<user>.properties
	appServerFile := fmt.Sprintf("app.server.%s.properties", username)
	appServerDst := filepath.Join(worktreeRoot, appServerFile)
	if fsutil.Exists(appServerDst) {
		results = append(results, fixAction{appServerFile, "skipped", "already exists — worktree will use existing server config"})
	} else {
		content := "app.server.parent.dir=${project.dir}/" + bundleSubdirName + "\n"
		if err := os.WriteFile(appServerDst, []byte(content), 0644); err != nil {
			results = append(results, fixAction{appServerFile, "failed", err.Error()})
		} else {
			results = append(results, fixAction{appServerFile, "generated", bundleSubdirName + "/ will be inside this worktree"})
		}
	}

	// <bundle>/portal-setup-wizard.properties
	setupWizardRel := filepath.Join(bundleSubdirName, "portal-setup-wizard.properties")
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
			"liferay.home=" + filepath.Join(worktreeRoot, bundleSubdirName) + "\n" +
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

func propagatePortalFiles(primaryRoot, worktreeRoot string, projectType portal.ProjectType) error {
	results := ensureWorktreeFiles(primaryRoot, worktreeRoot, projectType)

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
		fmt.Sprintf("This will remove the git worktree at %s along with its %s/ and CLI state directories.", absTarget, bundleSubdirName),
		assumeYes, in, out, isTTY,
	) {
		return ExitErr(ExitConfirmationDeclined,
			"worktree remove declined — pass --yes or set %s=1 to skip the prompt", AssumeYesEnvVar)
	}

	// git worktree remove refuses to remove the main worktree; keep that
	// guard, since deleting the directory ourselves below would not.
	if isPrimaryWorktree(absTarget) {
		return ExitErr(ExitGeneric,
			"refusing to remove the primary worktree at %s", absTarget)
	}

	stateDir := state.Dir(absTarget)

	// Stop the slot's Docker stack and Tomcat before deleting anything, so
	// the removal doesn't leak running containers or a Tomcat process.
	dockerState, hasSlot := docker.LoadState(absTarget)
	stopSlotRuntime(stateDir, absTarget, dockerState.Slot, hasSlot)

	// Delete the worktree directory outright rather than asking git to empty
	// it. "git worktree remove" refuses while the tree is dirty (the deployed
	// .bundles/ and build outputs always leave it so — the "Directory not
	// empty" failure), and "git clean -ffdx" gitignore-checks every file in
	// the monorepo, which takes minutes. A bulk remove plus "git worktree
	// prune" — which drops the entry once its directory is gone — is far
	// faster and needs no clean step.
	removeDir(absTarget)
	if err := gitRun("worktree", "prune"); err != nil {
		return err
	}

	removeDir(stateDir)
	return nil
}

// removeDir deletes dir if present, narrating the outcome.
func removeDir(dir string) {
	if !fsutil.Exists(dir) {
		return
	}
	fmt.Printf("Removing %s ... ", dir)
	if err := os.RemoveAll(dir); err != nil {
		fmt.Printf("error: %v\n", err)
	} else {
		fmt.Println("done")
	}
}

func runWorktreePrune(_ *cobra.Command, _ []string) error {
	claims, err := docker.ScanClaims(liveWorktreeParents())
	if err != nil {
		return err
	}

	var orphaned, unknown []docker.Claim
	for _, c := range claims {
		switch c.Status {
		case docker.ClaimOrphaned:
			orphaned = append(orphaned, c)
		case docker.ClaimUnknown:
			unknown = append(unknown, c)
		}
	}

	printPruneReport(orphaned, unknown, os.Stdout)

	if len(orphaned) == 0 {
		fmt.Println("Nothing to prune.")
		return nil
	}
	if worktreePruneDryRun {
		fmt.Println("\nDry run — nothing was stopped or removed.")
		return nil
	}

	if !confirmWithIO(
		fmt.Sprintf("This will stop any running Docker stack and Tomcat for the above %d orphan(s) and delete their CLI state directories.", len(orphaned)),
		worktreePruneYes, os.Stdin, os.Stdout, isStdinTTY(),
	) {
		return ExitErr(ExitConfirmationDeclined,
			"prune declined — pass --yes or set %s=1 to skip the prompt", AssumeYesEnvVar)
	}

	for _, c := range orphaned {
		fmt.Printf("\nPruning %s (slot %s) ...\n", state.DisplayHome(c.Dir), slotLabel(c))
		stopSlotRuntime(c.Dir, c.ResolvedPath, c.Slot, c.HasSlot)
		if err := os.RemoveAll(c.Dir); err != nil {
			fmt.Printf("  error removing state dir: %v\n", err)
		} else {
			fmt.Println("  removed state directory")
		}
	}
	return nil
}

// stopSlotRuntime stops a slot's Docker stack and Tomcat process before its
// state directory is removed. Shared by "prune" (worktree already gone) and
// "remove" (worktree still present). Tomcat is stopped by PID rather than
// catalina.sh so it works even when the bundle has been deleted; the PID
// must reference bundleDir, guarding against killing an unrelated process.
// hasSlot is false for state dirs that never ran a database.
func stopSlotRuntime(stateDirRoot, worktreePath string, slot int, hasSlot bool) {
	if worktreePath != "" {
		bundleDir := filepath.Join(worktreePath, bundleSubdirName)
		pidFile := filepath.Join(stateDirRoot, "tomcat.pid")
		if stopped, err := tomcat.ForceStop(pidFile, bundleDir); err != nil {
			fmt.Printf("  warning: %v\n", err)
		} else if stopped {
			fmt.Println("  stopped Tomcat process")
		}
	}
	if hasSlot {
		if err := docker.StopStack(filepath.Join(stateDirRoot, "docker"), slot); err != nil {
			fmt.Printf("  warning: could not stop Docker stack: %v\n", err)
		}
	}
}

func printPruneReport(orphaned, unknown []docker.Claim, out io.Writer) {
	if len(orphaned) > 0 {
		fmt.Fprintln(out, "Orphaned state (worktree deleted):")
		for _, c := range orphaned {
			fmt.Fprintf(out, "  %-40s slot %-7s %s\n",
				filepath.Base(c.Dir), slotLabel(c), state.DisplayHome(c.ResolvedPath))
		}
	}
	if len(unknown) > 0 {
		fmt.Fprintln(out, "\nUnverifiable — left untouched (may belong to another repo):")
		for _, c := range unknown {
			fmt.Fprintf(out, "  %-40s slot %s\n", filepath.Base(c.Dir), slotLabel(c))
		}
	}
}

// slotLabel renders the slot for display: the number, or "-" when the state
// dir holds no ports.json.
func slotLabel(c docker.Claim) string {
	if !c.HasSlot {
		return "-"
	}
	return fmt.Sprintf("%d", c.Slot)
}

// liveWorktreeParents returns the parent directories of the current repo's
// live worktrees, used to reconstruct paths for legacy state dirs. Returns nil
// when not inside a git repository — recorded-path orphans are still detected.
func liveWorktreeParents() []string {
	porcelain, err := gitOutput("worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}
	primary, _ := gitPrimaryRoot("")
	entries := parseWorktreePorcelain(porcelain, primary)
	seen := make(map[string]bool)
	var parents []string
	for _, e := range entries {
		parent := filepath.Dir(e.Path)
		if !seen[parent] {
			seen[parent] = true
			parents = append(parents, parent)
		}
	}
	return parents
}

// isPrimaryWorktree reports whether worktreeRoot is its repository's primary
// checkout (as opposed to a linked git worktree). Only the primary is allowed
// to claim slot 0; see docker.allocateFreshSlot. Returns false when the
// primary cannot be determined, so an indeterminate worktree never grabs the
// reserved slot.
func isPrimaryWorktree(worktreeRoot string) bool {
	primary, err := gitPrimaryRoot(worktreeRoot)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(worktreeRoot)
	if err != nil {
		return false
	}
	return abs == primary
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
