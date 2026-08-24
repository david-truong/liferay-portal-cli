# Brian Chan's Architectural Mandates (50,000 PR Analysis)

These mandates are derived from an analysis of 50,000 PRs in the `brianchandotcom/liferay-portal` repository. They represent the absolute standard for code acceptance.

## 1. Zero-Entropy Sorting

All code elements—properties, methods, array items, SQL `INSERT` statements in tests, and `service.xml` finders—must be sorted alphabetically.
- **Why:** Prevents "random" formatting changes and ensures predictable diffs.
- **Example:** In `service.xml`, if two finders are added, they must be in alphabetical order.

## 2. Semantic Pluralization

Variable and class names must reflect the cardinality of the data.
- **Rule:** Use `applesPage` for a collection of apples, not `applePage`.
- **Rule:** Use generic `page` when the context is clear, but prefixed names must be pluralized.

## 3. Interface-First Signatures

Method parameters and return types should always use the most generic interface possible.
- **Rule:** Use `Map<String, String>` instead of `HashMap`.
- **Rule:** Use `List<T>` instead of `ArrayList`.

## 4. Exception Hygiene
- **Rule:** Exception messages MUST NOT end with a period.
- **Rule:** Use translation keys (e.g., `invalid-site-navigation-menu-item-order`) instead of hardcoded strings for user-facing errors.
- **Rule:** Exception class names should include the entity (e.g., `PageAttachmentException` vs `NoSuchNode`).

## 5. Transactional & Concurrency Safety

Any code using `tx` (transactional) callbacks or modifying the core service layer requires core approval and rigorous verification.
- **Check:** Are you unsetting `ThreadLocal` variables in a `finally` block?
- **Check:** Are you handling exceptions properly within transactions? (No empty catch blocks).

## 6. Implicit Descriptions & Labels

Do not use words like "Enables" or "Supports" in feature flag descriptions.
- **Correct:** `Workflow for object entry folders`
- **Incorrect:** `Enables workflow support for object entry folders`
- **Rule:** Labels and Badge fields should be Title Cased (e.g., `On Create Company`).

## 7. ThreadLocal Stewardship

Every value set in a `ThreadLocal` (like `PermissionThreadLocal`) MUST be explicitly unset in a `finally` block to prevent security cross-contamination.

## 8. Resource Test Rigor

Every method in a REST resource test class MUST include an `@Override` annotation.
- **Reason:** Confirms correct implementation of the base resource test interface.
- **Rule:** Randomize all test inputs (URLs, IDs, Issuers). Do not use hardcoded "test" strings.

## 9. Naming & Domain Precision
- **Add vs. Append:** Use `_add*` instead of `_append*`.
- **Noun vs. Verb:** `cleanup` is a noun; `cleanUp` is a verb.
- **Backend Accuracy:** Use the actual backend class name (e.g., `Prototype`) instead of generic words like `Template` in method names.
- **Standardization:** Use `restClient` instead of `headlessHttpClient`. Use `source.map` instead of `sourcemap`.

## 10. Collision Prevention

Always prefix custom object definitions with `L_` (e.g., `L_AGENT_DEFINITION`). This isolates them from future Liferay core updates.

## 11. Clarity Over Brevity

Avoid "random" or overly short variable names.
- **Correct:** `clusterStatsRequest`
- **Incorrect:** `request` or `req`
- **Rule:** Variables should appear in the code in the same order they are used.

## 12. Modernizing Legacy Patterns
- **Standardization:** Move from "Journal" to "WebContent" and "Layout" to "Page" when creating new modules, but maintain consistency within existing ones.
- **Avoid Chaining:** Don't chain long method calls in `put` operations. Break them into readable steps.
- **Bnd/Gradle:** Sort `Import-Package` and dependencies. Use single spaces (e.g., `implementation group (`).

## 13. Git Hygiene
- **Rule:** Never "revert a revert." If a change needs to be brought back, squash the history to maintain a clean timeline.

## 14. No AI-Style Descriptive Text

Generated code must look like the rest of the code base. Strip the explanatory prose that LLMs add by default.

### Assert message argument

JUnit's `Assert.*` methods accept an optional leading message argument. Liferay uses it sparingly, and only with a specific shape.

**Default — no message:**

```java
Assert.assertTrue(condition);
Assert.assertFalse(condition);
Assert.assertEquals(expected, actual);
Assert.assertEquals(2, ctCollections.size());
```

The test method name already states intent (`testContainsRejectsCTRemoteFromOtherCompany`). A static prose message is restating the method name and adds nothing on failure.

**Allowed — diagnostic *value* as the message** (not a sentence):

```java
Assert.assertTrue(groups.toString(), groups.contains(group));
Assert.assertEquals(pageItems.toString(), 2, pageItems.size());
Assert.assertEquals(logEntries.toString(), 1, logEntries.size());
Assert.assertTrue(rootPKsMap.toString(), rootPKsMap.isEmpty());
```

The message is a runtime value (typically `someCollection.toString()`) that prints the offending state when the assert fails. Pass it when "expected 1 but was 0" wouldn't be enough to debug the failure on CI — usually for collections, maps, or paged results.

**Banned — static prose describing intent:**

```java
// NO
Assert.assertTrue(
    "contains() must return true for a CTRemote owned by the same " +
        "company when the user has owner permission",
    permission.contains(checker, ctRemote, ActionKeys.VIEW));

// NO
Assert.assertEquals(
    "The list should have exactly one element after insert",
    1, list.size());
```

These restate the test name and don't help on failure. Drop the message entirely, or — if the failure really would be ambiguous — replace it with a `.toString()` of the offending object.

- **Reference:** commit `24b741acb3aeb` ("not needed, let the code speak") removed a static-prose message from an `Assert.assertEquals` call and another from a chained `Assert.assertTrue`/`assertFalse` pair in `OfflineOpenIdConnectSessionManagerTest`, leaving only the value/condition arguments.

### Comments and javadoc

- **Rule:** Remove descriptive comments unless the *why* is non-obvious. Don't restate what the code does.
- **Rule:** Don't add javadoc / inline narration to setup blocks, helper methods, or obvious branches. Liferay test methods do not get javadoc; helper methods (`_createCTRemote`) do not get javadoc.
- **Reference:** [PR #174360 review](https://github.com/brianchandotcom/liferay-portal/pull/174360#issuecomment-4417162019) — "remove the descriptions in the text unless they are not obvious. You have to modify AI code to look like the rest of our code base".
- **Reference:** commits `6ffa82104dd9b`, `f8b3c47a19254`, and `597394269e8f3` (all "Let the code speak") strip, respectively: a comment explaining why a token stays unresolved immediately above the exact check that shows it; a stub-method comment ("Unused: this processor does not use the base primary key drivers.") repeated over two methods that already just return `null`; and a class-level javadoc paragraph restating what the class's single `@Component` and name already convey.