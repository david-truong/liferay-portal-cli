package tomcat

import (
	"flag"
	"os"
	"path/filepath"
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
