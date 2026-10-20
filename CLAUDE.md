# CLAUDE.md

This repository is the Go source of the `liferay` CLI. It is a tooling repository, not a Liferay portal checkout: the liferay-portal and liferay-portal-ee instructions that load from parent directories do not apply here. That covers Brian Chan's Java style and the `pr-reviewer` rules, the spec and plan workflow, Jira ticket keys, `format-source`, `liferay worktree`, and PRs to `liferay-site-management`.

## Conventions

- Write idiomatic Go that matches the surrounding code. `gofmt`, `go vet`, and `staticcheck` are the formatters and linters.
- Build, test, and pull request requirements are in `CONTRIBUTING.md`: run `go vet ./...`, `staticcheck ./...`, and `go test -race ./...` before pushing, and add an entry to `CHANGELOG.md` under `## [Unreleased]` for every behavior change.
- Commit subjects are plain sentence case with no ticket prefix.
- Open pull requests against `main` of `david-truong/liferay-portal-cli`, one topic per pull request.

## Do not use the CLI on itself

The `liferay` commands operate on liferay-portal checkouts, so never use them to develop this repository. Isolate work with plain `git worktree add -b <branch> .claude/worktrees/<branch> origin/main` (the directory is gitignored), and remove it with `git worktree remove`.

To exercise a change, run it against a portal checkout, for example `go run ./cmd/liferay -C <portal checkout> <command>`.

## Install

`go install ./cmd/liferay` writes the binary to `$(go env GOPATH)/bin`. Confirm with `type -a liferay` that this copy comes first on `PATH`, since a Homebrew install may also be present. On Apple silicon, re-sign the fresh binary with `codesign --sign - --force "$(go env GOPATH)/bin/liferay"`, or macOS kills it on launch.
