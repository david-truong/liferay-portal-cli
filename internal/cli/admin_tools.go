package cli

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

//go:embed admintools/*.jar
var adminToolsJars embed.FS

// Servlet-API-coupled bundles need a namespace-matched build: Tomcat 10+ is
// Jakarta EE 9+ (jakarta.servlet), everything before it is javax.servlet.
// Only oauth2.admin has a Jakarta variant so far — omni.admin.autologin and
// omni.admin.captcha also import javax.servlet/javax.portlet but aren't
// ported yet, so they still fail to resolve on a Jakarta bundle.
//
//go:embed admintools/jakarta/*.jar
var adminToolsJakartaJars embed.FS

const (
	adminToolsJarDir        = "admintools"
	adminToolsJakartaJarDir = "admintools/jakarta"
)

var adminToolsCmd = &cobra.Command{
	Use:   "admin-tools",
	Short: "Install dev-only admin bundles (auto-login, no-captcha, forgiving store, OAuth2 provisioning)",
	Long: `Installs four bundles into the active Liferay bundle's osgi/modules directory:

  omni.admin.autologin — auto-authenticates requests as an administrator
  omni.admin.captcha   — disables CAPTCHA portal-wide
  omni.admin.store     — returns empty files for missing documents
  oauth2.admin         — POST /o/oauth2-admin/applications mints or resets
                         OAuth2 client-credentials secrets, no auth required

These bypass authentication, validation, and permission checks. Never use on
a shared or production bundle.`,
}

var (
	adminToolsBypassAck     bool
	adminToolsAllowExternal bool
)

var adminToolsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Copy all admin-tools jars into the active bundle's osgi/modules",
	Long: `Installs the four admin-tools bundles into the active bundle's osgi/modules.

This bypasses authentication for every request and exposes an unauthenticated
endpoint that mints or resets OAuth2 client-credentials secrets. Use only for
local development against a throw-away database.

By default this command refuses to run if the resolved bundle path is not
under the current worktree (i.e. app.server.parent.dir points elsewhere).
Pass --allow-external-bundle to override.

Consent is required in every invocation. Pass --i-understand-this-bypasses-auth
(or set LIFERAY_CLI_ASSUME_YES=1) when scripting; otherwise the command
prompts interactively on a TTY and refuses on a non-TTY.`,
	RunE: runAdminToolsInstall,
}

var adminToolsUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove admin-tools jars from the active bundle's osgi/modules",
	RunE:  runAdminToolsUninstall,
}

func init() {
	adminToolsInstallCmd.Flags().BoolVar(&adminToolsBypassAck, "i-understand-this-bypasses-auth", false,
		"Confirm the auth-bypass install without an interactive prompt. Required when stdin is not a TTY (or set LIFERAY_CLI_ASSUME_YES=1).")
	adminToolsInstallCmd.Flags().BoolVar(&adminToolsAllowExternal, "allow-external-bundle", false,
		"Permit installation when the resolved bundle path is outside the current worktree.")
	adminToolsCmd.AddCommand(adminToolsInstallCmd)
	adminToolsCmd.AddCommand(adminToolsUninstallCmd)
	rootCmd.AddCommand(adminToolsCmd)
}

// adminToolsGuard is the testable gate that runAdminToolsInstall consults
// before touching the bundle. Returns nil when the install may proceed,
// or an *ExitError with code 6 (bundle outside worktree, no override) or
// 7 (consent not given). All inputs are injected so the test suite can hit
// every branch without a real terminal.
func adminToolsGuard(worktreeRoot, bundleDir string, allowExternal, assumeYes bool, in io.Reader, out io.Writer, isTTY bool) error {
	if !allowExternal && !isPathUnder(worktreeRoot, bundleDir) {
		return ExitErr(ExitBundleOutside,
			"bundle path %q is not a descendant of the worktree %q\n"+
				"pass --allow-external-bundle to override (only do this if you really mean to install admin-tools on a bundle outside the worktree)",
			bundleDir, worktreeRoot)
	}
	if !confirmWithIO(
		"admin-tools install bypasses authentication for every caller of the resolved bundle and mints OAuth2 credentials over an unauthenticated endpoint. Proceed?",
		assumeYes, in, out, isTTY,
	) {
		return ExitErr(ExitConfirmationDeclined,
			"admin-tools install declined — pass --i-understand-this-bypasses-auth or set %s=1 to skip the prompt",
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

func adminToolsModulesDir() (string, error) {
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

func runAdminToolsInstall(cmd *cobra.Command, args []string) error {
	portalRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}
	bundleDir, err := portal.BundleDir(portalRoot)
	if err != nil {
		return err
	}
	if err := adminToolsGuard(portalRoot, bundleDir,
		adminToolsAllowExternal, adminToolsBypassAck,
		os.Stdin, os.Stdout, isStdinTTY()); err != nil {
		return err
	}

	modulesDir := filepath.Join(bundleDir, "osgi", "modules")
	if _, err := os.Stat(modulesDir); err != nil {
		return fmt.Errorf("osgi/modules not found at %s: %w", modulesDir, err)
	}

	entries, err := adminToolsJars.ReadDir(adminToolsJarDir)
	if err != nil {
		return fmt.Errorf("reading embedded jars: %w", err)
	}

	jakarta := isJakartaBundle(portalRoot)

	for _, entry := range entries {
		srcFS, srcPath := adminToolsJars, filepath.Join(adminToolsJarDir, entry.Name())
		if jakarta {
			if jakartaEntry, err := adminToolsJakartaJars.Open(
				filepath.Join(adminToolsJakartaJarDir, entry.Name())); err == nil {

				jakartaEntry.Close()
				srcFS, srcPath = adminToolsJakartaJars, filepath.Join(adminToolsJakartaJarDir, entry.Name())
			}
		}

		dstPath := filepath.Join(modulesDir, entry.Name())
		if err := copyEmbeddedAdminToolsJar(srcFS, srcPath, dstPath); err != nil {
			return err
		}
		fmt.Printf("installed %s\n", dstPath)
	}
	return nil
}

// isJakartaBundle reports whether the resolved bundle's Tomcat is version
// 10+ (Jakarta EE 9+, jakarta.servlet) rather than 8.5/9 (javax.servlet).
// Any resolution failure defaults to false — the original javax jars, the
// long-standing behavior.
func isJakartaBundle(portalRoot string) bool {
	tomcatDir, err := portal.FindTomcatDir(portalRoot)
	if err != nil {
		return false
	}

	name := strings.TrimPrefix(filepath.Base(tomcatDir), "tomcat-")
	major, _, _ := strings.Cut(name, ".")

	version, err := strconv.Atoi(major)
	if err != nil {
		return false
	}
	return version >= 10
}

func copyEmbeddedAdminToolsJar(srcFS embed.FS, srcPath, dstPath string) error {
	src, err := srcFS.Open(srcPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", srcPath, err)
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

func runAdminToolsUninstall(cmd *cobra.Command, args []string) error {
	modulesDir, err := adminToolsModulesDir()
	if err != nil {
		return err
	}

	entries, err := adminToolsJars.ReadDir(adminToolsJarDir)
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
