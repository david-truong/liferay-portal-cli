# Changelog

All notable changes to `liferay-portal-cli` will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

The production-readiness pass that closes the four audit blockers
identified in `tasks/liferay-portal-cli-production-readiness-audit.md`.

### Added

- `LICENSE` (MIT), `CONTRIBUTING.md`, `SECURITY.md`, `CHANGELOG.md` at
  repo root.
- `liferay bundle unpatch` restores the active bundle from the most
  recent pre-patch snapshot. Snapshots are taken automatically before
  every `liferay server start`'s slot patches.
- `--json` flag on `liferay server status`, `liferay db ps`, and
  `liferay worktree list`. Schemas are part of the stable CLI surface
  (see each command's `--help`).
- `--yes` flag on `liferay server wipe` and `liferay worktree remove`
  for non-interactive consent. `LIFERAY_CLI_ASSUME_YES=1` environment
  variable applies the same bypass globally.
- `--i-understand-this-bypasses-auth` flag on `liferay omni-admin
  install`. Required when stdin is not a TTY.
- `--allow-external-bundle` flag on `liferay omni-admin install` for
  the rare case where the resolved bundle path is intentionally outside
  the worktree.
- Named exit codes 1–7 for failure modes agents can branch on (generic,
  not-in-portal, docker-unavailable, port-collision, module-not-found,
  bundle-outside-worktree, confirmation-declined).
- CI now runs `staticcheck`, `go test -race`, and
  `goreleaser release --snapshot --skip=publish --clean` on every PR.

### Changed

- `go.mod` bumped to `go 1.23`.
- Slot allocation is now serialized by a host-wide flock at
  `~/.liferay-cli/slot.lock` and considers slots already claimed by
  every other worktree under `~/.liferay-cli/worktrees/`. Two parallel
  `liferay db start` invocations against fresh worktrees now reliably
  receive distinct slots (audit blocker #1).
- `liferay server start` snapshots every file it is about to patch
  into `<state-dir>/bundle-snapshot/<timestamp>/` before applying the
  patch (audit blocker #2). Use `liferay bundle unpatch` to restore.
- `liferay omni-admin install` refuses to run when the resolved
  bundle path is outside the current worktree (exit 6), and requires
  explicit consent in every invocation (exit 7 when consent is missing
  and stdin is not a TTY). Audit blocker #3.
- `rewriteServerXML` now leaves AJP connectors untouched. Previously
  an uncommented AJP `<Connector>` had its port rewritten to the HTTP
  slot value, guaranteeing a Tomcat startup collision.
- `db logs` and `db ps` are graceful no-ops when the stored engine is
  `hypersonic`, matching `db stop` (and matching what the README
  already claimed).
- Docker subprocess invocations (`liferay db logs -f`, etc.) now
  forward SIGINT/SIGTERM to the docker compose child so Ctrl-C cleans
  up properly instead of orphaning the container process.
- `state.Root()` refuses to fall back to `os.TempDir()` when HOME is
  missing. The CLI exits cleanly at startup with a descriptive
  message; state never lands on a path that gets wiped on reboot.
- `docker-compose.yml` is written via `state.WriteFileAtomic` so
  rendering errors surface and concurrent readers never see a partial
  file.
- README slot-offset table reads `+N × 10` explicitly instead of
  relying on a footnote to clarify `+N`.

### Test coverage

| Package          | Before  | After   |
| ---------------- | ------- | ------- |
| `internal/state` | 0.0%    | 82.3%   |
| `internal/portal`| 46.6%   | 87.6%   |
| `internal/tomcat`| 0.0%    | 65.5%   |
| `internal/docker`| 53.0%   | 55.1%   |
| `internal/cli`   | 5.8%    | 23.3%   |
| **Total**        | **14.6%** | **40.1%** |

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
