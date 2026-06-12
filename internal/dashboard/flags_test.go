package dashboard

import (
	"strings"
	"testing"
)

func TestParseAddedFlags(t *testing.T) {
	diff := `+++ b/portal-impl/src/portal.properties
+    feature.flag.LPD-11111=false
-    feature.flag.LPD-99999=false
+    feature.flag.LPD-22222=true
+    feature.flag.LPD-11111=false
+    some.other.property=true
`
	got := parseAddedFlags(diff)
	want := []string{"LPD-11111", "LPD-22222"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("parseAddedFlags = %v, want %v", got, want)
	}

	if got := parseAddedFlags(""); got != nil {
		t.Errorf("parseAddedFlags(empty) = %v, want nil", got)
	}
}

func TestEnsureFlagLines(t *testing.T) {
	flags := []string{"LPD-11111", "LPD-22222"}

	// Empty file: both appended under the marker.
	content, changed := EnsureFlagLines("", flags)
	if !changed {
		t.Fatal("no change reported on empty content")
	}
	for _, want := range []string{flagsMarker, "feature.flag.LPD-11111=true", "feature.flag.LPD-22222=true"} {
		if !strings.Contains(content, want) {
			t.Errorf("content missing %q:\n%s", want, content)
		}
	}

	// Existing false assignment is flipped in place, enabled one untouched.
	content, changed = EnsureFlagLines("feature.flag.LPD-11111=false\nfeature.flag.LPD-22222=true\n", flags)
	if !changed {
		t.Fatal("no change reported for false assignment")
	}
	if !strings.Contains(content, "feature.flag.LPD-11111=true") {
		t.Errorf("false assignment not flipped:\n%s", content)
	}
	if strings.Contains(content, flagsMarker) {
		t.Errorf("marker appended although nothing was missing:\n%s", content)
	}

	// Already enabled: untouched.
	original := "jdbc.default.url=x\nfeature.flag.LPD-11111=true\nfeature.flag.LPD-22222=true\n"
	content, changed = EnsureFlagLines(original, flags)
	if changed || content != original {
		t.Errorf("idempotency broken: changed=%v\n%s", changed, content)
	}
}

func TestFlagStates(t *testing.T) {
	content := "feature.flag.LPD-11111=true\nfeature.flag.LPD-11111=false\nfeature.flag.LPD-33333=true\n"

	states := flagStates(content, []string{"LPD-11111", "LPD-22222"})
	if states["LPD-11111"] {
		t.Error("last assignment should win (false)")
	}
	if states["LPD-22222"] {
		t.Error("absent flag should be disabled")
	}
	if _, tracked := states["LPD-33333"]; tracked {
		t.Error("untracked flag leaked into states")
	}
}
