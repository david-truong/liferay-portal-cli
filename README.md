# liferay CLI

> **Built for AI agents and humans who think like them.**
>
> AI agents need every workflow runnable from one directory with no prompts and no flag soup. It turns out plenty of human developers want the same thing — driving `gw`, `blade`, the bundled Tomcat, and a per-worktree DB stack from the portal root, with no `cd` and no ceremony. Use it however suits you.

## Invariants

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
| `liferay client-extension <name> [-- <docker run args>]` | Build a workspace client extension, deploy its zip to the bundle, and run its container |
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
| `liferay hosts <add\|remove\|list>` | Manage a friendly `/etc/hosts` name for this worktree (maps to 127.0.0.1) |

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

The root-level Ant projects (`portal-impl`, `portal-kernel`, `util-bridges`, `util-java`, `util-slf4j`, `util-taglib`) are deployed via `ant deploy` from their own directory:

```sh
liferay build portal-impl
```

### `liferay client-extension`

Build, deploy, and run a workspace client extension. Names resolve against every
`workspaces/<workspace>/client-extensions/<name>` directory that contains a
`client-extension.yaml`, the same way `liferay build` resolves modules:

```sh
liferay client-extension liferay-sample-custom-element-1
liferay ce liferay-sample-etc-spring-boot
```

When the same name exists in more than one workspace, qualify it with
`workspace/name`:

```sh
liferay ce liferay-sample-workspace/liferay-sample-custom-element-1
```

The command runs `gw deploy` in the extension directory and copies the produced
`<name>.zip` into the bundle's `osgi/client-extensions/`. For a containerized
(microservice) extension — one with a `Dockerfile` — it then builds the image
from `build/liferay-client-extension-build` and starts the container detached:

```sh
docker build -t <name>:latest .
docker run -d --rm --name <name> -p <port>:<port> \
  --add-host host.docker.internal:host-gateway <name>:latest
```

The port comes from `LCP.json` (`loadBalancer.targetPort`, default `58081`). Any
running container of the same name is replaced, so the command is safe to re-run
after an edit. Frontend extensions (no `Dockerfile`) stop after the zip is deployed.

Extension-specific `docker run` flags pass through verbatim after a `--`
separator (network, environment variables, etc.):

```sh
liferay ce liferay-seostudio-crawler -- \
  --network crawler_elastic \
  -e LIFERAY_SEO_STUDIO_CRAWLER_ELASTICSEARCH_HOST=elasticsearch \
  -e COM_LIFERAY_LXC_DXP_MAINDOMAIN=host.docker.internal:8080
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

By default this also runs `ant all` to populate the bundle. To skip the build:

```sh
liferay worktree add LPD-99999 ../LPD-99999 --skip-build
```

Other worktree commands:

```sh
liferay worktree list
liferay worktree remove ../LPD-99999   # also removes ~/.liferay-cli/worktrees/<id>/ and bundles/
```

### `liferay db`

Per-worktree database container + Adminer in Docker. Tomcat is **not** in the container — it runs natively (see `liferay server`). `db start` allocates a slot, starts the chosen engine, and rewrites the bundle's `portal-ext.properties` so the host-native Tomcat can reach the DB on `localhost:<slot-DB>`.

```sh
liferay db start                       # reuses stored engine (default: mysql)
liferay db start --engine mariadb
liferay db start --engine postgres
liferay db start --engine hypersonic   # no container; strips CLI jdbc overrides
liferay db logs           # tail DB logs
liferay db logs adminer   # tail Adminer
liferay db ps
liferay db stop           # stops containers; data is not persisted
liferay db restart
```

`up`/`down` are accepted as aliases for `start`/`stop`.

**Supported engines**

| Engine     | Image         | JDBC stanza written to portal-ext.properties                 |
|------------|---------------|--------------------------------------------------------------|
| mysql      | `mysql:8.0`   | `com.mysql.cj.jdbc.Driver`, `jdbc:mysql://…/lportal`         |
| mariadb    | `mariadb:11`  | `org.mariadb.jdbc.Driver`, `jdbc:mariadb://…/lportal`        |
| postgres   | `postgres:17` | `org.postgresql.Driver`, `jdbc:postgresql://…/lportal`       |
| hypersonic | none          | no override; Liferay falls back to its built-in HSQL         |

The engine is persisted in `~/.liferay-cli/worktrees/<id>/docker/ports.json`. `db stop`, `db logs`, and `db ps` are no-ops (with a message) when the stored engine is hypersonic.

JDBC drivers for mysql, mariadb, postgres, and hsql already ship in `tomcat-*/webapps/ROOT/WEB-INF/shielded-container-lib/`, so no manual driver install is needed for the supported engines.

Regardless of engine, the managed block also sets `object.encryption.enabled`, `object.encryption.algorithm=AES`, and a fixed `object.encryption.key`, so Object framework `Encrypted` fields work out of the box (the portal ships these blank, which otherwise fails field validation). The key is constant by design — it must stay stable for previously-encrypted data to remain decryptable.

Requires Docker Desktop (macOS/Windows) or Docker Engine (Linux) for the non-hypersonic engines.

### Slots: running multiple Liferay instances side-by-side

Each worktree claims a **slot** the first time its DB stack comes up. Slots are allocated sequentially (0, 1, 2, …) by probing for free ports; the first worktree on a host claims slot 0 and runs with **stock** Liferay configuration (no bundle edits, standard ports). Subsequent worktrees claim slot > 0 and get a coordinated port offset plus a bundle patch that rewires the server-side ports.

Slot is persisted in `~/.liferay-cli/worktrees/<id>/docker/ports.json`. Docker stacks use `-p liferay-slot-<N>` as the compose project name so two worktrees never fight for container names.

**Per-slot ports**

| Service              | Slot 0 (stock) | Slot N offset        |
|----------------------|----------------|----------------------|
| Tomcat HTTP          | 8080           | +N × 10              |
| Tomcat shutdown      | 8005           | +N × 10              |
| Tomcat redirectPort  | 8443           | +N × 10              |
| JPDA debug           | 8000           | +N × 10              |
| OSGi console / Gogo  | 11311          | +N × 10              |
| Elasticsearch HTTP   | 9200           | +N × 101             |
| Elasticsearch xport  | 9300           | +N × 101             |
| Glowroot UI          | 4000           | +N × 10              |
| Arquillian connector | 32763          | +N × 10              |
| DataGuard connector  | 42763          | +N × 10              |
| DB (MySQL/MariaDB/PG)| 3306           | +N × 10              |
| Adminer              | 8081           | +N × 10              |

ES uses `+N × 101` instead of `+N × 10` so HTTP and transport ranges don't collide across slots.

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

Host-native Tomcat lifecycle. Wraps `catalina.sh` with `CATALINA_PID` pointing at `~/.liferay-cli/worktrees/<id>/tomcat.pid` so start/stop/status stay consistent (and survive `ant all`).

`start` and `run` automatically bring up the DB stack for the worktree's stored engine (equivalent to `liferay db start`), apply the slot-specific bundle patches (if slot > 0), and wait for the DB healthcheck before launching Tomcat. For hypersonic, the Docker step is skipped; the patcher still runs for slot > 0.

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

### `liferay hosts`

Gives a worktree a friendly hostname so you can browse it as
`http://lpd-12345:8090` instead of `http://localhost:8090`. Each entry is a line
in `/etc/hosts` mapping the name to `127.0.0.1`, tagged with a
`# liferay-cli <worktree-id>` marker so add/remove stay idempotent and never
touch other entries. The hostname is a label — every worktree still resolves to
loopback, so the per-slot Tomcat port (see [Slots](#slots-running-multiple-liferay-instances-side-by-side))
is what actually distinguishes instances.

```sh
liferay hosts add              # map a name from the worktree dir, e.g. lpd-12345
liferay hosts add demo.test    # map an explicit name
liferay hosts remove           # drop this worktree's entry
liferay hosts list             # show all liferay-cli-managed entries
```

Editing `/etc/hosts` needs root. Run the command directly and it writes the file
if it already has permission (e.g. under `sudo`); otherwise it prints a ready-to-paste,
idempotent `sudo` one-liner instead of writing the file:

```
$ liferay hosts add
Editing /etc/hosts needs root. Run:

  sudo sh -c "sed -i.bak '/# liferay-cli lpd-12345-ab12cd34$/d' /etc/hosts && printf '127.0.0.1\tlpd-12345\t# liferay-cli lpd-12345-ab12cd34\n' >> /etc/hosts"

Then browse http://lpd-12345:8090
```

Two Liferay notes when using a non-`localhost` name:

- Avoid the `.local` suffix on macOS — it's reserved for mDNS/Bonjour and resolves
  slowly. Prefer `.test` (guaranteed non-routable) or a bare label.
- Tell Liferay the host is valid or it redirects back to `localhost`. In the
  bundle's `portal-ext.properties`: `virtual.hosts.valid.hosts=*`.

## Release

Tag a version to publish binaries and update the homebrew tap:

```sh
git tag v1.0.0 && git push origin v1.0.0
```
