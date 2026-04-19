# liferay CLI

Developer CLI for `liferay-portal` / `liferay-portal-ee` workflows.

## Install

```sh
# macOS / Linux (homebrew)
brew install david-truong/liferay/liferay

# Any platform with Go installed
go install github.com/david-truong/liferay-portal-cli@latest

# Windows: download from GitHub Releases, or use go install above
```

## Commands

### `liferay build`

Full portal rebuild:

```sh
liferay build           # ant all from portal root
```

Deploy specific modules by name (resolves the path automatically):

```sh
liferay build change-tracking-web
liferay build change-tracking-web blogs-web
liferay build --no-format change-tracking-web   # skip formatSource
```

Module names match the leaf directory containing `bnd.bnd`. Use `group/name` to disambiguate:

```sh
liferay build change-tracking/change-tracking-web
```

### `liferay worktree`

Create a worktree with user-local files pre-propagated:

```sh
liferay worktree add LPD-99999 ../LPD-99999
```

This runs `git worktree add`, then:
- Symlinks `CLAUDE.md`, `GEMINI.md`, `.claude/`, `.gemini/`, `.idea/`
- Copies `build.*.properties`, `test.*.properties`, `release.*.properties`
- Generates `app.server.<user>.properties` pointing bundles inside the worktree

After adding, build the bundle:

```sh
cd ../LPD-99999 && ant all
```

Other worktree commands:

```sh
liferay worktree list
liferay worktree remove ../LPD-99999   # also removes .liferay-cli/ and bundles/
```

### `liferay server`

Start Tomcat + MySQL in Docker (one stack per worktree, isolated ports and data):

```sh
liferay server up
liferay server logs           # tail Tomcat logs
liferay server logs db        # tail MySQL logs
liferay server ps
liferay server down
liferay server down --wipe    # also deletes MySQL data volume
liferay server restart
```

Requires Docker Desktop (macOS/Windows) or Docker Engine (Linux).

Each worktree gets unique port offsets derived from its path:

| Service | Default | Worktree N |
|---------|---------|-----------|
| Tomcat  | 8080    | 8080+N    |
| Adminer | 8081    | 8081+N    |
| Debug   | 8000    | 8000+N    |
| Gogo    | 13331   | 13331+N   |
| MySQL   | 3306    | 3306+N    |

## Release

Tag a version to publish binaries and update the homebrew tap:

```sh
git tag v1.0.0 && git push origin v1.0.0
```
