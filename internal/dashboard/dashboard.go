// Package dashboard renders a full-screen terminal UI with one tab per
// Liferay worktree: live Tomcat/DB status, the Jira ticket behind the
// branch, a catalina.out tail, and single-key server actions. Actions shell
// out to the liferay binary itself (`liferay -C <worktree> server ...`) so
// they run exactly the code paths a human would.
package dashboard

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Worktree is one tab. The caller (the dashboard command) discovers these
// once at startup; status is re-probed continuously while the UI runs.
type Worktree struct {
	Path    string
	Branch  string
	Slot    int    // -1 when the worktree never claimed a slot (stock ports apply)
	Engine  string // "" until `liferay db start` has run there
	Primary bool
	Ticket  string // Jira key parsed from the branch name, "" when none
	// Flags are the feature flags this branch declares on top of master;
	// the dashboard enables them in portal-ext.properties before boots.
	Flags []string
	// Hostname is the slot-pool name from /etc/hosts (e.g. slot3.liferay.test),
	// "" when the pool is not installed — the UI then falls back to localhost.
	Hostname string
}

// Config carries everything Run needs that the model cannot derive itself.
type Config struct {
	Worktrees []Worktree
	// Active is the tab selected at startup — the worktree the dashboard
	// was launched from.
	Active int
	// SelfExe is the liferay binary to shell out to for server actions.
	SelfExe string
}

// Run blocks until the user quits the TUI.
func Run(cfg Config) error {
	program := tea.NewProgram(newModel(cfg), tea.WithAltScreen())

	_, err := program.Run()

	return err
}
