package docker

import (
	"fmt"
	"net"
)

// Ports holds every host-side port a Liferay instance claims. Slot 0 is the
// stock configuration (Liferay defaults, no bundle edits). Slot N > 0 shifts
// every port so two bundles can run side-by-side on the same host.
type Ports struct {
	Slot int // 0..99

	TomcatHTTP     int // server.xml <Connector port> (default 8080)
	TomcatShutdown int // server.xml <Server port>    (default 8005)
	TomcatRedirect int // server.xml redirectPort     (default 8443)
	JPDA           int // catalina.sh jpda start      (default 8000)
	OSGiConsole    int // module.framework.properties.osgi.console (default 11311)
	ESHTTP         int // elasticsearch sidecarHttpPort    (default 9200)
	ESTransport    int // elasticsearch transportTcpPort   (default 9300)
	Glowroot       int // glowroot/admin.json web.port     (default 4000)
	Arquillian     int // com.liferay.arquillian…ArquillianConnector (default 32763)
	DataGuard      int // com.liferay.data.guard.connector.DataGuardConnector (default 42763)
	MySQL          int // docker MySQL/MariaDB/Postgres host port (default 3306)
	Adminer        int // adminer host port               (default 8081)
}

// base port numbers mirror Liferay's stock configuration.
const (
	baseTomcatHTTP     = 8080
	baseTomcatShutdown = 8005
	baseTomcatRedirect = 8443
	baseJPDA           = 8000
	baseOSGiConsole    = 11311
	baseESHTTP         = 9200
	baseESTransport    = 9300
	baseGlowroot       = 4000
	baseArquillian     = 32763
	baseDataGuard      = 42763
	baseMySQL          = 3306
	baseAdminer        = 8081

	// esOffsetPerSlot keeps ES HTTP and transport from colliding with later
	// slots' HTTP. Matches drewbrokke's multi-bundle scheme (9301/9401).
	esOffsetPerSlot = 101
	// offsetPerSlot is the uniform offset applied to every other service.
	offsetPerSlot = 10
)

// AllocatePorts probes slots 0..99 in order and returns the first one whose
// host-side ports are all free right now. The result is deterministic for a
// given host state — the first worktree booted on a machine claims slot 0 and
// therefore runs against Liferay's stock configuration.
func AllocatePorts() Ports {
	for slot := 0; slot < 100; slot++ {
		p := slotPorts(slot)
		if !AnyPortInUse(ProbePorts(p)...) {
			return p
		}
	}
	return slotPorts(0)
}

// PortsFromSlot reconstructs the full Ports struct from a stored slot number.
func PortsFromSlot(slot int) Ports {
	return slotPorts(slot)
}

// IsStock reports whether the slot is the stock (slot 0) configuration.
// Used by the bundle patcher and portal-ext writer to short-circuit when
// the bundle should stay untouched.
func (p Ports) IsStock() bool {
	return p.Slot == 0
}

func slotPorts(slot int) Ports {
	return Ports{
		Slot:           slot,
		TomcatHTTP:     baseTomcatHTTP + slot*offsetPerSlot,
		TomcatShutdown: baseTomcatShutdown + slot*offsetPerSlot,
		TomcatRedirect: baseTomcatRedirect + slot*offsetPerSlot,
		JPDA:           baseJPDA + slot*offsetPerSlot,
		OSGiConsole:    baseOSGiConsole + slot*offsetPerSlot,
		ESHTTP:         baseESHTTP + slot*esOffsetPerSlot,
		ESTransport:    baseESTransport + slot*esOffsetPerSlot,
		Glowroot:       baseGlowroot + slot*offsetPerSlot,
		Arquillian:     baseArquillian + slot*offsetPerSlot,
		DataGuard:      baseDataGuard + slot*offsetPerSlot,
		MySQL:          baseMySQL + slot*offsetPerSlot,
		Adminer:        baseAdminer + slot*offsetPerSlot,
	}
}

// ProbePorts returns every port AllocatePorts should probe for conflicts.
// ES transport is skipped — sidecar ES picks it at startup, so we don't need
// to reserve it up front.
func ProbePorts(p Ports) []int {
	return []int{
		p.TomcatHTTP, p.TomcatShutdown, p.TomcatRedirect, p.JPDA,
		p.OSGiConsole, p.ESHTTP, p.Glowroot,
		p.Arquillian, p.DataGuard, p.MySQL, p.Adminer,
	}
}

func AnyPortInUse(ports ...int) bool {
	for _, port := range ports {
		if isPortInUse(port) {
			return true
		}
	}
	return false
}

func isPortInUse(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}
