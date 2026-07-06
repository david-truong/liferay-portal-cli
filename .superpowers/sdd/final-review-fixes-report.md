# Final review doc fixes

Two documentation-only findings from the whole-branch review, fixed in
`liferay-portal-cli-worktrees/workspace-support`. No behavior changed.

## Finding 1 — internal/cli/build.go

### `buildCmd.Long`

Before:
```
Long: `With no arguments: runs "ant all" from the portal root (full rebuild).
With module names: resolves each to its directory and runs "gw deploy -a".
```

After:
```
Long: `With no arguments: runs "ant all" from the portal root (full rebuild).
On a Liferay Workspace, no arguments deploys every discovered OSGi module
instead; client extensions are not included and must be deployed individually
via "liferay client-extension <name>".
With module names: resolves each to its directory and runs "gw deploy -a".
```

### `runWorkspaceBuildAll` doc comment

Before:
```go
// runWorkspaceBuildAll mirrors "ant all" for a Liferay Workspace: assemble
// the bundle if it doesn't exist yet, then deploy every discovered module.
```

After:
```go
// runWorkspaceBuildAll mirrors "ant all" for a Liferay Workspace: assemble
// the bundle if it doesn't exist yet, then deploy every discovered module.
// This covers OSGi modules only — client extensions live under a separate
// directory and are deployed individually via "liferay client-extension".
```

## Finding 2 — internal/cli/autofix.go

### `autofixWorktree` doc comment

Before:
```go
// autofixWorktree propagates the same set of files that "liferay worktree add"
// would have created — symlinks (CLAUDE.md, .claude/, etc.), per-user copies
// (build.*.properties, .env), and generated configs (app.server.<user>.properties,
// bundles/portal-setup-wizard.properties) — for linked worktrees that were
// created with plain "git worktree add" or had files removed since.
```

After:
```go
// autofixWorktree propagates the same set of files that "liferay worktree add"
// would have created — symlinks (CLAUDE.md, .claude/, etc.), per-user copies
// (build.*.properties, .env), and, for Monorepo projects only, generated
// configs (app.server.<user>.properties, bundles/portal-setup-wizard.properties)
// — for linked worktrees that were created with plain "git worktree add" or
// had files removed since.
```

## Verification

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./internal/cli/... -v` — all tests pass (including
  `TestEnsureWorktreeFiles_WorkspaceSkipsAppServerProperties` and
  `TestEnsureWorktreeFiles_WorkspaceSkipsSetupWizard`, which already codify
  the Monorepo-only behavior referenced in the updated comment)

## Commit

`d756256` — "Document Workspace build's module-only scope and fix stale autofix comment"
