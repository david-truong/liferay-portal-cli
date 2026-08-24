package review

import "embed"

// References embeds the brian-review rule corpus (dimensions.md, style.md,
// format-rules.md, mandates.md, liferay-conventions.md, and every
// rules/NNN-*.md file) so a review can run standalone, with no dependency on
// ~/.claude/skills/brian-review or any external Python interpreter.
//
//go:embed references/*.md references/rules/*.md
var References embed.FS
