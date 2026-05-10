package cli

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

var (
	omniAdminBypassAck      bool
	omniAdminAllowExternal  bool
)

var omniAdminInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Copy all omni-admin jars into the active bundle's osgi/modules",
	Long: `Installs the three omni-admin bundles into the active bundle's osgi/modules.

This bypasses authentication — every request becomes an administrator. Use
only for local development against a throw-away database.

By default this command refuses to run if the resolved bundle path is not
under the current worktree (i.e. app.server.parent.dir points elsewhere).
Pass --allow-external-bundle to override.

Consent is required in every invocation. Pass --i-understand-this-bypasses-auth
(or set LIFERAY_CLI_ASSUME_YES=1) when scripting; otherwise the command
prompts interactively on a TTY and refuses on a non-TTY.`,
	RunE: runOmniAdminInstall,
}

var omniAdminUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove omni-admin jars from the active bundle's osgi/modules",
	RunE:  runOmniAdminUninstall,
}

func init() {
	omniAdminInstallCmd.Flags().BoolVar(&omniAdminBypassAck, "i-understand-this-bypasses-auth", false,
		"Confirm the auth-bypass install without an interactive prompt. Required when stdin is not a TTY (or set LIFERAY_CLI_ASSUME_YES=1).")
	omniAdminInstallCmd.Flags().BoolVar(&omniAdminAllowExternal, "allow-external-bundle", false,
		"Permit installation when the resolved bundle path is outside the current worktree.")
	omniAdminCmd.AddCommand(omniAdminInstallCmd)
	omniAdminCmd.AddCommand(omniAdminUninstallCmd)
	rootCmd.AddCommand(omniAdminCmd)
}

// omniAdminGuard is the testable gate that runOmniAdminInstall consults
// before touching the bundle. Returns nil when the install may proceed,
// or an *ExitError with code 6 (bundle outside worktree, no override) or
// 7 (consent not given). All inputs are injected so the test suite can
// hit every branch without a real terminal.
func omniAdminGuard(worktreeRoot, bundleDir string, allowExternal, assumeYes bool, in io.Reader, out io.Writer, isTTY bool) error {
	if !allowExternal && !isPathUnder(worktreeRoot, bundleDir) {
		return ExitErr(ExitBundleOutside,
			"bundle path %q is not a descendant of the worktree %q\n"+
				"pass --allow-external-bundle to override (only do this if you really mean to install omni-admin on a bundle outside the worktree)",
			bundleDir, worktreeRoot)
	}
	if !confirmWithIO(
		"omni-admin install bypasses authentication for every caller of the resolved bundle. Proceed?",
		assumeYes, in, out, isTTY,
	) {
		return ExitErr(ExitConfirmationDeclined,
			"omni-admin install declined — pass --i-understand-this-bypasses-auth or set %s=1 to skip the prompt",
			AssumeYesEnvVar)
	}
	return nil
}

// isPathUnder reports whether child is the same path as parent or a
// descendant. Both arguments are resolved to absolute form first so
// relative paths produced by t.TempDir() still compare correctly.
func isPathUnder(parent, child string) bool {
	pAbs, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	cAbs, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(pAbs, cAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
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
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}
	bundleDir, err := portal.BundleDir(portalRoot)
	if err != nil {
		return err
	}
	if err := omniAdminGuard(portalRoot, bundleDir,
		omniAdminAllowExternal, omniAdminBypassAck,
		os.Stdin, os.Stdout, isStdinTTY()); err != nil {
		return err
	}

	modulesDir := filepath.Join(bundleDir, "osgi", "modules")
	if _, err := os.Stat(modulesDir); err != nil {
		return fmt.Errorf("osgi/modules not found at %s: %w", modulesDir, err)
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
