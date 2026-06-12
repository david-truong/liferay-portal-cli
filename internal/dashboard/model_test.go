package dashboard

import (
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
