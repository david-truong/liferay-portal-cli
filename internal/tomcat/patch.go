package tomcat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
)

// PatchBundle applies the set of per-slot edits that let multiple Liferay
// bundles run on the same host. For slot 0 (stock) it is a no-op — the user
// wants the first instance untouched. Every edit is idempotent so it is safe
// to call on every "liferay server start", including after a full rebuild that
// wipes the OSGi configs and the embedded portal-developer.properties.
//
// Files patched (slot > 0 only):
//   - tomcat-*/conf/server.xml              (shutdown, HTTP, redirect ports)
//   - tomcat-*/bin/setenv.sh                (JPDA_ADDRESS)
//   - <bundle>/portal-developer.properties  (OSGi console)
//   - tomcat-*/webapps/ROOT/WEB-INF/classes/portal-developer.properties (embedded)
//   - <bundle>/glowroot/admin.json          (web.port)
//   - <bundle>/osgi/configs/com.liferay.portal.search.elasticsearch8.configuration.ElasticsearchConfiguration.config
//   - <bundle>/osgi/configs/com.liferay.arquillian.extension.junit.bridge.connector.ArquillianConnector.config
//   - <bundle>/osgi/configs/com.liferay.data.guard.connector.DataGuardConnector.config
func PatchBundle(paths Paths, ports docker.Ports) error {
	if ports.IsStock() {
		return nil
	}

	// Snapshot the pre-patch state of every file the steps below will
	// touch, so a partial failure or an explicit `liferay bundle unpatch`
	// can roll back to a known-good state.
	stateDir := filepath.Dir(paths.PidFile)
	if _, err := Snapshot(paths, stateDir); err != nil {
		return fmt.Errorf("snapshotting bundle before patch: %w", err)
	}

	steps := []func() error{
		func() error { return patchServerXML(paths.Tomcat, ports) },
		func() error { return patchSetenvSh(paths.Bin, ports) },
		func() error { return patchPortalDeveloperProps(paths.Bundle, ports) },
		func() error { return patchEmbeddedPortalDeveloperProps(paths.Tomcat, ports) },
		func() error { return patchGlowrootAdmin(paths.Bundle, ports) },
		func() error { return writeElasticsearchConfig(paths.Bundle, ports) },
		func() error { return writeArquillianConfig(paths.Bundle, ports) },
		func() error { return writeDataGuardConfig(paths.Bundle, ports) },
	}

	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// patchServerXML rewrites Tomcat's shutdown port, HTTP connector port, and
// redirectPort attributes. The rewriter is line-based and comment-aware so
// stock server.xml's commented-out AJP and HTTPS connector examples stay
// dormant. Inline HTML comments that open and close on the same line are
// ignored; a multi-line <!-- … --> block is tracked so its contents are not
// touched.
func patchServerXML(tomcatDir string, ports docker.Ports) error {
	path := filepath.Join(tomcatDir, "conf", "server.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	return writePreservingMode(path, []byte(rewriteServerXML(string(data), ports)), 0644)
}

func rewriteServerXML(input string, ports docker.Ports) string {
	lines := strings.Split(input, "\n")

	inBlockComment := false
	inConnector := false
	var connectorLines []int

	closeConnector := func() {
		if shouldRewriteConnector(lines, connectorLines) {
			for _, i := range connectorLines {
				lines[i] = connectorPortRE.ReplaceAllString(lines[i],
					fmt.Sprintf(`${1}%d${2}`, ports.TomcatHTTP))
				lines[i] = redirectPortRE.ReplaceAllString(lines[i],
					fmt.Sprintf(`redirectPort="%d"`, ports.TomcatRedirect))
			}
		}
		connectorLines = nil
		inConnector = false
	}

	for i, raw := range lines {
		line := raw

		// Multi-line comment tracking. Handles the common case where the
		// opening and closing tokens are on different lines.
		if inBlockComment {
			if strings.Contains(line, "-->") {
				inBlockComment = false
			}
			continue
		}
		if idx := strings.Index(line, "<!--"); idx >= 0 && !strings.Contains(line[idx:], "-->") {
			inBlockComment = true
			continue
		}

		// <Server port="..." shutdown="SHUTDOWN"> — always a single line in
		// the stock file, exactly one match. Apply unconditionally; connector
		// rewriting happens in a deferred batch when the connector closes.
		lines[i] = serverShutdownRE.ReplaceAllString(line,
			fmt.Sprintf(`${1}%d${2}`, ports.TomcatShutdown))

		if !inConnector && strings.Contains(lines[i], "<Connector ") {
			inConnector = true
			connectorLines = []int{i}
			if strings.Contains(lines[i], "/>") || strings.Contains(lines[i], "</Connector>") {
				closeConnector()
			}
			continue
		}

		if inConnector {
			connectorLines = append(connectorLines, i)
			if strings.Contains(lines[i], "/>") || strings.Contains(lines[i], "</Connector>") {
				closeConnector()
			}
		}
	}

	return strings.Join(lines, "\n")
}

// shouldRewriteConnector returns false when any line of the connector block
// declares protocol="AJP…", so an enabled AJP connector keeps its declared
// port instead of getting clobbered to the HTTP port. Stock Liferay leaves
// AJP commented out, but users who uncomment it expect AJP to keep working.
func shouldRewriteConnector(lines []string, indices []int) bool {
	for _, i := range indices {
		if ajpProtocolRE.MatchString(lines[i]) {
			return false
		}
	}
	return true
}

var (
	serverShutdownRE = regexp.MustCompile(`(<Server\s+port=")\d+(")`)
	connectorPortRE  = regexp.MustCompile(`(\bport=")\d+(")`)
	redirectPortRE   = regexp.MustCompile(`redirectPort="\d+"`)
	ajpProtocolRE    = regexp.MustCompile(`protocol="AJP`)
)

// patchSetenvSh ensures the JPDA debug port in setenv.sh matches this slot.
// Stock Liferay's setenv.sh does not set JPDA_ADDRESS at all (catalina.sh
// defaults it to 8000 when running "jpda start"). For slot > 0 we append our
// own JPDA_ADDRESS line if none is present, or rewrite whatever is there.
//
// The write preserves the existing file's mode so a `chmod g+w setenv.sh`
// is not silently undone. (Go's os.WriteFile is already a no-op on the
// perm arg when the file exists, but going through writePreservingMode
// makes the contract explicit for reviewers and insulates against future
// changes to Go's open-file semantics.)
func patchSetenvSh(binDir string, ports docker.Ports) error {
	path := filepath.Join(binDir, "setenv.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)
	line := fmt.Sprintf("export JPDA_ADDRESS=%d", ports.JPDA)

	if jpdaAddressRE.MatchString(content) {
		content = jpdaAddressRE.ReplaceAllString(content, line)
	} else {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n# Added by liferay-cli: per-slot JPDA debug port.\n" + line + "\n"
	}

	return writePreservingMode(path, []byte(content), 0755)
}

// writePreservingMode replaces path's content. For an existing file the
// existing mode is preserved (regardless of defaultMode); for a new file
// defaultMode is applied (modulo umask).
func writePreservingMode(path string, content []byte, defaultMode os.FileMode) error {
	mode := defaultMode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, content, mode)
}

var jpdaAddressRE = regexp.MustCompile(`(?m)^\s*export\s+JPDA_ADDRESS=.*$`)

// patchPortalDeveloperProps rewrites (or creates) the bundle's root-level
// portal-developer.properties with the slot's OSGi console address. Users who
// add other developer overrides here keep them — only the one key we manage
// is touched.
func patchPortalDeveloperProps(bundleDir string, ports docker.Ports) error {
	path := filepath.Join(bundleDir, "portal-developer.properties")
	return upsertPropertyLine(path, osgiConsoleKey,
		fmt.Sprintf("%s=localhost:%d", osgiConsoleKey, ports.OSGiConsole))
}

// patchEmbeddedPortalDeveloperProps rewrites the copy of
// portal-developer.properties baked into the ROOT webapp. This copy is
// re-extracted on every full rebuild, so we have to re-apply the patch each
// "server start".
func patchEmbeddedPortalDeveloperProps(tomcatDir string, ports docker.Ports) error {
	path := filepath.Join(tomcatDir, "webapps", "ROOT", "WEB-INF", "classes", "portal-developer.properties")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // not extracted yet; nothing to rewrite
	}
	return upsertPropertyLine(path, osgiConsoleKey,
		fmt.Sprintf("%s=localhost:%d", osgiConsoleKey, ports.OSGiConsole))
}

const osgiConsoleKey = "module.framework.properties.osgi.console"

// upsertPropertyLine replaces the line beginning with key= with the full
// provided replacement, or appends the replacement if no such line exists.
// The file is created if missing.
func upsertPropertyLine(path, key, replacement string) error {
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}

	prefix := key + "="
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = replacement
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, replacement)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// patchGlowrootAdmin rewrites the "web.port" field inside glowroot/admin.json.
// glowroot ships with a JSON config that uses 4000 by default. If admin.json
// doesn't exist yet (user hasn't run glowroot), we skip.
func patchGlowrootAdmin(bundleDir string, ports docker.Ports) error {
	path := filepath.Join(bundleDir, "glowroot", "admin.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	web, _ := config["web"].(map[string]any)
	if web == nil {
		web = map[string]any{}
		config["web"] = web
	}
	web["port"] = ports.Glowroot

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}

// writeElasticsearchConfig materialises the OSGi config that pins the sidecar
// ES HTTP + transport ports and binds both sockets to IPv4 loopback (avoids
// occasional IPv6 binding failures drewbrokke documented). Both the ES7 and
// ES8 config files are written so the correct one is picked up regardless of
// which Elasticsearch bundle is installed in the portal.
func writeElasticsearchConfig(bundleDir string, ports docker.Ports) error {
	content := fmt.Sprintf(
		"sidecarHttpPort=\"%d\"\n"+
			"transportTcpPort=\"%d\"\n"+
			"networkBindHost=\"127.0.0.1\"\n"+
			"networkPublishHost=\"127.0.0.1\"\n",
		ports.ESHTTP, ports.ESTransport)

	for _, pid := range []string{
		"com.liferay.portal.search.elasticsearch7.configuration.ElasticsearchConfiguration",
		"com.liferay.portal.search.elasticsearch8.configuration.ElasticsearchConfiguration",
	} {
		path := filepath.Join(bundleDir, "osgi", "configs", pid+".config")
		if err := writeOSGiConfig(path, content); err != nil {
			return err
		}
	}
	return nil
}

// writeArquillianConfig sets the Arquillian junit-bridge connector port so
// slot>0 bundles don't fight slot 0 for 32763.
func writeArquillianConfig(bundleDir string, ports docker.Ports) error {
	path := filepath.Join(bundleDir, "osgi", "configs",
		"com.liferay.arquillian.extension.junit.bridge.connector.ArquillianConnector.config")
	return writeOSGiConfig(path, fmt.Sprintf("port=\"%d\"\n", ports.Arquillian))
}

// writeDataGuardConfig sets the DataGuard connector port for the same reason.
func writeDataGuardConfig(bundleDir string, ports docker.Ports) error {
	path := filepath.Join(bundleDir, "osgi", "configs",
		"com.liferay.data.guard.connector.DataGuardConnector.config")
	return writeOSGiConfig(path, fmt.Sprintf("port=\"%d\"\n", ports.DataGuard))
}

func writeOSGiConfig(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}
