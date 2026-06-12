package hosts

import (
	"strings"
	"testing"
)

func TestUpsertSlotPoolInstallsEverySlot(t *testing.T) {
	content := "127.0.0.1\tlocalhost\n"

	updated, err := UpsertSlotPool(content)
	if err != nil {
		t.Fatalf("UpsertSlotPool: %v", err)
	}

	if !strings.HasPrefix(updated, "127.0.0.1\tlocalhost\n") {
		t.Fatalf("existing content disturbed:\n%s", updated)
	}
	for slot := 0; slot < SlotPoolSize; slot++ {
		if got := SlotHostname(updated, slot); got != SlotName(slot) {
			t.Errorf("slot %d hostname = %q, want %q", slot, got, SlotName(slot))
		}
	}
}

func TestUpsertSlotPoolIsIdempotent(t *testing.T) {
	once, err := UpsertSlotPool("")
	if err != nil {
		t.Fatalf("UpsertSlotPool: %v", err)
	}
	twice, err := UpsertSlotPool(once)
	if err != nil {
		t.Fatalf("UpsertSlotPool (second): %v", err)
	}
	if once != twice {
		t.Errorf("second install changed content:\nfirst:\n%s\nsecond:\n%s", once, twice)
	}
}

func TestSlotPoolCoexistsWithWorktreeEntries(t *testing.T) {
	content, err := Upsert("", "lpd-12345", "LPD-12345-deadbeef")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	content, err = UpsertSlotPool(content)
	if err != nil {
		t.Fatalf("UpsertSlotPool: %v", err)
	}

	if got := SlotHostname(content, 3); got != "slot3.liferay.test" {
		t.Errorf("slot 3 hostname = %q", got)
	}
	entries := List(content)
	if len(entries) != SlotPoolSize+1 {
		t.Fatalf("entries = %d, want %d", len(entries), SlotPoolSize+1)
	}
	if entries[0].Name != "lpd-12345" {
		t.Errorf("worktree entry displaced: %+v", entries[0])
	}
}

func TestSlotHostnameMissingPool(t *testing.T) {
	if got := SlotHostname("127.0.0.1\tlocalhost\n", 0); got != "" {
		t.Errorf("SlotHostname on plain file = %q, want empty", got)
	}
}
