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
