package gradle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Find returns the absolute path to the nearest gradlew / gradlew.bat for the given module directory.
func Find(moduleDir string) (string, error) {
	wrapperName := "gradlew"
	if runtime.GOOS == "windows" {
		wrapperName = "gradlew.bat"
	}

	d := moduleDir
	for {
		candidate := filepath.Join(d, wrapperName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}

	return "", fmt.Errorf(
		"gradlew not found — ensure gradlew exists in the portal root")
}

// Command returns an *exec.Cmd ready to run gradlew from moduleDir with the given args.
func Command(moduleDir string, args ...string) (*exec.Cmd, error) {
	wrapper, err := Find(moduleDir)
	if err != nil {
		return nil, err
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" && filepath.Ext(wrapper) == ".bat" {
		cmdArgs := append([]string{"/C", wrapper}, args...)
		cmd = exec.Command("cmd.exe", cmdArgs...)
	} else {
		cmd = exec.Command(wrapper, args...)
	}
	cmd.Dir = moduleDir
	return cmd, nil
}
