package docker

import (
	"fmt"
	"net"
	"testing"
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
