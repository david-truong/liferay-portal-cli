package cli

import "github.com/spf13/cobra"

var buildRESTCmd = &cobra.Command{
	Use:     "build-rest <module>",
	Aliases: []string{"br"},
	Short:   "Run REST Builder for a Liferay module",
	Long: `Resolves the module by name and runs "gw buildREST" in its directory.
The module must contain a rest-config.yaml file.

All invocations work from the portal root — no cd required.

Examples:
  liferay build-rest headless-delivery-impl
  liferay build-rest headless-delivery/headless-delivery-impl`,
	Args: cobra.ExactArgs(1),
	RunE: runBuildREST,
}

func init() {
	rootCmd.AddCommand(buildRESTCmd)
}

func runBuildREST(cmd *cobra.Command, args []string) error {
	return runBuilder(args[0], builderSpec{
		requiredFile: "rest-config.yaml",
		moduleKind:   "REST impl module",
		gradleTask:   "buildREST",
		labelPrefix:  "build-rest-",
	})
}
