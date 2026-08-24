# 306: Prefer StringBundler Over StringBuilder and Concatenation

Always build a string with `StringBundler` rather than `StringBuilder`. For `+` concatenation, convert to `StringBundler` only when more than three string pieces are joined; a join of three or fewer pieces is fine and stays as `+`. Use `StringBundler.concat(...)` for a fixed set of pieces, or append to a single `StringBundler` when building conditionally.

**Rationale:** `StringBundler` holds its pieces without the repeated copying and resizing that `StringBuilder` does, and it avoids the intermediate `String` objects that `+` concatenation creates. Standardizing on it makes string building uniform and avoids the cost of the alternatives on hot paths. Below four pieces the savings do not justify the added ceremony, so short joins stay as `+`.

A violation is a `new StringBuilder()`, or a `+` concatenation of more than three string pieces, where `StringBundler` should be used. A `+` of two or three pieces, such as `"Bearer " + _authToken`, is not a violation. Count each operand of the top-level `+` as one piece, separator literals included: `a + "/" + b` is three pieces and not a violation, while `a + "/" + b + "/" + c` is five and is. Adjacent literals the compiler folds (`"a" + "b"`) count as one piece. A `+` join of compile-time constants (a `static final` initializer, an annotation attribute, a `case` label), where a runtime `StringBundler` is not usable, is not a violation regardless of piece count.

**Example:** commit `166a3f2` replaced `new StringBuilder()` with `new StringBundler()`; `db9744b` replaced a chain of `StringBuilder.append(...)` with `StringBundler.concat(...)`; `6b9c309` replaced `+` concatenation of URI pieces with `StringBundler.concat`.
