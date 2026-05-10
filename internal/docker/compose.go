package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/david-truong/liferay-portal-cli/internal/state"
)

// Engine names recognized by the CLI. Hypersonic means "no Docker container; use
// Liferay's built-in HSQL", so it's modelled here for completeness but Setup skips
// the Docker bits when it's selected.
const (
	EngineMySQL      = "mysql"
	EngineMariaDB    = "mariadb"
	EnginePostgres   = "postgres"
	EngineHypersonic = "hypersonic"
)

// DefaultEngine is used on the first "db up" in a fresh worktree.
const DefaultEngine = EngineMySQL

// SupportedEngines is the ordered list shown in --help text and validated on input.
var SupportedEngines = []string{EngineMySQL, EngineMariaDB, EnginePostgres, EngineHypersonic}

// IsSupportedEngine returns true when name is one of SupportedEngines.
func IsSupportedEngine(name string) bool {
	for _, e := range SupportedEngines {
		if e == name {
			return true
		}
	}
	return false
}

// IsDockerManagedEngine reports whether the engine runs as a Docker container
// (so db up/down/logs are meaningful).
func IsDockerManagedEngine(name string) bool {
	return name != EngineHypersonic
}

const composeTemplateMySQL = `services:
  db:
    image: mysql:8.0
    command:
      - --general-log=1
      - --general-log-file=/var/log/mysql/mysql.log
      - --slow-query-log=1
      - --slow-query-log-file=/var/log/mysql/slow.log
      - --lower-case-table-names=1
      - --innodb-buffer-pool-size=1G
      - --innodb_flush_log_at_trx_commit=0
      - --innodb-redo-log-capacity=1G
      - --net-buffer-length=1000000
      - --max-allowed-packet=1000000000
      - --skip-log-bin
    volumes:
      - ./db/log:/var/log/mysql
    restart: always
    environment:
      MYSQL_ROOT_PASSWORD: root
      MYSQL_DATABASE: lportal
    ports:
      - "{{.Ports.MySQL}}:3306"
    healthcheck:
      test: ["CMD", "mysql", "-h", "localhost", "-u", "root", "-proot", "-e", "SELECT 1"]
      interval: 5s
      timeout: 10s
      retries: 12
      start_period: 30s

  adminer:
    image: adminer:latest
    restart: always
    ports:
      - "{{.Ports.Adminer}}:8080"
`

const composeTemplateMariaDB = `services:
  db:
    image: mariadb:11
    command:
      - --lower-case-table-names=1
      - --innodb-buffer-pool-size=1G
      - --net-buffer-length=1000000
      - --max-allowed-packet=1000000000
    restart: always
    environment:
      MARIADB_ROOT_PASSWORD: root
      MARIADB_DATABASE: lportal
    ports:
      - "{{.Ports.MySQL}}:3306"
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 5s
      timeout: 10s
      retries: 12
      start_period: 30s

  adminer:
    image: adminer:latest
    restart: always
    ports:
      - "{{.Ports.Adminer}}:8080"
`

const composeTemplatePostgres = `services:
  db:
    image: postgres:17
    restart: always
    environment:
      POSTGRES_USER: liferay
      POSTGRES_PASSWORD: liferay
      POSTGRES_DB: lportal
    ports:
      - "{{.Ports.MySQL}}:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U liferay -d lportal"]
      interval: 5s
      timeout: 10s
      retries: 12
      start_period: 15s

  adminer:
    image: adminer:latest
    restart: always
    ports:
      - "{{.Ports.Adminer}}:8080"
`

// composeTemplateFor returns the compose template string for a Docker-managed engine.
func composeTemplateFor(engine string) (string, error) {
	switch engine {
	case EngineMySQL:
		return composeTemplateMySQL, nil
	case EngineMariaDB:
		return composeTemplateMariaDB, nil
	case EnginePostgres:
		return composeTemplatePostgres, nil
	}
	return "", fmt.Errorf("engine %q has no compose template", engine)
}

// portalExtStanza returns the block of jdbc.default.* properties to inject for
// the given engine, parameterised on the host-side DB port.
func portalExtStanza(engine string, dbPort int) string {
	switch engine {
	case EngineMySQL:
		return fmt.Sprintf(
			"jdbc.default.driverClassName=com.mysql.cj.jdbc.Driver\n"+
				"jdbc.default.url=jdbc:mysql://localhost:%d/lportal?characterEncoding=UTF-8&dontTrackOpenResources=true&holdResultsOpenOverStatementClose=true&serverTimezone=GMT&useFastDateParsing=false&useUnicode=true&databaseTerm=CATALOG\n"+
				"jdbc.default.username=root\n"+
				"jdbc.default.password=root\n",
			dbPort)
	case EngineMariaDB:
		return fmt.Sprintf(
			"jdbc.default.driverClassName=org.mariadb.jdbc.Driver\n"+
				"jdbc.default.url=jdbc:mariadb://localhost:%d/lportal?characterEncoding=UTF-8&useUnicode=true\n"+
				"jdbc.default.username=root\n"+
				"jdbc.default.password=root\n",
			dbPort)
	case EnginePostgres:
		return fmt.Sprintf(
			"jdbc.default.driverClassName=org.postgresql.Driver\n"+
				"jdbc.default.url=jdbc:postgresql://localhost:%d/lportal\n"+
				"jdbc.default.username=liferay\n"+
				"jdbc.default.password=liferay\n",
			dbPort)
	}
	return "" // hypersonic or unknown — no JDBC override, Liferay falls back to built-in HSQL
}

// portalExtOverrideKeys names every key the CLI owns inside portal-ext.properties.
// These are stripped from the user's portal-ext.properties before the CLI's
// stanza is appended, so switching engines or slots leaves no stale keys behind.
var portalExtOverrideKeys = map[string]bool{
	"jdbc.default.driverClassName":             true,
	"jdbc.default.url":                         true,
	"jdbc.default.username":                    true,
	"jdbc.default.password":                    true,
	"liferay.home":                             true,
	"portal.instance.http.socket.address":      true,
	"module.framework.properties.osgi.console": true,
	"browser.launcher.url":                     true,
	"include-and-override":                     true,
	"users.reminder.queries.enabled":           true,
	"terms.of.use.required":                    true,
	"passwords.default.policy.change.required": true,
}

// devModeOverrides is emitted unconditionally: turns on developer mode by
// chaining portal-developer.properties, plus the first-login bypass keys that
// portal-developer.properties does not set itself. Keeps agent and human dev
// flows free of reminder-queries, ToS, and forced-password-change prompts.
const devModeOverrides = "include-and-override=portal-developer.properties\n" +
	"users.reminder.queries.enabled=false\n" +
	"terms.of.use.required=false\n" +
	"passwords.default.policy.change.required=false\n"

// browserLauncherOverride suppresses Liferay's auto-open-browser-on-startup
// behavior. Always emitted — agent-driven workflows should never pop a window.
const browserLauncherOverride = "browser.launcher.url=\n"

const (
	managedBlockBegin = "# Begin liferay-cli portal-ext overrides"
	managedBlockEnd   = "# End liferay-cli portal-ext overrides"
)

// slotOverridesStanza returns the non-JDBC key block we inject for slot > 0.
// For slot 0 (stock) it returns the empty string so the bundle keeps its
// defaults (liferay.home auto-detects, HTTP port stays 8080, OSGi console
// stays 11311).
func slotOverridesStanza(bundleDir string, ports Ports) string {
	if ports.IsStock() {
		return ""
	}
	return fmt.Sprintf(
		"liferay.home=%s\n"+
			"portal.instance.http.socket.address=localhost:%d\n"+
			"module.framework.properties.osgi.console=localhost:%d\n",
		bundleDir, ports.TomcatHTTP, ports.OSGiConsole)
}

type composeParams struct {
	Ports Ports
}

// State is the persisted per-worktree CLI state (slot allocation + selected engine).
type State struct {
	Slot   int    `json:"slot"`
	Engine string `json:"engine,omitempty"`
}

// StateDir returns the path to the per-worktree CLI state directory. State
// lives outside the worktree (under ~/.liferay-cli/) so "ant all" cannot wipe
// it.
func StateDir(worktreeRoot string) string {
	return filepath.Join(state.Dir(worktreeRoot), "docker")
}

// Setup reads/writes the state file, generates docker-compose.yml for Docker-
// managed engines, and rewrites the bundle's portal-ext.properties with the
// correct JDBC stanza for the stored engine. If requestedEngine != "" it
// replaces whatever was in state (used by "liferay db up --engine X").
// Returns the State that is now authoritative.
func Setup(worktreeRoot, bundleDir, requestedEngine string) (State, Ports, error) {
	stateDir := StateDir(worktreeRoot)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return State{}, Ports{}, err
	}

	state, err := loadOrInitState(stateDir, requestedEngine)
	if err != nil {
		return State{}, Ports{}, err
	}
	ports := PortsFromSlot(state.Slot)

	if IsDockerManagedEngine(state.Engine) {
		if err := os.MkdirAll(filepath.Join(stateDir, "db", "log"), 0755); err != nil {
			return State{}, Ports{}, err
		}

		tmplStr, err := composeTemplateFor(state.Engine)
		if err != nil {
			return State{}, Ports{}, err
		}
		tmpl, err := template.New("compose").Parse(tmplStr)
		if err != nil {
			return State{}, Ports{}, fmt.Errorf("parsing compose template: %w", err)
		}

		composePath := filepath.Join(stateDir, "docker-compose.yml")
		f, err := os.Create(composePath)
		if err != nil {
			return State{}, Ports{}, fmt.Errorf("creating docker-compose.yml: %w", err)
		}
		defer f.Close()

		if err := tmpl.Execute(f, composeParams{Ports: ports}); err != nil {
			return State{}, Ports{}, fmt.Errorf("rendering compose template: %w", err)
		}
	} else {
		// hypersonic — remove any stale compose file so "db down/logs/ps" give
		// sensible errors instead of bringing up the wrong engine.
		_ = os.Remove(filepath.Join(stateDir, "docker-compose.yml"))
	}

	if err := writePortalExt(bundleDir, state.Engine, ports); err != nil {
		return State{}, Ports{}, fmt.Errorf("writing portal-ext overrides: %w", err)
	}

	return state, ports, nil
}

// LoadState returns the persisted state for a worktree, or a zero-value State with
// ok=false if no state has been written yet.
func LoadState(worktreeRoot string) (State, bool) {
	data, err := os.ReadFile(filepath.Join(StateDir(worktreeRoot), "ports.json"))
	if err != nil {
		return State{}, false
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, false
	}
	if s.Engine == "" {
		s.Engine = DefaultEngine
	}
	return s, true
}

// ComposePath returns the path to the generated docker-compose.yml.
func ComposePath(worktreeRoot string) string {
	return filepath.Join(StateDir(worktreeRoot), "docker-compose.yml")
}

// Run executes `docker compose -p liferay-slot-<N> -f <compose-file> <args...>`.
// The project name is slot-derived so two worktrees never clash on container
// names. Slot is read from the persisted state; Run errors out if no state
// file has been written yet (call Setup first).
func Run(worktreeRoot string, args ...string) error {
	if err := checkDocker(); err != nil {
		return err
	}
	state, ok := LoadState(worktreeRoot)
	if !ok {
		return fmt.Errorf("no Docker state for this worktree — run \"liferay db start\" first")
	}
	composePath := ComposePath(worktreeRoot)
	cmdArgs := append([]string{"compose", "-p", ProjectName(state.Slot), "-f", composePath}, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// ProjectName is the docker compose -p value derived from a slot.
// Keeps container names human-readable (e.g. "liferay-slot-0-db-1").
func ProjectName(slot int) string {
	return fmt.Sprintf("liferay-slot-%d", slot)
}

// loadOrInitState reads ports.json if present, otherwise allocates a new slot.
// requestedEngine, if non-empty, replaces whatever engine was previously stored.
// The first call on a fresh worktree defaults the engine to DefaultEngine.
//
// The probe-and-persist sequence is serialized by a host-wide flock on
// ~/.liferay-cli/slot.lock so two worktrees starting in parallel cannot
// both claim the same slot. Within the critical section we also consult
// every other worktree's persisted ports.json — that way the second
// worktree picks slot 1 even if it boots before the first has run
// `docker compose up` (i.e. before slot 0's host ports are actually bound).
func loadOrInitState(stateDir, requestedEngine string) (State, error) {
	if requestedEngine != "" && !IsSupportedEngine(requestedEngine) {
		return State{}, fmt.Errorf("unsupported engine %q (want one of: %s)",
			requestedEngine, strings.Join(SupportedEngines, ", "))
	}

	unlock, err := state.Lock(filepath.Join(state.Root(), "slot.lock"), 30*time.Second)
	if err != nil {
		return State{}, fmt.Errorf("acquiring slot lock: %w", err)
	}
	defer func() { _ = unlock() }()

	portsFile := filepath.Join(stateDir, "ports.json")
	var s State

	if data, err := os.ReadFile(portsFile); err == nil {
		_ = json.Unmarshal(data, &s)
	}

	if !isPersisted(portsFile) {
		s.Slot = allocateFreshSlot(stateDir)
	}
	if s.Engine == "" {
		s.Engine = DefaultEngine
	}
	if requestedEngine != "" {
		s.Engine = requestedEngine
	}

	data, err := json.Marshal(s)
	if err != nil {
		return State{}, fmt.Errorf("marshal state: %w", err)
	}
	if err := state.WriteFileAtomic(portsFile, data, 0644); err != nil {
		return State{}, fmt.Errorf("writing state file: %w", err)
	}
	return s, nil
}

// allocateFreshSlot returns the lowest slot that is (a) not already claimed
// by another worktree's persisted ports.json and (b) has no host ports
// currently bound by any local process. Caller must hold the slot lock.
//
// selfStateDir is excluded from the claimed-slot scan so a worktree
// re-running loadOrInitState against its own state directory doesn't see
// itself as a claimant.
func allocateFreshSlot(selfStateDir string) int {
	claimed := claimedSlots(selfStateDir)
	for slot := 0; slot < 100; slot++ {
		if claimed[slot] {
			continue
		}
		if !AnyPortInUse(ProbePorts(slotPorts(slot))...) {
			return slot
		}
	}
	return 0
}

// claimedSlots returns the set of slots currently persisted by other
// worktrees under ~/.liferay-cli/worktrees/. selfStateDir is omitted so a
// worktree never sees its own (potentially in-flight) write as a foreign
// claim.
func claimedSlots(selfStateDir string) map[int]bool {
	claimed := make(map[int]bool)
	worktreesDir := filepath.Join(state.Root(), "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return claimed
	}
	selfAbs, _ := filepath.Abs(selfStateDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		portsPath := filepath.Join(worktreesDir, e.Name(), "docker", "ports.json")
		if abs, _ := filepath.Abs(filepath.Dir(portsPath)); abs == selfAbs {
			continue
		}
		data, err := os.ReadFile(portsPath)
		if err != nil {
			continue
		}
		var s State
		if json.Unmarshal(data, &s) == nil {
			claimed[s.Slot] = true
		}
	}
	return claimed
}

// isPersisted returns true when the state file exists; distinguishes "slot 0
// because we've never written state" from "slot 0 because we persisted slot 0".
func isPersisted(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// writePortalExt rewrites <bundleDir>/portal-ext.properties, stripping any lines
// whose keys liferay-cli manages, then appending the current overrides for the
// selected engine and slot. Hypersonic + slot 0 together are a full no-op
// relative to stock Liferay; hypersonic + slot > 0 still emits the slot
// overrides (liferay.home, HTTP socket address, OSGi console port) even
// though there is no JDBC block. Existing user-owned lines are preserved.
func writePortalExt(bundleDir, engine string, ports Ports) error {
	path := filepath.Join(bundleDir, "portal-ext.properties")

	var sb strings.Builder

	if data, err := os.ReadFile(path); err == nil {
		inManagedBlock := false
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, managedBlockBegin) {
				inManagedBlock = true
				continue
			}
			if strings.HasPrefix(trimmed, managedBlockEnd) {
				inManagedBlock = false
				continue
			}
			if inManagedBlock {
				continue
			}
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				sb.WriteString(line)
				sb.WriteByte('\n')
				continue
			}
			eqIdx := strings.IndexByte(trimmed, '=')
			if eqIdx < 0 {
				sb.WriteString(line)
				sb.WriteByte('\n')
				continue
			}
			key := strings.TrimSpace(trimmed[:eqIdx])
			if !portalExtOverrideKeys[key] {
				sb.WriteString(line)
				sb.WriteByte('\n')
			}
		}
	}

	jdbcStanza := portalExtStanza(engine, ports.MySQL)
	slotStanza := slotOverridesStanza(bundleDir, ports)

	sb.WriteString(managedBlockBegin + " — regenerated on each \"liferay db up\". Do not edit.\n")
	sb.WriteString(devModeOverrides)
	sb.WriteString(jdbcStanza)
	sb.WriteString(slotStanza)
	sb.WriteString(browserLauncherOverride)
	sb.WriteString(managedBlockEnd + ".\n")

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func checkDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf(
			"docker not found on PATH\n\n" +
				"Install Docker:\n" +
				"  macOS/Windows: https://www.docker.com/products/docker-desktop\n" +
				"  Linux: https://docs.docker.com/engine/install/")
	}
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker is not running — start Docker Desktop (or the Docker daemon) and try again")
	}
	return nil
}
