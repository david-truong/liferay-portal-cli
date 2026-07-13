package cli

import (
	"fmt"
	"os"

	"github.com/david-truong/liferay-portal-cli/internal/portal"
)

// autofixWorktree propagates the same set of files that "liferay worktree add"
// would have created — symlinks (CLAUDE.md, CLAUDE.local.md, .claude/, etc.),
// per-user copies
// (build.*.properties, .env), and, for Monorepo projects only, generated
// configs (app.server.<user>.properties, bundles/portal-setup-wizard.properties)
// — for linked worktrees that were created with plain "git worktree add" or
// had files removed since.
//
// A standalone (non-worktree) Workspace checkout also gets the setup-wizard
// file regenerated: unlike a Monorepo primary, a Workspace never gets the
// "stock, slot 0, never touched" treatment (see isPrimarySlot), so "server
// wipe" always deletes its wizard file and it needs the same self-healing.
//
// Idempotent and quiet: emits a single "[liferay] auto-fixed worktree: ..."
// line per file actually written, and nothing when everything's in place.
func autofixWorktree(portalRoot string) {
	if !isLinkedWorktree(portalRoot) {
		if portal.DetectProjectType(portalRoot) == portal.Workspace {
			runAutofix(portalRoot, portalRoot, portal.Workspace)
		}
		return
	}

	primaryRoot, err := gitPrimaryRoot(portalRoot)
	if err != nil || primaryRoot == portalRoot {
		return
	}

	runAutofix(primaryRoot, portalRoot, portal.DetectProjectType(primaryRoot))
}

func runAutofix(primaryRoot, worktreeRoot string, projectType portal.ProjectType) {
	for _, r := range ensureWorktreeFiles(primaryRoot, worktreeRoot, projectType) {
		switch r.action {
		case "linked", "copied", "generated":
			fmt.Fprintf(os.Stderr, "[liferay] auto-fixed worktree: %s %s\n", r.action, r.name)
		case "failed":
			fmt.Fprintf(os.Stderr, "[liferay] auto-fix failed for %s: %s\n", r.name, r.note)
		}
	}
}
