# 502: Inline a Private Constant Used Once

A `private static final` constant that is referenced only once should be inlined at its single use site rather than declared separately.

**Rationale:** A named constant earns its place by being reused, or by naming a value whose meaning would not otherwise be obvious. A constant referenced exactly once, whose name only echoes its own literal, adds a declaration to read and a lookup to resolve for no benefit. Inlining it puts the value where it is used.

A violation is a `private static final` constant with exactly one reference, where the literal can be inlined at that reference without losing meaning. Count references as textual uses of the constant's identifier in the file, including annotation arguments and `{@value}` Javadoc. It is not a violation when the initializer allocates or computes (`Pattern.compile(...)`, `Set.of(...)`, `new ...`, a method call) and the single use site sits inside a loop or runs repeatedly, since inlining there would recompute the value each time; inline only a literal or trivially constant value evaluated once at the use site.

**Example:** commit `f7fbc55` inlined `_DEFAULT_SCRIPTING_JOB_NAME`, used once, back to its literal; `94f7e4a` collapsed a static initializer that populated a single field into a direct `Set.of(...)` initializer.
