package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/david-truong/liferay-portal-cli/internal/state"
)

func TestSeedSourceProblem(t *testing.T) {
	const head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const other = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	built := state.BuildBase{SHA: head}

	tests := []struct {
		name       string
		goos       string
		srcType    portal.ProjectType
		rec        state.BuildBase
		hasRec     bool
		srcHead    string
		srcDirty   bool
		targetHead string
		wantSubstr string
	}{
		{"matching build", "darwin", portal.Monorepo, built, true, head, false, head, ""},
		{"linux is supported", "linux", portal.Monorepo, built, true, head, false, head, ""},
		{"windows is not", "windows", portal.Monorepo, built, true, head, false, head, "macOS and Linux"},
		{"workspace source", "darwin", portal.Workspace, built, true, head, false, head, "not a Liferay monorepo"},
		{"no recorded build", "darwin", portal.Monorepo, state.BuildBase{}, false, head, false, head, "no recorded build"},
		{"built at another commit", "darwin", portal.Monorepo, state.BuildBase{SHA: other}, true, other, false, head, "was built at bbbbbbbbbbbb"},
		{"source moved since its build", "darwin", portal.Monorepo, built, true, other, false, head, "has moved to bbbbbbbbbbbb"},
		{"source has uncommitted changes", "darwin", portal.Monorepo, built, true, head, true, head, "uncommitted changes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := seedSourceProblem(tt.goos, tt.srcType, tt.rec, tt.hasRec, tt.srcHead, tt.srcDirty, tt.targetHead)
			if tt.wantSubstr == "" {
				if got != "" {
					t.Errorf("got problem %q, want none", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("got %q, want it to contain %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestSeedCloneEntries(t *testing.T) {
	ignored := []string{
		".bundles/",
		".gradle/",
		".m2/",
		"CLAUDE.local.md",
		"app.server.dtruong.properties",
		"build.dtruong.properties",
		"build/",
		"lib/development/guava.jar",
		"modules/apps/blogs/blogs-api/build/",
		"portal-impl/classes/",
		"tmp/",
		"tools/gradle-8.5-bin.zip",
		"tools/sdk/dist/",
	}

	got := seedCloneEntries(ignored)
	want := []string{
		".m2/",
		"lib/development/guava.jar",
		"portal-impl/classes/",
		"tmp/",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCloneCommand(t *testing.T) {
	darwin := cloneCommand("darwin", "/src/.", "/dst").Args
	if want := []string{"cp", "-c", "-R", "-P", "/src/.", "/dst"}; !reflect.DeepEqual(darwin, want) {
		t.Errorf("darwin: got %v, want %v", darwin, want)
	}

	linux := cloneCommand("linux", "/src/.", "/dst").Args
	if want := []string{"cp", "--reflink=auto", "-R", "-P", "/src/.", "/dst"}; !reflect.DeepEqual(linux, want) {
		t.Errorf("linux: got %v, want %v", linux, want)
	}
}

func TestRemoveBundleRuntimeState(t *testing.T) {
	bundle := t.TempDir()
	for _, rel := range []string{
		"data/hypersonic/lportal.script",
		"logs/liferay.log",
		"osgi/configs/a.config",
		"osgi/modules/omni.admin.jar",
		"osgi/portal/keep.jar",
		"osgi/state/bundle.info",
		"portal-setup-wizard.properties",
		"tomcat-10.1.59/conf/server.xml",
		"tomcat-10.1.59/logs/catalina.out",
		"tomcat-10.1.59/temp/x",
		"tomcat-10.1.59/work/Catalina/y",
	} {
		path := filepath.Join(bundle, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeBundleRuntimeState(bundle); err != nil {
		t.Fatalf("removeBundleRuntimeState: %v", err)
	}

	for _, rel := range []string{"osgi/portal/keep.jar", "tomcat-10.1.59/conf/server.xml"} {
		if _, err := os.Stat(filepath.Join(bundle, rel)); err != nil {
			t.Errorf("%s should survive: %v", rel, err)
		}
	}
	for _, rel := range []string{
		"data", "osgi/configs", "osgi/modules", "osgi/state", "portal-setup-wizard.properties",
		"logs/liferay.log", "tomcat-10.1.59/logs/catalina.out", "tomcat-10.1.59/temp/x", "tomcat-10.1.59/work/Catalina",
	} {
		if _, err := os.Stat(filepath.Join(bundle, rel)); !os.IsNotExist(err) {
			t.Errorf("%s should be gone, stat err: %v", rel, err)
		}
	}
	for _, rel := range []string{"logs", "tomcat-10.1.59/logs", "tomcat-10.1.59/temp", "tomcat-10.1.59/work"} {
		info, err := os.Stat(filepath.Join(bundle, rel))
		if err != nil || !info.IsDir() {
			t.Errorf("%s should remain as an empty directory: %v", rel, err)
		}
	}
}

func TestCopyRewritingRoot(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.properties")
	dst := filepath.Join(dir, "nested", "dst.properties")
	content := "sdk.dir=/old/root/tools/sdk\ncom.liferay.portal.kernel.version=1.0.0-SNAPSHOT\n"
	if err := os.WriteFile(src, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyRewritingRoot(src, dst, "/old/root", "/new/root"); err != nil {
		t.Fatalf("copyRewritingRoot: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	want := "sdk.dir=/new/root/tools/sdk\ncom.liferay.portal.kernel.version=1.0.0-SNAPSHOT\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFindSeedSource confirms that a worktree whose recorded build matches
// the new worktree's HEAD is picked as its seed source, and that one built at
// another commit is not.
func TestFindSeedSource(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("seeding is only supported on macOS and Linux")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Setenv("HOME", t.TempDir())

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "master")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test")
	if err := os.MkdirAll(filepath.Join(repoDir, "modules"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"build.xml", filepath.Join("modules", "build.gradle")} {
		if err := os.WriteFile(filepath.Join(repoDir, name), []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "monorepo")

	head := gitOutputInDir(t, repoDir, "rev-parse", "HEAD")

	// git reports worktree paths with symlinks resolved (macOS's /var is
	// /private/var), and CLI state is keyed by path.
	worktreesDir := resolveSymlinksOrSelf(t.TempDir())
	sourcePath := filepath.Join(worktreesDir, "source")
	runGit(t, repoDir, "worktree", "add", "-b", "source", sourcePath)
	targetPath := filepath.Join(worktreesDir, "target")
	runGit(t, repoDir, "worktree", "add", "-b", "target", targetPath)

	if got := findSeedSource(targetPath); got != "" {
		t.Errorf("findSeedSource with no built worktree = %q, want none", got)
	}

	if err := state.SaveBuildBase(sourcePath, head); err != nil {
		t.Fatal(err)
	}
	if got := findSeedSource(targetPath); got != sourcePath {
		t.Errorf("findSeedSource = %q, want %q", got, sourcePath)
	}

	if err := state.SaveBuildBase(sourcePath, strings.Repeat("b", 40)); err != nil {
		t.Fatal(err)
	}
	if got := findSeedSource(targetPath); got != "" {
		t.Errorf("findSeedSource with a build at another commit = %q, want none", got)
	}
}
