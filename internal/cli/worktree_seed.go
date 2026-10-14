package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/david-truong/liferay-portal-cli/internal/state"
)

// seedGradlePropertiesRel is the Gradle properties file "ant all" writes. It
// holds absolute paths into the worktree it was generated for, plus the
// com.liferay.*.version entries "install-portal-snapshot" records, so it
// can't be cloned verbatim.
var seedGradlePropertiesRel = filepath.Join(".gradle", "gradle.properties")

// seedSkipPaths are git-ignored top-level entries that are not cloned from a
// built worktree: the bundle and Gradle state have dedicated handling, the
// rest is user-local config that "worktree add" already propagates or
// generates, or holds absolute paths into the source.
var seedSkipPaths = []string{
	".bundles",
	".claude",
	".gradle",
	".idea",
	"CLAUDE.local.md",
	"GEMINI.local.md",
	"build",
	"modules",
	"tools",
}

// seedExtraPaths are cloned from a built worktree although their top-level
// entry is in seedSkipPaths. tools/sdk/dist holds the module jars Gradle
// resolves project dependencies from, and .gradle/caches the downloaded
// third-party artifacts.
var seedExtraPaths = []string{
	filepath.Join(".gradle", "caches"),
	filepath.Join("tools", "sdk", "dist"),
}

// seedSkipGlobs are seedSkipPaths patterns, matched against the top-level
// entry name.
var seedSkipGlobs = []string{
	"app.server.*.properties",
	"build.*.properties",
	"release.*.properties",
	"test.*.properties",
}

// seedBundleExcludes are bundle-relative paths holding runtime state or
// per-worktree configuration that must not be inherited from the source.
var seedBundleExcludes = []string{
	"data",
	"osgi/configs",
	"osgi/modules",
	"osgi/state",
	"portal-setup-wizard.properties",
}

// seedBundleEmptyDirs are bundle-relative glob patterns of directories Tomcat
// expects to exist, so they are emptied rather than removed.
var seedBundleEmptyDirs = []string{
	"logs",
	"tomcat-*/logs",
	"tomcat-*/temp",
	"tomcat-*/work",
}

// seedSourceProblem returns why srcRoot can't seed a worktree whose HEAD is
// targetHead, or "" when it can. The source must be a Monorepo that was fully
// built ("ant all") at exactly targetHead and still sits there, with no
// uncommitted changes to tracked files that its build artifacts could carry.
func seedSourceProblem(goos string, srcType portal.ProjectType, rec state.BuildBase, hasRec bool, srcHead string, srcDirty bool, targetHead string) string {
	if goos != "darwin" && goos != "linux" {
		return "cloning bundles is only supported on macOS and Linux"
	}
	if srcType != portal.Monorepo {
		return "the source is not a Liferay monorepo"
	}
	if !hasRec {
		return "the source has no recorded build"
	}
	if rec.SHA != targetHead {
		return fmt.Sprintf("the source was built at %s, not %s", shortSHA(rec.SHA), shortSHA(targetHead))
	}
	if srcHead != rec.SHA {
		return fmt.Sprintf("the source has moved to %s since its build", shortSHA(srcHead))
	}
	if srcDirty {
		return "the source has uncommitted changes"
	}
	return ""
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// cloneCommand returns the command that clones src to dst using copy-on-write
// where the filesystem supports it. A src ending in "/." clones a directory's
// contents into the existing directory dst.
func cloneCommand(goos, src, dst string) *exec.Cmd {
	if goos == "darwin" {
		return exec.Command("cp", "-c", "-R", "-P", src, dst)
	}
	return exec.Command("cp", "--reflink=auto", "-R", "-P", src, dst)
}

// cloneEntry clones the file or directory src to dst with copy-on-write.
func cloneEntry(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		return runClone(cloneCommand(runtime.GOOS, src+"/.", dst), src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return runClone(cloneCommand(runtime.GOOS, src, dst), src)
}

func runClone(cmd *exec.Cmd, src string) error {
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cloning %s: %w\n%s", src, err, out)
	}
	return nil
}

// seedCloneEntries filters the git-ignored entries of a built worktree
// (relative paths, directories with a trailing slash) down to the ones to
// clone, dropping seedSkipPaths and seedSkipGlobs.
func seedCloneEntries(ignored []string) []string {
	var entries []string
	for _, entry := range ignored {
		top := strings.SplitN(strings.TrimSuffix(entry, "/"), "/", 2)[0]
		if top == "" || slices.Contains(seedSkipPaths, top) || matchesAnyGlob(seedSkipGlobs, top) {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func matchesAnyGlob(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if ok, _ := filepath.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

// listIgnoredEntries returns the git-ignored files and directories in root,
// with ignored directories collapsed to a single entry.
func listIgnoredEntries(root string) ([]string, error) {
	out, err := gitOutput("-C", root, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil, err
	}
	var entries []string
	for _, entry := range strings.Split(out, "\x00") {
		if entry = strings.TrimSpace(entry); entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// removeBundleRuntimeState deletes the seedBundleExcludes from a cloned
// bundle and empties its seedBundleEmptyDirs.
func removeBundleRuntimeState(bundleDir string) error {
	for _, rel := range seedBundleExcludes {
		if err := os.RemoveAll(filepath.Join(bundleDir, rel)); err != nil {
			return err
		}
	}
	for _, pattern := range seedBundleEmptyDirs {
		matches, err := filepath.Glob(filepath.Join(bundleDir, pattern))
		if err != nil {
			return err
		}
		for _, match := range matches {
			if err := os.RemoveAll(match); err != nil {
				return err
			}
			if err := os.MkdirAll(match, 0755); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyRewritingRoot copies the text file src to dst, replacing every
// occurrence of oldRoot with newRoot.
func copyRewritingRoot(src, dst, oldRoot, newRoot string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(strings.ReplaceAll(string(data), oldRoot, newRoot)), 0644)
}

// seedWorktree clones srcRoot's build artifacts into dstRoot. On failure it
// removes whatever it created so the caller can fall back to a full build.
func seedWorktree(srcRoot, dstRoot string) (err error) {
	srcBundle, err := portal.BundleDir(srcRoot)
	if err != nil {
		return err
	}
	dstBundle, err := portal.BundleDir(dstRoot)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(srcBundle, srcRoot+string(filepath.Separator)) ||
		!strings.HasPrefix(dstBundle, dstRoot+string(filepath.Separator)) {
		return fmt.Errorf("the bundle is not inside the worktree")
	}

	ignored, err := listIgnoredEntries(srcRoot)
	if err != nil {
		return err
	}
	entries := append(seedCloneEntries(ignored), seedExtraPaths...)

	cleanup := []string{dstBundle}
	defer func() {
		if err != nil {
			for _, path := range cleanup {
				_ = os.RemoveAll(path)
			}
		}
	}()

	// propagatePortalFiles already generated a portal-setup-wizard.properties
	// here; removing the directory keeps the clone a pure copy of the source.
	if err = os.RemoveAll(dstBundle); err != nil {
		return err
	}
	if err = cloneEntry(srcBundle, dstBundle); err != nil {
		return err
	}
	if err = removeBundleRuntimeState(dstBundle); err != nil {
		return err
	}

	for _, entry := range entries {
		dst := filepath.Join(dstRoot, entry)
		cleanup = append(cleanup, dst)
		if err = cloneEntry(filepath.Join(srcRoot, entry), dst); err != nil {
			return err
		}
	}
	return nil
}

// restoreGradleProperties rewrites dstRoot's .gradle/gradle.properties from
// srcRoot's, pointing its absolute paths at dstRoot. It runs after
// "ant setup-sdk", which regenerates the file and drops the
// com.liferay.*.version entries "install-portal-snapshot" recorded.
func restoreGradleProperties(srcRoot, dstRoot string) error {
	return copyRewritingRoot(
		filepath.Join(srcRoot, seedGradlePropertiesRel),
		filepath.Join(dstRoot, seedGradlePropertiesRel),
		srcRoot, dstRoot)
}

// seedProblem returns why srcRoot can't seed dstRoot, or "" when it can.
func seedProblem(srcRoot, dstRoot string) string {
	rec, hasRec, err := state.LoadBuildBase(srcRoot)
	if err != nil {
		hasRec = false
	}
	srcHead, _ := gitOutput("-C", srcRoot, "rev-parse", "HEAD")
	srcStatus, _ := gitOutput("-C", srcRoot, "status", "--porcelain", "--untracked-files=no")
	targetHead, _ := gitOutput("-C", dstRoot, "rev-parse", "HEAD")

	return seedSourceProblem(runtime.GOOS, portal.DetectProjectType(srcRoot), rec, hasRec,
		strings.TrimSpace(srcHead), strings.TrimSpace(srcStatus) != "", strings.TrimSpace(targetHead))
}

// findSeedSource returns the first worktree of dstRoot's repository that can
// seed it — one built ("ant all") at dstRoot's HEAD — or "" when none can. A
// worktree whose recorded build is at another commit is skipped before any
// git call, so scanning every worktree stays cheap.
func findSeedSource(dstRoot string) string {
	porcelain, err := gitOutput("-C", dstRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	targetHead, err := gitOutput("-C", dstRoot, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}

	for _, entry := range parseWorktreePorcelain(porcelain, "") {
		rec, ok, err := state.LoadBuildBase(entry.Path)
		if err != nil || !ok || rec.SHA != strings.TrimSpace(targetHead) {
			continue
		}
		if seedProblem(entry.Path, dstRoot) == "" {
			return entry.Path
		}
	}
	return ""
}

// trySeedWorktree seeds dstRoot from srcRoot when srcRoot qualifies, and
// reports whether it did. Otherwise it explains why and returns false so the
// caller runs a normal build.
func trySeedWorktree(srcRoot, dstRoot string) bool {
	if reason := seedProblem(srcRoot, dstRoot); reason != "" {
		fmt.Fprintf(os.Stderr, "\nNot seeding from %s: %s. Falling back to a full build.\n", srcRoot, reason)
		return false
	}

	fmt.Printf("\nSeeding build artifacts from %s ...\n", srcRoot)
	if err := seedWorktree(srcRoot, dstRoot); err != nil {
		fmt.Fprintf(os.Stderr, "\nNot seeding from %s: %v. Falling back to a full build.\n", srcRoot, err)
		return false
	}
	return true
}

// finishSeededWorktree completes a seeded worktree: it regenerates the setup
// wizard file the clone dropped, runs "ant setup-sdk", restores the Gradle
// properties it overwrites, and records the build base.
func finishSeededWorktree(primaryRoot, srcRoot, dstRoot string, projectType portal.ProjectType) error {
	ensureWorktreeFiles(primaryRoot, dstRoot, projectType)

	fmt.Println("\nRunning ant setup-sdk ...")
	if err := runAnt(dstRoot, dstRoot, "setup-sdk", "setup-sdk"); err != nil {
		return err
	}
	if err := restoreGradleProperties(srcRoot, dstRoot); err != nil {
		return err
	}
	recordBuildBase(dstRoot)

	fmt.Printf("\nSeeded. Next: liferay -C %s server up   (starts Tomcat + MySQL)\n", dstRoot)
	return nil
}
