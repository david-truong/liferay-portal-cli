# Review Dimensions

The review is split into nine cohesive dimensions so each can be reviewed in a
small, focused context — one rule family at a time, never all 50+ rules at once.
The orchestrator dispatches **one subagent per dimension, all in parallel —
Convention and architecture with `model: opus`, the other eight with
`model: sonnet`** — then aggregates the findings.

Every dimension subagent always reads `style.md` first (the overriding
"follow the existing convention" rule and the five-principle philosophy), then
the files listed for its dimension, then reviews the scope (the diff or files)
against only those rules.

Each subagent returns a JSON array of findings:

```
[{"file": "<path>", "anchor": "<line text or @@ hunk>", "rule": "<citation>", "confidence": "high|partial", "description": "<one line>"}]
```

Use backticks or single quotes inside `description` — never double quotes. Cite
per the SKILL.md "Citing Rules" section. Return `[]` when the dimension finds
nothing. Never report a finding inside a generated file (see SKILL.md "Ignore
Generated Files"). Sort the array by file, then by the anchor's position within
the file.

`confidence` is defined, not felt:

- `high` — the added line matches the cited rule's violation definition as
  written, with nothing left to check; **or** the subagent opened a specific
  precedent file that contradicts the diff, and the `description` names that
  file.
- `partial` — everything else: a suspicion, an unopened precedent, or a
  judgment call.

A rule the sources frame as taste or judgment ([601] consolidation, the
"Taste:" bullets, the Simplify instinct) may be reported `high` only when a
named precedent — a sibling file or method showing the established form —
backs it; otherwise report it `partial` or not at all. When a candidate
finding matches no rule's text and no named precedent, it is not a finding —
do not report "consider"-style suggestions.

Always dispatch all nine dimensions on every review — do not prune by perceived
relevance. Measurement showed the full fan-out is the only configuration that
consistently finds every issue; narrowing it traded away real findings for a
modest cost saving. The one exception is language: dimensions 4 (Liferay
utilities) and 6 (Tests) are Java/JUnit-specific and dimension 9 (Frontend) is
JS/TS-specific. Each of those three still runs on every review, but scopes its
own reading of the diff to the hunks in files of its language and returns `[]`
when the diff has none.

---

## 1. Convention and architecture

- **Focus:** Does it match the file's existing convention and Liferay's
  architectural mandates? This is the cross-cutting dimension — it does the
  precedent grepping the others rely on.
- **Read:** `style.md` (overriding rule + philosophy), `rules/001-*`,
  `rules/002-*`, `mandates.md`, `liferay-conventions.md`.
- **Notes:** [001] violations are invisible from the diff alone — the
  established convention lives in files the PR never touched. For every new name
  the diff introduces, `git grep --cached <pattern>` the checkout for the
  established form before accepting it (REST OpenAPI field casing, `@Before`/
  `@After` method names, acronym casing, constant/helper/key names). Skipping
  this is the single most common source of missed findings. Also flag the
  architectural mandates and the Liferay conventions (imports, banned APIs,
  naming suffixes, API signature hygiene, `WebKeys`, logging form, client
  extension naming). Flag a `ThreadLocal` that is set without being cleared in a
  `finally` (request-scoped state leaking across pooled threads).
- **Type-scoped pattern grep.** When the diff touches a file of a recognizable
  type, grep one telltale string across *every* file of that type to surface the
  established convention and any outliers, then inspect each hit (a hit is a
  lead, not a violation). Brian's own examples:
  `git grep "static class" -- '*Exception.java'` (the nested
  `public static class MustXxx extends ...Exception` convention — verify a new
  exception conforms) and `git grep "super.test" -- '*ResourceTest.java'` (see
  the Tests dimension). Build the equivalent grep for whatever type the diff
  introduces.
- **Filename-pattern survey.** The filename/path counterpart to the content
  grep: `git ls-files '<glob>'` enumerates every file whose *name or path*
  matches a pattern, exposing the naming and location convention a new file
  must follow. Brian's example: `git ls-files '*SpringBootApplication.java'`
  (client-extension Spring Boot apps all live under
  `workspaces/*/client-extensions/.../*SpringBootApplication.java`). Use it to
  check that a new file is named and placed like its siblings — e.g.
  `git ls-files '*ConstraintResolver.java'` shows resolvers belong in
  `.internal.change.tracking.spi.resolver`. Survey *across* modules, not just
  the new file's own folder — the folder itself can be the outlier:
  `git ls-files '*/site-initializer/object-definitions/*.json'` shows a new
  `10-*-object-definition.json` matched its module's own prefixed siblings but
  violated the convention set by ai-hub (see Convention (Site Initializer
  Resources); PR #178185).

## 2. Naming

- **Focus:** Identifier names — variables, fields, methods, booleans, acronyms,
  constants.
- **Read:** `style.md` (Naming), `rules/101-*` through `rules/108-*`.
- **Notes:** A hand-written identifier follows [101] even when the value comes
  from a generated DTO or getter whose own casing differs
  (`crawledPage.getCanonicalUrl()` still yields a local named `canonicalURL`).
  Group sibling constants under a shared prefix, general to specific [108].
  A method that wraps a single HTTP primitive takes its prefix from that
  primitive [105] — a client extension `BaseService` wrapper calling `get(...)`
  is `get*`, not `fetch*`; one calling `patch(...)` is `patch*`, not `update*`
  (see Convention (Client Extension Content)). On finding one naming mismatch
  of a repeated shape, scan every sibling method in the class for the same
  shape and report each as its own finding — Brian flags one instance and
  expects the principle applied to the rest ("I'm not a compiler",
  PR #178437).

## 3. Ordering and declaration

- **Focus:** Sort order of every sortable sequence, and where each local is
  declared.
- **Read:** `style.md` (Ordering and declaration), `rules/201-*`,
  `rules/202-*`, `rules/203-*`, `rules/204-*`.
- **Notes:** Check the parameter order of every new or changed method signature,
  test helpers included (alphabetical, vararg last) [202]. Walk each new or
  changed method body explicitly for [203]: each local declared just before its
  first use, and the wrapping object — the value the method builds and returns,
  or a local that absorbs another through a setter — declared **first**, before
  the helpers that fill it (the "burrito"). This is structural and easy to skip.

## 4. Liferay utilities

- **Focus:** Hand-written code where a Liferay utility already exists.
- **Read:** `style.md` (Prefer Liferay utilities), `rules/301-*` through
  `rules/306-*`.
- **Notes:** When you catch a null/empty check, a map into a new list, or string
  concatenation, reach for `GetterUtil`, `ListUtil`/`SetUtil`/`MapUtil`/
  `ArrayUtil.isEmpty`, `ArrayUtil.contains`, `TransformUtil.transform`,
  `JSONUtil`, or `StringBundler`.
- **Scope:** `.java` files only. Skip any `.ts`/`.tsx`/`.js`/`.jsx` hunk in the
  diff — these utilities have no JS/TS equivalent.

## 5. Control flow, form, and visibility

- **Focus:** Control-flow form, over-engineering, member visibility, and shell
  conventions.
- **Read:** `style.md` (Control flow and form; Simplify), `rules/401-*` through
  `rules/406-*`, `rules/501-*`, `rules/502-*`, `rules/801-*`, `rules/901-*`
  through `rules/908-*`.
- **Notes:** No `switch`/`case`, non-builder chaining, or explicit iterators
  [401-403]. For every `private static final` constant the diff adds, count its
  references in the file — exactly one reference means inline it [502]
  ("rebuilds the collection on every call" is not an exemption). A method used
  once must be private [801]. Loose checks, trailing slashes, and shell
  conventions [901-908]; never flag a *missing* trailing newline [908].

## 6. Tests

- **Focus:** Test placement, naming, consolidation, randomization, and resource
  test overrides.
- **Read:** `style.md` (Tests), `rules/601-*` through `rules/605-*`.
- **Notes:** Test placement is on both axes [604] — the test sits in the `-test`
  module that pairs with its subject class's module (a `layout` class →
  `layout-test`) **and** mirrors the subject's package; follow an existing
  sibling test's precedent. Consolidate parallel test methods [601], randomize
  unasserted values [602], name a test after the method it tests [603], override
  every generated `Base*ResourceTestCase` method [605]. No static-prose message
  arguments on `Assert.*` calls — the test name carries intent.
- **Grep recipe for [605].** Run `git grep "super.test" -- '*ResourceTest.java'`
  to find generated resource-test methods overridden only to call
  `super.testXxx()`. A bare super-delegation is a lead: confirm the override
  exists for a real reason (the test needs special data, or is deliberately
  disabled), not because the author punted on implementing it.
- **[606] Implementation-family test naming.** When the diff adds or renames a
  test for one member of a family of sibling implementations of a shared
  interface (`ObjectActionExecutor`, `MVCActionCommand`, `UpgradeStep`, and
  similar), confirm the test class is named after its implementation class
  (dropping only `Impl`), not a shortened behavioral name — `git ls-files
  '*<Interface>.java'` enumerates the family, `git ls-files
  '*<Interface>Test.java'` shows the naming precedent. This is easy to miss
  when a diff also extracts or renames a shared base test case for the family,
  since the base-class refactor reads as the interesting change and the sibling
  renames read as incidental. See PR #178354 (rejected — `ComputeNextScanDateSEOStudioTest`
  did not say which `ObjectActionExecutor` it tested) vs. #178436 (fixed).
- **Scope:** `.java` files only. Skip any `.ts`/`.tsx`/`.js`/`.jsx` hunk — Jest
  tests are governed by `.claude/rules/jest-testing.md`, not this dimension.

## 7. Prose and wordsmithing

- **Focus:** Prose in labels, log/exception messages, comments, Markdown, and
  `Language.properties`.
- **Read:** `style.md` (Prose), `rules/701-*` through `rules/708-*`.
- **Notes:** No hyphens [701], Title Case or complete sentence for labels [702],
  log/exception message form [703] (no trailing period, "Unable to ..."), spell
  out contractions [704], include articles [705], capitalize ID [706], complete
  sentences [707], one-line Markdown paragraphs [708]. Read every added key in
  the source `Language.properties` line-by-line; a lowercase value that "reads
  naturally" is not an exemption — the sibling convention decides.

## 8. Format rules

- **Focus:** Line-level mechanical and small-refactor conventions.
- **Read:** `format-rules.md`.
- **Notes:** These overlap SourceFormatter. When a finding is a mechanical
  formatting or layout suggestion that SourceFormatter governs (collapsing or
  expanding a boolean return or ternary, method chaining, line wrapping,
  whitespace, blank lines, import order, member/parameter/declaration ordering),
  append "SourceFormatter is authoritative: if applying this suggestion fails
  SourceFormatter, ignore it." to that finding's description.

## 9. Frontend (TypeScript/React)

- **Focus:** JS/TS/JSX/TSX-specific conventions — `Liferay.Language.get`
  literal keys, the `fetch` import source, AI-prose comments, and the
  canonical rules that generalize to this language.
- **Scope:** `.ts`, `.tsx`, `.js`, `.jsx` files only. Return `[]` if the diff
  touches none of these.
- **Read:** `style.md` (overriding rule + philosophy),
  `liferay-conventions.md` ("Frontend (TypeScript / React)"), `rules/105-*`
  through `rules/108-*`, `rules/201-*` through `rules/203-*`, `rules/404-*`,
  `rules/502-*`, `rules/701-*` through `rules/707-*`, `rules/901-*`,
  `rules/902-*`, `rules/905-*`.
- **Notes:** Do not cite [301]-[306], [401]-[403], [601]-[606], or [908]
  against a JS/TS file — `liferay-conventions.md`'s Frontend section explains
  why each is out of scope (Java-only utility classes, idiomatic
  chaining/`switch`/iteration, JUnit-specific test conventions, and an
  opposite trailing-newline convention enforced by Prettier). [101] acronym
  casing and [801] method-used-once-private apply only in the adapted form
  documented there — file-local precedent for casing, "should not be
  exported" for [801], dropping the Java underscore-prefix detail. [501]
  adapts to Jest's `expect(...).toBeDefined()`.
