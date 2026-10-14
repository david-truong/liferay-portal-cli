package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/david-truong/liferay-portal-cli/internal/dashboard"
	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/fsutil"
	"github.com/david-truong/liferay-portal-cli/internal/hosts"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/david-truong/liferay-portal-cli/internal/state"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Terminal dashboard for every worktree: status, Jira, logs, one-key server control",
	Long: `Opens a full-screen terminal UI with one tab per Liferay project in the git
worktrees of the current repository: each worktree root that is a Liferay
project, plus each Liferay Workspace under its workspaces/ directory that
liferay has run in or that the dashboard was launched from. Each tab shows the
project's slot, Tomcat and database status, its Jira ticket (parsed from the
branch name), and a catalina.out tail, with single-key server
start/stop/restart and open-in-browser.

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
	// Launching from a directory that is no Liferay project, such as
	// liferay-portal-ee's sparse master-private checkout, only loses the
	// initial tab selection.
	worktreeRoot, rootErr := findWorktreeRoot()

	worktrees, active, err := discoverWorktrees(worktreeRoot)
	if err != nil {
		if rootErr != nil {
			return ExitErr(ExitNotInPortal, "not inside a Liferay worktree: %w", rootErr)
		}
		return err
	}

	primaryRoot, err := gitPrimaryRoot("")
	if err != nil {
		return ExitErr(ExitGeneric, "%w", err)
	}

	selfExe, err := os.Executable()
	if err != nil {
		return ExitErr(ExitGeneric, "resolving liferay binary: %w", err)
	}

	return dashboard.Run(dashboard.Config{
		Worktrees:   worktrees,
		Active:      active,
		PrimaryRoot: primaryRoot,
		SelfExe:     selfExe,
		Reload: func() []dashboard.Worktree {
			worktrees, _, err := discoverWorktrees(worktreeRoot)
			if err != nil {
				return nil
			}
			return worktrees
		},
	})
}

// discoverWorktrees enumerates the repository's Liferay projects (see
// dashboardProjects) and resolves each one's slot, database engine, feature
// flags, and slot hostname. active is the index of worktreeRoot in the
// returned slice (0 when not found). The dashboard runs this once at startup
// and again on every refresh, since a slot or engine can be claimed after
// launch.
func discoverWorktrees(worktreeRoot string) ([]dashboard.Worktree, int, error) {
	porcelain, err := gitOutput("worktree", "list", "--porcelain")
	if err != nil {
		return nil, 0, ExitErr(ExitGeneric, "listing worktrees: %w", err)
	}
	primary, _ := gitPrimaryRoot("")
	worktrees := dashboardProjects(parseWorktreePorcelain(porcelain, primary), worktreeRoot)
	if len(worktrees) == 0 {
		return nil, 0, ExitErr(ExitGeneric, "no Liferay projects found in any worktree")
	}

	hostsContent := readHostsFile()

	active := 0

	for i := range worktrees {
		w := &worktrees[i]
		if w.Path == worktreeRoot {
			active = i
		}
		w.Slot = readPersistedSlot(w.Path)
		w.Ticket = dashboard.TicketKey(w.Branch)

		// A workspace runs a released bundle, not the branch's build, so
		// the branch's feature flags do not apply to it.
		if !w.Workspace {
			w.Flags = dashboard.BranchFlags(w.Path)
		}

		if st, ok := docker.LoadState(w.Path); ok {
			w.Engine = st.Engine
		}
		if w.Slot >= 0 {
			w.Hostname = hosts.SlotHostname(hostsContent, w.Slot)
		} else {
			w.Hostname = hosts.SlotHostname(hostsContent, 0)
		}
	}

	return worktrees, active, nil
}

// dashboardProjects expands git worktrees into the Liferay projects the
// dashboard shows as tabs: each worktree root that is itself a Liferay
// project (liferay-portal-ee's sparse master-private checkout is not),
// followed by the Liferay Workspaces nested in it that the CLI has state for
// or that the dashboard was launched from. liferay-portal alone carries
// dozens of workspaces, most of which are never run.
func dashboardProjects(entries []worktreeEntry, launchRoot string) []dashboard.Worktree {
	var worktrees []dashboard.Worktree
	for _, e := range entries {
		if portal.DetectProjectType(e.Path) != portal.ProjectTypeUnknown {
			worktrees = append(worktrees, dashboard.Worktree{
				Path:    e.Path,
				Branch:  e.Branch,
				Primary: e.Primary,
			})
		}
		for _, workspace := range portal.NestedWorkspaces(e.Path) {
			if workspace != launchRoot && !fsutil.Exists(state.Dir(workspace)) {
				continue
			}
			worktrees = append(worktrees, dashboard.Worktree{
				Path:      workspace,
				Branch:    e.Branch,
				Workspace: true,
			})
		}
	}
	return worktrees
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
