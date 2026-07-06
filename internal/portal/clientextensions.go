package portal

import (
	"os"
	"path/filepath"
)

// BuildClientExtensionIndex indexes every client extension under portalRoot,
// recognizing two conventions: the monorepo's nested
// workspaces/<workspace>/client-extensions/<name>/, and a standalone Liferay
// Workspace's top-level client-extensions/<name>/. Both are identified by a
// client-extension.yaml file. The returned index resolves by bare name,
// falling back to a "workspace/name" qualifier when the same name exists in
// more than one place.
func BuildClientExtensionIndex(portalRoot string) (*ModuleIndex, error) {
	idx := &ModuleIndex{
		byName:    make(map[string][]string),
		bySuffix:  make(map[string][]string),
		noun:      "client extension",
		qualifier: "workspace/name",
	}

	workspacesDir := filepath.Join(portalRoot, "workspaces")
	if workspaces, err := os.ReadDir(workspacesDir); err == nil {
		for _, ws := range workspaces {
			if !ws.IsDir() {
				continue
			}
			ceRoot := filepath.Join(workspacesDir, ws.Name(), "client-extensions")
			indexClientExtensions(idx, ceRoot, ws.Name())
		}
	}

	indexClientExtensions(idx, filepath.Join(portalRoot, "client-extensions"), filepath.Base(portalRoot))

	return idx, nil
}

// indexClientExtensions adds every client-extension.yaml-bearing directory
// under ceRoot to idx, qualified by workspaceName.
func indexClientExtensions(idx *ModuleIndex, ceRoot, workspaceName string) {
	entries, err := os.ReadDir(ceRoot)
	if err != nil {
		return
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
		suffix := workspaceName + "/" + e.Name()
		idx.bySuffix[suffix] = appendUnique(idx.bySuffix[suffix], abs)
	}
}
