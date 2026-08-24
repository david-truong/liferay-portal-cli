# 801: A Method Used Once Must Be Private

A method called from only one place must be `private`, and, per house style, prefixed with an underscore. A `protected` method is justified only when it overrides a base class or base test case method; otherwise make it `private`.

**Rationale:** Visibility should match reach. A method with a single caller that is not an override has no reason to be `protected`, package visible, or `public` — wider visibility advertises an extension point that does not exist and invites callers that should not exist. Narrowing it to `private` states its true scope and keeps the class's surface honest.

A violation is a nonoverriding method with a single caller declared `protected`, package visible, or `public`, where it should be `private`. Count callers across the codebase, not just the diff; if a single caller cannot be confirmed, do not flag. It is also not a violation, regardless of caller count, when the method is invoked by name rather than by a direct call: an override of a base class or interface method, a method bearing a framework annotation (`@Test`, `@Before`/`@After`, `@Activate`/`@Deactivate`, an `@Reference` setter or bind/unbind), an MVC action or render command method, or a method reached by reflection or EL/JSP.

**Example:** commit `cbe50a17` states the rule directly ("If it's a protected method, it must @Override a base test case method. Otherwise, make it private.") and renames a `protected` helper to a `private` one; `6093ee34` and `752e65f` made the same `protected` to `private` change for single caller methods.
