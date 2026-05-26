// Package descriptionlint validates ICU native description strings before
// they are pushed to intervals.icu.
//
// A description that looks syntactically valid to the agent may still fail
// to produce a structured workout_doc on the ICU side, silently resulting
// in a planned event with no load projection. This package catches the two
// most common mistakes:
//
//  1. A step line that carries an intensity/pace target but has no
//     duration or distance — ICU cannot compute load without knowing
//     how long or far the step is.
//
//  2. A duration written in a format ICU does not recognise, such as
//     "10min" or "30sec" instead of the correct "10m" / "30s".
package descriptionlint

import (
	"fmt"
	"regexp"
	"strings"
)

// Warning is a single lint finding.
type Warning struct {
	// Line is the 1-based line number within the description.
	Line int
	// Message describes the problem.
	Message string
}

// String returns a human-readable representation.
func (w Warning) String() string {
	return fmt.Sprintf("line %d: %s", w.Line, w.Message)
}

// Lint checks an ICU native description string for common mistakes and
// returns any warnings found. An empty slice means no problems were
// detected. Lint never returns an error — a description that cannot be
// parsed is treated as opaque and passes through without warnings.
func Lint(desc string) []Warning {
	if strings.TrimSpace(desc) == "" {
		return nil
	}
	var warnings []Warning
	lines := strings.Split(desc, "\n")
	for i, raw := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(raw)

		// Skip blank lines, comments, machine-managed sentinels, and
		// repeat-count headers (e.g. "4x" or "4x\n").
		if trimmed == "" ||
			strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "<!--") ||
			repeatHeader.MatchString(trimmed) {
			continue
		}

		// Only inspect step lines: those starting with "- " after
		// optional leading whitespace.
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		body := strings.TrimPrefix(trimmed, "- ")

		// Check 1: wrong duration format (e.g. "10min", "30sec").
		if m := badDuration.FindString(body); m != "" {
			warnings = append(warnings, Warning{
				Line:    lineNo,
				Message: fmt.Sprintf("unrecognised duration %q — use ICU format (e.g. %s)", m, suggestDuration(m)),
			})
		}

		// Check 2: step has an intensity/pace target but no valid
		// duration or distance — ICU will not project load for it.
		if hasTarget(body) && !hasAmount(body) {
			warnings = append(warnings, Warning{
				Line:    lineNo,
				Message: "step has a pace/zone target but no duration or distance — ICU cannot project load without one (e.g. add \"3m\" before the pace)",
			})
		}
	}
	return warnings
}

// ── regexps ──────────────────────────────────────────────────────────────────

// repeatHeader matches stand-alone repeat-count lines like "4x" or "12x".
var repeatHeader = regexp.MustCompile(`^\d+x\s*$`)

// badDuration matches duration tokens written in formats ICU does not
// accept, e.g. "10min", "30sec", "2hour".
var badDuration = regexp.MustCompile(`(?i)\b\d+(min|mins|minute|minutes|hour|hours|sec|secs|second|seconds)\b`)

// validAmount matches a standalone ICU-accepted duration or distance token.
// Duration: 1h, 30m, 45s, 1h30m, 1h30m45s, 30m45s, 1h45s
// Distance: 1km, 0.3km, 400mtr, 100y
var validAmount = regexp.MustCompile(
	`(?i)\b(` +
		`\d+h\d+m\d+s` + // 1h30m45s
		`|\d+h\d+m` + // 1h30m
		`|\d+h\d+s` + // 1h45s
		`|\d+m\d+s` + // 30m45s
		`|\d+h` + // 1h
		`|\d+m` + // 30m  (NOT followed by 't' to avoid matching 'mtr')
		`|\d+s` + // 45s
		`|\d+\.\d+km` + // 0.3km
		`|\d+km` + // 5km
		`|\d+mtr` + // 400mtr
		`|\d+y` + // 100y
		`)\b`,
)

// zoneTarget matches ICU pace-zone targets like "Z2", "Z1-Z2", "Z2 Pace".
var zoneTarget = regexp.MustCompile(`(?i)\bZ\d+(-Z\d+)?(\s+Pace)?\b`)

// paceTarget matches explicit pace targets like "4:35 Pace" or
// "4:55-4:35 Pace".
var paceTarget = regexp.MustCompile(`(?i)\b\d+:\d+(-\d+:\d+)?\s+Pace\b`)

// intensityKV matches "intensity=<word>" (e.g. intensity=recovery).
var intensityKV = regexp.MustCompile(`(?i)\bintensity=\w+`)

// ── helpers ───────────────────────────────────────────────────────────────────

// hasTarget reports whether body contains an intensity/pace target
// that ICU needs a duration/distance to pair with.
func hasTarget(body string) bool {
	// intensity=recovery steps carry their own implicit zero-load intent;
	// warn only if they also happen to have a bad duration — handled by
	// the badDuration check above.
	if intensityKV.MatchString(body) {
		return false
	}
	return zoneTarget.MatchString(body) || paceTarget.MatchString(body)
}

// hasAmount reports whether body contains a valid ICU duration or
// distance token.
func hasAmount(body string) bool {
	// The validAmount regexp can accidentally match the minute portion of
	// a pace like "4:35" (the "35" followed by nothing — but "s" in "Pace"
	// could confuse things). To be safe, we strip pace patterns first.
	stripped := paceTarget.ReplaceAllString(body, "")
	stripped = zoneTarget.ReplaceAllString(stripped, "")
	return validAmount.MatchString(stripped)
}

// suggestDuration returns a corrected ICU duration token for a
// human-friendly "wrong format" token.
func suggestDuration(bad string) string {
	lower := strings.ToLower(bad)
	// Extract the numeric prefix.
	n := ""
	for _, c := range lower {
		if c >= '0' && c <= '9' {
			n += string(c)
		} else {
			break
		}
	}
	switch {
	case strings.Contains(lower, "min"):
		return n + "m"
	case strings.Contains(lower, "hour"):
		return n + "h"
	case strings.Contains(lower, "sec"):
		return n + "s"
	default:
		return n + "m"
	}
}
