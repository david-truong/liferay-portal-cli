# Liferay Coding Conventions

Conventions from Liferay's coding guidelines that the canonical rules, mandates, and format rules do not already cover. Cite as "Convention (<section>)", e.g. "Convention (Java Style: no static imports)". On conflict, the canonical rules win.

## Java Style

- No wildcard imports and no static imports.
- No `org.jsoup` — use `HtmlUtil`, `StringUtil`, or regex instead.
- Use `StringUtil.toLowerCase()`, `StringUtil.replace()`, and the other `StringUtil` methods over the corresponding `String` instance methods.
- Use `Objects.equals()`, not `Validator.equals()`. Example: commit `93f2bb3c0961d` replaced `Validator.equals(getInlineLabel(), "right")` with `Objects.equals(getInlineLabel(), "right")`.
- Build maps with `HashMapBuilder.put().put().build()`, not manual `new HashMap` construction followed by `put` calls. Exception: Format Rule 53 — for two or three static entries, `HashMapBuilder` is overkill. Example: commit `f1200cf0952d6` replaced `Map<String, String> properties = new HashMap<>(); properties.put("version", _licenseVersion);` with `HashMapBuilder.put("version", _licenseVersion).build()`; `511b16b59f7cb` made the same change for an `additionalProps` map.
- Use `Collections.addAll(collection, array)`, not `collection.addAll(Arrays.asList(array))`.
- Use enhanced for-loops over index-based loops (extends [403], which covers explicit iterators).
- Parenthesize relational sub-expressions: `if ((a < 0) || (b < 0))`, not `if (a < 0 || b < 0)`.
- Avoid Java enums — use String constants or polymorphism.

## Method Calls and Chaining

- Mockito joins the [402] chaining allow list (alongside builders, Stream, Optional).
- No chaining on `computeIfAbsent()` — assign the result to a variable first.

## Naming

- Maps take a `Map` suffix: `Map<String, Layout> layoutsMap`.
- A method returning an `Optional` must end with `Optional`.
- A method returning a functional interface ends with that interface name (a method returning a `Function` ends with `Function`).
- When a name would collide with a Java reserved word, adjust it: `Class` → `clazz`, `Package` → `pkg`.
- Multiple variables of the same type: suffix with 1-based numbers (`layout1`, `layout2`) or prefix each with a distinguishing word. Where there's a 2, there's a 1 — never a bare `layout` beside a `layout2`.
- `expected`/`actual` prefixes are allowed for test assertion variables.
- In a REST resource impl, when the DTO shares its simple name with the Service Builder model it wraps (e.g., `com.liferay.change.tracking.rest.dto.v1_0.CTCollection` vs. `com.liferay.change.tracking.model.CTCollection`), the DTO keeps the plain name (`ctCollection`) and every local or parameter holding the model instance is prefixed `serviceBuilder<Name>` (`serviceBuilderCTCollection`) — never a bare `ctCollection` or an ad hoc `ctCollectionModel`. Applies consistently across the whole class, including methods the current change does not touch. Refines [104]. Reference: [PR #178762 review](https://github.com/brianchandotcom/liferay-portal/pull/178762#issuecomment-5009382067) — "change those all to serviceBuilderCTCollection ... There are a few places where this is inconsistent. Find and fix them all."

## Variable Declarations

- Declare a loop variable inside the loop (not hoisted) when its initializer is inexpensive — a map or service lookup — so initialization is skipped when the loop body never runs (consistent with [404]).

## Ordering

- When a sorted chunk mixes constants and string literals, list all constants first (alphabetical), then all strings (alphabetical) (refines [202]).
- `StringBundler`/`StringBuilder` appends are exempt from alphabetical ordering — they follow output order.

## API Method Signatures

- New extension points go in `-spi` modules (`@ConsumerType`), not `-api`; API modules hold interfaces only, no new concrete classes.
- Keep `HttpServletRequest`/`HttpServletResponse` out of API signatures (exception: rendering methods).
- Keep `ThemeDisplay` out of API signatures.
- Keep `ServiceContext` out of API signatures (exception: service methods); never pass a `ServiceContext` from one service to another.
- Pass the specific parameters needed, or a bean with cohesive fields, instead of these grab-bag objects.

## Util Classes

- A `FooUtil` may share a name with a `Foo` interface only as a singleton-delegation Util (legacy kernel only); unnecessary in OSGi modules, where the interface is injected via `@Reference`.
- Add a new utility method as a `default` method on the relevant interface rather than introducing a new Util class.

## Request Attributes

- Declare request attribute names as constants in a `WebKeys` class — the standard `WebKeys` in portal-kernel, or a module-specific `*WebKeys`.
- For display contexts, prefer `WebKeys.PORTLET_DISPLAY_CONTEXT`; declare a portlet-specific constant only when namespace isolation can't prevent clobbering.
- For a Configuration object, the request attribute name is the configuration class name itself.

## Logging

- Phrase failures as "Unable to ...", never "Error ..." (see also Format Rule 38).
- Use human-readable names, not Java class/interface names — decompose camel case when in doubt.
- No colon before an id/PK/name specifier, even when the message ends with it.
- Write null values as uppercase `NULL`.

## Test Code

- Keep setup close to its usage; prefer explicit setup in the test method over a complex `@Before`/`@After` when it reads more clearly.
- Use an `original*` variable only when you already have a base variable you temporarily change and reset.
- Add helper methods only when they make intent clearer, not just to save lines.

## OSGi Components

- In `@Component` classes, use `@Reference` for service injection — never `*ServiceUtil` static lookups; those exist only for legacy compatibility.

## Client Extension Naming

From `workspaces/liferay-sample-workspace/README.md` — the directory name of a client extension is built from `-`-separated parts:

- Part 1 is the owner and part 2 is the project (`liferay` and `sample` in `liferay-sample-batch`). Neither may contain a `-`, since `-` separates the parts.
- Part 3 must be a client extension type: `batch`, `custom-element`, `fds-cell-renderer`, `global-css`, `global-js`, `iframe`, `notification-type`, `oahs`, `oaua`, `object-action`, `site-initializer`, `static-content`, `theme-css`, `theme-favicon`, `theme-spritemap`, or `workflow-action` — or the special keyword `etc` when no type applies (`liferay-sample-etc-cron`).
- Part 4 (optional) is a free-form description (`1`, `2`, `cron`, `spring-boot`).

A violation is a client extension directory whose third part is neither a valid type nor `etc`, or whose owner or project part contains a `-`. The sample workspace (`workspaces/liferay-sample-workspace/client-extensions`) is the primary source of truth for client extension structure — a new client extension should mirror the matching sample.

## Client Extension Content

From `workspaces/liferay-sample-workspace/.workspace-rules/guided-client-extension.md`:

- No `build.gradle` in a client extension — the workspace plugin auto-detects everything under `client-extensions`. A violation is a `build.gradle` added to a client extension directory.
- A custom element that submits to Liferay APIs must use `Liferay.Util.fetch` when available, so the session cookies and CSRF token (`p_auth`) are attached — plain `fetch` against Liferay endpoints draws a 403 even for logged-in users. Falling back to native `fetch` outside Liferay is fine.
- In `*.batch-engine-data.json` files defining Objects:
	- `indexedLanguageId` is only valid for `String` and `Clob` DBTypes — never on `Date`, `DateTime`, or other nontext fields.
	- Every `Date`/`DateTime` field requires a `timeStorage` entry in its `objectFieldSettings` (`convertToUTC` or `useInputAsFormatted`); without it the batch import fails.
	- `permissions` are not supported in batch imports — they are set in the UI after deployment, so a `permissions` block in the file is a violation.
- A batch client extension that defines Objects needs both `Liferay.Headless.Batch.Engine.everything` and `Liferay.Object.Admin.REST.everything` OAuth scopes in its `client-extension.yaml`.

From the [PR #178437 review](https://github.com/brianchandotcom/liferay-portal/pull/178437) ("fetch vs. get?") of a Spring Boot client extension service extending `BaseService` (`modules/util/client-extension-util-spring-boot*`):

- A wrapper method's name starts with the `BaseService` HTTP primitive its body delegates to (`delete`, `get`, `patch`, `post`, `put`): a wrapper calling `get(...)` is `get*`, not `fetch*`; one calling `patch(...)` is `patch*`, not `update*`. This applies [105] strictly — the name's wording comes from the call the body delegates to, so a violation is any prefix/primitive mismatch, even one that reads naturally on its own.

## Frontend (TypeScript / React)

From `CLAUDE.local.md`'s Frontend section, plus the canonical rules that generalize to JS/TS. The Liferay-utility rules [301]-[306] (`GetterUtil`, `ListUtil`/`ArrayUtil`, `TransformUtil`, `JSONUtil`, `StringBundler`), the control-flow rules [401]-[403] (`switch`, chaining, iterators — `switch` and method chaining are idiomatic in JS/TS), and the Tests dimension [601]-[606] (JUnit/REST Builder specific; Jest tests are governed by `.claude/rules/jest-testing.md` instead) do not apply here — do not cite them against a `.ts`/`.tsx`/`.js`/`.jsx` file.

- `Liferay.Language.get('some-key')` must take a plain string literal. A variable, ternary, or computed key is silently stripped at build time by the language-get esbuild plugin and renders nothing at runtime; branch to separate literal calls instead.
- Use `sub(Liferay.Language.get('x-of-y'), a, b)` for interpolation — never build the string by concatenation.
- Import `fetch` from `frontend-js-web`, not the global, so the portal auth/CSRF headers are applied (distinct from the custom-element `Liferay.Util.fetch` convention under Client Extension Content, above).
- No block/JSDoc comments narrating a component, hook, or service function — the same "No AI prose" principle as [707], and the most common tell in generated `.ts`/`.tsx`.
- Match the surrounding module's existing patterns (service file shape, `openToast` for feedback, `Clay*` components, FDS cell renderers) rather than introduce a second way to do something the app already does one way — the frontend form of [001].
- [101] acronym casing is inconsistent across the existing frontend codebase (`resourceURL` and `resourceUrl` both occur) — flag it only when the *same file* already establishes one casing and the diff breaks it, never as a codebase-wide default.
- [105]-[107] (name a function/value for what it does or where it comes from) and [108] (group sibling constants under a shared prefix) apply as written.
- [201]-[203] (order grouped assignments, sort sortable sequences, declare as used) apply as written, minus the Java-tooling citations (`JavaTermComparator`, `JavaConstructorParametersCheck`) — sort alphabetically by the same principle.
- [404] (hoist only real work out of a loop, not a cheap property or map access) applies as written.
- [502] (inline a constant referenced exactly once) applies as written.
- [501] adapts to Jest: drop an `expect(x).toBeDefined()` immediately before a line that already dereferences or asserts more on `x`.
- [801] adapts: a helper used only within its own module should not be `export`ed. Drop the Java underscore-prefix detail.
- [701]-[707] (prose: no hyphens, Title Case or complete sentence for labels, log/toast message form, contractions, articles, "ID", complete sentences) apply to comments, `Liferay.Language.get` keys, and thrown/logged messages exactly as written; [708] (one-line Markdown paragraphs) is out of scope for source files.
- [901]-[902] (rigid runtime checks, no trailing slash) apply as written.
- [905] (capitalize and blank-line an inline comment) applies as written; [906] (space inline JSON) stays out of scope for a native JS object literal, per its own text — it only reaches an actual quoted-key JSON string embedded in the file.
- [908] does not apply — the opposite convention holds. Prettier (`modules/.prettierrc.js`) adds a trailing newline to every file it formats; do not flag a `.ts`/`.tsx`/`.js`/`.jsx` file for having one, and do not flag one for lacking it either — leave end-of-file whitespace to Prettier entirely.

## Site Initializer Resources

From the [PR #178185 review](https://github.com/brianchandotcom/liferay-portal/pull/178185#issuecomment-4899593664) ("Better sorted right? … Do the same for the other files in this PR") and its resubmission [PR #178432](https://github.com/brianchandotcom/liferay-portal/pull/178432#issuecomment-4930064856) — for the JSON files under `<module>-site-initializer/src/main/resources/site-initializer/{object-definitions,object-relationships,list-type-definitions}`. The model precedent is `modules/dxp/apps/ai-hub/ai-hub-site-initializer`; older initializers (customer-portal, partner-portal, testray, dsr) predate the convention and are not precedent for new modules.

- Filenames are `<subject>-object-definition.json`, `<subject>-object-relationship.json`, and `<subject>-list-type-definition.json` — never a numeric prefix. A violation is any numeric-prefixed filename (`10-seo-studio-integration-object-definition.json`), even when the module's own folder already carries prefixed siblings — the module-local prefixes are the outlier Brian rejected, not the convention.
- When one subject has multiple relationship files, suffix them `-1`, `-2`, … assigned in alphabetical order of each file's own `"name"` field (ai-hub: `agent-definition-object-relationship-{1..4}.json` hold `accountToAgentDefinitions` < `agentDefinitionsToChatbots` < `agentDefinitionsToContentRetrievers` < `aiHubAgentDefinitionsToAIHubGuardrails`).
- Top-level JSON keys are alphabetically ordered (`deletionType`, `externalReferenceCode`, `label`, `name`, `objectDefinitionId1`, …) — the file-level form of Mandate 1.
- Never force object creation order with a filename prefix. When one object definition depends on another, express the dependency as a separate `object-relationships/` file with its own `externalReferenceCode` — not as inline relationship fields on the object definition plus a numbered filename.
