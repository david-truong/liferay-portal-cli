# 906: Space Inline JSON

In inline JSON — a JSON literal inside a shell command, a test, or a single line string — put a space after each `:` and each `,`, but no space after `{` or before `}`. Write `{"key": "value", "other": "data"}`.

**Rationale:** The spacing makes an inline object read like the formatted JSON it stands for, so a reader parses it at a glance, and keeping the braces tight avoids the ragged look of padded braces.

A violation is inline JSON with no space after a `:` or `,`, or with a space just inside a `{` or `}`. This applies only to actual JSON — double-quoted keys and string values. A language-native object or map literal with unquoted keys (a JS or Groovy object) is out of scope. Not a violation: JSON spanning multiple lines (already formatted), or compact JSON in a test where the exact byte sequence is asserted or must match an external payload.
