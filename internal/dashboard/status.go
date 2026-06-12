package dashboard

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/david-truong/liferay-portal-cli/internal/docker"
	"github.com/david-truong/liferay-portal-cli/internal/portal"
	"github.com/david-truong/liferay-portal-cli/internal/tomcat"
)

// TomcatState is the dashboard's read of a worktree's Tomcat.
type TomcatState int

const (
	TomcatStopped TomcatState = iota
	TomcatStale               // pid file present, process gone
	TomcatStarting            // process alive, HTTP not answering yet
	TomcatReady               // process alive and HTTP answering
)

// Status is one probe result for one worktree.
type Status struct {
	Tomcat TomcatState
	PID    int
	DBUp   bool
	CatOut string          // catalina.out path, "" when the bundle is unresolvable
	Flags  map[string]bool // branch flag -> enabled in portal-ext.properties
	Err    error           // bundle resolution failure (e.g. never built)
}

// probe inspects one worktree. Everything is path-parameterized, so no chdir
// is needed and probes for different worktrees can run concurrently.
func probe(w Worktree) Status {
	ports := docker.PortsFromSlot(effectiveSlot(w))

	var st Status

	bundleDir, err := portal.BundleDir(w.Path)
	if err == nil {
		var paths tomcat.Paths
		paths, err = tomcat.Resolve(w.Path, bundleDir)
		if err == nil {
			st.CatOut = paths.CatOut

			pid, alive := tomcat.Status(paths)
			st.PID = pid
			switch {
			case alive && httpReady(ports.TomcatHTTP):
				st.Tomcat = TomcatReady
			case alive:
				st.Tomcat = TomcatStarting
			case pid > 0:
				st.Tomcat = TomcatStale
			}
		}
	}
	st.Err = err

	if docker.IsDockerManagedEngine(w.Engine) {
		st.DBUp = portOpen(ports.MySQL)
	}

	if len(w.Flags) > 0 {
		if path, err := portalExtPath(w.Path); err == nil {
			data, _ := os.ReadFile(path)
			st.Flags = flagStates(string(data), w.Flags)
		}
	}

	return st
}

// probeAll probes every worktree concurrently. A full round is bounded by
// the slowest single probe (≤1s when a Tomcat is mid-boot), not the sum.
func probeAll(worktrees []Worktree) []Status {
	statuses := make([]Status, len(worktrees))

	var wg sync.WaitGroup
	for i, w := range worktrees {
		wg.Add(1)
		go func(i int, w Worktree) {
			defer wg.Done()
			statuses[i] = probe(w)
		}(i, w)
	}
	wg.Wait()

	return statuses
}

// effectiveSlot maps "never claimed a slot" (-1) to slot 0, since a worktree
// without CLI state runs against Liferay's stock ports.
func effectiveSlot(w Worktree) int {
	if w.Slot < 0 {
		return 0
	}
	return w.Slot
}

// httpReady reports whether anything answers HTTP on the port. Any response,
// including an error status, counts — Tomcat is accepting requests.
func httpReady(port int) bool {
	client := &http.Client{Timeout: 750 * time.Millisecond}

	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/", port))
	if err != nil {
		return false
	}
	resp.Body.Close()

	return true
}

// portOpen reports whether something is listening on the port.
func portOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 250*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()

	return true
}
