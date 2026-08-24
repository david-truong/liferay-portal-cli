# 901: Prefer Rigid Checks Over Flexible Ones

In runtime logic, prefer an explicit, exact check over a loose one. Write `host.equals("localhost:8080")`, not `host.contains("localhost")`; match the whole value rather than a substring, unless a substring is genuinely what you mean.

This applies to production runtime code, not to test assertions. A test is any code reached only from a `@Test` method, including helpers and fixtures in `*-test`/`*-test-util` modules; a loose check there is not a violation, and a loose check in non-test runtime logic is a violation regardless of which module it lives in. In a test it is often easier, and perfectly fine, to assert on part of a value — `assertTrue(message.contains("quota exceeded"))` rather than matching the whole message — so do not flag a partial check in a test.

**Rationale:** In runtime logic a loose check quietly accepts input you did not intend and hides bugs behind apparent success, so an exact check is safer. A test only needs to confirm the relevant part of an outcome, and pinning it to the whole message makes the test brittle for no benefit.

A violation is a substring or otherwise loose check standing in for an exact comparison in runtime logic. A partial check in a test is not a violation. Treat a substring check as intentional only when the value legitimately contains the target as one part of many (searching free text or a delimited list); when the checked value is a single discrete token — a host, status, id, or full URL — a loose check is a violation.
