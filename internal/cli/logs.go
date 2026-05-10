package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/state"
	"github.com/spf13/cobra"
)

var (
	logsTail int
	logsGrep string
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print the most recent CLI command's output",
	Long: `Routes to whichever subsystem ran most recently in this worktree:
- build/test/deploy/etc: prints the saved transcript (under
  ~/.liferay-cli/worktrees/<id>/logs/)
- server: live-tails tomcat-*/logs/catalina.out
- db: live-tails docker compose logs for the last service used

--tail and --grep apply to all three. For live tails, --tail sets the initial
backlog before following.`,
	Args: cobra.NoArgs,
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().IntVar(&logsTail, "tail", 0, "Print only the last N lines (live tails: initial backlog before following)")
	logsCmd.Flags().StringVar(&logsGrep, "grep", "", "Only print lines matching this regex")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(_ *cobra.Command, _ []string) error {
	worktreeRoot, err := findWorktreeRoot()
	if err != nil {
		return err
	}

	var grepRE *regexp.Regexp
	if logsGrep != "" {
		grepRE, err = regexp.Compile(logsGrep)
		if err != nil {
			return fmt.Errorf("invalid --grep regex: %w", err)
		}
	}

	rec, ok, err := state.LoadLastCmd(worktreeRoot)
	if err != nil {
		return err
	}
	if !ok {
		return showNewestArchive(worktreeRoot, grepRE, logsTail)
	}

	switch rec.Kind {
	case state.LastCmdArchive:
		return showArchive(rec.LogPath, grepRE, logsTail)
	case state.LastCmdServer:
		return tailServer(rec.LogPath, grepRE, logsTail)
	case state.LastCmdDB:
		service := rec.Service
		if service == "" {
			service = "db"
		}
		return tailDB(worktreeRoot, service, grepRE, logsTail)
	default:
		return fmt.Errorf("unknown last-command kind %q in %s", rec.Kind,
			filepath.Join(state.Dir(worktreeRoot), "last_command.json"))
	}
}

func showNewestArchive(worktreeRoot string, grepRE *regexp.Regexp, tail int) error {
	logDir := filepath.Join(state.Dir(worktreeRoot), "logs")
	path, err := newestLog(logDir)
	if err != nil {
		return err
	}
	return showArchive(path, grepRE, tail)
}

func showArchive(path string, grepRE *regexp.Regexp, tail int) error {
	announce("archive", path)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if grepRE == nil && tail <= 0 {
		_, err := io.Copy(os.Stdout, f)
		return err
	}
	return scanFiltered(f, grepRE, tail)
}

func tailServer(catOut string, grepRE *regexp.Regexp, tail int) error {
	if catOut == "" {
		return fmt.Errorf("no catalina.out path stored for the last server command")
	}
	if _, err := os.Stat(catOut); err != nil {
		return fmt.Errorf("no catalina.out at %s (server has not been started)", catOut)
	}
	announce("server", catOut)

	args := []string{}
	if tail > 0 {
		args = append(args, "-n", strconv.Itoa(tail))
	}
	args = append(args, "-F", catOut)
	cmd := exec.Command("tail", args...)
	return streamCommand(cmd, grepRE)
}

func tailDB(worktreeRoot, service string, grepRE *regexp.Regexp, tail int) error {
	if err := requireDockerEngine(worktreeRoot); err != nil {
		return err
	}
	dockerState, _ := docker.LoadState(worktreeRoot)
	announce("db", fmt.Sprintf("docker compose logs %s (slot %d)", service, dockerState.Slot))

	composeArgs := []string{
		"compose",
		"-p", fmt.Sprintf("liferay-slot-%d", dockerState.Slot),
		"-f", docker.ComposePath(worktreeRoot),
		"logs",
		"-f",
	}
	if tail > 0 {
		composeArgs = append(composeArgs, "--tail", strconv.Itoa(tail))
	}
	composeArgs = append(composeArgs, service)
	cmd := exec.Command("docker", composeArgs...)
	return streamCommand(cmd, grepRE)
}

// streamCommand runs cmd, piping its stdout through grepRE (if set) to our
// stdout. stderr passes through directly. Returns when the command exits or
// stdout closes.
func streamCommand(cmd *exec.Cmd, grepRE *regexp.Regexp) error {
	if grepRE == nil {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = nil
		return cmd.Run()
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanErr := scanFiltered(stdout, grepRE, 0)
	waitErr := cmd.Wait()
	if scanErr != nil {
		return scanErr
	}
	return waitErr
}

// scanFiltered reads r line-by-line, applies grepRE (if set), and prints the
// result. When tail > 0, only the last `tail` matching lines are kept (ring
// buffer). For live streams pass tail == 0 so output flushes immediately.
func scanFiltered(r io.Reader, grepRE *regexp.Regexp, tail int) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	if tail > 0 {
		ring := make([]string, 0, tail)
		for scanner.Scan() {
			line := scanner.Text()
			if grepRE != nil && !grepRE.MatchString(line) {
				continue
			}
			if len(ring) == tail {
				ring = ring[1:]
			}
			ring = append(ring, line)
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		for _, line := range ring {
			fmt.Println(line)
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if grepRE != nil && !grepRE.MatchString(line) {
			continue
		}
		fmt.Println(line)
	}
	return scanner.Err()
}

func announce(kind, target string) {
	fmt.Fprintf(os.Stderr, "[logs: %s — %s]\n", kind, state.DisplayHome(target))
}

func newestLog(logDir string) (string, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no recent CLI runs (no log dir at %s)", logDir)
		}
		return "", err
	}
	var newest string
	var newestMod int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		t := info.ModTime().UnixNano()
		if t > newestMod || (t == newestMod && e.Name() > newest) {
			newestMod = t
			newest = e.Name()
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no log files in %s", logDir)
	}
	return filepath.Join(logDir, newest), nil
}
