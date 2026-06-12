package dashboard

import "testing"

func TestTicketKey(t *testing.T) {
	cases := []struct {
		branch string
		want   string
	}{
		{"LPD-89485", "LPD-89485"},
		{"BPR-88517-backport", "BPR-88517"},
		{"feature/LRCI-1234-cleanup", "LRCI-1234"},
		{"master", ""},
		{"lpd-12345", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := TicketKey(c.branch); got != c.want {
			t.Errorf("TicketKey(%q) = %q, want %q", c.branch, got, c.want)
		}
	}
}
