package portal

import "testing"

func TestSanitizeUsername(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"dtruong", "dtruong"},
		{`DOMAIN\runneradmin`, "runneradmin"},
		{`a/b`, "b"},
		{`a\b\c`, "c"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := sanitizeUsername(tc.in); got != tc.want {
			t.Errorf("sanitizeUsername(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
