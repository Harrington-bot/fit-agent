package descriptionlint_test

import (
	"strings"
	"testing"

	"github.com/jogvan-k/fit-agent/internal/descriptionlint"
)

func TestLint_NoIssues(t *testing.T) {
	cases := []struct {
		name string
		desc string
	}{
		{
			name: "empty",
			desc: "",
		},
		{
			name: "whitespace only",
			desc: "   \n\n\t  ",
		},
		{
			name: "valid duration steps",
			desc: "- 10m Z1-Z2 Pace\n- 30m Z2 Pace\n- 5m Z1-Z2 Pace",
		},
		{
			name: "valid pace target with duration",
			desc: "- 3m 4:35 Pace\n- Recovery 60s intensity=recovery",
		},
		{
			name: "valid repeat block",
			desc: "5x\n- 3m 4:35 Pace\n- Recovery 60s intensity=recovery",
		},
		{
			name: "valid distance",
			desc: "- 1km Z2 Pace",
		},
		{
			name: "valid fractional distance",
			desc: "- 0.3km 4:00 Pace",
		},
		{
			name: "valid mtr",
			desc: "- 400mtr Z5 Pace",
		},
		{
			name: "valid hours+minutes",
			desc: "- 1h30m Z1-Z2 Pace",
		},
		{
			name: "valid hours only",
			desc: "- 2h Z2 Pace",
		},
		{
			name: "valid minutes+seconds",
			desc: "- 30m45s Z2 Pace",
		},
		{
			name: "intensity=recovery no target",
			desc: "- Recovery 60s intensity=recovery",
		},
		{
			name: "pace range with duration",
			desc: "- 3m 4:55-4:35 Pace",
		},
		{
			name: "unstructured easy run no target",
			desc: "- 40m Z1-Z2 Pace",
		},
		{
			name: "comment lines ignored",
			desc: "# This is a comment\n- 10m Z2 Pace",
		},
		{
			name: "repeat header ignored",
			desc: "4x\n- 3m 4:35 Pace\n- 60s intensity=recovery",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warnings := descriptionlint.Lint(tc.desc)
			if len(warnings) != 0 {
				t.Errorf("expected no warnings, got: %v", warnings)
			}
		})
	}
}

func TestLint_MissingDuration(t *testing.T) {
	cases := []struct {
		name    string
		desc    string
		wantMsg string
	}{
		{
			name:    "pace target no duration",
			desc:    "- 4:35 Pace",
			wantMsg: "no duration or distance",
		},
		{
			name:    "zone target no duration",
			desc:    "- Z2 Pace",
			wantMsg: "no duration or distance",
		},
		{
			name:    "zone range no duration",
			desc:    "- Z1-Z2 Pace",
			wantMsg: "no duration or distance",
		},
		{
			name:    "pace range no duration",
			desc:    "- 4:55-4:35 Pace",
			wantMsg: "no duration or distance",
		},
		{
			name:    "missing duration in repeat block",
			desc:    "5x\n- 4:35 Pace\n- Recovery 60s intensity=recovery",
			wantMsg: "no duration or distance",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warnings := descriptionlint.Lint(tc.desc)
			if len(warnings) == 0 {
				t.Fatalf("expected warning containing %q, got none", tc.wantMsg)
			}
			found := false
			for _, w := range warnings {
				if strings.Contains(w.Message, tc.wantMsg) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected warning containing %q, got: %v", tc.wantMsg, warnings)
			}
		})
	}
}

func TestLint_BadDurationFormat(t *testing.T) {
	cases := []struct {
		name    string
		desc    string
		badTok  string
		suggest string
	}{
		{
			name:    "10min",
			desc:    "- 10min Z1-Z2 Pace",
			badTok:  "10min",
			suggest: "10m",
		},
		{
			name:    "30sec",
			desc:    "- 30sec intensity=recovery",
			badTok:  "30sec",
			suggest: "30s",
		},
		{
			name:    "2hours",
			desc:    "- 2hours Z2 Pace",
			badTok:  "2hours",
			suggest: "2h",
		},
		{
			name:    "5mins",
			desc:    "- 5mins 4:35 Pace",
			badTok:  "5mins",
			suggest: "5m",
		},
		{
			name:    "60seconds",
			desc:    "- 60seconds intensity=recovery",
			badTok:  "60seconds",
			suggest: "60s",
		},
		{
			name:    "1minute",
			desc:    "- 1minute Z2 Pace",
			badTok:  "1minute",
			suggest: "1m",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warnings := descriptionlint.Lint(tc.desc)
			if len(warnings) == 0 {
				t.Fatalf("expected warning for bad duration %q, got none", tc.badTok)
			}
			found := false
			for _, w := range warnings {
				if strings.Contains(w.Message, tc.badTok) && strings.Contains(w.Message, tc.suggest) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected warning mentioning %q and suggestion %q, got: %v", tc.badTok, tc.suggest, warnings)
			}
		})
	}
}

func TestLint_LineNumbers(t *testing.T) {
	desc := "- 10m Z1-Z2 Pace\n- 4:35 Pace\n- 5m Z2 Pace"
	warnings := descriptionlint.Lint(desc)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0].Line != 2 {
		t.Errorf("expected warning on line 2, got line %d", warnings[0].Line)
	}
}

func TestLint_MultipleIssues(t *testing.T) {
	desc := "- 10min Z2 Pace\n- 4:35 Pace\n- 30s intensity=recovery"
	warnings := descriptionlint.Lint(desc)
	// Line 1: bad duration "10min"
	// Line 2: pace target with no duration
	if len(warnings) < 2 {
		t.Errorf("expected at least 2 warnings, got %d: %v", len(warnings), warnings)
	}
}
