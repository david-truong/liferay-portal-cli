package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SymlinkOrCopy creates a symlink from dst → src.
// On Windows without the required privilege, falls back to a recursive copy and returns a non-nil note.
// Returns (action, note, error) where action is "linked" or "copied" and note explains the fallback.
func SymlinkOrCopy(src, dst string) (action string, note string, err error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", "", err
	}

	linkErr := os.Symlink(src, dst)
	if linkErr == nil {
		return "linked", "", nil
	}

	if runtime.GOOS == "windows" && isPrivilegeError(linkErr) {
		note = "symlinks require Developer Mode on Windows — falling back to copy (enable Developer Mode to avoid this)"
		info, statErr := os.Stat(src)
		if statErr != nil {
			return "", note, fmt.Errorf("stat %s: %w", src, statErr)
		}
		if info.IsDir() {
			err = CopyDir(src, dst)
		} else {
			err = CopyFile(src, dst)
		}
		if err != nil {
			return "", note, err
		}
		return "copied", note, nil
	}

	return "", "", fmt.Errorf("symlink %s → %s: %w", dst, src, linkErr)
}

func isPrivilegeError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsStr(msg, "privilege") || containsStr(msg, "A required privilege is not held")
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
