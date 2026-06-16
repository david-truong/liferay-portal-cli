package tomcat

import (
	"os"
	"path/filepath"
	"testing"
)

func wipeFixture(t *testing.T) Paths {
	t.Helper()
	bundle := t.TempDir()
	tomcat := filepath.Join(bundle, "tomcat-10")

	for _, dir := range []string{
		filepath.Join(bundle, "data"),
		filepath.Join(bundle, "logs"),
		filepath.Join(bundle, "osgi", "state"),
		filepath.Join(tomcat, "work"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(dir, "marker"), []byte("x"))
	}
	mustWrite(t, filepath.Join(bundle, "portal-setup-wizard.properties"), []byte("setup.wizard.enabled=false\n"))

	return Paths{Bundle: bundle, Tomcat: tomcat}
}

func TestWipe_RemovesSetupWizardByDefault(t *testing.T) {
	paths := wipeFixture(t)

	Wipe(paths, false /* keepSetupWizard */)

	wizard := filepath.Join(paths.Bundle, "portal-setup-wizard.properties")
	if _, err := os.Stat(wizard); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err = %v", wizard, err)
	}
}

func TestWipe_KeepsSetupWizardOnSlot0(t *testing.T) {
	paths := wipeFixture(t)

	Wipe(paths, true /* keepSetupWizard */)

	wizard := filepath.Join(paths.Bundle, "portal-setup-wizard.properties")
	if _, err := os.Stat(wizard); err != nil {
		t.Errorf("expected %s preserved, stat err = %v", wizard, err)
	}
	if _, err := os.Stat(filepath.Join(paths.Bundle, "data")); !os.IsNotExist(err) {
		t.Error("derived state (data/) should still be wiped on slot 0")
	}
}
