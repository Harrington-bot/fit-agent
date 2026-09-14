package cli

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jogvan-k/fit-agent/internal/icu"
	"github.com/jogvan-k/fit-agent/internal/workspace"
)

const athleteProfileSyncLabel = "Last synced from intervals.icu"

// athleteProfileSyncResult summarizes the targeted profile changes.
type athleteProfileSyncResult struct {
	Updated int
	DryRun  bool
}

func (r athleteProfileSyncResult) String() string {
	if r.DryRun {
		return "dry-run (no changes)"
	}
	return fmt.Sprintf("updated %d fitness marker(s)", r.Updated)
}

func newSyncAthleteProfileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync-athlete-profile",
		Short: "Sync intervals.icu fitness markers into ATHLETE-PROFILE.md",
		Long: `sync-athlete-profile fetches the authenticated athlete endpoint and updates
only the recognised lines in ATHLETE-PROFILE.md's "Current fitness markers"
section. All hand-authored text is preserved. It also records a clearly labelled
last-synced timestamp in that section.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, dryRun, err := resolveRuntime(cmd)
			if err != nil {
				return err
			}
			if dryRun {
				fmt.Fprintf(stdoutOrStderrForResults(cmd), "sync-athlete-profile: %s\n", athleteProfileSyncResult{DryRun: true})
				return nil
			}

			athlete, err := res.Client.GetAthlete(ctxOrBackground(cmd), icu.SelfAthleteID)
			if err != nil {
				return fmt.Errorf("get athlete profile: %w", err)
			}
			result, err := syncAthleteProfile(res.Layout, athlete, time.Now())
			if err != nil {
				return err
			}
			fmt.Fprintf(stdoutOrStderrForResults(cmd), "sync-athlete-profile: %s\n", result)
			return nil
		},
	}
}

// syncAthleteProfile updates only recognised marker lines and the sync
// timestamp. It refuses to write a profile that has none of those lines, so a
// custom profile cannot unexpectedly receive an unrelated appended section.
func syncAthleteProfile(layout workspace.Layout, athlete *icu.Athlete, now time.Time) (athleteProfileSyncResult, error) {
	if athlete == nil {
		return athleteProfileSyncResult{}, fmt.Errorf("sync athlete profile: nil athlete")
	}

	path := layout.AthleteProfilePath()
	body, err := os.ReadFile(path)
	if err != nil {
		return athleteProfileSyncResult{}, fmt.Errorf("read athlete profile %s: %w", path, err)
	}

	sectionStart, sectionEnd, ok := currentFitnessMarkerSection(body)
	if !ok {
		return athleteProfileSyncResult{}, fmt.Errorf("athlete profile %s has no recognised fitness marker lines (missing Current fitness markers section)", path)
	}
	section := body[sectionStart:sectionEnd]
	values := athleteFitnessMarkers(athlete)
	updated := 0
	lastMarkerEnd := -1
	for _, marker := range profileFitnessMarkers {
		re := markerLineRegexp(marker.label)
		loc := re.FindIndex(section)
		if loc != nil && loc[1] > lastMarkerEnd {
			lastMarkerEnd = loc[1]
		}
		value, ok := values[marker.label]
		if !ok || loc == nil {
			continue
		}
		section = re.ReplaceAll(section, []byte("${1} "+value+"${2}"))
		updated++
	}
	if lastMarkerEnd == -1 {
		return athleteProfileSyncResult{}, fmt.Errorf("athlete profile %s has no recognised fitness marker lines", path)
	}

	timestamp := now.UTC().Format(time.RFC3339)
	timestampRE := markerLineRegexp(athleteProfileSyncLabel)
	if timestampRE.Match(section) {
		section = timestampRE.ReplaceAll(section, []byte("${1} "+timestamp+"${2}"))
	} else {
		// Re-find the final marker after replacements. Preserve the document's
		// native line ending when inserting the one machine-managed line.
		lastMarkerEnd = -1
		for _, marker := range profileFitnessMarkers {
			if loc := markerLineRegexp(marker.label).FindIndex(section); loc != nil && loc[1] > lastMarkerEnd {
				lastMarkerEnd = loc[1]
			}
		}
		eol := "\n"
		if strings.Contains(string(section), "\r\n") {
			eol = "\r\n"
		}
		// The regexp ends before LF (but includes a preceding CR). Insert
		// after the existing line ending when there is one, preserving every
		// byte in the surrounding authored text.
		insertAt := lastMarkerEnd
		if insertAt < len(section) && section[insertAt] == '\n' {
			insertAt++
			section = insertProfileSectionLine(section, insertAt, []byte("- "+athleteProfileSyncLabel+": "+timestamp+eol))
		} else {
			section = insertProfileSectionLine(section, insertAt, []byte(eol+"- "+athleteProfileSyncLabel+": "+timestamp))
		}
	}
	// Build a separate buffer rather than appending into body. The replacement
	// values can be shorter than the authored lines, and append's in-place
	// reuse would overwrite the still-needed suffix before it is copied.
	updatedBody := make([]byte, 0, sectionStart+len(section)+len(body)-sectionEnd)
	updatedBody = append(updatedBody, body[:sectionStart]...)
	updatedBody = append(updatedBody, section...)
	updatedBody = append(updatedBody, body[sectionEnd:]...)
	body = updatedBody

	if err := workspace.AtomicWrite(path, body, workspace.DefaultFileMode); err != nil {
		return athleteProfileSyncResult{}, fmt.Errorf("write athlete profile %s: %w", path, err)
	}
	return athleteProfileSyncResult{Updated: updated}, nil
}

func insertProfileSectionLine(section []byte, at int, line []byte) []byte {
	updated := make([]byte, 0, len(section)+len(line))
	updated = append(updated, section[:at]...)
	updated = append(updated, line...)
	updated = append(updated, section[at:]...)
	return updated
}

type profileFitnessMarker struct {
	label string
}

var profileFitnessMarkers = []profileFitnessMarker{
	{label: "Running threshold pace (min/km)"},
	{label: "Running max heart rate (bpm)"},
	{label: "Running resting heart rate (bpm)"},
	{label: "Cycling FTP (W)"},
	{label: "Cycling LTHR (bpm)"},
}

func athleteFitnessMarkers(athlete *icu.Athlete) map[string]string {
	values := make(map[string]string)
	if athlete.ThresholdPace > 0 {
		values["Running threshold pace (min/km)"] = formatPacePerKM(athlete.ThresholdPace)
	}
	if athlete.MaxHR > 0 {
		values["Running max heart rate (bpm)"] = strconv.Itoa(athlete.MaxHR)
	}
	if athlete.RestingHR > 0 {
		values["Running resting heart rate (bpm)"] = strconv.Itoa(athlete.RestingHR)
	}
	if athlete.FTP > 0 {
		values["Cycling FTP (W)"] = strconv.Itoa(athlete.FTP)
	}
	if athlete.LTHR > 0 {
		values["Cycling LTHR (bpm)"] = strconv.Itoa(athlete.LTHR)
	}
	return values
}

func formatPacePerKM(metersPerSecond float64) string {
	seconds := int(metersPerSecondToPaceSeconds(metersPerSecond) + 0.5)
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func metersPerSecondToPaceSeconds(metersPerSecond float64) float64 {
	return 1000 / metersPerSecond
}

func markerLineRegexp(label string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^([\t ]*-[\t ]*` + regexp.QuoteMeta(label) + `:)[^\r\n]*(\r?)$`)
}

// currentFitnessMarkerSection returns the byte range after the exact marker
// heading through, but not including, the next level-two Markdown heading.
func currentFitnessMarkerSection(body []byte) (start, end int, ok bool) {
	heading := regexp.MustCompile(`(?m)^## Current fitness markers\r?$`).FindIndex(body)
	if heading == nil {
		return 0, 0, false
	}
	start = heading[1]
	if start < len(body) && body[start] == '\n' {
		start++
	}
	end = len(body)
	if next := regexp.MustCompile(`(?m)^## `).FindIndex(body[start:]); next != nil {
		end = start + next[0]
	}
	return start, end, true
}
