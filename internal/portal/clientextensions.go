package portal

import (
	"os"
	"path/filepath"
)

// BuildClientExtensionIndex walks workspaces/<workspace>/client-extensions/<name>
// under portalRoot and indexes every directory containing a client-extension.yaml.
// The returned index resolves by bare name, falling back to a "workspace/name"
// qualifier when the same name exists in more than one workspace.
func BuildClientExtensionIndex(portalRoot string) (*ModuleIndex, error) {
	idx := &ModuleIndex{
		byName:    make(map[string][]string),
		bySuffix:  make(map[string][]string),
		noun:      "client extension",
		qualifier: "workspace/name",
	}

	workspacesDir := filepath.Join(portalRoot, "workspaces")
	workspaces, err := os.ReadDir(workspacesDir)
	if err != nil {
		return idx, nil // no workspaces directory → empty index
	}

	for _, ws := range workspaces {
		if !ws.IsDir() {
			continue
		}
		ceRoot := filepath.Join(workspacesDir, ws.Name(), "client-extensions")
		entries, err := os.ReadDir(ceRoot)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			abs := filepath.Join(ceRoot, e.Name())
			if !fileExists(filepath.Join(abs, "client-extension.yaml")) {
				continue
			}
			idx.byName[e.Name()] = appendUnique(idx.byName[e.Name()], abs)
			suffix := ws.Name() + "/" + e.Name()
			idx.bySuffix[suffix] = appendUnique(idx.bySuffix[suffix], abs)
		}
	}
	return idx, nil
}
