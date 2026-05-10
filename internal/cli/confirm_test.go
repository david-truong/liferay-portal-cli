package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfirm_AssumeYesFlag(t *testing.T) {
	var out bytes.Buffer
	got := confirmWithIO("Wipe?", true, strings.NewReader(""), &out, true)
	if !got {
		t.Error("--yes flag should bypass prompting and return true")
	}
	if out.Len() != 0 {
		t.Errorf("--yes should not print anything, got %q", out.String())
	}
}

func TestConfirm_EnvVarAssumeYes(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "1")
	var out bytes.Buffer
	got := confirmWithIO("Wipe?", false, strings.NewReader(""), &out, false)
	if !got {
		t.Error("LIFERAY_CLI_ASSUME_YES=1 should bypass prompting and return true")
	}
}

func TestConfirm_EnvVarSetToZero(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "0")
	var out bytes.Buffer
	got := confirmWithIO("Wipe?", false, strings.NewReader(""), &out, false)
	if got {
		t.Error("LIFERAY_CLI_ASSUME_YES=0 should not imply consent")
	}
}

func TestConfirm_NoTTYNoFlagNoEnv_Declines(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	var out bytes.Buffer
	got := confirmWithIO("Wipe?", false, strings.NewReader(""), &out, false)
	if got {
		t.Error("non-TTY without --yes or env var should decline")
	}
	if !strings.Contains(out.String(), "--yes") {
		t.Errorf("expected refusal message to mention --yes escape hatch, got %q", out.String())
	}
}

func TestConfirm_TTYUserYes(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	var out bytes.Buffer
	got := confirmWithIO("Wipe?", false, strings.NewReader("y\n"), &out, true)
	if !got {
		t.Error("user typed 'y' — should consent")
	}
}

func TestConfirm_TTYUserYESYes(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	var out bytes.Buffer
	got := confirmWithIO("Wipe?", false, strings.NewReader("YES\n"), &out, true)
	if !got {
		t.Error("user typed 'YES' — should consent (case-insensitive)")
	}
}

func TestConfirm_TTYUserNo(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	var out bytes.Buffer
	got := confirmWithIO("Wipe?", false, strings.NewReader("n\n"), &out, true)
	if got {
		t.Error("user typed 'n' — should decline")
	}
}

func TestConfirm_TTYUserEmpty_DefaultsToNo(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	var out bytes.Buffer
	got := confirmWithIO("Wipe?", false, strings.NewReader("\n"), &out, true)
	if got {
		t.Error("empty input should default to no (the prompt is y/N, not Y/n)")
	}
}

func TestConfirm_TTYUserGarbage_TreatedAsNo(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	var out bytes.Buffer
	got := confirmWithIO("Wipe?", false, strings.NewReader("maybe\n"), &out, true)
	if got {
		t.Error("non-y/yes response should be treated as decline")
	}
}

func TestConfirm_TTYPromptIncludesYN(t *testing.T) {
	t.Setenv("LIFERAY_CLI_ASSUME_YES", "")
	var out bytes.Buffer
	_ = confirmWithIO("Wipe everything?", false, strings.NewReader("n\n"), &out, true)
	if !strings.Contains(out.String(), "[y/N]") {
		t.Errorf("expected prompt to include '[y/N]', got %q", out.String())
	}
	if !strings.Contains(out.String(), "Wipe everything?") {
		t.Errorf("expected prompt body to appear, got %q", out.String())
	}
}
