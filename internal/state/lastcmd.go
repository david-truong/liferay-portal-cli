package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// LastCmdKind tags which subsystem last produced output, so "liferay logs"
// can route to the right viewer.
type LastCmdKind string

const (
	LastCmdArchive LastCmdKind = "archive" // build/test/deploy/etc — replayable file under <stateDir>/logs/
	LastCmdServer  LastCmdKind = "server"  // tomcat — live tail of catalina.out
	LastCmdDB      LastCmdKind = "db"      // docker compose — live tail via docker
)

type LastCmd struct {
	Kind    LastCmdKind `json:"kind"`
	LogPath string      `json:"log_path,omitempty"` // archive: the saved transcript; server: catalina.out
	Service string      `json:"service,omitempty"`  // db: compose service name
	When    time.Time   `json:"when"`
}

func lastCmdPath(worktreeRoot string) string {
	return filepath.Join(Dir(worktreeRoot), "last_command.json")
}

// SaveLastCmd persists rec for the given worktree. Best-effort — callers
// generally ignore the error so a transient FS issue doesn't break the
// underlying command.
func SaveLastCmd(worktreeRoot string, rec LastCmd) error {
	rec.When = time.Now()
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	p := lastCmdPath(worktreeRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

// LoadLastCmd returns (record, true) if a record exists for the worktree;
// (zero, false) when no record has been written yet.
func LoadLastCmd(worktreeRoot string) (LastCmd, bool, error) {
	data, err := os.ReadFile(lastCmdPath(worktreeRoot))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return LastCmd{}, false, nil
		}
		return LastCmd{}, false, err
	}
	var rec LastCmd
	if err := json.Unmarshal(data, &rec); err != nil {
		return LastCmd{}, false, err
	}
	return rec, true, nil
}
