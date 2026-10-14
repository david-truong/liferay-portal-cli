package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/david-truong/liferay-portal-cli/internal/state"
)

// writeClaim creates ~/.liferay-cli/worktrees/<id>/docker/ports.json holding s.
func writeClaim(t *testing.T, id string, s State) {
	t.Helper()
	dockerDir := filepath.Join(state.Root(), "worktrees", id, "docker")
	if err := os.MkdirAll(dockerDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(s)
	if err := os.WriteFile(filepath.Join(dockerDir, "ports.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func setHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

// livePortal stages a directory with the monorepo markers, so a claim on it
// is still a Liferay project.
func livePortal(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "build.xml"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "modules"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func findClaim(claims []Claim, id string) (Claim, bool) {
	for _, c := range claims {
		if filepath.Base(c.Dir) == id {
			return c, true
		}
	}
	return Claim{}, false
}

func TestScanClaimsRecordedPath(t *testing.T) {
	setHome(t)

	live := livePortal(t)
	gone := filepath.Join(t.TempDir(), "deleted-worktree")

	writeClaim(t, "live-aaaaaaaa", State{Slot: 0, Engine: "mysql", WorktreePath: live})
	writeClaim(t, "gone-bbbbbbbb", State{Slot: 1, Engine: "mysql", WorktreePath: gone})

	claims, err := ScanClaims(nil)
	if err != nil {
		t.Fatal(err)
	}

	if c, ok := findClaim(claims, "live-aaaaaaaa"); !ok || c.Status != ClaimLive {
		t.Errorf("live claim: status = %q, want live", c.Status)
	}
	c, ok := findClaim(claims, "gone-bbbbbbbb")
	if !ok || c.Status != ClaimOrphaned {
		t.Fatalf("gone claim: status = %q, want orphaned", c.Status)
	}
	if c.ResolvedPath != gone || c.Slot != 1 || !c.HasSlot {
		t.Errorf("orphan resolved=%q slot=%d hasSlot=%v", c.ResolvedPath, c.Slot, c.HasSlot)
	}
}

// A Tomcat left running when its worktree was deleted recreates
// bundles/<tomcat>/logs, so the recorded path exists again without being a
// Liferay project. Its claim is orphaned and no longer holds its slot.
func TestScanClaimsRecordedPathNoLongerAProject(t *testing.T) {
	setHome(t)
	shell := filepath.Join(t.TempDir(), "workspaces", "liferay-aihub-workspace")
	if err := os.MkdirAll(filepath.Join(shell, "bundles", "tomcat-10.1.59", "logs"), 0755); err != nil {
		t.Fatal(err)
	}
	writeClaim(t, "shell-dddddddd", State{Slot: 5, WorktreePath: shell})

	claims, err := ScanClaims(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := findClaim(claims, "shell-dddddddd"); c.Status != ClaimOrphaned {
		t.Errorf("status = %q, want orphaned", c.Status)
	}
	if claimedSlots("")[5] {
		t.Error("slot 5 (no longer a Liferay project) should NOT be claimed")
	}
}

// A sparse checkout's root is no Liferay project, yet still a live checkout.
func TestScanClaimsRecordedPathInsideGitCheckout(t *testing.T) {
	setHome(t)
	checkout := t.TempDir()
	if err := os.Mkdir(filepath.Join(checkout, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	writeClaim(t, "sparse-eeeeeeee", State{Slot: 6, WorktreePath: checkout})

	claims, err := ScanClaims(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := findClaim(claims, "sparse-eeeeeeee"); c.Status != ClaimLive {
		t.Errorf("status = %q, want live", c.Status)
	}
}

// A recorded path that still exists is authoritative even when its parent is
// not among liveParents — a live worktree from another repo is never an orphan.
func TestScanClaimsRecordedPathIsCrossRepoSafe(t *testing.T) {
	setHome(t)
	otherRepo := livePortal(t)
	writeClaim(t, "ee-cccccccc", State{Slot: 4, WorktreePath: otherRepo})

	claims, err := ScanClaims([]string{"/some/unrelated/parent"})
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := findClaim(claims, "ee-cccccccc"); c.Status != ClaimLive {
		t.Errorf("status = %q, want live", c.Status)
	}
}

func TestScanClaimsLegacyHashReconstruction(t *testing.T) {
	setHome(t)
	parent := t.TempDir()

	// Orphan: path does not exist, but its hash is reproducible from parent.
	goneBase := "LPD-99999"
	gonePath := filepath.Join(parent, goneBase)
	writeClaim(t, state.ID(gonePath), State{Slot: 7}) // no WorktreePath (legacy)

	// Live: path exists under the same parent.
	liveBase := "LPD-11111"
	livePath := filepath.Join(parent, liveBase)
	if err := os.MkdirAll(filepath.Join(livePath, "modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(livePath, "build.xml"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	writeClaim(t, state.ID(livePath), State{Slot: 8})

	claims, err := ScanClaims([]string{parent})
	if err != nil {
		t.Fatal(err)
	}

	if c, _ := findClaim(claims, state.ID(gonePath)); c.Status != ClaimOrphaned || c.ResolvedPath != gonePath {
		t.Errorf("legacy gone: status=%q resolved=%q, want orphaned %q", c.Status, c.ResolvedPath, gonePath)
	}
	if c, _ := findClaim(claims, state.ID(livePath)); c.Status != ClaimLive {
		t.Errorf("legacy live: status=%q, want live", c.Status)
	}
}

// Legacy state whose path cannot be reproduced from any known parent is left
// untouched rather than guessed at.
func TestScanClaimsLegacyUnknown(t *testing.T) {
	setHome(t)
	writeClaim(t, "LPD-55555-deadbeef", State{Slot: 9}) // no WorktreePath, no matching parent

	claims, err := ScanClaims([]string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := findClaim(claims, "LPD-55555-deadbeef")
	if c.Status != ClaimUnknown {
		t.Errorf("status = %q, want unknown", c.Status)
	}
	if c.ResolvedPath != "" {
		t.Errorf("resolved = %q, want empty", c.ResolvedPath)
	}
}

// A linked (non-primary) worktree must never be handed slot 0 — it is
// reserved for the repository's primary checkout. Asserting ">= 1" keeps the
// test independent of which host ports happen to be bound.
func TestAllocateFreshSlotReservesZeroForLinkedWorktree(t *testing.T) {
	setHome(t)
	if got := allocateFreshSlot(t.TempDir(), false); got < 1 {
		t.Errorf("linked worktree got slot %d, want >= 1 (slot 0 is reserved)", got)
	}
}

// A linked worktree skips both the reserved slot 0 and any slot another
// worktree already claims.
func TestAllocateFreshSlotLinkedSkipsClaimed(t *testing.T) {
	setHome(t)
	live := livePortal(t)
	writeClaim(t, "other-aaaaaaaa", State{Slot: 1, WorktreePath: live})
	if got := allocateFreshSlot(t.TempDir(), false); got < 2 {
		t.Errorf("got slot %d, want >= 2 (0 reserved, 1 claimed)", got)
	}
}

func TestClaimedSlotsSkipsDeletedWorktree(t *testing.T) {
	setHome(t)
	live := livePortal(t)
	gone := filepath.Join(t.TempDir(), "deleted")

	writeClaim(t, "live-12340000", State{Slot: 2, WorktreePath: live})
	writeClaim(t, "gone-12340001", State{Slot: 3, WorktreePath: gone})

	claimed := claimedSlots("")
	if !claimed[2] {
		t.Error("slot 2 (live worktree) should be claimed")
	}
	if claimed[3] {
		t.Error("slot 3 (deleted worktree) should NOT be claimed")
	}
}
