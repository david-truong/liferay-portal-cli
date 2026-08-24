# 708: Write Markdown Paragraphs on One Line

In a Markdown file, put each paragraph on a single line and rely on the editor's soft wrap; do not hard wrap at eighty columns. Separate paragraphs with a blank line.

**Rationale:** A one line paragraph keeps diffs clean, since an edit changes one line rather than reflowing a whole block, and it lets the rendered output decide wrapping rather than the width of the source. This is intentional, and it contradicts most editor defaults.

A violation is a Markdown paragraph hard wrapped across several lines. This applies only to ordinary prose paragraphs; line breaks inside list items, table rows, fenced or indented code blocks, blockquotes, and frontmatter are structural, not hard wraps, and are never violations. Flag only consecutive non-blank prose lines that could be joined into one line without changing the rendered output.
