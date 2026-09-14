package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jogvan-k/fit-agent/internal/icu"
	"github.com/jogvan-k/fit-agent/internal/workspace"
)

func TestSyncAthleteProfileFetchesTypedEndpointAndPreservesAuthoredText(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/athlete/0" {
			t.Errorf("path = %q, want /athlete/0", r.URL.Path)
		}
		user, password, ok := r.BasicAuth()
		if !ok || user != "API_KEY" || password != "test-key" {
			t.Errorf("basic auth = %q/%q (present %t)", user, password, ok)
		}
		_, _ = io.WriteString(w, `{
			"id":"i42", "name":"Test Athlete", "icu_ftp":253,
			"icu_lthr":171, "icu_resting_hr":43, "icu_max_hr":192,
			"icu_threshold_pace":4.1666666667
		}`)
	}))
	t.Cleanup(srv.Close)

	client, err := icu.NewClient("test-key", icu.Options{BaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	athlete, err := client.GetAthlete(context.Background(), icu.SelfAthleteID)
	if err != nil {
		t.Fatal(err)
	}

	layout := workspace.New(t.TempDir())
	const authored = `# Athlete profile

This paragraph and every non-marker line are hand-authored.

## Current fitness markers

- Running threshold pace (min/km): old pace
- Running max heart rate (bpm): 180
- Running resting heart rate (bpm): 50
- Cycling FTP (W): 200
- Cycling LTHR (bpm): 160
- Recent VO₂max estimate: 55

## Goals

- Primary goal: A hand-authored goal.
- Cycling FTP (W): an archived 200 W result, not a marker to update.
`
	if err := os.MkdirAll(layout.FitAgentDir(), workspace.DefaultDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.AthleteProfilePath(), []byte(authored), workspace.DefaultFileMode); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 14, 12, 14, 15, 0, time.FixedZone("BST", 3600))
	result, err := syncAthleteProfile(layout, athlete, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 5 {
		t.Errorf("updated = %d, want 5", result.Updated)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}

	gotBytes, err := os.ReadFile(layout.AthleteProfilePath())
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotBytes)
	for _, want := range []string{
		"- Running threshold pace (min/km): 4:00",
		"- Running max heart rate (bpm): 192",
		"- Running resting heart rate (bpm): 43",
		"- Cycling FTP (W): 253",
		"- Cycling LTHR (bpm): 171",
		"- Recent VO₂max estimate: 55",
		"- Last synced from intervals.icu: 2026-09-14T11:14:15Z",
		"This paragraph and every non-marker line are hand-authored.",
		"- Primary goal: A hand-authored goal.",
		"- Cycling FTP (W): an archived 200 W result, not a marker to update.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("profile missing %q:\n%s", want, got)
		}
	}
}

func TestSyncAthleteProfileDoesNotClearUnavailableMarkers(t *testing.T) {
	layout := workspace.New(t.TempDir())
	const profile = "# Profile\r\n\r\n## Current fitness markers\r\n- Running threshold pace (min/km): 4:20\r\n- Running max heart rate (bpm): 185\r\n- Running resting heart rate (bpm): 45\r\n- Cycling FTP (W): 250\r\n- Cycling LTHR (bpm): 165\r\n\r\n## Notes\r\nHand-authored.\r\n"
	if err := os.MkdirAll(layout.FitAgentDir(), workspace.DefaultDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.AthleteProfilePath(), []byte(profile), workspace.DefaultFileMode); err != nil {
		t.Fatal(err)
	}

	result, err := syncAthleteProfile(layout, &icu.Athlete{FTP: 260}, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 {
		t.Errorf("updated = %d, want 1", result.Updated)
	}
	gotBytes, err := os.ReadFile(layout.AthleteProfilePath())
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotBytes)
	for _, want := range []string{
		"Running threshold pace (min/km): 4:20\r\n",
		"Running max heart rate (bpm): 185\r\n",
		"Cycling FTP (W): 260\r\n",
		"- Last synced from intervals.icu: 2026-09-14T12:00:00Z\r\n",
		"## Notes\r\nHand-authored.\r\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("profile missing preserved/updated content %q:\n%q", want, got)
		}
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Errorf("expected CRLF line endings to be preserved: %q", got)
	}
}

func TestSyncAthleteProfileRejectsProfileWithoutMarkers(t *testing.T) {
	layout := workspace.New(t.TempDir())
	const profile = "# My hand-authored profile\n\nNothing to sync here.\n"
	if err := os.MkdirAll(layout.FitAgentDir(), workspace.DefaultDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.AthleteProfilePath(), []byte(profile), workspace.DefaultFileMode); err != nil {
		t.Fatal(err)
	}

	_, err := syncAthleteProfile(layout, &icu.Athlete{FTP: 250}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "no recognised fitness marker") {
		t.Fatalf("expected missing-marker error, got %v", err)
	}
	got, err := os.ReadFile(layout.AthleteProfilePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != profile {
		t.Errorf("profile changed on failure:\nwant %q\ngot  %q", profile, got)
	}
}
