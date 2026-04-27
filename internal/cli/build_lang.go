package cli

import (
	"os"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/gradle"
	"github.com/david-truong/liferay-portal-cli/internal/logrun"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/spf13/cobra"
)

var buildLangCmd = &cobra.Command{
	Use:     "build-lang",
	Aliases: []string{"bl"},
	Short:   "Build portal language files",
	Long: `Runs "gw buildLang" in modules/apps/portal-language/portal-language-lang.

This is the canonical location for portal-wide language file generation.
All invocations work from the portal root — no cd required.

Example:
  liferay build-lang`,
	Args: cobra.NoArgs,
	RunE: runBuildLang,
}

func init() {
	rootCmd.AddCommand(buildLangCmd)
}

func runBuildLang(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	portalRoot, err := portal.FindRoot(cwd)
	if err != nil {
		return err
	}

	moduleDir := filepath.Join(portalRoot, "modules", "apps", "portal-language", "portal-language-lang")

	gwCmd, err := gradle.Command(moduleDir, "buildLang")
	if err != nil {
		return err
	}
	return logrun.Run(gwCmd, logrun.Options{Label: "build-lang", Verbose: verbose, WorktreeRoot: portalRoot})
}
