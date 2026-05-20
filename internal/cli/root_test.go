package cli

import (
	"reflect"
	"testing"
)

func TestParseGlobalFlagsExtractsDirectoryAndStops(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantDir  string
		wantVerb bool
		wantRest []string
	}{
		{
			name:     "short with space",
			args:     []string{"-C", "/some/path", "module", "task"},
			wantDir:  "/some/path",
			wantRest: []string{"module", "task"},
		},
		{
			name:     "short concatenated",
			args:     []string{"-C/some/path", "module", "task"},
			wantDir:  "/some/path",
			wantRest: []string{"module", "task"},
		},
		{
			name:     "long with space",
			args:     []string{"--directory", "/some/path", "module", "task"},
			wantDir:  "/some/path",
			wantRest: []string{"module", "task"},
		},
		{
			name:     "long with equals",
			args:     []string{"--directory=/some/path", "module", "task"},
			wantDir:  "/some/path",
			wantRest: []string{"module", "task"},
		},
		{
			name:     "verbose short",
			args:     []string{"-v", "module", "task"},
			wantVerb: true,
			wantRest: []string{"module", "task"},
		},
		{
			name:     "verbose long combined with directory",
			args:     []string{"--verbose", "-C", "/p", "module"},
			wantDir:  "/p",
			wantVerb: true,
			wantRest: []string{"module"},
		},
		{
			name:     "stops at module so unknown gradle flags pass through",
			args:     []string{"-C", "/p", "module", "--tests", "Foo", "--info"},
			wantDir:  "/p",
			wantRest: []string{"module", "--tests", "Foo", "--info"},
		},
		{
			name:     "no global flags returns args unchanged",
			args:     []string{"module", "task", "--info"},
			wantRest: []string{"module", "task", "--info"},
		},
		{
			name:     "empty args",
			args:     []string{},
			wantRest: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gf, rest, err := parseGlobalFlags(tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gf.dir != tt.wantDir {
				t.Errorf("dir = %q, want %q", gf.dir, tt.wantDir)
			}
			if gf.verbose != tt.wantVerb {
				t.Errorf("verbose = %v, want %v", gf.verbose, tt.wantVerb)
			}
			if !reflect.DeepEqual(rest, tt.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
		})
	}
}

func TestParseGlobalFlagsMissingDirectoryArgErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-C"},
		{"--directory"},
	} {
		if _, _, err := parseGlobalFlags(args); err == nil {
			t.Errorf("parseGlobalFlags(%v) = nil error, want error for missing value", args)
		}
	}
}
