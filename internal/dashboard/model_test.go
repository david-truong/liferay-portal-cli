package dashboard

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testModel() model {
	m := newModel(Config{
		Worktrees: []Worktree{
			{Path: "/w/master", Branch: "master", Slot: 0, Primary: true},
			{Path: "/w/LPD-1", Branch: "LPD-1", Slot: 1, Engine: "mysql", Ticket: "LPD-1"},
		},
		SelfExe: "/bin/true",
	})
	m.width = 120
	m.height = 40
	m.ready = true
	return m
}

func TestTabSwitching(t *testing.T) {
	m := testModel()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if m.active != 1 {
		t.Fatalf("after tab, active = %d, want 1", m.active)
	}

	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(model)
	if m.active != 0 {
		t.Fatalf("after wrap, active = %d, want 0", m.active)
	}

	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(model)
	if m.active != 1 {
		t.Fatalf("after left, active = %d, want 1", m.active)
	}
}

func TestCommandPrompt(t *testing.T) {
	m := testModel()
	m.cfg.SelfExe = "/bin/echo"
	m.active = 1

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = next.(model)
	if !m.inputMode {
		t.Fatal("':' did not open the command prompt")
	}

	// 'q' must edit the input, not quit.
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(model)
	if !m.inputMode {
		t.Fatal("typing closed the prompt")
	}
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("'q' quit while typing a command")
		}
	}

	m.cmdInput.SetValue("build foo-web")
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.inputMode {
		t.Fatal("enter did not close the prompt")
	}
	run := m.runs[1]
	if !run.running || run.line != "build foo-web" || run.logPath == "" {
		t.Fatalf("run not started: %+v", run)
	}
	if m.logSrc[1] != srcCommand || !m.showLogs {
		t.Fatal("drawer did not open on command output")
	}

	if err := run.cmd.Wait(); err != nil {
		t.Fatalf("stub command failed: %v", err)
	}
	data, err := os.ReadFile(run.logPath)
	if err != nil {
		t.Fatalf("reading command log: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "-C /w/LPD-1 build foo-web" {
		t.Errorf("command args = %q", got)
	}
	os.Remove(run.logPath)
}

func TestEscCancelsPromptOnly(t *testing.T) {
	m := testModel()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = next.(model)

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.inputMode {
		t.Fatal("esc did not close the prompt")
	}
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("esc quit instead of closing the prompt")
		}
	}

	_, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc outside the prompt did nothing")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Fatal("esc outside the prompt did not quit")
	}
}

func TestViewShowsJiraBlock(t *testing.T) {
	m := testModel()
	m.active = 1
	m.jira["LPD-1"] = jiraResult{view: "LPD-1  Fix the thing\n  Status:   In Progress\n  URL:      https://x"}

	view := m.View()
	for _, want := range []string{"Fix the thing", "Status:   In Progress", "https://x"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}
