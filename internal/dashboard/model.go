package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
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
		key  string
		view string
		err  error
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

	cmdDoneMsg struct {
		index int
		err   error
	}
)

type jiraResult struct {
	view    string
	err     error
	loading bool
}

// runState tracks one ad-hoc liferay command (or sequence) launched from
// the dashboard for one worktree. logPath outlives the run so the output
// stays inspectable after completion.
type runState struct {
	line    string
	logPath string
	running bool
}

// log drawer content sources.
const (
	srcServer = iota
	srcCommand
)

type model struct {
	cfg Config

	active   int
	statuses []Status
	jira     map[string]jiraResult

	// action holds a per-tab in-flight verb ("" when idle); note holds the
	// last action outcome shown in the panel.
	action []string
	note   []string
	runs   []runState

	showLogs bool
	logSrc   []int
	logView  viewport.Model

	inputMode bool
	cmdInput  textinput.Model

	width  int
	height int
	ready  bool
}

func newModel(cfg Config) model {
	cmdInput := textinput.New()
	cmdInput.Prompt = "liferay> "
	cmdInput.Placeholder = "build <module> · test-integration <module> --tests <filter> · ..."

	active := cfg.Active
	if active < 0 || active >= len(cfg.Worktrees) {
		active = 0
	}

	return model{
		cfg:      cfg,
		active:   active,
		statuses: make([]Status, len(cfg.Worktrees)),
		jira:     map[string]jiraResult{},
		action:   make([]string, len(cfg.Worktrees)),
		note:     make([]string, len(cfg.Worktrees)),
		runs:     make([]runState, len(cfg.Worktrees)),
		logSrc:   make([]int, len(cfg.Worktrees)),
		showLogs: true,
		cmdInput: cmdInput,
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
		view, err := FetchIssueView(key)
		return jiraMsg{key: key, view: view, err: err}
	}
}

func actionCmd(selfExe string, index int, w Worktree, verb string) tea.Cmd {
	commands := [][]string{{"server", verb}}
	if verb == "stop" {
		commands = append(commands, []string{"db", "stop"})
	}

	return func() tea.Msg {
		for _, args := range commands {
			out, err := exec.Command(selfExe, append([]string{"-C", w.Path}, args...)...).CombinedOutput()
			if err != nil {
				return actionDoneMsg{
					index: index,
					verb:  verb,
					err: fmt.Errorf("%s: %v\n%s",
						strings.Join(args, " "), err, lastLines(string(out), 3)),
				}
			}
		}
		return actionDoneMsg{index: index, verb: verb}
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
		m.logView = viewport.New(msg.Width-2, 5)
		m.logView.Height = m.availLogHeight()
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickMsg:
		cmds := []tea.Cmd{probeCmd(m.cfg.Worktrees), tickCmd()}
		if tail := m.tailNow(); tail != nil {
			cmds = append(cmds, tail)
		}
		return m, tea.Batch(cmds...)

	case statusesMsg:
		m.statuses = msg
		m.logView.Height = m.availLogHeight()
		// The probe delivers the catalina.out path the default drawer
		// needs, so refresh the tail now rather than on the next tick.
		if tail := m.tailNow(); tail != nil {
			return m, tail
		}
		return m, nil

	case jiraMsg:
		m.jira[msg.key] = jiraResult{view: msg.view, err: msg.err}
		m.logView.Height = m.availLogHeight()
		return m, nil

	case actionDoneMsg:
		m.action[msg.index] = ""
		if msg.err != nil {
			m.note[msg.index] = msg.err.Error()
		} else if msg.verb == "stop" {
			m.note[msg.index] = "server and db stopped"
		} else {
			m.note[msg.index] = fmt.Sprintf("server %s done", msg.verb)
		}
		return m, probeCmd(m.cfg.Worktrees)

	case cmdDoneMsg:
		run := m.runs[msg.index]
		run.running = false
		m.runs[msg.index] = run
		if msg.err != nil {
			m.note[msg.index] = fmt.Sprintf("liferay %s failed: %v", run.line, msg.err)
		} else {
			m.note[msg.index] = fmt.Sprintf("liferay %s done", run.line)
		}
		cmds := []tea.Cmd{probeCmd(m.cfg.Worktrees)}
		if tail := m.tailNow(); tail != nil {
			cmds = append(cmds, tail)
		}
		return m, tea.Batch(cmds...)

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
	if m.inputMode {
		return m.handleInputKey(msg)
	}

	w := m.cfg.Worktrees[m.active]

	switch msg.String() {
	case "q", "esc", "ctrl+c":
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
		if verb != "stop" {
			m = m.applyBranchFlags(w)
		}
		return m, actionCmd(m.cfg.SelfExe, m.active, w, verb)

	case "w":
		// Full reset: wipe bundle state, bounce the DB stack (container
		// data is not persisted, so this yields a fresh database), boot.
		if m.action[m.active] != "" {
			return m, nil
		}
		m = m.applyBranchFlags(w)
		return m.startSequence(
			"server wipe && db restart && server start",
			[][]string{
				{"server", "wipe", "--yes"},
				{"db", "restart"},
				{"server", "start"},
			})

	case ":":
		m.inputMode = true
		m.cmdInput.SetValue("")
		m.cmdInput.Focus()
		return m, textinput.Blink

	case "l":
		// Cycle the drawer: closed -> command output (when one exists) ->
		// server log -> closed.
		switch {
		case !m.showLogs:
			m.showLogs = true
			if m.runs[m.active].logPath != "" {
				m.logSrc[m.active] = srcCommand
			} else {
				m.logSrc[m.active] = srcServer
			}
		case m.logSrc[m.active] == srcCommand:
			m.logSrc[m.active] = srcServer
		default:
			m.showLogs = false
		}
		if m.showLogs {
			m.logView.Height = m.availLogHeight()
			m.logView.SetContent("")
			if tail := m.tailNow(); tail != nil {
				return m, tail
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
	m.logView.Height = m.availLogHeight()
	m.logView.SetContent("")
	if tail := m.tailNow(); tail != nil {
		return m, tail
	}
	return m, nil
}

// handleInputKey routes keys while the command prompt is open: enter runs
// the typed liferay command against the active worktree, esc cancels, and
// everything else edits the input.
func (m model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.inputMode = false
		m.cmdInput.Blur()
		return m, nil

	case "enter":
		line := strings.TrimSpace(m.cmdInput.Value())
		m.inputMode = false
		m.cmdInput.Blur()
		if line == "" {
			return m, nil
		}
		return m.startCommand(line)
	}

	var cmd tea.Cmd
	m.cmdInput, cmd = m.cmdInput.Update(msg)
	return m, cmd
}

// startCommand launches `liferay -C <worktree> <line>` with output streamed
// to a temp log file the drawer tails.
func (m model) startCommand(line string) (tea.Model, tea.Cmd) {
	return m.startSequence(line, [][]string{strings.Fields(line)})
}

// startSequence launches one or more liferay invocations back-to-back
// against the active worktree, all writing to one log file the drawer
// tails. One run per worktree at a time.
func (m model) startSequence(line string, argSets [][]string) (tea.Model, tea.Cmd) {
	index := m.active
	if m.runs[index].running {
		m.note[index] = "a command is already running for this worktree"
		return m, nil
	}

	logFile, err := os.CreateTemp("", "liferay-dashboard-*.log")
	if err != nil {
		m.note[index] = err.Error()
		return m, nil
	}

	m.runs[index] = runState{line: line, logPath: logFile.Name(), running: true}
	m.note[index] = ""
	m.showLogs = true
	m.logSrc[index] = srcCommand
	m.logView.Height = m.availLogHeight()
	m.logView.SetContent("")

	seq := seqCmd(index, m.cfg.SelfExe, m.cfg.Worktrees[index].Path, argSets, logFile)

	return m, tea.Batch(seq, logCmd(index, logFile.Name()))
}

// seqCmd runs each invocation in order, stopping at the first failure. The
// log file is closed here because the children write to it until then.
func seqCmd(index int, selfExe, path string, argSets [][]string, logFile *os.File) tea.Cmd {
	return func() tea.Msg {
		defer logFile.Close()

		for _, args := range argSets {
			fmt.Fprintf(logFile, "$ liferay %s\n", strings.Join(args, " "))

			cmd := exec.Command(selfExe, append([]string{"-C", path}, args...)...)
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			if err := cmd.Run(); err != nil {
				return cmdDoneMsg{
					index: index,
					err:   fmt.Errorf("liferay %s: %v", strings.Join(args, " "), err),
				}
			}
		}
		return cmdDoneMsg{index: index}
	}
}

// applyBranchFlags enables the branch's feature flags in the bundle's
// portal-ext.properties ahead of a boot. Success is silent — the panel's
// Flags line reflects the new state on the next probe; failures land in
// the note.
func (m model) applyBranchFlags(w Worktree) model {
	if _, err := enableBranchFlags(w); err != nil {
		m.note[m.active] = "feature flags: " + err.Error()
	}
	return m
}

// tailNow returns the refresh command for the drawer's current source, or
// nil when the drawer is closed or the source has no file yet.
func (m model) tailNow() tea.Cmd {
	if !m.showLogs {
		return nil
	}
	if m.logSrc[m.active] == srcCommand && m.runs[m.active].logPath != "" {
		return logCmd(m.active, m.runs[m.active].logPath)
	}
	if catOut := m.statuses[m.active].CatOut; catOut != "" {
		return logCmd(m.active, catOut)
	}
	return nil
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

// availLogHeight measures the rendered chrome (tabs, panel, drawer title,
// footer) and returns the rows left for the log body, so the full view
// always fits the terminal without scrolling. The panel height varies with
// the Jira block, flags, and notes, so this is recomputed as they change.
func (m model) availLogHeight() int {
	chrome := lipgloss.Height(m.viewTabs()+"\n\n"+m.viewPanel()) +
		1 + // drawer title line
		2 // footer/input line and its separating newline

	h := m.height - chrome
	if h < 3 {
		h = 3
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
		logView := m.logView
		logView.Height = m.availLogHeight()
		b.WriteString("\n" + dimStyle.Render("── "+m.drawerTitle()+" ") + "\n")
		b.WriteString(logView.View())
	}

	if m.inputMode {
		b.WriteString("\n" + m.cmdInput.View())
	} else {
		b.WriteString("\n" + dimStyle.Render(
			"←/→ tabs · o open · s start · x stop · r restart · w reset · : run · l logs · u refresh · q quit"))
	}

	return b.String()
}

func (m model) drawerTitle() string {
	if m.logSrc[m.active] == srcCommand {
		run := m.runs[m.active]
		if run.logPath != "" {
			title := "$ liferay " + run.line
			if run.running {
				return title + " (running)"
			}
			return title + " (finished)"
		}
	}
	if catOut := m.statuses[m.active].CatOut; catOut != "" {
		return catOut
	}
	return "no bundle log"
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
	if len(w.Flags) > 0 {
		line("Flags", viewFlags(w, st))
	}
	b.WriteString(m.viewJira(w))

	if verb := m.action[m.active]; verb != "" {
		b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("server %s in progress...", verb)) + "\n")
	} else if run := m.runs[m.active]; run.running {
		b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("running: liferay %s ...", run.line)) + "\n")
	} else if note := m.note[m.active]; note != "" {
		b.WriteString("\n" + noteStyle.Render(note) + "\n")
	}

	if w.Hostname == "" {
		b.WriteString("\n" + dimStyle.Render(
			"slot hostnames not installed — run: sudo liferay dashboard install-hosts") + "\n")
	}

	return b.String()
}

// viewFlags lists the branch's feature flags with their current state in
// portal-ext.properties; they are enabled automatically before every boot.
func viewFlags(w Worktree, st Status) string {
	parts := make([]string, 0, len(w.Flags))
	for _, flag := range w.Flags {
		mark := stoppedDot
		if st.Flags[flag] {
			mark = readyDot
		}
		parts = append(parts, mark+" "+flag)
	}
	return strings.Join(parts, " · ") + dimStyle.Render("   (enabled on start)")
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

// viewJira renders the `issues view` header block under the Jira label,
// with continuation lines indented to the label column.
func (m model) viewJira(w Worktree) string {
	label := labelStyle.Render("Jira")

	if w.Ticket == "" {
		return label + dimStyle.Render("no ticket on branch") + "\n"
	}

	result, ok := m.jira[w.Ticket]
	switch {
	case !ok || result.loading:
		return label + w.Ticket + dimStyle.Render(" — loading...") + "\n"
	case result.err != nil:
		return label + w.Ticket + " " + noteStyle.Render(result.err.Error()) + "\n"
	}

	pad := strings.Repeat(" ", 8)

	var b strings.Builder
	for i, l := range strings.Split(result.view, "\n") {
		if i == 0 {
			b.WriteString(label + l + "\n")
			continue
		}
		b.WriteString(pad + l + "\n")
	}
	return b.String()
}
