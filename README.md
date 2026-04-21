# liferay CLI

> **Built for AI agents, not human developers.**
>
> Human devs already have `gw`, `blade`, and IDE tooling. This CLI exists so an AI agent can drive every common Liferay workflow from a **single working directory** — the portal root — with no `cd`, no interactive prompts, and no arcane flags.

## Agent Invariants

Every command in this CLI upholds three guarantees:

1. **Runnable from the portal root.** Pass module names by identifier; the CLI resolves paths automatically via the module index.
2. **Zero interactive prompts.** If a required argument is missing or a target doesn't exist, the command errors with a clear message and suggestions, then exits non-zero.
3. **One obvious invocation per workflow.** The common case is the default; no flag soup.

## Install

```sh
# macOS / Linux (homebrew)
brew install david-truong/liferay/liferay

# Any platform with Go installed
go install github.com/david-truong/liferay-portal-cli@latest

# Windows: download from GitHub Releases, or use go install above
```

## Commands

| Command | Description |
|---|---|
| `liferay build [module ...]` | Full portal rebuild (`ant all`) or deploy specific modules (`gw deploy -a`) |
| `liferay clean [module ...]` | Full portal clean (`ant clean`) or clean specific modules (`gw clean`) |
| `liferay sf [module ...]` | Format source portal-wide (`ant format-source-current-branch`) or per module |
| `liferay gw <module> [gradle-args...]` | Run any Gradle task in a module by name |
| `liferay build-service <module>` | Run Service Builder (`gw buildService`) — module must have `service.xml` |
| `liferay build-rest <module>` | Run REST Builder (`gw buildREST`) — module must have `rest-config.yaml` |
| `liferay build-lang` | Build portal language files in `portal-language-lang` |
| `liferay poshi --tests <TestFile#TestCase>` | Run Poshi functional tests from `portal-web` |
| `liferay playwright --tests <filter>` | Run Playwright e2e tests from `modules/test/playwright` |
| `liferay test <module> --tests <filter>` | Run unit tests in a module (`gw test --tests`) |
| `liferay test-integration <module> --tests <filter>` | Run integration tests in a module (`gw testIntegration --tests`) |
| `liferay worktree <add\|list\|remove>` | Create and manage git worktrees |
| `liferay server <up\|down\|logs\|ps\|restart>` | Start Tomcat + MySQL in Docker |

---

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

### `liferay clean`

Full portal clean:

```sh
liferay clean           # ant clean from portal root
```

Clean specific modules by name:

```sh
liferay clean change-tracking-web
liferay clean change-tracking-web blogs-web
```

### `liferay sf`

Format source for the entire branch or for specific modules:

```sh
liferay sf                     # ant format-source-current-branch from portal-impl
liferay sf change-tracking-web # gw formatSource in that module
```

### `liferay gw`

Run any Gradle task in a module by name without knowing its path:

```sh
liferay gw change-tracking-web deploy
liferay gw change-tracking-web clean deploy
liferay gw change-tracking/change-tracking-web deploy --info
```

### `liferay build-service`

Run Service Builder for a module that contains `service.xml`. Errors immediately if the module has no `service.xml`:

```sh
liferay build-service change-tracking-service
liferay build-service change-tracking/change-tracking-service
```

### `liferay build-rest`

Run REST Builder for a module that contains `rest-config.yaml`. Errors immediately if the module has no `rest-config.yaml`:

```sh
liferay build-rest headless-delivery-impl
liferay build-rest headless-delivery/headless-delivery-impl
```

### `liferay build-lang`

Build portal language files. Always targets `modules/apps/portal-language/portal-language-lang` — no arguments required:

```sh
liferay build-lang
```

### `liferay poshi`

Run Poshi functional tests. `--tests` accepts `TestCaseFile#TestCaseName` format. Always runs from `portal-web/`:

```sh
liferay poshi --tests Login#viewWelcomePage
liferay poshi --tests Foo#testBar
```

### `liferay playwright`

Run Playwright e2e tests. `--tests` is passed as `--grep` to `npx playwright test`. Always runs from `modules/test/playwright/`. Requires `npx` on PATH:

```sh
liferay playwright --tests "my test description"
liferay playwright --tests myTestName
```

### `liferay test`

Run unit tests in a module. `--tests` is a standard Gradle test filter (class name, wildcard, or `ClassName.methodName`):

```sh
liferay test change-tracking-web --tests "*FooTest"
liferay test change-tracking-web --tests "com.liferay.foo.FooTest.testBar"
```

### `liferay test-integration`

Run integration tests in a module. Requires a running Liferay server for most tests:

```sh
liferay test-integration change-tracking-web --tests "*FooTest"
liferay test-integration change-tracking-web --tests "com.liferay.foo.FooTest"
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

Optionally build the bundle immediately:

```sh
liferay worktree add LPD-99999 ../LPD-99999 --build
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
