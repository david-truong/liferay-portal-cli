package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// BuildBase records the merge-base a worktree's most recent full build ("ant
// all") ran against, so a later rebase that moves the base can be detected
// and the CLI can tell the user a rebuild is due.
type BuildBase struct {
	SHA string `json:"sha"`
}

func buildBasePath(worktreeRoot string) string {
	return filepath.Join(Dir(worktreeRoot), "build_base.json")
}

// SaveBuildBase persists sha as the base the worktree was last fully built
// against.
func SaveBuildBase(worktreeRoot, sha string) error {
	data, err := json.Marshal(BuildBase{SHA: sha})
	if err != nil {
		return err
	}
	p := buildBasePath(worktreeRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return WriteFileAtomic(p, data, 0644)
}

// LoadBuildBase returns (record, true) if a record exists for the worktree;
// (zero, false) when "ant all" has never recorded a base for it.
func LoadBuildBase(worktreeRoot string) (BuildBase, bool, error) {
	data, err := os.ReadFile(buildBasePath(worktreeRoot))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return BuildBase{}, false, nil
		}
		return BuildBase{}, false, err
	}
	var rec BuildBase
	if err := json.Unmarshal(data, &rec); err != nil {
		return BuildBase{}, false, err
	}
	return rec, true, nil
}
