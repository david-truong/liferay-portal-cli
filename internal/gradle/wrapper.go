package gradle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Find returns the absolute path to the gradle wrapper command for the given module directory.
// Prefers `gw` on PATH (the GraalVM helper from homebrew-liferay); falls back to the nearest gradlew / gradlew.bat.
func Find(moduleDir string) (string, error) {
	// Prefer gw if available
	for _, name := range gwNames() {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}

	// Fall back to nearest gradlew / gradlew.bat
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
		"gradle wrapper not found\n\n" +
			"Install gw (the Liferay Gradle wrapper helper):\n" +
			"  brew install david-truong/liferay/gw\n" +
			"Or ensure gradlew exists in the portal root.")
}

// Command returns an *exec.Cmd ready to run the gradle wrapper from moduleDir with the given args.
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

func gwNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"gw.exe", "gw"}
	}
	return []string{"gw"}
}
