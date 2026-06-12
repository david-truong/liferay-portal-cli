package hosts

import "fmt"

// The slot pool precreates one hostname per slot so whatever worktree claims
// slot N is always reachable at the same name — no per-worktree hosts edit,
// and nothing changes when a worktree is removed and a new one takes its
// slot. Pool entries reuse the regular managed-line machinery with a
// synthetic "slot-<n>" owner id, so they show up in `liferay hosts list` and
// never collide with per-worktree entries (worktree ids always end in an
// 8-hex hash).

// SlotPoolSize is how many slots get a precreated hostname. Ten covers every
// realistic number of concurrent worktrees on one machine.
const SlotPoolSize = 10

// SlotID returns the synthetic owner id for slot's pool entry.
func SlotID(slot int) string {
	return fmt.Sprintf("slot-%d", slot)
}

// SlotName returns the pool hostname for slot. The reserved .test TLD never
// resolves in real DNS and is exempt from mDNS, so the names are safe to
// claim locally.
func SlotName(slot int) string {
	return fmt.Sprintf("slot%d.liferay.test", slot)
}

// UpsertSlotPool returns content with a managed line present for every slot
// in the pool. Existing pool lines are rewritten in place, so the call is
// idempotent.
func UpsertSlotPool(content string) (string, error) {
	for slot := 0; slot < SlotPoolSize; slot++ {
		var err error
		content, err = Upsert(content, SlotName(slot), SlotID(slot))
		if err != nil {
			return "", err
		}
	}
	return content, nil
}

// SlotHostname returns the pool hostname for slot when its managed entry
// exists in content, or "" when the pool has not been installed.
func SlotHostname(content string, slot int) string {
	id := SlotID(slot)
	for _, e := range List(content) {
		if e.ID == id {
			return e.Name
		}
	}
	return ""
}
