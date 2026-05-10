package tomcat

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// snapshotRoot is the directory under stateDir where pre-patch backups live.
const snapshotRoot = "bundle-snapshot"

// absentMarkerExt is appended to a file path inside the snapshot to record
// that the file did NOT exist before the patch. Restore deletes any current
// file at that path.
const absentMarkerExt = ".absent"

// targetFiles enumerates every absolute path PatchBundle is allowed to
// touch for the given Paths. Each entry is either a regular file we will
// snapshot (if present) or a file that must be deleted on restore (if it
// was absent pre-patch).
func targetFiles(paths Paths) []string {
	return []string{
		filepath.Join(paths.Tomcat, "conf", "server.xml"),
		filepath.Join(paths.Bin, "setenv.sh"),
		filepath.Join(paths.Bundle, "portal-developer.properties"),
		filepath.Join(paths.Tomcat, "webapps", "ROOT", "WEB-INF", "classes", "portal-developer.properties"),
		filepath.Join(paths.Bundle, "glowroot", "admin.json"),
		filepath.Join(paths.Bundle, "osgi", "configs",
			"com.liferay.portal.search.elasticsearch7.configuration.ElasticsearchConfiguration.config"),
		filepath.Join(paths.Bundle, "osgi", "configs",
			"com.liferay.portal.search.elasticsearch8.configuration.ElasticsearchConfiguration.config"),
		filepath.Join(paths.Bundle, "osgi", "configs",
			"com.liferay.arquillian.extension.junit.bridge.connector.ArquillianConnector.config"),
		filepath.Join(paths.Bundle, "osgi", "configs",
			"com.liferay.data.guard.connector.DataGuardConnector.config"),
	}
}

// snapshotRelPath maps an absolute target file path to its location inside
// the snapshot directory. We mirror the bundle/tomcat layout so the
// reverse mapping (restoreRelPath) is unambiguous.
func snapshotRelPath(paths Paths, abs string) (string, error) {
	if rel, err := filepath.Rel(paths.Tomcat, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("tomcat", rel), nil
	}
	if rel, err := filepath.Rel(paths.Bundle, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("bundle", rel), nil
	}
	return "", fmt.Errorf("snapshot target %s is not under bundle or tomcat root", abs)
}

// Snapshot copies every file PatchBundle is about to touch into a
// timestamped directory under stateDir/bundle-snapshot/. Files that don't
// exist pre-patch are recorded with an .absent marker so RestoreFromSnapshot
// can re-delete them. Returns the path to the created snapshot directory.
func Snapshot(paths Paths, stateDir string) (string, error) {
	dir := filepath.Join(stateDir, snapshotRoot, time.Now().Format("20060102-150405.000"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating snapshot dir: %w", err)
	}

	for _, abs := range targetFiles(paths) {
		rel, err := snapshotRelPath(paths, abs)
		if err != nil {
			return "", err
		}
		dst := filepath.Join(dir, rel)

		if _, err := os.Stat(abs); errors.Is(err, os.ErrNotExist) {
			// Record the absence so restore knows to re-delete.
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
			}
			if err := os.WriteFile(dst+absentMarkerExt, nil, 0644); err != nil {
				return "", fmt.Errorf("write absent marker for %s: %w", abs, err)
			}
			continue
		} else if err != nil {
			return "", fmt.Errorf("stat %s: %w", abs, err)
		}

		if err := copyFile(abs, dst); err != nil {
			return "", err
		}
	}

	return dir, nil
}

// RestoreFromSnapshot copies the snapshotted files back to their original
// locations, and deletes any file flagged with an .absent marker.
func RestoreFromSnapshot(snapshotDir string, paths Paths) error {
	for _, abs := range targetFiles(paths) {
		rel, err := snapshotRelPath(paths, abs)
		if err != nil {
			return err
		}
		stored := filepath.Join(snapshotDir, rel)

		if _, err := os.Stat(stored + absentMarkerExt); err == nil {
			if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove %s: %w", abs, err)
			}
			continue
		}

		if _, err := os.Stat(stored); errors.Is(err, os.ErrNotExist) {
			// Neither the file nor the absent marker is in the snapshot.
			// That can happen if a future PatchBundle adds a new target
			// but an older snapshot predates it — leave the live file
			// untouched and move on.
			continue
		} else if err != nil {
			return fmt.Errorf("stat %s: %w", stored, err)
		}

		if err := copyFile(stored, abs); err != nil {
			return err
		}
	}
	return nil
}

// MostRecentSnapshot returns the path to the newest snapshot directory
// under stateDir/bundle-snapshot/, or ok=false when no snapshots exist.
// Ordering is lexicographic on the timestamp-formatted directory name,
// which corresponds to chronological order for any snapshots produced by
// Snapshot.
func MostRecentSnapshot(stateDir string) (string, bool, error) {
	base := filepath.Join(stateDir, snapshotRoot)
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("reading %s: %w", base, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", false, nil
	}
	sort.Strings(names)
	return filepath.Join(base, names[len(names)-1]), true, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy to %s: %w", dst, err)
	}
	return out.Close()
}
