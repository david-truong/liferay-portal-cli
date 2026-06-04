// Package hosts manages liferay-cli-owned entries in the system hosts file
// (/etc/hosts). Each managed line maps a friendly hostname to 127.0.0.1 and
// carries a trailing marker comment tying it to a specific worktree id, so
// entries can be upserted and removed without disturbing anything else in the
// file.
//
// All functions here operate on file *content* (strings) and never touch the
// filesystem — the caller reads and writes /etc/hosts. This keeps the parsing
// and rewriting logic fully unit-testable without root.
package hosts

import (
	"fmt"
	"regexp"
	"strings"
)

// Path is the system hosts file every Unix resolver consults first.
const Path = "/etc/hosts"

// loopback is the address every managed entry points at. The hostname is a
// label; the per-worktree Tomcat port still distinguishes instances.
const loopback = "127.0.0.1"

// marker tags every managed line. The worktree id follows it so each line is
// attributable to exactly one worktree.
const marker = "# liferay-cli"

// Entry is one managed hosts line.
type Entry struct {
	Name string // the hostname mapped to 127.0.0.1
	ID   string // the owning worktree id
}

// hostnameRE validates a name as one or more dot-separated DNS labels. Each
// label is 1-63 chars of [a-z0-9-], not starting or ending with a hyphen.
// Resolution is case-insensitive, but we require lowercase so the file stays
// canonical and Remove/Upsert matching is exact.
var hostnameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// ValidateName reports whether name is a usable hosts-file hostname.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("hostname is empty")
	}
	if len(name) > 253 {
		return fmt.Errorf("hostname %q is longer than 253 characters", name)
	}
	if !hostnameRE.MatchString(name) {
		return fmt.Errorf("hostname %q is not a valid lowercase DNS name (use letters, digits, hyphens, and dots)", name)
	}
	return nil
}

// Sanitize turns an arbitrary string (typically a worktree directory name)
// into a valid single-label hostname: lowercased, with every run of invalid
// characters collapsed to a single hyphen and leading/trailing hyphens
// trimmed. Returns "" if nothing usable remains.
func Sanitize(raw string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(raw) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	return out
}

// formatLine renders the canonical managed line for (name, id).
func formatLine(name, id string) string {
	return fmt.Sprintf("%s\t%s\t%s %s", loopback, name, marker, id)
}

// idOf returns the worktree id of a managed line, or "" if the line is not
// one of ours.
func idOf(line string) string {
	idx := strings.Index(line, marker)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(line[idx+len(marker):])
}

// nameOf returns the hostname declared on a managed line, or "" if the line
// has no host token between the address and the marker.
func nameOf(line string) string {
	idx := strings.Index(line, marker)
	if idx < 0 {
		return ""
	}
	fields := strings.Fields(line[:idx])
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

// Upsert returns content with the managed line for id set to map name. Any
// pre-existing managed line for the same id is replaced in place; if none
// exists the new line is appended. Non-managed lines and managed lines for
// other ids are preserved verbatim.
func Upsert(content, name, id string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("worktree id is empty")
	}

	line := formatLine(name, id)
	lines := splitLines(content)

	replaced := false
	for i, l := range lines {
		if idOf(l) == id {
			lines[i] = line
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, line)
	}
	return joinLines(lines), nil
}

// Remove returns content with the managed line for id deleted. If no managed
// line for id exists, content is returned unchanged (second result false).
func Remove(content, id string) (string, bool) {
	lines := splitLines(content)
	out := make([]string, 0, len(lines))
	removed := false
	for _, l := range lines {
		if idOf(l) == id {
			removed = true
			continue
		}
		out = append(out, l)
	}
	if !removed {
		return content, false
	}
	return joinLines(out), true
}

// List returns every managed entry in content, in file order.
func List(content string) []Entry {
	var entries []Entry
	for _, l := range splitLines(content) {
		id := idOf(l)
		if id == "" {
			continue
		}
		entries = append(entries, Entry{Name: nameOf(l), ID: id})
	}
	return entries
}

// splitLines splits content into lines without a trailing empty element, so a
// file ending in "\n" does not gain a phantom blank line on every rewrite.
func splitLines(content string) []string {
	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// joinLines reassembles lines into file content with a single trailing newline.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
