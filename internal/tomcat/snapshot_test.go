package tomcat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
)

// fakeBundle builds a minimal bundle dir + tomcat dir + state dir that
// PatchBundle can run against without exploding. Returns a Paths struct
// pointing at the fakes.
func fakeBundle(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()

	bundle := filepath.Join(root, "bundles")
	tomcat := filepath.Join(bundle, "tomcat-9.0")
	bin := filepath.Join(tomcat, "bin")
	conf := filepath.Join(tomcat, "conf")
	embedded := filepath.Join(tomcat, "webapps", "ROOT", "WEB-INF", "classes")
	glowroot := filepath.Join(bundle, "glowroot")
	stateDir := filepath.Join(root, "state")

	for _, d := range []string{bin, conf, embedded, glowroot, stateDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	stockServerXML, err := os.ReadFile(filepath.Join("testdata", "serverxml", "stock.in.xml"))
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(conf, "server.xml"), stockServerXML)
	mustWrite(t, filepath.Join(bin, "setenv.sh"), []byte("#!/bin/sh\n"))
	mustWrite(t, filepath.Join(bundle, "portal-developer.properties"), []byte("# user note\n"))
	mustWrite(t, filepath.Join(embedded, "portal-developer.properties"), []byte("# embedded\n"))
	mustWrite(t, filepath.Join(glowroot, "admin.json"), []byte(`{"web":{"port":4000}}`))

	return Paths{
		Bundle:    bundle,
		Tomcat:    tomcat,
		Bin:       bin,
		CatalinaS: filepath.Join(bin, "catalina.sh"),
		PidFile:   filepath.Join(stateDir, "tomcat.pid"),
		CatOut:    filepath.Join(tomcat, "logs", "catalina.out"),
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// snapshotFingerprint captures every file PatchBundle is expected to touch
// so a test can assert byte-identity after restore.
func snapshotFingerprint(t *testing.T, paths Paths) map[string]string {
	t.Helper()
	files := []string{
		filepath.Join(paths.Tomcat, "conf", "server.xml"),
		filepath.Join(paths.Bin, "setenv.sh"),
		filepath.Join(paths.Bundle, "portal-developer.properties"),
		filepath.Join(paths.Tomcat, "webapps", "ROOT", "WEB-INF", "classes", "portal-developer.properties"),
		filepath.Join(paths.Bundle, "glowroot", "admin.json"),
	}
	out := make(map[string]string)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		out[f] = string(data)
	}
	return out
}

func TestSnapshot_RoundTripRestoresPrePatchState(t *testing.T) {
	paths := fakeBundle(t)
	before := snapshotFingerprint(t, paths)

	if err := PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("PatchBundle: %v", err)
	}

	after := snapshotFingerprint(t, paths)
	if before[filepath.Join(paths.Tomcat, "conf", "server.xml")] == after[filepath.Join(paths.Tomcat, "conf", "server.xml")] {
		t.Fatal("patch had no observable effect on server.xml — test fixture is wrong")
	}

	stateDir := filepath.Dir(paths.PidFile)
	snapshotDir, ok, err := MostRecentSnapshot(stateDir)
	if err != nil {
		t.Fatalf("MostRecentSnapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected a snapshot to exist after PatchBundle")
	}

	if err := RestoreFromSnapshot(snapshotDir, paths); err != nil {
		t.Fatalf("RestoreFromSnapshot: %v", err)
	}

	restored := snapshotFingerprint(t, paths)
	for path, want := range before {
		if got := restored[path]; got != want {
			t.Errorf("file %s not byte-identical after restore\n--- want ---\n%s\n--- got ---\n%s",
				path, want, got)
		}
	}
}

func TestSnapshot_RestoresAbsentOSGiConfigs(t *testing.T) {
	paths := fakeBundle(t)
	osgiConfigs := filepath.Join(paths.Bundle, "osgi", "configs")
	// Pre-patch: the OSGi config files do NOT exist. PatchBundle will
	// create them; restore must delete them again so the bundle returns
	// to a truly stock state.
	if _, err := os.Stat(osgiConfigs); err == nil {
		t.Fatal("test fixture invariant: osgi/configs should not exist pre-patch")
	}

	if err := PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("PatchBundle: %v", err)
	}

	created, err := filepath.Glob(filepath.Join(osgiConfigs, "*.config"))
	if err != nil {
		t.Fatal(err)
	}
	if len(created) == 0 {
		t.Fatal("expected PatchBundle to create OSGi config files")
	}

	stateDir := filepath.Dir(paths.PidFile)
	snapshotDir, _, err := MostRecentSnapshot(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := RestoreFromSnapshot(snapshotDir, paths); err != nil {
		t.Fatalf("RestoreFromSnapshot: %v", err)
	}

	for _, c := range created {
		if _, err := os.Stat(c); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed by restore (was absent pre-patch); err=%v", c, err)
		}
	}
}

// TestPatchBundle_DoublePatchThenUnpatchRestoresStock is the regression test
// for audit finding HIGH-3: PatchBundle used to snapshot unconditionally on
// every call, so a second "liferay server start" against an already-patched
// bundle snapshotted the *patched* files as if they were pristine. From then
// on, `bundle unpatch` restored patched-over-patched and reported success
// while never getting back to stock.
func TestPatchBundle_DoublePatchThenUnpatchRestoresStock(t *testing.T) {
	paths := fakeBundle(t)
	stock := snapshotFingerprint(t, paths)

	if err := PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("first PatchBundle: %v", err)
	}
	if err := PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("second PatchBundle: %v", err)
	}

	stateDir := filepath.Dir(paths.PidFile)
	snapshotDir, ok, err := MostRecentSnapshot(stateDir)
	if err != nil {
		t.Fatalf("MostRecentSnapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected a snapshot to exist after PatchBundle")
	}

	if err := RestoreFromSnapshot(snapshotDir, paths); err != nil {
		t.Fatalf("RestoreFromSnapshot: %v", err)
	}

	restored := snapshotFingerprint(t, paths)
	for path, want := range stock {
		if got := restored[path]; got != want {
			t.Errorf("file %s not restored to stock after two patch cycles\n--- want (stock) ---\n%s\n--- got ---\n%s",
				path, want, got)
		}
	}
}

// rebuildFiles are the subset of PatchBundle's targets that a full `ant all`
// rebuild actually resets to stock: it re-extracts the Tomcat directory
// (server.xml, setenv.sh) and the ROOT webapp (the embedded
// portal-developer.properties), per the doc comment on PatchBundle. The
// bundle-root portal-developer.properties and glowroot/admin.json live
// outside the rebuilt artifacts and are untouched by a rebuild.
func rebuildFiles(paths Paths) []string {
	return []string{
		filepath.Join(paths.Tomcat, "conf", "server.xml"),
		filepath.Join(paths.Bin, "setenv.sh"),
		filepath.Join(paths.Tomcat, "webapps", "ROOT", "WEB-INF", "classes", "portal-developer.properties"),
	}
}

// simulateRebuild resets the files an `ant all` rebuild resets back to their
// stock content, and removes the osgi/configs directory PatchBundle wrote.
func simulateRebuild(t *testing.T, paths Paths, stock map[string]string) {
	t.Helper()
	for _, path := range rebuildFiles(paths) {
		mustWrite(t, path, []byte(stock[path]))
	}
	if err := os.RemoveAll(filepath.Join(paths.Bundle, "osgi")); err != nil {
		t.Fatal(err)
	}
}

// TestPatchBundle_RebuildRetakesSnapshotAndUnpatchRestoresStock covers the
// scenario a naive "only snapshot once" fix would break: `ant all` rewrites
// server.xml and setenv.sh back to stock and wipes the generated OSGi
// configs. The next PatchBundle call must recognize the bundle is pristine
// again and take a fresh snapshot, not reuse the (now stale) first one.
func TestPatchBundle_RebuildRetakesSnapshotAndUnpatchRestoresStock(t *testing.T) {
	paths := fakeBundle(t)
	stock := snapshotFingerprint(t, paths)

	if err := PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("first PatchBundle: %v", err)
	}

	simulateRebuild(t, paths, stock)

	if err := PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("second PatchBundle (post-rebuild): %v", err)
	}

	stateDir := filepath.Dir(paths.PidFile)
	snapshotDir, ok, err := MostRecentSnapshot(stateDir)
	if err != nil {
		t.Fatalf("MostRecentSnapshot: %v", err)
	}
	if !ok {
		t.Fatal("expected a snapshot to exist after PatchBundle")
	}

	if err := RestoreFromSnapshot(snapshotDir, paths); err != nil {
		t.Fatalf("RestoreFromSnapshot: %v", err)
	}

	restored := snapshotFingerprint(t, paths)
	for _, path := range rebuildFiles(paths) {
		if got, want := restored[path], stock[path]; got != want {
			t.Errorf("file %s not restored to stock after rebuild + re-patch\n--- want (stock) ---\n%s\n--- got ---\n%s",
				path, want, got)
		}
	}
}

// TestPatchBundle_PrunesOldSnapshots asserts snapshot dirs don't accumulate:
// an idempotent re-patch (same slot, no rebuild) must not add a snapshot at
// all, and a rebuild's fresh pristine snapshot must replace the stale one
// rather than piling up alongside it.
func TestPatchBundle_PrunesOldSnapshots(t *testing.T) {
	paths := fakeBundle(t)
	stock := snapshotFingerprint(t, paths)

	if err := PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("first PatchBundle: %v", err)
	}
	if err := PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("idempotent re-patch: %v", err)
	}

	simulateRebuild(t, paths, stock)

	if err := PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("PatchBundle after rebuild: %v", err)
	}

	stateDir := filepath.Dir(paths.PidFile)
	entries, err := os.ReadDir(filepath.Join(stateDir, snapshotRoot))
	if err != nil {
		t.Fatalf("reading snapshot root: %v", err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 snapshot dir to survive pruning, got %d", count)
	}
}

func TestMostRecentSnapshot_NoneExists(t *testing.T) {
	_, ok, err := MostRecentSnapshot(t.TempDir())
	if err != nil {
		t.Fatalf("MostRecentSnapshot returned error on missing dir: %v", err)
	}
	if ok {
		t.Error("expected ok=false when no snapshots exist")
	}
}

func TestMostRecentSnapshot_PicksNewest(t *testing.T) {
	stateDir := t.TempDir()
	base := filepath.Join(stateDir, "bundle-snapshot")
	if err := os.MkdirAll(filepath.Join(base, "20260101-000000"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "20260510-153045"), 0755); err != nil {
		t.Fatal(err)
	}

	got, ok, err := MostRecentSnapshot(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected to find a snapshot")
	}
	if !strings.HasSuffix(got, "20260510-153045") {
		t.Errorf("expected newest snapshot (20260510-153045), got %s", got)
	}
}
