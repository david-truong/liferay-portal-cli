package tomcat

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
)

// updateGolden, when set via `go test -update`, rewrites every want file from
// the current rewriteServerXML output. Useful when a deliberate regex change
// shifts the expected output for every fixture at once. Always inspect the
// diff before committing.
var updateGolden = flag.Bool("update", false, "rewrite golden files in testdata/")

// TestRewriteServerXML_Goldens locks down the line-based rewriter's behavior
// across the fixture shapes the audit called out: stock, multi-line connectors,
// comment-spanning connectors, and a (currently buggy) uncommented AJP case.
//
// The AJP fixture documents existing behavior: the regex matches any port=
// inside an open <Connector> block, so the AJP port gets rewritten to the
// same value as the HTTP port. This is fixed by Phase 5 T18 — when that
// lands, the ajp_enabled.slot1.want.xml file will need regenerating via
// `go test -update`.
func TestRewriteServerXML_Goldens(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		slot int
	}{
		{"stock_slot1", "stock.in.xml", "stock.slot1.want.xml", 1},
		{"stock_slot12", "stock.in.xml", "stock.slot12.want.xml", 12},
		{"ajp_enabled_slot1", "ajp_enabled.in.xml", "ajp_enabled.slot1.want.xml", 1},
		{"multiline_slot1", "multiline.in.xml", "multiline.slot1.want.xml", 1},
		{"comment_spanning_slot1", "comment_spanning.in.xml", "comment_spanning.slot1.want.xml", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inputPath := filepath.Join("testdata", "serverxml", tc.in)
			wantPath := filepath.Join("testdata", "serverxml", tc.want)

			input, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			got := rewriteServerXML(string(input), docker.PortsFromSlot(tc.slot))

			if *updateGolden {
				if err := os.WriteFile(wantPath, []byte(got), 0644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated %s", wantPath)
				return
			}

			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("read want: %v\n(run `go test -update ./internal/tomcat/...` to create)", err)
			}
			if got != string(want) {
				t.Errorf("rewriteServerXML output mismatch for %s\n--- want\n%s\n--- got\n%s",
					tc.in, want, got)
			}
		})
	}
}

// TestRewriteServerXML_Slot0Identity asserts the rewriter is a no-op when the
// requested slot is 0 — i.e. PatchBundle's stock-short-circuit is not the only
// thing protecting slot 0; rewriteServerXML itself is byte-stable for slot 0
// because all replacements re-emit the base port values.
func TestRewriteServerXML_Slot0Identity(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "serverxml", "stock.in.xml"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := rewriteServerXML(string(input), docker.PortsFromSlot(0))
	if got != string(input) {
		t.Errorf("slot 0 should be byte-identical to input")
	}
}

func TestPatchSetenvSh_NoExistingLine(t *testing.T) {
	binDir := t.TempDir()
	original := "#!/bin/sh\nexport CATALINA_OPTS=\"-Xmx2g\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "setenv.sh"), []byte(original), 0744); err != nil {
		t.Fatal(err)
	}

	if err := patchSetenvSh(binDir, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("patchSetenvSh: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(binDir, "setenv.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "export JPDA_ADDRESS=8010") {
		t.Errorf("expected appended JPDA_ADDRESS=8010, got:\n%s", got)
	}
	if !strings.Contains(string(got), original) {
		t.Errorf("expected original content preserved, got:\n%s", got)
	}
}

func TestPatchSetenvSh_SingleExistingLine(t *testing.T) {
	binDir := t.TempDir()
	original := "#!/bin/sh\nexport JPDA_ADDRESS=8000\nexport CATALINA_OPTS=\"-Xmx2g\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "setenv.sh"), []byte(original), 0744); err != nil {
		t.Fatal(err)
	}

	if err := patchSetenvSh(binDir, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("patchSetenvSh: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(binDir, "setenv.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "export JPDA_ADDRESS=8010") {
		t.Errorf("expected JPDA_ADDRESS=8010, got:\n%s", got)
	}
	if strings.Contains(string(got), "export JPDA_ADDRESS=8000") {
		t.Errorf("old JPDA_ADDRESS=8000 should be gone, got:\n%s", got)
	}
}

func TestPatchSetenvSh_NoTrailingNewline(t *testing.T) {
	binDir := t.TempDir()
	original := "#!/bin/sh\nexport CATALINA_OPTS=\"-Xmx2g\"" // no \n at end
	if err := os.WriteFile(filepath.Join(binDir, "setenv.sh"), []byte(original), 0744); err != nil {
		t.Fatal(err)
	}

	if err := patchSetenvSh(binDir, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("patchSetenvSh: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(binDir, "setenv.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(got), "\n") {
		t.Errorf("expected trailing newline, got: %q", got)
	}
	if !strings.Contains(string(got), "export JPDA_ADDRESS=8010") {
		t.Errorf("expected JPDA_ADDRESS=8010 appended, got:\n%s", got)
	}
}

func TestPatchSetenvSh_MissingFile(t *testing.T) {
	binDir := t.TempDir() // no setenv.sh inside

	err := patchSetenvSh(binDir, docker.PortsFromSlot(1))
	if err == nil {
		t.Error("expected error when setenv.sh is missing, got nil")
	}
}

func TestPatchSetenvSh_MultipleExistingLines(t *testing.T) {
	binDir := t.TempDir()
	original := "#!/bin/sh\nexport JPDA_ADDRESS=8000\nexport JPDA_ADDRESS=9999\n"
	if err := os.WriteFile(filepath.Join(binDir, "setenv.sh"), []byte(original), 0744); err != nil {
		t.Fatal(err)
	}

	if err := patchSetenvSh(binDir, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("patchSetenvSh: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(binDir, "setenv.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "8000") || strings.Contains(string(got), "9999") {
		t.Errorf("all prior JPDA_ADDRESS values should be replaced, got:\n%s", got)
	}
	count := strings.Count(string(got), "export JPDA_ADDRESS=")
	if count != 2 {
		t.Errorf("expected 2 JPDA_ADDRESS lines after rewrite, got %d:\n%s", count, got)
	}
}

func TestUpsertPropertyLine_FileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "props.properties")

	if err := upsertPropertyLine(path, "foo", "foo=bar"); err != nil {
		t.Fatalf("upsertPropertyLine: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after upsert: %v", err)
	}
	if string(got) != "foo=bar\n" {
		t.Errorf("expected 'foo=bar\\n', got %q", got)
	}
}

func TestUpsertPropertyLine_KeyAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "props.properties")
	if err := os.WriteFile(path, []byte("alpha=1\nbeta=2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := upsertPropertyLine(path, "gamma", "gamma=3"); err != nil {
		t.Fatalf("upsertPropertyLine: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "alpha=1\nbeta=2\ngamma=3\n"
	if string(got) != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestUpsertPropertyLine_KeyPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "props.properties")
	if err := os.WriteFile(path, []byte("alpha=1\nbeta=old\ngamma=3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := upsertPropertyLine(path, "beta", "beta=new"); err != nil {
		t.Fatalf("upsertPropertyLine: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "alpha=1\nbeta=new\ngamma=3\n"
	if string(got) != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestPatchGlowrootAdmin_FileMissing(t *testing.T) {
	bundle := t.TempDir()
	// No glowroot/admin.json — should be a silent no-op.
	if err := patchGlowrootAdmin(bundle, docker.PortsFromSlot(1)); err != nil {
		t.Errorf("missing admin.json should be a no-op, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundle, "glowroot", "admin.json")); err == nil {
		t.Error("patchGlowrootAdmin should not create admin.json when it does not exist")
	}
}

func TestPatchGlowrootAdmin_NoWebKey(t *testing.T) {
	bundle := t.TempDir()
	dir := filepath.Join(bundle, "glowroot")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "admin.json"), []byte(`{"foo":"bar"}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := patchGlowrootAdmin(bundle, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("patchGlowrootAdmin: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, data)
	}
	web, ok := got["web"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'web' object, got %v", got["web"])
	}
	if port, _ := web["port"].(float64); int(port) != 4010 {
		t.Errorf("expected web.port=4010 for slot 1, got %v", web["port"])
	}
	if got["foo"] != "bar" {
		t.Errorf("sibling key 'foo' should be preserved, got %v", got["foo"])
	}
}

func TestPatchGlowrootAdmin_WithWebKey(t *testing.T) {
	bundle := t.TempDir()
	dir := filepath.Join(bundle, "glowroot")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	input := `{"web":{"port":4000,"bindAddress":"0.0.0.0"},"other":"keep"}`
	if err := os.WriteFile(filepath.Join(dir, "admin.json"), []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	if err := patchGlowrootAdmin(bundle, docker.PortsFromSlot(1)); err != nil {
		t.Fatalf("patchGlowrootAdmin: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, data)
	}
	web := got["web"].(map[string]any)
	if int(web["port"].(float64)) != 4010 {
		t.Errorf("expected web.port=4010, got %v", web["port"])
	}
	if web["bindAddress"] != "0.0.0.0" {
		t.Errorf("bindAddress should be preserved, got %v", web["bindAddress"])
	}
	if got["other"] != "keep" {
		t.Errorf("top-level 'other' should be preserved, got %v", got["other"])
	}
}

func TestPatchGlowrootAdmin_CorruptJSON(t *testing.T) {
	bundle := t.TempDir()
	dir := filepath.Join(bundle, "glowroot")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "admin.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	err := patchGlowrootAdmin(bundle, docker.PortsFromSlot(1))
	if err == nil {
		t.Error("expected error for corrupt JSON, got nil")
	}
}
