package cli

import (
	"fmt"
	"os"
)

// autofixWorktree propagates the same set of files that "liferay worktree add"
// would have created — symlinks (CLAUDE.md, .claude/, etc.), per-user copies
// (build.*.properties, .env), and generated configs (app.server.<user>.properties,
// bundles/portal-setup-wizard.properties) — for linked worktrees that were
// created with plain "git worktree add" or had files removed since.
//
// Idempotent and quiet: emits a single "[liferay] auto-fixed worktree: ..."
// line per file actually written, and nothing when everything's in place.
func autofixWorktree(portalRoot string) {
	if !isLinkedWorktree(portalRoot) {
		return
	}

	primaryRoot, err := gitPrimaryRoot(portalRoot)
	if err != nil || primaryRoot == portalRoot {
		return
	}

	for _, r := range ensureWorktreeFiles(primaryRoot, portalRoot) {
		switch r.action {
		case "linked", "copied", "generated":
			fmt.Fprintf(os.Stderr, "[liferay] auto-fixed worktree: %s %s\n", r.action, r.name)
		case "failed":
			fmt.Fprintf(os.Stderr, "[liferay] auto-fix failed for %s: %s\n", r.name, r.note)
		}
	}
}
