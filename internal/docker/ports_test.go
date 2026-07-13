package docker

import (
	"fmt"
	"net"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/hosts"
)

func TestSlot0IsStock(t *testing.T) {
	p := PortsFromSlot(0)
	if p.TomcatHTTP != 8080 {
		t.Errorf("slot 0 TomcatHTTP = %d, want 8080", p.TomcatHTTP)
	}
	if p.MySQL != 3306 {
		t.Errorf("slot 0 MySQL = %d, want 3306", p.MySQL)
	}
	if !p.IsStock() {
		t.Error("slot 0 should be stock")
	}
}

func TestSlotOffsetsAreConsistent(t *testing.T) {
	p0 := PortsFromSlot(0)
	p1 := PortsFromSlot(1)

	if p1.TomcatHTTP-p0.TomcatHTTP != offsetPerSlot {
		t.Errorf("TomcatHTTP offset = %d, want %d", p1.TomcatHTTP-p0.TomcatHTTP, offsetPerSlot)
	}
	if p1.MySQL-p0.MySQL != offsetPerSlot {
		t.Errorf("MySQL offset = %d, want %d", p1.MySQL-p0.MySQL, offsetPerSlot)
	}
	if p1.ESHTTP-p0.ESHTTP != esOffsetPerSlot {
		t.Errorf("ESHTTP offset = %d, want %d", p1.ESHTTP-p0.ESHTTP, esOffsetPerSlot)
	}
	if p1.IsStock() {
		t.Error("slot 1 should not be stock")
	}
}

func TestProbePortsDoesNotIncludeESTransport(t *testing.T) {
	p := PortsFromSlot(0)
	probed := ProbePorts(p)
	for _, port := range probed {
		if port == p.ESTransport {
			t.Error("ProbePorts should not include ESTransport")
		}
	}
}

func TestAnyPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	if !AnyPortInUse(port) {
		t.Errorf("port %d should be in use", port)
	}
	if AnyPortInUse(0) {
		t.Error("port 0 should not be reported as in use")
	}
}

// TestAnyPortInUseDetectsWildcardBind reproduces HIGH-2: a listener bound to
// 0.0.0.0 (as real services like Tomcat and docker-proxy do) must be detected
// even though a loopback-only bind on the same port can still succeed due to
// SO_REUSEADDR semantics on macOS/BSD. A dial-based check catches this where
// a bind-only check would miss it.
func TestAnyPortInUseDetectsWildcardBind(t *testing.T) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	if !AnyPortInUse(port) {
		t.Errorf("port %d has a wildcard listener and should be reported in use", port)
	}
}

// TestNoPortCollisionsAcrossSlots proves every probed port is unique across
// the slot pool (the only range with a precreated hostname, and therefore the
// only range a worktree is actually expected to run in). A base port that is
// a multiple of offsetPerSlot away from another base (e.g. baseJPDA=8000 vs
// baseTomcatHTTP=8080, 80 apart) makes the two ports alias at some slot
// distance — here 8, so slot N's JPDA port equals slot (N-8)'s Tomcat HTTP
// port whenever both run at once.
func TestNoPortCollisionsAcrossSlots(t *testing.T) {
	seen := make(map[int]int)
	for slot := 0; slot < hosts.SlotPoolSize; slot++ {
		for _, port := range ProbePorts(slotPorts(slot)) {
			if otherSlot, ok := seen[port]; ok {
				t.Fatalf("port %d used by both slot %d and slot %d", port, otherSlot, slot)
			}
			seen[port] = slot
		}
	}
}

func TestAllocatePortsSkipsOccupied(t *testing.T) {
	slot0 := PortsFromSlot(0)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", slot0.Adminer))
	if err != nil {
		t.Skipf("cannot bind slot 0 Adminer port %d: %v", slot0.Adminer, err)
	}
	defer ln.Close()

	got := AllocatePorts()
	if got.Slot == 0 {
		t.Error("AllocatePorts returned slot 0 even though a slot-0 port is occupied")
	}
}
