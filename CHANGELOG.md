# Changelog

All notable changes to `liferay-portal-cli` will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Added

- `LICENSE`, `CONTRIBUTING.md`, `SECURITY.md`, `CHANGELOG.md` at repo root.

### Changed

- `go.mod` bumped to `go 1.23`.

## Tech debt accepted in v1

These items are known limitations documented for users and tracked for
future work. They are not bugs.

- **Regex-based `server.xml` rewriter** (`internal/tomcat/patch.go`).
  `rewriteServerXML` parses Tomcat's `server.xml` line by line with regexes
  rather than a real XML parser. The known fixture shapes (stock, slot 1,
  slot 12, AJP-enabled, multi-line connectors, comment-spanning connectors)
  are covered by golden tests. New Tomcat connector attribute shapes may
  require a fixture update or a future migration to `encoding/xml`.
- **4-byte SHA-1 truncation in `state.Dir`** (`internal/state/state.go`).
  The per-worktree state directory under `~/.liferay-cli/worktrees/` is
  suffixed with a 4-byte (32-bit) hash of the absolute worktree path.
  Birthday-collision probability becomes meaningful around ~65k distinct
  worktree paths on the same host. Acceptable for v1; a one-shot migration
  to 8 bytes is planned for a future release.
- **Homebrew tap on a personal account** (`david-truong/homebrew-liferay`).
  The Homebrew formula is published under a personal GitHub account rather
  than an organization-owned tap. If the account becomes unavailable, the
  `brew install` snippet in `README.md` breaks. Migration to an
  organization-owned tap is planned but deferred from v1.
