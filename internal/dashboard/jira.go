package dashboard

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ticketRE matches a Jira issue key (e.g. LPD-12345). Branch names in this
// workspace are named after the ticket, so the first match is the ticket.
var ticketRE = regexp.MustCompile(`[A-Z][A-Z0-9]+-[0-9]+`)

// TicketKey extracts the Jira issue key from a branch name, or "" when the
// branch (e.g. master) carries none.
func TicketKey(branch string) string {
	return ticketRE.FindString(branch)
}

// FetchIssueView renders a ticket through the workspace's `issues` CLI
// (`issues view <key>`), which owns Jira credentials and formatting. Only
// the header block is returned — the description would crowd the panel.
func FetchIssueView(key string) (string, error) {
	path, err := exec.LookPath("issues")
	if err != nil {
		return "", fmt.Errorf("issues CLI not found in PATH")
	}

	out, err := exec.Command(path, "view", key).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("issues view %s: %v\n%s", key, err, lastLines(string(out), 3))
	}

	return headerBlock(string(out)), nil
}

// headerBlock cuts `issues view` output at the first blank line, keeping the
// key/summary headline and the fields block (status, assignee, URL, ...) and
// dropping the free-form description that follows.
func headerBlock(s string) string {
	s = strings.TrimRight(s, "\n")
	if idx := strings.Index(s, "\n\n"); idx >= 0 {
		s = s[:idx]
	}
	return s
}
