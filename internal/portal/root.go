package portal

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// FindRoot walks up from dir looking for a portal root (build.xml + modules/).
func FindRoot(dir string) (string, error) {
	d := dir
	for {
		if isPortalRoot(d) {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", errors.New("not inside a liferay-portal repository (build.xml + modules/ not found)")
		}
		d = parent
	}
}

// IsPortalRepo returns true when dir looks like a liferay-portal root.
func IsPortalRepo(dir string) bool {
	return isPortalRoot(dir)
}

// BundleDir returns the resolved bundle directory for the given portal root,
// honouring app.server.properties and app.server.<user>.properties overrides.
func BundleDir(portalRoot string) (string, error) {
	props, err := ReadProperties(filepath.Join(portalRoot, "app.server.properties"))
	if err != nil {
		return "", fmt.Errorf("reading app.server.properties: %w", err)
	}

	if u, err := user.Current(); err == nil {
		override := filepath.Join(portalRoot, "app.server."+u.Username+".properties")
		if overrideProps, err := ReadProperties(override); err == nil {
			for k, v := range overrideProps {
				props[k] = v
			}
		}
	}

	dir := props["app.server.parent.dir"]
	if dir == "" {
		dir = filepath.Join(filepath.Dir(portalRoot), "bundles")
	} else {
		dir = strings.ReplaceAll(dir, "${project.dir}", portalRoot)
	}
	return filepath.Clean(dir), nil
}

// FindTomcatDir locates the tomcat-* directory inside bundleDir.
func FindTomcatDir(bundleDir string) (string, error) {
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return "", fmt.Errorf("cannot read bundle dir %s: %w", bundleDir, err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "tomcat-") {
			return filepath.Join(bundleDir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no tomcat-* directory found in %s", bundleDir)
}

func isPortalRoot(dir string) bool {
	return fileExists(filepath.Join(dir, "build.xml")) && dirExists(filepath.Join(dir, "modules"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
