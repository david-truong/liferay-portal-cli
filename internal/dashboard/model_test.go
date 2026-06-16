package dashboard

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestInitialActiveTab(t *testing.T) {
	cfg := testModel().cfg

	cfg.Active = 1
	if m := newModel(cfg); m.active != 1 {
		t.Errorf("active = %d, want 1", m.active)
	}

	cfg.Active = 5
	if m := newModel(cfg); m.active != 0 {
		t.Errorf("out-of-range active = %d, want 0", m.active)
	}
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
	next, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
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

	done := runBatch(t, cmd)
	if done.err != nil {
		t.Fatalf("stub command failed: %v", done.err)
	}
	data, err := os.ReadFile(run.logPath)
	if err != nil {
		t.Fatalf("reading command log: %v", err)
	}
	if got := strings.TrimSpace(string(data)); !strings.HasSuffix(got, "-C /w/LPD-1 build foo-web") {
		t.Errorf("command log = %q", got)
	}
	os.Remove(run.logPath)
}

// runBatch executes every command in a tea.Batch result and returns the
// cmdDoneMsg it produces.
func runBatch(t *testing.T, cmd tea.Cmd) cmdDoneMsg {
	t.Helper()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected a tea.BatchMsg")
	}
	for _, c := range batch {
		if done, ok := c().(cmdDoneMsg); ok {
			return done
		}
	}
	t.Fatal("no cmdDoneMsg in batch")
	return cmdDoneMsg{}
}

func TestResetSequence(t *testing.T) {
	m := testModel()
	m.cfg.SelfExe = "/bin/echo"
	m.active = 1

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = next.(model)

	run := m.runs[1]
	if !run.running || run.logPath == "" {
		t.Fatalf("reset not started: %+v", run)
	}

	done := runBatch(t, cmd)
	if done.err != nil {
		t.Fatalf("reset sequence failed: %v", done.err)
	}
	data, err := os.ReadFile(run.logPath)
	if err != nil {
		t.Fatalf("reading reset log: %v", err)
	}
	log := string(data)
	for _, want := range []string{
		"$ liferay server wipe --yes",
		"$ liferay db restart",
		"$ liferay server start",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("reset log missing %q:\n%s", want, log)
		}
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

func TestRefreshReloadsSlotInfo(t *testing.T) {
	m := testModel()
	m.cfg.Reload = func() []Worktree {
		return []Worktree{
			{Path: "/w/master", Branch: "master", Slot: 0, Primary: true},
			// The second worktree has since claimed a slot and an engine.
			{Path: "/w/LPD-1", Branch: "LPD-1", Slot: 3, Engine: "postgres", Ticket: "LPD-1"},
		}
	}

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})

	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected a tea.BatchMsg from refresh")
	}
	var reloaded worktreesMsg
	for _, c := range batch {
		if msg, ok := c().(worktreesMsg); ok {
			reloaded = msg
		}
	}
	if reloaded == nil {
		t.Fatal("refresh did not issue a worktrees reload")
	}

	next, _ := m.Update(reloaded)
	m = next.(model)

	if got := m.cfg.Worktrees[1].Slot; got != 3 {
		t.Errorf("slot = %d, want 3 after reload", got)
	}
	if got := m.cfg.Worktrees[1].Engine; got != "postgres" {
		t.Errorf("engine = %q, want postgres after reload", got)
	}
	if got := len(m.cfg.Worktrees); got != 2 {
		t.Errorf("tab count = %d, want 2 (alignment preserved)", got)
	}
}

func TestRefreshDropsVanishedTab(t *testing.T) {
	m := testModel()
	m.active = 1 // on LPD-1
	m.statuses = make([]Status, len(m.cfg.Worktrees))

	// LPD-1's worktree is gone; only master comes back from discovery.
	m.mergeWorktrees([]Worktree{
		{Path: "/w/master", Branch: "master", Slot: 0, Primary: true},
	})

	if got := len(m.cfg.Worktrees); got != 1 {
		t.Fatalf("worktree count = %d, want 1 after the tab vanished", got)
	}
	if m.cfg.Worktrees[0].Branch != "master" {
		t.Errorf("surviving tab = %q, want master", m.cfg.Worktrees[0].Branch)
	}
	for _, n := range []int{len(m.statuses), len(m.action), len(m.note), len(m.runs), len(m.logSrc)} {
		if n != 1 {
			t.Fatalf("per-tab slices not realigned: got length %d", n)
		}
	}
	if m.active != 0 {
		t.Errorf("active = %d, want 0 after its tab was dropped", m.active)
	}
}

func TestRefreshWithoutReloadHook(t *testing.T) {
	m := testModel()

	if _, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}); cmd == nil {
		t.Fatal("refresh with no Reload hook did nothing")
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	m := testModel()
	m.active = 1 // the non-primary LPD-1 worktree

	// ctrl+d only arms the confirmation; nothing is deleted yet.
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = next.(model)
	if !m.confirmDelete {
		t.Fatal("ctrl+d did not arm the delete confirmation")
	}
	if cmd != nil {
		t.Fatal("ctrl+d issued a command before confirmation")
	}
	if m.action[1] != "" {
		t.Fatal("ctrl+d marked the tab busy before confirmation")
	}

	// A non-"y" key cancels without deleting.
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(model)
	if m.confirmDelete {
		t.Fatal("'n' did not cancel the confirmation")
	}
	if m.action[1] != "" {
		t.Fatal("'n' started a delete")
	}
}

func TestDeleteConfirmedRunsRemoval(t *testing.T) {
	m := testModel()
	m.cfg.SelfExe = "/bin/echo"
	m.active = 1

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = next.(model)
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = next.(model)

	if m.confirmDelete {
		t.Fatal("'y' did not close the confirmation")
	}
	if m.action[1] != "delete" {
		t.Fatalf("tab not marked deleting: %q", m.action[1])
	}
	if cmd == nil {
		t.Fatal("'y' did not launch the removal command")
	}

	msg, ok := cmd().(deleteDoneMsg)
	if !ok {
		t.Fatalf("expected a deleteDoneMsg, got %T", cmd())
	}
	if msg.err != nil {
		t.Fatalf("stub removal failed: %v", msg.err)
	}

	next, _ = m.Update(msg)
	m = next.(model)
	if got := len(m.cfg.Worktrees); got != 1 {
		t.Fatalf("worktree count = %d, want 1 after removal", got)
	}
	for _, slice := range [][]int{{len(m.statuses)}, {len(m.action)}, {len(m.note)}, {len(m.logSrc)}} {
		if slice[0] != 1 {
			t.Fatalf("per-tab slices not realigned after removal: %d", slice[0])
		}
	}
	if m.cfg.Worktrees[0].Branch != "master" {
		t.Errorf("surviving tab = %q, want master", m.cfg.Worktrees[0].Branch)
	}
	if m.active != 0 {
		t.Errorf("active = %d, want 0 after removing the last tab", m.active)
	}
}

func TestDeletePrimaryBlocked(t *testing.T) {
	m := testModel()
	m.active = 0 // master is primary

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = next.(model)
	if m.confirmDelete {
		t.Fatal("ctrl+d armed deletion of the primary worktree")
	}
	if m.note[0] == "" {
		t.Fatal("no explanation shown when blocking primary deletion")
	}
}

func TestWrapTabsBreaksOnNarrowWidth(t *testing.T) {
	tabs := []string{
		tabStyle.Render("alpha"),
		tabStyle.Render("bravo"),
		tabStyle.Render("charlie"),
	}

	// Wide enough for everything: one row.
	if got := strings.Count(wrapTabs(tabs, 200), "\n"); got != 0 {
		t.Errorf("wide layout has %d breaks, want 0", got)
	}

	// Narrow enough that not all tabs fit: at least one break, and every tab
	// survives the wrap.
	wrapped := wrapTabs(tabs, 12)
	if strings.Count(wrapped, "\n") == 0 {
		t.Error("narrow layout did not wrap")
	}
	for _, label := range []string{"alpha", "bravo", "charlie"} {
		if !strings.Contains(wrapped, label) {
			t.Errorf("wrapped tabs dropped %q:\n%s", label, wrapped)
		}
	}
}

func TestViewFitsTerminalHeight(t *testing.T) {
	m := testModel()
	m.logView = viewport.New(m.width-2, 5)
	m.logView.SetContent(strings.Repeat("log line\n", 500))
	m.jira["LPD-1"] = jiraResult{view: "LPD-1  Fix\n  Status: Open\n  URL: https://x"}
	m.note[0] = "server start done"

	for _, height := range []int{20, 30, 50} {
		m.height = height
		if got := lipgloss.Height(m.View()); got > height {
			t.Errorf("view is %d lines for a %d-line terminal", got, height)
		}
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
