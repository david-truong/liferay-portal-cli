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
go install github.com/david-truong/liferay-portal-cli/cmd/liferay@latest

# Windows: download from GitHub Releases, or use go install above
```

### Local development

From a clone of this repo:

```sh
go install ./cmd/liferay
```

This drops a `liferay` binary into `$(go env GOPATH)/bin`. Make sure that directory is on your `PATH` (or symlink the binary into one that is). All other forms (`go build`, `go build ./...`) only compile-check — only `go install ./cmd/liferay` updates the binary on `PATH`.

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
| `liferay db <up\|down\|logs\|ps\|restart> [--engine mysql\|mariadb\|postgres\|hypersonic]` | Manage the per-worktree database stack |
| `liferay server <start\|stop\|restart\|run\|status\|logs\|wipe>` | Manage the host-native Tomcat bundle |
| `liferay omni-admin <install\|uninstall>` | Install/remove dev-only omni-admin bundles (auto-login, no-captcha, forgiving store) |

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

Run integration tests in a module against the host-native Liferay server started by `liferay server start`. On slot 0 (stock ports) this is a pass-through to `gw testIntegration --tests <filter>`. On slot > 0 the CLI writes a Gradle init script that pins `testIntegrationTomcat.portNumber` to the bundle's HTTP port and forwards `-Dliferay.arquillian.port` / `-Dliferay.data.guard.port` to the test JVM so it matches the server-side OSGi-configured ports.

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

### `liferay db`

Per-worktree database container + Adminer in Docker. Tomcat is **not** in the container — it runs natively (see `liferay server`). `db up` allocates a slot, starts the chosen engine, and rewrites the bundle's `portal-ext.properties` so the host-native Tomcat can reach the DB on `localhost:<slot-DB>`.

```sh
liferay db up                       # reuses stored engine (default: mysql)
liferay db up --engine mariadb
liferay db up --engine postgres
liferay db up --engine hypersonic   # no container; strips CLI jdbc overrides
liferay db logs           # tail DB logs
liferay db logs adminer   # tail Adminer
liferay db ps
liferay db down
liferay db down --wipe    # also deletes the DB data volume
liferay db restart
```

**Supported engines**

| Engine     | Image         | JDBC stanza written to portal-ext.properties                 |
|------------|---------------|--------------------------------------------------------------|
| mysql      | `mysql:8.0`   | `com.mysql.cj.jdbc.Driver`, `jdbc:mysql://…/lportal`         |
| mariadb    | `mariadb:11`  | `org.mariadb.jdbc.Driver`, `jdbc:mariadb://…/lportal`        |
| postgres   | `postgres:17` | `org.postgresql.Driver`, `jdbc:postgresql://…/lportal`       |
| hypersonic | none          | no override; Liferay falls back to its built-in HSQL         |

The engine is persisted in `.liferay-cli/docker/ports.json`. `db down`, `db logs`, and `db ps` are no-ops (with a message) when the stored engine is hypersonic.

JDBC drivers for mysql, mariadb, postgres, and hsql already ship in `tomcat-*/webapps/ROOT/WEB-INF/shielded-container-lib/`, so no manual driver install is needed for the supported engines.

Requires Docker Desktop (macOS/Windows) or Docker Engine (Linux) for the non-hypersonic engines.

### Slots: running multiple Liferay instances side-by-side

Each worktree claims a **slot** the first time its DB stack comes up. Slots are allocated sequentially (0, 1, 2, …) by probing for free ports; the first worktree on a host claims slot 0 and runs with **stock** Liferay configuration (no bundle edits, standard ports). Subsequent worktrees claim slot > 0 and get a coordinated port offset plus a bundle patch that rewires the server-side ports.

Slot is persisted in `.liferay-cli/docker/ports.json`. Docker stacks use `-p liferay-slot-<N>` as the compose project name so two worktrees never fight for container names.

**Per-slot ports**

| Service              | Slot 0 (stock) | Slot N offset        |
|----------------------|----------------|----------------------|
| Tomcat HTTP          | 8080           | +N                   |
| Tomcat shutdown      | 8005           | +N                   |
| Tomcat redirectPort  | 8443           | +N                   |
| JPDA debug           | 8000           | +N                   |
| OSGi console / Gogo  | 11311          | +N                   |
| Elasticsearch HTTP   | 9200           | +N × 101             |
| Elasticsearch xport  | 9300           | +N × 101             |
| Glowroot UI          | 4000           | +N                   |
| Arquillian connector | 32763          | +N                   |
| DataGuard connector  | 42763          | +N                   |
| DB (MySQL/MariaDB/PG)| 3306           | +N                   |
| Adminer              | 8081           | +N                   |

Offset is `+N * 10` for everything except ES HTTP and transport, which use `+N * 101` to keep HTTP and transport ranges from colliding across slots.

**What the patcher touches when slot > 0**

`liferay server start` applies idempotent edits to the bundle before launching Tomcat:

- `tomcat-*/conf/server.xml` — `<Server port>`, HTTP `<Connector port>`, and `redirectPort` attributes
- `tomcat-*/bin/setenv.sh` — `export JPDA_ADDRESS=<port>`
- `portal-developer.properties` — `module.framework.properties.osgi.console`
- `tomcat-*/webapps/ROOT/WEB-INF/classes/portal-developer.properties` — same key (re-applied after rebuild wipes it)
- `glowroot/admin.json` — `web.port`
- `osgi/configs/com.liferay.portal.search.elasticsearch8.configuration.ElasticsearchConfiguration.config`
- `osgi/configs/com.liferay.arquillian.extension.junit.bridge.connector.ArquillianConnector.config`
- `osgi/configs/com.liferay.data.guard.connector.DataGuardConnector.config`

Slot > 0 also gets `liferay.home=<bundleDir>`, `portal.instance.http.socket.address=localhost:<tomcat>`, and `module.framework.properties.osgi.console=localhost:<osgi>` injected into `portal-ext.properties` alongside the JDBC stanza. Slot 0's `portal-ext.properties` gets only the JDBC stanza — nothing else changes.

### `liferay server`

Host-native Tomcat lifecycle. Wraps `catalina.sh` with `CATALINA_PID` pointing at `<bundle>/.liferay-cli/tomcat.pid` so start/stop/status stay consistent.

`start` and `run` automatically bring up the DB stack for the worktree's stored engine (equivalent to `liferay db up`), apply the slot-specific bundle patches (if slot > 0), and wait for the DB healthcheck before launching Tomcat. For hypersonic, the Docker step is skipped; the patcher still runs for slot > 0.

```sh
liferay server start             # background (catalina.sh start)
liferay server start --debug     # background with JDWP on (catalina.sh jpda start)
liferay server run               # foreground (catalina.sh run)
liferay server run --debug       # foreground with JDWP on
liferay server stop              # catalina.sh stop -force
liferay server restart [--debug]
liferay server status            # alive / stale-pid / not-running
liferay server logs              # tail tomcat-*/logs/catalina.out
liferay server wipe              # stop and delete data/, logs/, osgi/state/, work/, portal-setup-wizard.properties
```

Debug mode is opt-in so the JPDA port is only bound when you actually need it. `JPDA_ADDRESS` comes from `setenv.sh` — stock slot 0 uses catalina's default 8000; slot > 0 gets the per-slot port the bundle patcher wrote into `setenv.sh`.

Integration tests rely on this host-native Tomcat — the Arquillian junit-bridge and DataGuard connectors both use loopback sockets, which cannot cross the Docker network boundary.

### `liferay omni-admin`

Installs three dev-only OSGi bundles into the active bundle's `osgi/modules/`:

- `omni.admin.autologin` — AutoLogin filter that authenticates requests as an administrator
- `omni.admin.captcha` — no-op `CaptchaProvider` that disables CAPTCHA portal-wide
- `omni.admin.store` — `DLStoreWrapper`/`PDFProcessorWrapper` that returns empty files for missing documents

```sh
liferay omni-admin install     # copy all three jars into osgi/modules
liferay omni-admin uninstall   # remove them
```

These bundles bypass authentication and validation. Never install on a shared or production bundle.

## Release

Tag a version to publish binaries and update the homebrew tap:

```sh
git tag v1.0.0 && git push origin v1.0.0
```
