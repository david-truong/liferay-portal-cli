package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/tomcat"
)

// fakeBundlePaths assembles a minimal Paths struct under t.TempDir() —
// bundle/, tomcat/conf, tomcat/bin, glowroot, plus the bundle files
// PatchBundle expects. Returns Paths and the stateDir where snapshots
// will be written.
func fakeBundlePaths(t *testing.T) (tomcat.Paths, string) {
	t.Helper()
	root := t.TempDir()

	bundle := filepath.Join(root, "bundles")
	tomcatDir := filepath.Join(bundle, "tomcat-9.0")
	bin := filepath.Join(tomcatDir, "bin")
	conf := filepath.Join(tomcatDir, "conf")
	embedded := filepath.Join(tomcatDir, "webapps", "ROOT", "WEB-INF", "classes")
	glowroot := filepath.Join(bundle, "glowroot")
	stateDir := filepath.Join(root, "state")

	for _, d := range []string{bin, conf, embedded, glowroot, stateDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Minimal but sufficient stock server.xml — gives rewriteServerXML
	// something to chew on.
	serverXML := `<?xml version='1.0' encoding='utf-8'?>
<Server port="8005" shutdown="SHUTDOWN">
  <Service name="Catalina">
    <Connector port="8080" protocol="HTTP/1.1"
               redirectPort="8443" />
  </Service>
</Server>
`
	mustWriteCLI(t, filepath.Join(conf, "server.xml"), []byte(serverXML))
	mustWriteCLI(t, filepath.Join(bin, "setenv.sh"), []byte("#!/bin/sh\n"))
	mustWriteCLI(t, filepath.Join(bundle, "portal-developer.properties"), []byte("# user\n"))
	mustWriteCLI(t, filepath.Join(embedded, "portal-developer.properties"), []byte("# embedded\n"))
	mustWriteCLI(t, filepath.Join(glowroot, "admin.json"), []byte(`{"web":{"port":4000}}`))

	return tomcat.Paths{
		Bundle:    bundle,
		Tomcat:    tomcatDir,
		Bin:       bin,
		CatalinaS: filepath.Join(bin, "catalina.sh"),
		PidFile:   filepath.Join(stateDir, "tomcat.pid"),
		CatOut:    filepath.Join(tomcatDir, "logs", "catalina.out"),
	}, stateDir
}

func mustWriteCLI(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestUnpatchBundle_NoSnapshot(t *testing.T) {
	paths, stateDir := fakeBundlePaths(t)

	out := captureStdout(t, func() {
		if err := unpatchBundle(paths, stateDir); err != nil {
			t.Errorf("unpatchBundle: %v", err)
		}
	})
	if !strings.Contains(out, "No snapshot") {
		t.Errorf("expected 'No snapshot' message, got %q", out)
	}
}

func TestUnpatchBundle_RestoresPrePatchState(t *testing.T) {
	paths, stateDir := fakeBundlePaths(t)

	// Snapshot the pre-patch contents.
	before := map[string]string{}
	for _, f := range []string{
		filepath.Join(paths.Tomcat, "conf", "server.xml"),
		filepath.Join(paths.Bin, "setenv.sh"),
		filepath.Join(paths.Bundle, "glowroot", "admin.json"),
	} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		before[f] = string(data)
	}

	if err := tomcat.PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("PatchBundle: %v", err)
	}

	if err := unpatchBundle(paths, stateDir); err != nil {
		t.Fatalf("unpatchBundle: %v", err)
	}

	for f, want := range before {
		got, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s not restored\nwant: %q\ngot:  %q", f, want, got)
		}
	}
}

func TestUnpatchBundle_RefusesWhileTomcatRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tomcat.Status uses Signal(0), which Windows doesn't accept for arbitrary PIDs")
	}
	paths, stateDir := fakeBundlePaths(t)

	if err := tomcat.PatchBundle(paths, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("PatchBundle: %v", err)
	}

	// Write our own PID into the tomcat.pid file. tomcat.Status will
	// then report "alive" because the OS confirms the PID is live.
	if err := os.WriteFile(paths.PidFile, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}

	err := unpatchBundle(paths, stateDir)
	if err == nil {
		t.Error("expected error when tomcat is running, got nil")
	}
}
