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

func TestHeaderBlock(t *testing.T) {
	view := "LPD-1  Summary\n────\n  Status:   Open\n  URL:      https://x\n\nLong description\nmore\n"

	want := "LPD-1  Summary\n────\n  Status:   Open\n  URL:      https://x"
	if got := headerBlock(view); got != want {
		t.Errorf("headerBlock = %q, want %q", got, want)
	}

	if got := headerBlock("LPD-1  Summary\n  Status: Open\n"); got != "LPD-1  Summary\n  Status: Open" {
		t.Errorf("headerBlock without description = %q", got)
	}
}
