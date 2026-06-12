package dashboard

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/state"
)

// refreshEvery paces the probe/log tick. Probes run concurrently and are
// bounded well under one period, so ticks never pile up.
const refreshEvery = 2 * time.Second

type (
	tickMsg     struct{}
	statusesMsg []Status

	jiraMsg struct {
		key   string
		issue Issue
		err   error
	}

	actionDoneMsg struct {
		index int
		verb  string
		err   error
	}

	logMsg struct {
		index   int
		content string
		err     error
	}
)

type jiraResult struct {
	issue   Issue
	err     error
	loading bool
}

type model struct {
	cfg Config

	active   int
	statuses []Status
	jira     map[string]jiraResult

	// action holds a per-tab in-flight verb ("" when idle); note holds the
	// last action outcome shown in the panel.
	action []string
	note   []string

	showLogs bool
	logView  viewport.Model

	width  int
	height int
	ready  bool
}

func newModel(cfg Config) model {
	return model{
		cfg:      cfg,
		statuses: make([]Status, len(cfg.Worktrees)),
		jira:     map[string]jiraResult{},
		action:   make([]string, len(cfg.Worktrees)),
		note:     make([]string, len(cfg.Worktrees)),
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{probeCmd(m.cfg.Worktrees), tickCmd()}
	for key := range uniqueTickets(m.cfg.Worktrees) {
		cmds = append(cmds, jiraCmd(key))
	}
	return tea.Batch(cmds...)
}

func uniqueTickets(worktrees []Worktree) map[string]bool {
	tickets := map[string]bool{}
	for _, w := range worktrees {
		if w.Ticket != "" {
			tickets[w.Ticket] = true
		}
	}
	return tickets
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func probeCmd(worktrees []Worktree) tea.Cmd {
	return func() tea.Msg {
		return statusesMsg(probeAll(worktrees))
	}
}

func jiraCmd(key string) tea.Cmd {
	return func() tea.Msg {
		issue, err := FetchIssue(key)
		return jiraMsg{key: key, issue: issue, err: err}
	}
}

func actionCmd(selfExe string, index int, w Worktree, verb string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command(selfExe, "-C", w.Path, "server", verb).CombinedOutput()
		if err != nil {
			err = fmt.Errorf("server %s: %v\n%s", verb, err, lastLines(string(out), 3))
		}
		return actionDoneMsg{index: index, verb: verb, err: err}
	}
}

func logCmd(index int, catOut string) tea.Cmd {
	return func() tea.Msg {
		content, err := tailFile(catOut)
		return logMsg{index: index, content: content, err: err}
	}
}

func openCmd(url string) tea.Cmd {
	return func() tea.Msg {
		opener := "xdg-open"
		if runtime.GOOS == "darwin" {
			opener = "open"
		}
		exec.Command(opener, url).Start()
		return nil
	}
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.logView = viewport.New(msg.Width-2, m.logHeight())
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickMsg:
		cmds := []tea.Cmd{probeCmd(m.cfg.Worktrees), tickCmd()}
		if m.showLogs {
			if catOut := m.statuses[m.active].CatOut; catOut != "" {
				cmds = append(cmds, logCmd(m.active, catOut))
			}
		}
		return m, tea.Batch(cmds...)

	case statusesMsg:
		m.statuses = msg
		return m, nil

	case jiraMsg:
		m.jira[msg.key] = jiraResult{issue: msg.issue, err: msg.err}
		return m, nil

	case actionDoneMsg:
		m.action[msg.index] = ""
		if msg.err != nil {
			m.note[msg.index] = msg.err.Error()
		} else {
			m.note[msg.index] = fmt.Sprintf("server %s done", msg.verb)
		}
		return m, probeCmd(m.cfg.Worktrees)

	case logMsg:
		if msg.index != m.active || !m.showLogs {
			return m, nil
		}
		if msg.err != nil {
			m.logView.SetContent("cannot read log: " + msg.err.Error())
			return m, nil
		}
		atBottom := m.logView.AtBottom()
		m.logView.SetContent(msg.content)
		if atBottom {
			m.logView.GotoBottom()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.logView, cmd = m.logView.Update(msg)
	return m, cmd
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	w := m.cfg.Worktrees[m.active]

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "right", "tab":
		m.active = (m.active + 1) % len(m.cfg.Worktrees)
		return m.afterTabSwitch()

	case "left", "shift+tab":
		m.active = (m.active - 1 + len(m.cfg.Worktrees)) % len(m.cfg.Worktrees)
		return m.afterTabSwitch()

	case "o":
		return m, openCmd(m.portalURL(w))

	case "s", "x", "r":
		if m.action[m.active] != "" {
			return m, nil
		}
		verb := map[string]string{"s": "start", "x": "stop", "r": "restart"}[msg.String()]
		m.action[m.active] = verb
		m.note[m.active] = ""
		return m, actionCmd(m.cfg.SelfExe, m.active, w, verb)

	case "l":
		m.showLogs = !m.showLogs
		if m.showLogs {
			m.logView.Height = m.logHeight()
			if catOut := m.statuses[m.active].CatOut; catOut != "" {
				return m, logCmd(m.active, catOut)
			}
		}
		return m, nil

	case "u":
		cmds := []tea.Cmd{probeCmd(m.cfg.Worktrees)}
		if w.Ticket != "" {
			m.jira[w.Ticket] = jiraResult{loading: true}
			cmds = append(cmds, jiraCmd(w.Ticket))
		}
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.logView, cmd = m.logView.Update(msg)
	return m, cmd
}

func (m model) afterTabSwitch() (tea.Model, tea.Cmd) {
	if !m.showLogs {
		return m, nil
	}
	m.logView.SetContent("")
	if catOut := m.statuses[m.active].CatOut; catOut != "" {
		return m, logCmd(m.active, catOut)
	}
	return m, nil
}

// portalURL prefers the slot-pool hostname so virtual-instance cookies and
// hostnames behave like a real deployment; localhost is the fallback when
// the pool is not installed.
func (m model) portalURL(w Worktree) string {
	host := w.Hostname
	if host == "" {
		host = "localhost"
	}
	ports := docker.PortsFromSlot(effectiveSlot(w))
	return fmt.Sprintf("http://%s:%d/", host, ports.TomcatHTTP)
}

// logHeight reserves the top of the screen for tabs + panel + footer.
func (m model) logHeight() int {
	h := m.height - 14
	if h < 5 {
		h = 5
	}
	return h
}

var (
	tabStyle = lipgloss.NewStyle().Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().Padding(0, 1).
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("57"))

	labelStyle = lipgloss.NewStyle().Bold(true).Width(8)

	readyDot    = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
	startingDot = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("●")
	staleDot    = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("●")
	stoppedDot  = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("○")

	noteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

func statusDot(st Status) string {
	switch st.Tomcat {
	case TomcatReady:
		return readyDot
	case TomcatStarting:
		return startingDot
	case TomcatStale:
		return staleDot
	default:
		return stoppedDot
	}
}

func statusWord(st Status) string {
	switch st.Tomcat {
	case TomcatReady:
		return "ready"
	case TomcatStarting:
		return "starting"
	case TomcatStale:
		return "stale pid"
	default:
		return "stopped"
	}
}

func (m model) View() string {
	if !m.ready {
		return "loading..."
	}

	var b strings.Builder

	b.WriteString(m.viewTabs())
	b.WriteString("\n\n")
	b.WriteString(m.viewPanel())

	if m.showLogs {
		st := m.statuses[m.active]
		title := st.CatOut
		if title == "" {
			title = "no bundle log"
		}
		b.WriteString("\n" + dimStyle.Render("── "+title+" ") + "\n")
		b.WriteString(m.logView.View())
	}

	b.WriteString("\n" + dimStyle.Render(
		"←/→ tabs · o open · s start · x stop · r restart · l logs · u refresh · q quit"))

	return b.String()
}

func (m model) viewTabs() string {
	tabs := make([]string, len(m.cfg.Worktrees))
	for i, w := range m.cfg.Worktrees {
		label := statusDot(m.statuses[i]) + " " + tabLabel(w)
		if i == m.active {
			tabs[i] = activeTabStyle.Render(label)
		} else {
			tabs[i] = tabStyle.Render(label)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func tabLabel(w Worktree) string {
	if w.Branch != "" {
		return w.Branch
	}
	return filepath.Base(w.Path)
}

func (m model) viewPanel() string {
	w := m.cfg.Worktrees[m.active]
	st := m.statuses[m.active]
	ports := docker.PortsFromSlot(effectiveSlot(w))

	var b strings.Builder

	line := func(label, value string) {
		b.WriteString(labelStyle.Render(label) + value + "\n")
	}

	slot := fmt.Sprintf("%d", w.Slot)
	if w.Slot < 0 {
		slot = "unclaimed (stock ports)"
	}
	if w.Primary {
		slot += " · primary"
	}
	line("Slot", slot)
	line("Path", state.DisplayHome(w.Path))

	tomcatValue := statusDot(st) + " " + statusWord(st)
	if st.PID > 0 && st.Tomcat != TomcatStopped {
		tomcatValue += fmt.Sprintf(" (pid %d)", st.PID)
	}
	if st.Err != nil {
		tomcatValue = stoppedDot + " " + st.Err.Error()
	}
	tomcatValue += dimStyle.Render(fmt.Sprintf("   http %d · jpda %d", ports.TomcatHTTP, ports.JPDA))
	line("Tomcat", tomcatValue)

	line("DB", m.viewDB(w, st, ports))
	line("URL", m.portalURL(w))
	line("Jira", m.viewJira(w))

	if verb := m.action[m.active]; verb != "" {
		b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("server %s in progress...", verb)) + "\n")
	} else if note := m.note[m.active]; note != "" {
		b.WriteString("\n" + noteStyle.Render(note) + "\n")
	}

	if w.Hostname == "" {
		b.WriteString("\n" + dimStyle.Render(
			"slot hostnames not installed — run: sudo liferay dashboard install-hosts") + "\n")
	}

	return b.String()
}

func (m model) viewDB(w Worktree, st Status, ports docker.Ports) string {
	if w.Engine == "" {
		return stoppedDot + " not configured"
	}
	if !docker.IsDockerManagedEngine(w.Engine) {
		return readyDot + " " + w.Engine + " (embedded)"
	}
	dot := stoppedDot
	word := "stopped"
	if st.DBUp {
		dot = readyDot
		word = "up"
	}
	return dot + fmt.Sprintf(" %s %s", w.Engine, word) +
		dimStyle.Render(fmt.Sprintf("   db %d · adminer %d", ports.MySQL, ports.Adminer))
}

func (m model) viewJira(w Worktree) string {
	if w.Ticket == "" {
		return dimStyle.Render("no ticket on branch")
	}

	result, ok := m.jira[w.Ticket]
	switch {
	case !ok || result.loading:
		return w.Ticket + dimStyle.Render(" — loading...")
	case result.err != nil:
		return w.Ticket + " " + noteStyle.Render(result.err.Error())
	}

	issue := result.issue
	value := fmt.Sprintf("%s — %s", issue.Key, issue.Status)
	if issue.Summary != "" {
		value += fmt.Sprintf(" — %q", issue.Summary)
	}
	if issue.Assignee != "" {
		value += dimStyle.Render(" (" + issue.Assignee + ")")
	}
	return value
}
