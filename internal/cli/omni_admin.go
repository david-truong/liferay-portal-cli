package cli

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

//go:embed omniadmin/*.jar
var omniAdminJars embed.FS

const omniAdminJarDir = "omniadmin"

var omniAdminCmd = &cobra.Command{
	Use:   "omni-admin",
	Short: "Install dev-only omni-admin bundles (auto-login, no-captcha, forgiving store)",
	Long: `Installs three bundles into the active Liferay bundle's osgi/modules directory:

  omni.admin.autologin  — auto-authenticates requests as an administrator
  omni.admin.captcha    — disables CAPTCHA portal-wide
  omni.admin.store      — returns empty files for missing documents

These bypass authentication and validation. Never use on a shared or production bundle.`,
}

var omniAdminInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Copy all omni-admin jars into the active bundle's osgi/modules",
	RunE:  runOmniAdminInstall,
}

var omniAdminUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove omni-admin jars from the active bundle's osgi/modules",
	RunE:  runOmniAdminUninstall,
}

func init() {
	omniAdminCmd.AddCommand(omniAdminInstallCmd)
	omniAdminCmd.AddCommand(omniAdminUninstallCmd)
	rootCmd.AddCommand(omniAdminCmd)
}

func omniAdminModulesDir() (string, error) {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return "", err
	}
	bundleDir, err := portal.BundleDir(portalRoot)
	if err != nil {
		return "", err
	}
	modulesDir := filepath.Join(bundleDir, "osgi", "modules")
	if _, err := os.Stat(modulesDir); err != nil {
		return "", fmt.Errorf("osgi/modules not found at %s: %w", modulesDir, err)
	}
	return modulesDir, nil
}

func runOmniAdminInstall(cmd *cobra.Command, args []string) error {
	modulesDir, err := omniAdminModulesDir()
	if err != nil {
		return err
	}

	entries, err := omniAdminJars.ReadDir(omniAdminJarDir)
	if err != nil {
		return fmt.Errorf("reading embedded jars: %w", err)
	}

	for _, entry := range entries {
		dstPath := filepath.Join(modulesDir, entry.Name())
		if err := copyEmbeddedJar(entry.Name(), dstPath); err != nil {
			return err
		}
		fmt.Printf("installed %s\n", dstPath)
	}
	return nil
}

func copyEmbeddedJar(name, dstPath string) error {
	src, err := omniAdminJars.Open(filepath.Join(omniAdminJarDir, name))
	if err != nil {
		return fmt.Errorf("opening %s: %w", name, err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dstPath, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("writing %s: %w", dstPath, err)
	}
	return dst.Close()
}

func runOmniAdminUninstall(cmd *cobra.Command, args []string) error {
	modulesDir, err := omniAdminModulesDir()
	if err != nil {
		return err
	}

	entries, err := omniAdminJars.ReadDir(omniAdminJarDir)
	if err != nil {
		return fmt.Errorf("reading embedded jars: %w", err)
	}

	for _, entry := range entries {
		path := filepath.Join(modulesDir, entry.Name())
		err := os.Remove(path)
		if err == nil {
			fmt.Printf("removed %s\n", path)
		} else if os.IsNotExist(err) {
			fmt.Printf("skipped %s (not present)\n", path)
		} else {
			return fmt.Errorf("removing %s: %w", path, err)
		}
	}
	return nil
}
