package catalog

import "testing"

// volumeAnomaly is the pure rule; these tests pin the threshold (< 30% of the trailing
// mean) and the two false-positive guards (min runs, min mean) without a DB. PSY-1156.
func TestVolumeAnomaly(t *testing.T) {
	tests := []struct {
		name         string
		currentPlays int
		baseline     []int
		wantAnomaly  bool
	}{
		{
			name:         "canonical PSY-1126: 0 plays vs ~50 trailing mean",
			currentPlays: 0,
			baseline:     []int{48, 50, 52, 45, 51, 49},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, detail := volumeAnomaly(tt.currentPlays, tt.baseline)
			if got != tt.wantAnomaly {
				t.Fatalf("volumeAnomaly(%d, %v) = %v, want %v", tt.currentPlays, tt.baseline, got, tt.wantAnomaly)
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
