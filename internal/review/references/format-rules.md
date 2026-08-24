# Format-Source Rules (20–56)

Mechanical and small-refactor rules applied during review. Companion to `mandates.md` — those capture architectural mandates; these capture line-level conventions. Rule numbers preserved from the upstream `format-source` skill source (rules 1–19 live there; 20–56 below), so the sequence has gaps. Some rules overlap with mandates in `mandates.md`; both apply.

Rules that merely restated a canonical rule have been removed — cite the canonical rule instead: declaration order against the consuming call is `[203]`/`[201]`/`[202]` (was Format Rule 28); grouping and sorting sibling constants is `[108]`/`[202]` (was Format Rules 33 and 51). The numbers are not reused.

### Rule 20: Drop Unnecessary `L` Suffix on Long Literals

**Why:** When the receiving slot is already typed `long`, the `L` suffix on an integer literal adds visual noise without changing the value, since the integer is widened automatically.

**Examples:**

```diff
-public static final long TIMEOUT = 300000L;
+public static final long TIMEOUT = 300000;
```

### Rule 21: Avoid `iterator().next()` for First-Element Access

**Why:** A multi-method chain to reach the first element hides the intent behind two calls; pick the most direct accessor the type already offers so the call site reads as a single lookup.

**Examples:**

```diff
-Foo foo = page.getItems().iterator().next();
+Foo foo = page.getItems().get(0);
```

### Rule 22: Prefer `while` Over `do-while`

**Why:** A `while` loop checks its guard before the body, matching the dominant convention in the codebase; `do-while` belongs only to cases where the body genuinely must run before the first check.

**Examples:**

```diff
-do {
-	advance();
-}
-while (!done());
+while (!done()) {
+	advance();
+}
```

### Rule 23: No Blank Line Between Paired Statements

**Why:** Two statements that form a tight pair (parallel setup, parallel assertions, paired declarations) read as one logical step; a blank line between them suggests two paragraphs and slows the reader down.

**Examples:**

Paired declarations:

```diff
 Foo first = create("first");
-
 Foo second = create("second");
```

Paired assertions on parallel results:

```diff
 Assert.assertEquals("alpha", alpha.getName());
-
 Assert.assertEquals("beta", beta.getName());
```

### Rule 24: Single Space After a Period in Prose

**Why:** Two spaces after a period is a legacy typewriter convention; modern Liferay prose in comments, JSPs, language strings, and Markdown uses one.

**Examples:**

```diff
-// Compute the score.  Cache it for next time.
+// Compute the score. Cache it for next time.
```

### Rule 25: Method-Name Plurality Matches Return-Type Plurality

**Why:** A method that returns a collection should signal that in its name, so a reader can predict the return type without checking the signature; a singular-sounding name on a `List` return forces a second look.

**Examples:**

```diff
-public List<Foo> getFooOverview() {
+public List<Foo> getFoos() {
 	return _fooLocalService.findAll();
 }
```

### Rule 26: Mirror Methods Share a Naming Suffix

**Why:** When two methods are paired (data-fetch with count, get with getCount, by-key with by-key-count), they should differ only by the operation noun; a mismatched suffix hides the pairing from `git grep` and from the reader.

**Examples:**

```diff
 public List<Foo> getFoosByGroupIds(long[] groupIds);
-public int getFoosCount(long[] groupIds);
+public int getFoosCountByGroupIds(long[] groupIds);
```

### Rule 27: "Cleanup" Is a Noun, "Clean Up" Is a Verb

**Why:** Treating `cleanup` and `clean up` as interchangeable produces noise in symbols and prose; the noun form names a thing (a method, a phase, a section header), the verb form describes an action.

**Examples:**

The verb form belongs in imperative comments and method names.

```diff
-// Cleanup the cache after the test runs
+// Clean up the cache after the test runs
```

The noun form belongs in identifiers and section headers.

```diff
-public void runCleanUp() {
+public void runCleanup() {
```

### Rule 29: Blank Line After a Logical Setup Block

**Why:** Companion to Rule 23: when one logical setup block finishes (configuring a context object, mocking a service, building a fixture), a single blank line marks the boundary so the next block reads as a new paragraph.

**Examples:**

```diff
 themeDisplay.setSiteGroupId(siteGroupId);
 themeDisplay.setUser(user);
+
 ThemeRequest themeRequest = new ThemeRequest();
 themeRequest.setLocale(locale);
```

### Rule 30: Descriptive Lambda Parameter Names

**Why:** A bare `k`, `v`, or `e` lambda parameter forces the reader to scroll up to the enclosing call to know what it represents; a domain name reads on its own line.

**Examples:**

```diff
-fooMap.computeIfAbsent(name, k -> new HashSet<>());
+fooMap.computeIfAbsent(name, fooName -> new HashSet<>());
```

### Rule 31: Use Guard Clauses Over Nested Positive Conditions

**Why:** Inverting the condition and exiting early flattens the indentation of the bulk of the method, so the reader follows one straight column instead of an arrow-shaped nest.

**Examples:**

```diff
 for (Foo foo : foos) {
-	if ((foo != null) && foo.isEnabled()) {
-		// thirty lines of work
-	}
+	if ((foo == null) || !foo.isEnabled()) {
+		continue;
+	}
+
+	// thirty lines of work
 }
```

### Rule 32: Prefer `@Before` and `@After` Over `@BeforeClass` and `@AfterClass`

**Why:** Instance-level setup gives each test a fresh state; class-level setup creates hidden coupling between tests that share the static fixture, and the failure mode is silent when one test mutates it.

**Examples:**

```diff
-@BeforeClass
-public static void setUpClass() throws Exception {
+@Before
+public void setUp() throws Exception {
 	_fixture = createFixture();
 }
```

### Rule 34: Variable Name Matches the Expression Assigned to It

**Why:** When a local's name disagrees with the right-hand side, the reader has to verify which name is the truthful one at every use; pick the name from the expression so the two stay in sync.

**Examples:**

```diff
-long[] currentAndAncestorGroupIds = getReferencedGroupIds();
+long[] referencedGroupIds = getReferencedGroupIds();
```

### Rule 35: Declare a Variable Inside the `try` Block When Used Only There

**Why:** Hoisting a local out of the `try` only makes sense when the `catch` or `finally` needs it; otherwise the wider scope adds visual noise and a reader has to confirm the local is not reused later.

**Examples:**

```diff
-Foo foo = makeFoo();
-
-try {
-	foo.consume();
-}
-catch (Exception exception) {
-	_log.error("Failed", exception);
-}
+try {
+	Foo foo = makeFoo();
+
+	foo.consume();
+}
+catch (Exception exception) {
+	_log.error("Failed", exception);
+}
```

### Rule 36: One Dependency Per Line in Gradle `dependencies` Blocks

**Why:** Multiple dependencies on a single line obscure additions and removals in diffs and break the alphabetical order surrounding `build.gradle` files use.

**Examples:**

```diff
 dependencies {
-	compileOnly group: "com.alpha", name: "foo"; compileOnly group: "com.beta", name: "bar"
+	compileOnly group: "com.alpha", name: "foo"
+	compileOnly group: "com.beta", name: "bar"
 }
```

### Rule 37: Add `@Override` on Every Overriding Method

**Why:** `@Override` makes the override explicit, lets the compiler catch signature drift in the supertype, and matches every other override in the codebase.

**Examples:**

```diff
+@Override
 public void doSomething() {
 	super.doSomething();
 }
```

### Rule 38: Use Passive Past Tense in Status and Error Messages

**Why:** Status and error sentences read as complete clauses when they include the auxiliary verb; "X was not found" beats "X not found", and "Unable to delete X" beats "Cannot delete X" / "Failed to delete X" / "Error deleting X".

**Examples:**

```diff
-throw new IllegalStateException("Foo not found for id " + id);
+throw new IllegalStateException("Foo was not found for id " + id);
```

```diff
-_log.warn("Cannot delete foo " + id);
+_log.warn("Unable to delete foo " + id);
```

### Rule 39: Separate Must-Be-First or Must-Be-Last Items With a Blank Line

**Why:** When a list is mostly alphabetical but a few items must lead or trail (initialization, defaults, "all"), a blank line between the special items and the sorted block signals that the leading items are intentionally first and that the rest of the order has no further meaning.

**Examples:**

```diff
 public enum FooStep {

 	INIT,
+
 	ALPHA,
 	BETA,
 	GAMMA;

 }
```

### Rule 40: No ASCII-Art Separators in CSS Comments

**Why:** ASCII-art rules duplicate what indentation and blank lines already convey; matching the surrounding terse comment style keeps the file scannable and consistent.

**Examples:**

```diff
-/* ---------- Hide the column when narrow ---------- */
+/* Hide the column when narrow */
 .foo .col {
 	display: none;
 }
```

### Rule 41: Combine Consecutive `StringBundler.append` of Literal Strings

**Why:** Two literal-only `append` calls collapse to one without changing behavior, and the merged form lets the reader see the full literal in a single line.

**Examples:**

```diff
-sb.append("<?xml version=\"1.0\"?>");
-sb.append("<foo><bar>");
+sb.append("<?xml version=\"1.0\"?><foo><bar>");
```

### Rule 42: Drop the Fully-Qualified Class Name When the Class Is Imported

**Why:** Once a class is in the import block, the fully-qualified form at the use site adds noise and forces a line wrap; the simple name is what every other reference in the file already uses.

**Examples:**

```diff
 import com.example.foo.FooResource;

 // ...

-com.example.foo.FooResource fooResource = factory.create();
+FooResource fooResource = factory.create();
```

### Rule 43: Bash Scripts Exit on First Failure

**Why:** Without an explicit fail-fast directive, a failing command silently passes through to the next; either `set -e` at the top of the script or `|| exit 1` on each command makes the failure surface immediately.

**Examples:**

```diff
 #!/bin/bash
+
+set -e

 _execute "step-1"
 _execute "step-2"
```

### Rule 44: Class-Level Constants Live at the Top of the Class, Sorted

**Why:** Constants scattered between methods force the reader to scan the whole class to confirm what is and is not a constant; a single sorted block at the top, after fields, is the canonical place to look.

**Examples:**

```diff
 public class Foo {

+	private static final String _ALPHA = "alpha";
+
+	private static final String _BAR = "bar";
+
 	public void doFirst() {
-	}
-
-	private static final String _BAR = "bar";
-
-	public void doSecond() {
 	}

-	private static final String _ALPHA = "alpha";
+	public void doSecond() {
+	}

 }
```

### Rule 45: Methods Used Only Inside the Class Must Be `private`

**Why:** Default or `public` visibility on a class-internal method overstates the contract; a reader cannot tell from the signature alone whether external callers exist, and tooling cannot prune the method when its callers go away.

**Examples:**

```diff
-void _doInternal() {
+private void _doInternal() {
 	// ...
 }
```

### Rule 46: Method Names Start With a Verb

**Why:** A method represents an action, so its name should begin with one (`get`, `set`, `is`, `make`, `attach`, `verify`, …); a noun-only name reads as a field, not a call.

**Examples:**

```diff
-private boolean _osgiAware() {
+private boolean _isOsgiAware() {
 	return _bundleContext != null;
 }
```

### Rule 47: Hyphenation Is Consistent Within a File

**Why:** Mixing `non-account` and `nonaccount` (or any other hyphenation variant) inside a single file makes the term read as two different concepts; pick one form per file and apply it everywhere.

**Examples:**

```diff
-// non account entry path
+// nonaccount entry path
 String path = ...;

 // ...

-// non-account fallback
+// nonaccount fallback
 String fallback = ...;
```

### Rule 48: Sort Mockito Mock Declarations by Usage, Then Alphabetically

**Why:** Companion to Rule 28: ordering test fixtures in the order the system under test exercises them lets a reader trace the mocks against the production call sequence; alphabetical breaks ties.

**Examples:**

When the system under test calls `_serviceA` first and `_serviceB` second:

```diff
-Mockito.when(_serviceB.findFoo(id)).thenReturn(foo);
 Mockito.when(_serviceA.exists(id)).thenReturn(true);
+Mockito.when(_serviceB.findFoo(id)).thenReturn(foo);
```

### Rule 49: Inline Single-Use Anonymous Classes

**Why:** When an anonymous-class instance is referenced exactly once at the next call site, naming it adds a hop without adding meaning; passing the `new ...() {}` directly into the call removes the local.

**Examples:**

```diff
-FooDelegate fooDelegate = new FooDelegate() {};
-
-method.invoke(fooDelegate);
+method.invoke(new FooDelegate() {});
```

### Rule 50: Constants in the Same Group Share a Value Shape

**Why:** When a contiguous block of constants names the same kind of thing (renderer keys, attribute names, action IDs), every value should follow the same shape — all class FQNs, or all symbolic strings, but not a mix — so a reader knows what to expect from the group at a glance.

**Examples:**

```diff
-public static final String _FOO_KEY = "FOO_KEY";

+public static final String _FOO_KEY =
+	"com.liferay.foo.Foo";

 public static final String _BAR_KEY =
 	"com.liferay.foo.Bar";
```

### Rule 52: Blank Line Between Dependent Resources in `try`-With-Resources

**Why:** When a multi-resource `try` mixes independent resources with one that consumes them, a blank line between the independent group and the consumer makes the dependency visible at the resource declaration level instead of forcing the reader to trace it through the body.

**Examples:**

```diff
 try (
 	PreparedStatement preparedStatement1 = connection.prepareStatement(selectSQL);
 	PreparedStatement preparedStatement2 = connection.prepareStatement(updateSQL);
+
 	ResultSet resultSet = preparedStatement1.executeQuery()) {
```

### Rule 53: `HashMapBuilder` Is Overkill for Two or Three Static Entries

**Why:** A `HashMapBuilder.put(...).put(...).build()` chain for a couple of static entries adds visual machinery that earns its keep only when the map gains more entries or conditional ones; for a two-or-three-way lookup, an `if/else` is shorter and reads as the code's intent.

**Examples:**

```diff
-Map<String, String> aliases = HashMapBuilder.put(
-	"lat", "latitude"
-).put(
-	"lng", "longitude"
-).build();
-
-String full = aliases.get(abbrev);
+String full;
+
+if ("lat".equals(abbrev)) {
+	full = "latitude";
+}
+else {
+	full = "longitude";
+}
```

### Rule 54: Extract Repeated Test Literals Into `private static final` Constants

**Why:** A literal repeated across mock setups and assertions starts to read like a variable; promoting it to a `private static final` makes the value's identity explicit and prevents one of the call sites from drifting away from the others. Boundary with Rule 12: this rule catches the *repeated* case, while Rule 12 still inlines a *single-use* literal.

**Examples:**

```diff
+private static final String _GROUP_DISPLAY_URL = "groupDisplayURL";
+
 // ...

-Mockito.when(_request.getAttribute("groupDisplayURL")).thenReturn(url);
+Mockito.when(_request.getAttribute(_GROUP_DISPLAY_URL)).thenReturn(url);

-Assert.assertEquals("groupDisplayURL", attribute);
+Assert.assertEquals(_GROUP_DISPLAY_URL, attribute);
```

### Rule 55: Do Not Wrap and Rethrow a Checked Exception

**Why:** Wrapping a caught exception in a generic runtime exception (or a framework-specific equivalent) hides both the original type and the original stack frame; let the original propagate by declaring it on the method, or use a known utility for signature-blocked cases.

**Examples:**

```diff
-try {
-	parser.read(input);
-}
-catch (IOException ioException) {
-	throw new PortalException(ioException);
-}
+parser.read(input);
```

### Rule 56: Drop Defensive `Math.ceil` on Whole-Unit Division

**Why:** When converting a whole-unit value (milliseconds to seconds, bytes to kilobytes) for an input whose realistic values are already multiples of the divisor, `Math.ceil` adds a double promotion and a primitive cast that all collapse for any actual input; integer division does the same job.

**Examples:**

```diff
-long seconds = (long)Math.ceil(milliseconds / 1000.0);
+long seconds = milliseconds / 1000;
```
