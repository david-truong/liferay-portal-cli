package docker

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"net"
)

// Ports holds the host-side port mapping for a single worktree's Docker stack.
type Ports struct {
	Slot    int // 0–99; each slot is 10 ports wide
	Tomcat  int
	Debug   int
	Gogo    int
	MySQL   int
	Adminer int
}

const (
	baseTomcat  = 8080
	baseDebug   = 8000
	baseGogo    = 13331
	baseMySQL   = 3306
	baseAdminer = 8081
)

// AllocatePorts derives a deterministic slot from worktreeAbsPath (hash % 100),
// probes for port conflicts, and bumps by one slot at a time until a free slot is found.
// Each slot is 10 ports wide, so slots are fully isolated from each other.
func AllocatePorts(worktreeAbsPath string) Ports {
	slot := baseSlot(worktreeAbsPath)
	for i := 0; i < 100; i++ {
		p := slotPorts(slot)
		if !anyPortInUse(p.Tomcat, p.Debug, p.MySQL, p.Adminer) {
			return p
		}
		slot = (slot + 1) % 100
	}
	// All slots busy — return the original slot anyway
	return slotPorts(baseSlot(worktreeAbsPath))
}

// PortsFromSlot reconstructs Ports from a stored slot number.
func PortsFromSlot(slot int) Ports {
	return slotPorts(slot)
}

func baseSlot(path string) int {
	h := sha1.Sum([]byte(path))
	n := binary.BigEndian.Uint32(h[:4])
	return int(n % 100)
}

func slotPorts(slot int) Ports {
	return Ports{
		Slot:    slot,
		Tomcat:  baseTomcat + slot*10,
		Debug:   baseDebug + slot*10,
		Gogo:    baseGogo + slot*10,
		MySQL:   baseMySQL + slot*10,
		Adminer: baseAdminer + slot*10,
	}
}

func anyPortInUse(ports ...int) bool {
	for _, p := range ports {
		if isPortInUse(p) {
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
