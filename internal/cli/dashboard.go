package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/david-truong/liferay-portal-cli/internal/dashboard"
	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/hosts"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Terminal dashboard for every worktree: status, Jira, logs, one-key server control",
	Long: `Opens a full-screen terminal UI with one tab per git worktree of the current
repository. Each tab shows the worktree's slot, Tomcat and database status,
its Jira ticket (parsed from the branch name), and a catalina.out tail, with
single-key server start/stop/restart and open-in-browser.

Worktrees are reachable at stable per-slot hostnames (slot0.liferay.test,
slot1.liferay.test, ...) once the pool is installed via
"sudo liferay dashboard install-hosts".`,
	Args: cobra.NoArgs,
	RunE: runDashboard,
}

var dashboardInstallHostsCmd = &cobra.Command{
	Use:   "install-hosts",
	Short: "Precreate the per-slot /etc/hosts names (slot0.liferay.test ... slot9.liferay.test)",
	Args:  cobra.NoArgs,
	RunE:  runDashboardInstallHosts,
}

func init() {
	dashboardCmd.AddCommand(dashboardInstallHostsCmd)
	rootCmd.AddCommand(dashboardCmd)
}

func runDashboard(_ *cobra.Command, _ []string) error {
	if _, err := findWorktreeRoot(); err != nil {
		return ExitErr(ExitNotInPortal, "not inside a Liferay worktree: %w", err)
	}

	porcelain, err := gitOutput("worktree", "list", "--porcelain")
	if err != nil {
		return ExitErr(ExitGeneric, "listing worktrees: %w", err)
	}
	primary, _ := gitPrimaryRoot("")
	entries := parseWorktreePorcelain(porcelain, primary)
	if len(entries) == 0 {
		return ExitErr(ExitGeneric, "no worktrees found")
	}

	hostsContent := readHostsFile()

	worktrees := make([]dashboard.Worktree, 0, len(entries))
	for _, e := range entries {
		w := dashboard.Worktree{
			Path:    e.Path,
			Branch:  e.Branch,
			Slot:    e.Slot,
			Primary: e.Primary,
			Ticket:  dashboard.TicketKey(e.Branch),
		}
		if st, ok := docker.LoadState(e.Path); ok {
			w.Engine = st.Engine
		}
		if e.Slot >= 0 {
			w.Hostname = hosts.SlotHostname(hostsContent, e.Slot)
		} else {
			w.Hostname = hosts.SlotHostname(hostsContent, 0)
		}
		worktrees = append(worktrees, w)
	}

	selfExe, err := os.Executable()
	if err != nil {
		return ExitErr(ExitGeneric, "resolving liferay binary: %w", err)
	}

	return dashboard.Run(dashboard.Config{Worktrees: worktrees, SelfExe: selfExe})
}

func runDashboardInstallHosts(_ *cobra.Command, _ []string) error {
	content := readHostsFile()

	updated, err := hosts.UpsertSlotPool(content)
	if err != nil {
		return ExitErr(ExitGeneric, "%w", err)
	}
	if updated == content {
		fmt.Printf("Slot hostnames already installed (%s ... %s)\n",
			hosts.SlotName(0), hosts.SlotName(hosts.SlotPoolSize-1))
		return nil
	}

	if err := writeHostsFile(updated); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			printSudoHint(fmt.Sprintf(
				"Editing %s needs root. Run:\n\n  sudo liferay dashboard install-hosts",
				hosts.Path))
			return ExitErr(ExitGeneric, "could not write %s without root", hosts.Path)
		}
		return ExitErr(ExitGeneric, "writing %s: %w", hosts.Path, err)
	}

	fmt.Printf("Installed %d slot hostnames (%s ... %s)\n",
		hosts.SlotPoolSize, hosts.SlotName(0), hosts.SlotName(hosts.SlotPoolSize-1))
	return nil
}
