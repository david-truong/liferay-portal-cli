package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"time"
)

const jiraBaseURL = "https://liferay.atlassian.net"

// ticketRE matches a Jira issue key (e.g. LPD-12345). Branch names in this
// workspace are named after the ticket, so the first match is the ticket.
var ticketRE = regexp.MustCompile(`[A-Z][A-Z0-9]+-[0-9]+`)

// TicketKey extracts the Jira issue key from a branch name, or "" when the
// branch (e.g. master) carries none.
func TicketKey(branch string) string {
	return ticketRE.FindString(branch)
}

// Issue is the slice of a Jira issue the dashboard displays.
type Issue struct {
	Key      string
	Summary  string
	Status   string
	Assignee string
}

// FetchIssue loads an issue from the Jira REST API using the same
// JIRA_API_USER/JIRA_API_TOKEN credentials the rest of the workspace tooling
// uses.
func FetchIssue(key string) (Issue, error) {
	user := os.Getenv("JIRA_API_USER")
	token := os.Getenv("JIRA_API_TOKEN")
	if user == "" || token == "" {
		return Issue{}, fmt.Errorf("set JIRA_API_USER and JIRA_API_TOKEN to show ticket status")
	}

	req, err := http.NewRequest(http.MethodGet,
		jiraBaseURL+"/rest/api/3/issue/"+key+"?fields=summary,status,assignee", nil)
	if err != nil {
		return Issue{}, err
	}
	req.SetBasicAuth(user, token)

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return Issue{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Issue{}, fmt.Errorf("jira returned %s for %s", resp.Status, key)
	}

	var payload struct {
		Fields struct {
			Summary string `json:"summary"`
			Status  struct {
				Name string `json:"name"`
			} `json:"status"`
			Assignee struct {
				DisplayName string `json:"displayName"`
			} `json:"assignee"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Issue{}, err
	}

	return Issue{
		Key:      key,
		Summary:  payload.Fields.Summary,
		Status:   payload.Fields.Status.Name,
		Assignee: payload.Fields.Assignee.DisplayName,
	}, nil
}
