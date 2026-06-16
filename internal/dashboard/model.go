package dashboard

import (
	"errors"
	"fmt"
	"io/fs"
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
	tickMsg      struct{}
	statusesMsg  []Status
	worktreesMsg []Worktree

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

	deleteDoneMsg struct {
		index  int
		branch string
		err    error
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

	// confirmDelete gates the destructive ctrl+d worktree removal: the key
	// arms it, the panel shows a y/n prompt, and only "y" goes through.
	confirmDelete bool

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

func reloadCmd(reload func() []Worktree) tea.Cmd {
	if reload == nil {
		return nil
	}
	return func() tea.Msg {
		return worktreesMsg(reload())
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

// deleteCmd removes the worktree at targetPath via `liferay worktree remove`.
// It runs from runDir — the primary worktree — rather than the target, so git
// is never asked to remove the worktree it is standing in. The CLI stops the
// slot's Docker stack and Tomcat and deletes the bundle and state dirs itself.
func deleteCmd(selfExe, runDir, targetPath string, index int, branch string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command(
			selfExe, "-C", runDir, "worktree", "remove", targetPath, "--yes",
		).CombinedOutput()
		if err != nil {
			err = fmt.Errorf("worktree remove: %v\n%s", err, lastLines(string(out), 3))
		}
		return deleteDoneMsg{index: index, branch: branch, err: err}
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

	case worktreesMsg:
		m.mergeWorktrees(msg)
		m.logView.Height = m.availLogHeight()
		return m, nil

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

	case deleteDoneMsg:
		if msg.err != nil {
			if msg.index < len(m.action) {
				m.action[msg.index] = ""
				m.note[msg.index] = msg.err.Error()
			}
			// The worktree may be gone even though the command reported an
			// error (e.g. interrupted mid-run); reconcile to drop a dead tab.
			return m, reloadCmd(m.cfg.Reload)
		}
		m.removeTab(msg.index)
		m.logView.Height = m.availLogHeight()
		return m, probeCmd(m.cfg.Worktrees)

	case logMsg:
		if msg.index != m.active || !m.showLogs {
			return m, nil
		}
		if msg.err != nil {
			if errors.Is(msg.err, fs.ErrNotExist) {
				m.logView.SetContent(dimStyle.Render(
					"no log yet — the file appears once the server starts (s)"))
			} else {
				m.logView.SetContent("cannot read log: " + msg.err.Error())
			}
			return m, nil
		}
		atBottom := m.logView.AtBottom()
		m.logView.SetContent(softWrap(msg.content, m.logView.Width))
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
	if m.confirmDelete {
		return m.handleConfirmDelete(msg)
	}
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

	case "ctrl+d":
		if w.Primary {
			m.note[m.active] = "cannot delete the primary worktree"
			return m, nil
		}
		if m.action[m.active] != "" || m.runs[m.active].running {
			m.note[m.active] = "finish the running action before deleting"
			return m, nil
		}
		m.confirmDelete = true
		return m, nil

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
		if reload := reloadCmd(m.cfg.Reload); reload != nil {
			cmds = append(cmds, reload)
		}
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

// handleConfirmDelete resolves the armed worktree removal: "y" runs it, any
// other key cancels.
func (m model) handleConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.confirmDelete = false
	if s := msg.String(); s == "y" || s == "Y" {
		return m.startDelete()
	}
	return m, nil
}

// startDelete shells out to remove the active worktree, marking the tab busy
// so it ignores further actions until deleteDoneMsg lands.
func (m model) startDelete() (tea.Model, tea.Cmd) {
	index := m.active
	w := m.cfg.Worktrees[index]
	m.action[index] = "delete"
	m.note[index] = ""
	return m, deleteCmd(m.cfg.SelfExe, m.primaryPath(), w.Path, index, tabLabel(w))
}

// primaryPath is the main worktree's path — the safe cwd for git to remove a
// sibling worktree from. Falls back to the target's parent if no primary is
// tagged (it always is in practice).
func (m model) primaryPath() string {
	for _, w := range m.cfg.Worktrees {
		if w.Primary {
			return w.Path
		}
	}
	return filepath.Dir(m.cfg.Worktrees[m.active].Path)
}

// removeTab drops tab i from every per-tab slice in lockstep so they stay
// aligned, then clamps active onto a surviving tab.
func (m *model) removeTab(i int) {
	if i < 0 || i >= len(m.cfg.Worktrees) {
		return
	}
	m.cfg.Worktrees = append(m.cfg.Worktrees[:i], m.cfg.Worktrees[i+1:]...)
	if i < len(m.statuses) {
		m.statuses = append(m.statuses[:i], m.statuses[i+1:]...)
	}
	m.action = append(m.action[:i], m.action[i+1:]...)
	m.note = append(m.note[:i], m.note[i+1:]...)
	m.runs = append(m.runs[:i], m.runs[i+1:]...)
	m.logSrc = append(m.logSrc[:i], m.logSrc[i+1:]...)

	if m.active >= len(m.cfg.Worktrees) {
		m.active = len(m.cfg.Worktrees) - 1
	}
	if m.active < 0 {
		m.active = 0
	}
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

// mergeWorktrees reconciles the tabs with freshly discovered worktrees,
// matched by path: surviving tabs take the fresh metadata (slot, engine,
// hostname, flags) and keep their per-tab status/run/note state, while tabs
// whose worktree has vanished — deleted here or elsewhere — are dropped. Newly
// added worktrees are ignored until the next launch. Active follows its tab.
func (m *model) mergeWorktrees(fresh []Worktree) {
	byPath := make(map[string]Worktree, len(fresh))
	for _, w := range fresh {
		byPath[w.Path] = w
	}

	activePath := ""
	if m.active < len(m.cfg.Worktrees) {
		activePath = m.cfg.Worktrees[m.active].Path
	}

	var (
		worktrees []Worktree
		statuses  []Status
		action    []string
		note      []string
		runs      []runState
		logSrc    []int
	)
	for i, w := range m.cfg.Worktrees {
		updated, ok := byPath[w.Path]
		if !ok {
			continue
		}
		worktrees = append(worktrees, updated)
		if i < len(m.statuses) {
			statuses = append(statuses, m.statuses[i])
		} else {
			statuses = append(statuses, Status{})
		}
		action = append(action, m.action[i])
		note = append(note, m.note[i])
		runs = append(runs, m.runs[i])
		logSrc = append(logSrc, m.logSrc[i])
	}

	// Never blank the UI; the primary worktree should always survive.
	if len(worktrees) == 0 {
		return
	}

	m.cfg.Worktrees = worktrees
	m.statuses = statuses
	m.action = action
	m.note = note
	m.runs = runs
	m.logSrc = logSrc

	m.active = 0
	for i, w := range worktrees {
		if w.Path == activePath {
			m.active = i
			break
		}
	}
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

// availLogHeight returns the rows left for the log body after the surrounding
// chrome (tabs, panel, drawer title, footer) is laid out, so the full view
// always fits the terminal without scrolling. It measures the real chrome
// string — which wraps with the terminal width — so wrapped tabs, panel
// values, or footer never push the view past the bottom.
func (m model) availLogHeight() int {
	parts := m.viewTabs() + "\n\n" + m.viewPanel()
	if m.showLogs {
		parts += "\n" + m.drawerTitleLine()
	}
	parts += "\n" + m.viewFooter()

	h := m.height - lipgloss.Height(parts)
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

	parts := m.viewTabs() + "\n\n" + m.viewPanel()

	if m.showLogs {
		logView := m.logView
		logView.Height = m.availLogHeight()
		parts += "\n" + m.drawerTitleLine() + "\n" + logView.View()
	}

	parts += "\n" + m.viewFooter()

	return parts
}

// drawerTitleLine renders the drawer's "── title ──" header, wrapped so a long
// command line or log path does not overflow the terminal.
func (m model) drawerTitleLine() string {
	return softWrap(dimStyle.Render("── "+m.drawerTitle()+" "), m.width)
}

// viewFooter renders the bottom line — the command prompt, the delete
// confirmation, or the key help — wrapped to the terminal width. The command
// input is left unwrapped so its cursor renders correctly.
func (m model) viewFooter() string {
	if m.inputMode {
		return m.cmdInput.View()
	}
	if m.confirmDelete {
		return softWrap(noteStyle.Render(fmt.Sprintf(
			"Delete worktree %s and its bundle? This cannot be undone.  (y/n)",
			tabLabel(m.cfg.Worktrees[m.active]))), m.width)
	}
	return softWrap(dimStyle.Render(
		"←/→ tabs · o open · s start · x stop · r restart · w reset · ctrl+d delete · : run · l logs · u refresh · q quit"),
		m.width)
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
	return wrapTabs(tabs, m.width)
}

// softWrap wraps content to width so long lines (log stack traces, long paths,
// the footer help, panel values) stay on screen instead of being clipped.
// width <= 0 leaves the content untouched.
func softWrap(content string, width int) string {
	if width <= 0 {
		return content
	}
	return lipgloss.NewStyle().Width(width).Render(content)
}

// wrapTabs lays the tabs left to right, breaking to a new row when the next
// tab would overflow the terminal width, so every tab stays visible on a
// narrow terminal instead of being clipped. width <= 0 keeps them on one row.
func wrapTabs(tabs []string, width int) string {
	if len(tabs) == 0 {
		return ""
	}

	var rows []string
	var row []string
	rowWidth := 0

	for _, tab := range tabs {
		w := lipgloss.Width(tab)
		if len(row) > 0 && width > 0 && rowWidth+w > width {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
			row = nil
			rowWidth = 0
		}
		row = append(row, tab)
		rowWidth += w
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))

	return strings.Join(rows, "\n")
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
		progress := fmt.Sprintf("server %s in progress...", verb)
		if verb == "delete" {
			progress = "removing worktree..."
		}
		b.WriteString("\n" + dimStyle.Render(progress) + "\n")
	} else if run := m.runs[m.active]; run.running {
		b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("running: liferay %s ...", run.line)) + "\n")
	} else if note := m.note[m.active]; note != "" {
		b.WriteString("\n" + noteStyle.Render(note) + "\n")
	}

	if w.Hostname == "" {
		b.WriteString("\n" + dimStyle.Render(
			"slot hostnames not installed — run: sudo liferay dashboard install-hosts") + "\n")
	}

	return softWrap(b.String(), m.width)
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
