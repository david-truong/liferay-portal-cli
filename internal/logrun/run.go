// Package logrun executes a command while redirecting its output to a temp
// log file, keeping agent context clean. On failure the tail of the log is
// surfaced so the caller sees the error without scrolling through the full
// build transcript.
package logrun

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Options configures a single Run invocation.
type Options struct {
	// Label is used in log filename and status lines, e.g. "build" or "test".
	Label string
	// Verbose, when true, tees output to the terminal in addition to the log
	// file and disables the failure-tail dump (since the user already saw it).
	Verbose bool
	// TailLines is the number of trailing log lines to print on failure when
	// not running verbose. Zero means use the default (120).
	TailLines int
	// WorktreeRoot, when non-empty, places logs under
	// <root>/.liferay-cli/logs/. Empty falls back to os.TempDir().
	WorktreeRoot string
}

const defaultTailLines = 120

// Run executes cmd, capturing stdout+stderr to a temp log file. cmd.Stdout,
// cmd.Stderr, and cmd.Stdin must not already be set — Run owns them.
func Run(cmd *exec.Cmd, opts Options) error {
	if opts.Label == "" {
		opts.Label = "run"
	}
	if opts.TailLines <= 0 {
		opts.TailLines = defaultTailLines
	}

	logPath, err := newLogPath(opts.Label, opts.WorktreeRoot)
	if err != nil {
		return err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("creating log file: %w", err)
	}
	defer logFile.Close()

	var out, errOut io.Writer = logFile, logFile
	if opts.Verbose {
		out = io.MultiWriter(os.Stdout, logFile)
		errOut = io.MultiWriter(os.Stderr, logFile)
	}
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.Stdin = nil

	fmt.Fprintf(os.Stderr, "[%s] running (log: %s)\n", opts.Label, logPath)
	start := time.Now()
	runErr := cmd.Run()
	dur := time.Since(start).Round(time.Second)

	if runErr != nil {
		if !opts.Verbose {
			printTail(logPath, opts.TailLines)
		}
		fmt.Fprintf(os.Stderr, "[%s] FAILED in %s (log: %s)\n", opts.Label, dur, logPath)
		return runErr
	}
	fmt.Fprintf(os.Stderr, "[%s] OK in %s (log: %s)\n", opts.Label, dur, logPath)
	return nil
}

func newLogPath(label, worktreeRoot string) (string, error) {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return '-'
	}, label)
	name := fmt.Sprintf("%s-%s.log", safe, time.Now().Format("20060102-150405"))

	dir := os.TempDir()
	if worktreeRoot != "" {
		dir = filepath.Join(worktreeRoot, ".liferay-cli", "logs")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("creating log dir: %w", err)
		}
	} else {
		name = "liferay-cli-" + name
	}
	return filepath.Join(dir, name), nil
}

// printTail writes the last n lines of logPath to stderr, framed so the
// failure block stands out in agent transcripts.
func printTail(logPath string, n int) {
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer f.Close()

	ring := make([]string, 0, n)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		if len(ring) == n {
			ring = ring[1:]
		}
		ring = append(ring, scanner.Text())
	}

	fmt.Fprintf(os.Stderr, "----- last %d lines of %s -----\n", len(ring), logPath)
	for _, line := range ring {
		fmt.Fprintln(os.Stderr, line)
	}
	fmt.Fprintln(os.Stderr, "----- end log tail -----")
}
