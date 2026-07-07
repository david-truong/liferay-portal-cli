package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/hosts"
	"github.com/david-truong/liferay-portal-cli/internal/state"
	"github.com/spf13/cobra"
)

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "Manage a friendly /etc/hosts name for this worktree (maps to 127.0.0.1)",
	Long: `Adds, removes, or lists liferay-cli-managed entries in /etc/hosts.

Each entry maps a hostname to 127.0.0.1 so you can browse a worktree at, e.g.,
http://lpd-12345:8090 instead of http://localhost:8090. The hostname is a label
— every worktree still resolves to loopback, so the per-slot Tomcat port is what
distinguishes instances. Managed lines carry a trailing "# liferay-cli <id>"
marker so add/remove stay idempotent and never disturb other entries.

Editing /etc/hosts needs root. When run without write permission these commands
print an idempotent sudo one-liner you can paste, instead of writing the file.`,
}

var hostsAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Map a hostname to 127.0.0.1 for this worktree (default name: the worktree directory)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runHostsAdd,
}

var hostsRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove this worktree's managed /etc/hosts entry",
	Args:  cobra.NoArgs,
	RunE:  runHostsRemove,
}

var hostsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all liferay-cli-managed /etc/hosts entries",
	Args:  cobra.NoArgs,
	RunE:  runHostsList,
}

func init() {
	hostsCmd.AddCommand(hostsAddCmd)
	hostsCmd.AddCommand(hostsRemoveCmd)
	hostsCmd.AddCommand(hostsListCmd)
	rootCmd.AddCommand(hostsCmd)
}

func runHostsAdd(_ *cobra.Command, args []string) error {
	rerootHomeForSudo()

	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return ExitErr(ExitNotInPortal, "not inside a Liferay worktree: %w", err)
	}
	id := state.ID(worktreeRoot)

	name := hosts.Sanitize(filepath.Base(worktreeRoot))
	if len(args) == 1 {
		name = args[0]
	}
	if err := hosts.ValidateName(name); err != nil {
		return ExitErr(ExitGeneric, "%w", err)
	}

	content := readHostsFile()
	updated, err := hosts.Upsert(content, name, id)
	if err != nil {
		return ExitErr(ExitGeneric, "%w", err)
	}

	port := tomcatPortFor(worktreeRoot)

	if updated == content {
		fmt.Printf("Already mapped: http://%s:%d\n", name, port)
		return nil
	}

	if err := writeHostsFile(updated); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			printSudoHint(
				fmt.Sprintf("Editing %s needs root. Run:\n\n  sudo sh -c \"sed -i.bak '/%s %s$/d' %s && printf '127.0.0.1\\t%s\\t%s %s\\n' >> %s\"\n\nThen browse http://%s:%d",
					hosts.Path,
					hostsMarker(), id, hosts.Path,
					name, hostsMarker(), id, hosts.Path,
					name, port))
			return ExitErr(ExitGeneric, "could not write %s without root", hosts.Path)
		}
		return ExitErr(ExitGeneric, "writing %s: %w", hosts.Path, err)
	}

	fmt.Printf("Mapped %s -> 127.0.0.1\nBrowse: http://%s:%d\n", name, name, port)
	return nil
}

func runHostsRemove(_ *cobra.Command, _ []string) error {
	rerootHomeForSudo()

	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return ExitErr(ExitNotInPortal, "not inside a Liferay worktree: %w", err)
	}
	id := state.ID(worktreeRoot)

	content := readHostsFile()
	updated, removed := hosts.Remove(content, id)
	if !removed {
		fmt.Printf("No managed entry for this worktree (%s)\n", id)
		return nil
	}

	if err := writeHostsFile(updated); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			printSudoHint(
				fmt.Sprintf("Editing %s needs root. Run:\n\n  sudo sed -i.bak '/%s %s$/d' %s",
					hosts.Path, hostsMarker(), id, hosts.Path))
			return ExitErr(ExitGeneric, "could not write %s without root", hosts.Path)
		}
		return ExitErr(ExitGeneric, "writing %s: %w", hosts.Path, err)
	}

	fmt.Printf("Removed managed entry for %s\n", id)
	return nil
}

func runHostsList(_ *cobra.Command, _ []string) error {
	entries := hosts.List(readHostsFile())
	if len(entries) == 0 {
		fmt.Println("No liferay-cli-managed /etc/hosts entries")
		return nil
	}
	for _, e := range entries {
		fmt.Printf("%s\t%s\n", e.Name, e.ID)
	}
	return nil
}

// tomcatPortFor returns the worktree's slot Tomcat HTTP port, falling back to
// the stock slot-0 port when no slot has been allocated yet (the worktree has
// never run `liferay db start` / `liferay server start`).
func tomcatPortFor(worktreeRoot string) int {
	slot := 0
	if st, ok := docker.LoadState(worktreeRoot); ok {
		slot = st.Slot
	}
	return docker.PortsFromSlot(slot).TomcatHTTP
}

func readHostsFile() string {
	data, err := os.ReadFile(hosts.Path)
	if err != nil {
		return ""
	}
	return string(data)
}

// writeHostsFile replaces /etc/hosts content.
func writeHostsFile(content string) error {
	return writeHostsFileAt(hosts.Path, content)
}

// writeHostsFileAt atomically replaces path's content, preserving the
// existing file's mode. Split out from writeHostsFile so tests can exercise
// the write logic against a temp file instead of the real /etc/hosts. A
// torn write to /etc/hosts breaks name resolution system-wide, so this goes
// through state.WriteFileAtomic (temp file + rename) rather than
// os.WriteFile.
func writeHostsFileAt(path, content string) error {
	return state.WriteFileAtomic(path, []byte(content), 0644)
}

func printSudoHint(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

// hostsMarker returns the literal marker comment used in managed lines.
func hostsMarker() string { return "# liferay-cli" }

// rerootHomeForSudo points $HOME at the invoking user's home when the command
// is run via sudo, so per-worktree state (slot/port) resolves under the real
// user's ~/.liferay-cli rather than root's home.
func rerootHomeForSudo() {
	if os.Geteuid() != 0 {
		return
	}
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" || sudoUser == "root" {
		return
	}
	if u, err := user.Lookup(sudoUser); err == nil && u.HomeDir != "" {
		os.Setenv("HOME", u.HomeDir)
	}
}
