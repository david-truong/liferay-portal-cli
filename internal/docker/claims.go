package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"github.com/david-truong/liferay-portal-cli/internal/fsutil"
	"github.com/david-truong/liferay-portal-cli/internal/state"
)

// ClaimStatus classifies a per-worktree CLI state directory.
type ClaimStatus string

const (
	// ClaimLive: the worktree this state belongs to still exists.
	ClaimLive ClaimStatus = "live"
	// ClaimOrphaned: the worktree was deleted; the state dir (and any slot
	// it claims) is safe to prune.
	ClaimOrphaned ClaimStatus = "orphaned"
	// ClaimUnknown: the worktree path could not be determined (legacy state
	// with no recorded path, and no live-worktree parent reproduces the
	// directory's hash). Never pruned — this is the cross-repo safety valve.
	ClaimUnknown ClaimStatus = "unknown"
)

// Claim describes one directory under ~/.liferay-cli/worktrees/.
type Claim struct {
	// Dir is the state directory root (~/.liferay-cli/worktrees/<id>), the
	// path "prune" removes.
	Dir string
	// HasSlot is true when the dir holds a docker/ports.json and therefore
	// claims a slot.
	HasSlot bool
	Slot    int
	Engine  string
	// WorktreePath is the path recorded in ports.json (empty for legacy
	// state). ResolvedPath is the worktree path used to decide Status:
	// the recorded path, or the hash-reconstructed candidate for legacy
	// dirs. Empty when Status is ClaimUnknown.
	WorktreePath string
	ResolvedPath string
	Status       ClaimStatus
}

// hashSuffix matches the trailing "-<8 hex>" that state.ID appends. The 8 hex
// chars are the first 4 bytes of sha1(absolute worktree path).
var hashSuffix = regexp.MustCompile(`^(.*)-[0-9a-f]{8}$`)

// ScanClaims classifies every directory under ~/.liferay-cli/worktrees/.
//
// liveParents is the set of parent directories of the current repo's live
// worktrees; it seeds path reconstruction for legacy state dirs that predate
// path recording. A legacy dir is only classified orphaned when a candidate
// path positively reproduces its hash (proving the path) AND that path is
// absent — so a live worktree from another repo is never mistaken for an
// orphan.
func ScanClaims(liveParents []string) ([]Claim, error) {
	worktreesDir := filepath.Join(state.Root(), "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	claims := make([]Claim, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirName := e.Name()
		dir := filepath.Join(worktreesDir, dirName)

		claim := Claim{Dir: dir}

		var s State
		if data, err := os.ReadFile(filepath.Join(dir, "docker", "ports.json")); err == nil {
			if json.Unmarshal(data, &s) == nil {
				claim.HasSlot = true
				claim.Slot = s.Slot
				claim.Engine = s.Engine
				claim.WorktreePath = s.WorktreePath
			}
		}

		claim.ResolvedPath, claim.Status = resolveStatus(dirName, s.WorktreePath, liveParents)
		claims = append(claims, claim)
	}
	return claims, nil
}

// resolveStatus determines a claim's status. A recorded path is authoritative
// (repo-agnostic). Otherwise it reconstructs the path from the dir basename
// against each live-worktree parent, trusting a candidate only when its hash
// matches the directory name.
func resolveStatus(dirName, recordedPath string, liveParents []string) (string, ClaimStatus) {
	if recordedPath != "" {
		if fsutil.Exists(recordedPath) {
			return recordedPath, ClaimLive
		}
		return recordedPath, ClaimOrphaned
	}

	m := hashSuffix.FindStringSubmatch(dirName)
	if m == nil {
		return "", ClaimUnknown
	}
	basename := m[1]
	for _, parent := range liveParents {
		candidate := filepath.Join(parent, basename)
		if state.ID(candidate) != dirName {
			continue
		}
		if fsutil.Exists(candidate) {
			return candidate, ClaimLive
		}
		return candidate, ClaimOrphaned
	}
	return "", ClaimUnknown
}
