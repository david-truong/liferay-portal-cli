package portal

import (
	"errors"
	"fmt"
	"os"
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
	props := readAppServerProps(portalRoot)

	dir := props["app.server.parent.dir"]
	if dir == "" {
		dir = filepath.Join(filepath.Dir(portalRoot), "bundles")
	} else {
		dir = resolveProperty(dir, portalRoot, props)
	}
	return filepath.Clean(dir), nil
}

// FindTomcatDir resolves the Tomcat directory from app.server.properties
// rather than scanning the filesystem, so the version is always authoritative.
func FindTomcatDir(portalRoot string) (string, error) {
	props := readAppServerProps(portalRoot)

	dir := props["app.server.tomcat.dir"]
	if dir == "" {
		version := props["app.server.tomcat.version"]
		if version == "" {
			return "", fmt.Errorf("app.server.tomcat.version not set in %s/app.server.properties", portalRoot)
		}
		bundleDir, err := BundleDir(portalRoot)
		if err != nil {
			return "", err
		}
		return filepath.Join(bundleDir, "tomcat-"+version), nil
	}

	return filepath.Clean(resolveProperty(dir, portalRoot, props)), nil
}

func readAppServerProps(portalRoot string) map[string]string {
	props, err := ReadProperties(filepath.Join(portalRoot, "app.server.properties"))
	if err != nil {
		props = map[string]string{}
	}

	if username, err := SafeUsername(); err == nil {
		override := filepath.Join(portalRoot, "app.server."+username+".properties")
		if overrideProps, err := ReadProperties(override); err == nil {
			for k, v := range overrideProps {
				props[k] = v
			}
		}
	}
	return props
}

func resolveProperty(value, portalRoot string, props map[string]string) string {
	props["project.dir"] = portalRoot
	for i := 0; i < 10 && strings.Contains(value, "${"); i++ {
		prev := value
		for k, v := range props {
			value = strings.ReplaceAll(value, "${"+k+"}", v)
		}
		if value == prev {
			break
		}
	}
	return value
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
