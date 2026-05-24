package render

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jogvan-k/fit-agent/internal/fitparse"
	"github.com/jogvan-k/fit-agent/internal/icu"
)

func TestAutoSplitLap_ExactDivisible(t *testing.T) {
	l := fitparse.Lap{
		Index:           1,
		Intensity:       "active",
		Distance:        10000.0,
		Duration:        55 * time.Minute,
		AvgHR:           132,
		MaxHR:           142,
		AvgCadence:      80,
		AvgPaceSecPerKm: 330,
	}
	segs := autoSplitLap(l, 1000, nil, 0, 0) // nil records → fallback
	if len(segs) != 10 {
		t.Fatalf("expected 10 segments, got %d", len(segs))
	}
	for i, s := range segs {
		if s.segment != i+1 {
			t.Errorf("seg %d: segment index = %d", i, s.segment)
		}
		if s.distanceM != 1000.0 {
			t.Errorf("seg %d: distance = %f, want 1000", i, s.distanceM)
		}
		if s.avgHR != 132 {
			t.Errorf("seg %d: avg_hr = %d", i, s.avgHR)
		}
		if s.avgCadence != 80 {
			t.Errorf("seg %d: avg_cadence = %d", i, s.avgCadence)
		}
	}
}

func TestAutoSplitLap_RemainderTail(t *testing.T) {
	l := fitparse.Lap{
		Index:     1,
		Intensity: "active",
		Distance:  10500.0,
		Duration:  58 * time.Minute,
	}
	segs := autoSplitLap(l, 1000, nil, 0, 0)
	if len(segs) != 11 {
		t.Fatalf("expected 11 segments (10 + remainder), got %d", len(segs))
	}
	last := segs[10]
	if last.distanceM != 500.0 {
		t.Errorf("last segment distance = %f, want 500", last.distanceM)
	}
}

func TestAutoSplitLap_SubThreshold(t *testing.T) {
	l := fitparse.Lap{
		Index:     1,
		Intensity: "active",
		Distance:  800.0,
		Duration:  3 * time.Minute,
	}
	segs := autoSplitLap(l, 1000, nil, 0, 0)
	if len(segs) != 0 {
		t.Errorf("expected no segments for sub-threshold lap, got %d", len(segs))
	}
}

func TestAutoSplitLap_NonActiveNotSplit(t *testing.T) {
	l := fitparse.Lap{
		Index:     1,
		Intensity: "recovery",
		Distance:  5000.0,
		Duration:  30 * time.Minute,
	}
	segs := autoSplitLap(l, 1000, nil, 0, 0)
	if len(segs) != 0 {
		t.Errorf("expected no segments for non-active lap, got %d", len(segs))
	}
}

func makeLongRunDay(loc *time.Location, autoSplitM int) ActivityDay {
	return ActivityDay{
		Date:               time.Date(2026, 5, 20, 0, 0, 0, 0, loc),
		GeneratedAt:        time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC),
		Location:           loc,
		AutoSplitDistanceM: autoSplitM,
		Activities: []ActivityInput{
			{
				Summary: icu.ActivitySummary{
					ID:             "i149909045",
					Name:           "Barcelona - Easy Run 55min",
					Type:           "Run",
					StartDateLocal: "2026-05-20T06:58:33",
					ElapsedTime:    3492,
					MovingTime:     3480,
					Distance:       10149.0,
					AverageHR:      132,
					MaxHR:          142,
					AverageSpeed:   2.915,
				},
				FIT: &fitparse.ParsedActivity{
					Laps: []fitparse.Lap{
						{
							Index:           1,
							Intensity:       "active",
							Distance:        9632.0,
							Duration:        3301 * time.Second,
							AvgHR:           131,
							MaxHR:           142,
							AvgCadence:      80,
							AvgSpeed:        2.918,
							AvgPaceSecPerKm: 343,
						},
						{
							Index:           2,
							Intensity:       "active",
							Distance:        516.1,
							Duration:        192 * time.Second,
							AvgHR:           134,
							MaxHR:           141,
							AvgCadence:      79,
							AvgSpeed:        2.852,
							AvgPaceSecPerKm: 351,
						},
					},
				},
			},
		},
	}
}

func TestActivityDayYAML_AutoSplits_Appear(t *testing.T) {
	loc := mustLoadLocation(t, "Europe/Madrid")
	got, err := ActivityDayYAML(makeLongRunDay(loc, 1000))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if !strings.Contains(out, "auto_splits:") {
		t.Error("expected auto_splits block in output")
	}
	if !strings.Contains(out, "source: auto_split") {
		t.Error("expected source: auto_split in output")
	}
	// First lap (9.6 km) should produce 9 segments of 1 km + 1 remainder
	if !strings.Contains(out, "segment: 9") {
		t.Error("expected at least 9 segments for 9.6 km lap")
	}
	// Second lap (516 m) is below threshold — no auto_splits for it
	// (only one auto_splits block expected, for the first lap)
	count := strings.Count(out, "auto_splits:")
	if count != 1 {
		t.Errorf("expected 1 auto_splits block, got %d", count)
	}
}

func TestActivityDayYAML_AutoSplits_Disabled(t *testing.T) {
	loc := mustLoadLocation(t, "Europe/Madrid")
	got, err := ActivityDayYAML(makeLongRunDay(loc, 0))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "auto_splits:") {
		t.Error("expected no auto_splits block when disabled (autoSplitM=0)")
	}
}

// TestAutoSplitLap_ElevationScaleInvariant verifies that the sum of per-segment
// elevation_gain_m equals the lap's ElevationGain (and same for loss).
func TestAutoSplitLap_ElevationScaleInvariant(t *testing.T) {
	lapStart := time.Date(2024, 6, 1, 8, 0, 0, 0, time.UTC)
	// Build records with altitude variation over a 3 km lap.
	alts := []float64{100, 110, 108, 120, 115, 118, 100}
	dists := []float64{0, 500, 1000, 1500, 2000, 2500, 3000}
	var recs []fitparse.Record
	for i, alt := range alts {
		recs = append(recs, fitparse.Record{
			Timestamp:     lapStart.Add(time.Duration(i*60) * time.Second),
			Distance:      dists[i],
			Altitude:      alt,
			AltitudeValid: true,
		})
	}
	l := fitparse.Lap{
		Index:         1,
		Intensity:     "active",
		Distance:      3000.0,
		Duration:      420 * time.Second,
		StartLocal:    lapStart,
		ElevationGain: 50.0,
		ElevationLoss: 30.0,
	}
	segs := autoSplitLap(l, 1000, recs, 0, 0)
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segs))
	}
	var totalGain, totalLoss float64
	for _, s := range segs {
		totalGain += s.elevationGainM
		totalLoss += s.elevationLossM
	}
	const epsilon = 1e-9
	if math.Abs(totalGain-l.ElevationGain) > epsilon {
		t.Errorf("sum of segment gains = %.6f, want %.6f", totalGain, l.ElevationGain)
	}
	if math.Abs(totalLoss-l.ElevationLoss) > epsilon {
		t.Errorf("sum of segment losses = %.6f, want %.6f", totalLoss, l.ElevationLoss)
	}
}
