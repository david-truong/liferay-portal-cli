# Contributing

Thanks for taking the time to contribute to `liferay-portal-cli`.

## Development setup

```sh
git clone https://github.com/david-truong/liferay-portal-cli
cd liferay-portal-cli
go build ./...
go test ./...
```

Go 1.23 or newer is required (see `go.mod`).

## Local install

```sh
go install ./cmd/liferay
```

This puts `liferay` on `$PATH` from `$GOBIN` (typically `~/go/bin`).

## Tests

```sh
go test ./...           # unit tests
go test -race ./...     # race detector — required to pass before merging
```

If you touch `internal/tomcat`, run the golden-file tests:

```sh
go test ./internal/tomcat/...
```

When a golden test fails because of an intentional change, update the
fixtures under `internal/tomcat/testdata/` and confirm the diff is what you
expect.

## Building a release locally

```sh
goreleaser release --snapshot --skip=publish --clean
```

The same command runs in CI on every PR.

## Pull requests

- One topic per PR.
- Run `go vet ./...`, `staticcheck ./...`, and `go test -race ./...` locally
  before pushing.
- Include or update tests for behavior changes. Test coverage targets per
  package live in the production-readiness spec.
- Update `CHANGELOG.md` under `## Unreleased`.
- The CLI's public surface (commands, flags, exit codes, `--json` schemas)
  is treated as stable. Breaking changes require a major-version bump and
  a note in `CHANGELOG.md`.

## Filing issues

Bug reports should include:

- `liferay --version` output
- OS and architecture
- The exact command that triggered the problem
- Relevant excerpts of `~/.liferay-cli/worktrees/<id>/` state if applicable

For security-sensitive reports, see `SECURITY.md`.
