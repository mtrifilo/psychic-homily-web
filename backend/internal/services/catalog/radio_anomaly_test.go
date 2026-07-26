package catalog

import "testing"

// lowStreak builds the prior-sweep window a full streak requires, so these fixtures
// stay correct if volumeAnomalyStreakRuns is retuned.
func lowStreak(v int) []int {
	out := make([]int, volumeAnomalyStreakRuns-1)
	for i := range out {
		out[i] = v
	}
	return out
}

// volumeAnomaly is the pure rule; these tests pin the threshold (< 30% of the trailing
// mean) and the two false-positive guards (min runs, min mean) without a DB. PSY-1156.
func TestVolumeAnomaly(t *testing.T) {
	tests := []struct {
		name         string
		currentPlays int
		baseline     []int
		priorPlays   []int
		wantAnomaly  bool
	}{
		{
			name:         "canonical PSY-1126: 0 plays vs ~50 trailing mean",
			currentPlays: 0,
			baseline:     []int{48, 50, 52, 45, 51, 49},
			priorPlays:   lowStreak(0), // preceding sweeps also empty → sustained collapse
			wantAnomaly:  true,
		},
		{
			name:         "normal volume is not flagged",
			currentPlays: 50,
			baseline:     []int{48, 50, 52, 45, 51},
			wantAnomaly:  false,
		},
		{
			name:         "just below 30% of mean is flagged",
			currentPlays: 14, // mean 50 → threshold 15.0; 14 < 15
			baseline:     []int{50, 50, 50, 50, 50},
			priorPlays:   lowStreak(10),
			wantAnomaly:  true,
		},
		{
			name:         "exactly at 30% of mean is not flagged (strict <)",
			currentPlays: 15, // mean 50 → threshold 15.0; 15 is not < 15
			baseline:     []int{50, 50, 50, 50, 50},
			wantAnomaly:  false,
		},
		{
			name:         "below min-runs baseline is never flagged (no trustworthy baseline)",
			currentPlays: 0,
			baseline:     []int{50, 50, 50, 50}, // only 4 < volumeAnomalyMinRuns
			wantAnomaly:  false,
		},
		{
			name:         "empty baseline is never flagged",
			currentPlays: 0,
			baseline:     nil,
			wantAnomaly:  false,
		},
		{
			name:         "low-traffic station (mean < min) is never flagged",
			currentPlays: 0,
			baseline:     []int{1, 2, 1, 2, 1, 0}, // mean ~1.17 < volumeAnomalyMinMean
			wantAnomaly:  false,
		},
		{
			name:         "moderate dip above threshold is not flagged",
			currentPlays: 30, // mean 50 → threshold 15; 30 >= 15
			baseline:     []int{48, 50, 52, 45, 51, 54},
			wantAnomaly:  false,
		},
		{
			// PSY-1555: the case that flooded production. A single quiet sweep is
			// normal now that slot fetches carry most volume — it must NOT flag,
			// which is also what keeps it in the success baseline so the mean can
			// still adapt to a genuine downward shift.
			name:         "isolated low sweep after healthy ones is not flagged",
			currentPlays: 0,
			baseline:     []int{200, 210, 190, 205, 195, 203},
			priorPlays:   lowStreak(200), // preceding sweeps were healthy
			wantAnomaly:  false,
		},
		{
			name:         "streak broken by one healthy sweep is not flagged",
			currentPlays: 0,
			baseline:     []int{200, 210, 190, 205, 195},
			priorPlays:   append([]int{0}, lowStreak(200)...), // one healthy sweep breaks it
			wantAnomaly:  false,
		},
		{
			// NTS on 2026-07-26: sustained zero across consecutive sweeps must still
			// scream. This is the signal the false positives were burying.
			name:         "sustained collapse across the full streak is flagged",
			currentPlays: 0,
			baseline:     []int{180, 190, 185, 175, 195, 184},
			priorPlays:   lowStreak(0),
			wantAnomaly:  true,
		},
		{
			name:         "insufficient prior history stays quiet rather than flagging",
			currentPlays: 0,
			baseline:     []int{50, 50, 50, 50, 50},
			priorPlays:   []int{0}, // one prior run only; a full streak needs more
			wantAnomaly:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, detail := volumeAnomaly(tt.currentPlays, tt.baseline, tt.priorPlays)
			if got != tt.wantAnomaly {
				t.Fatalf("volumeAnomaly(%d, %v, %v) = %v, want %v", tt.currentPlays, tt.baseline, tt.priorPlays, got, tt.wantAnomaly)
			}
			if got && detail == "" {
				t.Fatalf("an anomaly must carry a non-empty detail for the error row")
			}
			if !got && detail != "" {
				t.Fatalf("a non-anomaly must carry no detail, got %q", detail)
			}
		})
	}
}
