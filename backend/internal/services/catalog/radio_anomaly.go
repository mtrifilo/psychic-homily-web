package catalog

import (
	"fmt"
	"log/slog"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1156 volume-anomaly guard. A fetch that imports far fewer plays than the
// station's recent norm is recorded as partial + empty_unexpected instead of passing
// as a silent success — the canonical failure (PSY-1126): KEXP returned 0 plays vs a
// ~50 trailing average and nothing flagged it. The guard is observational: it never
// drops data, it does not trip the breaker or page Sentry (status=partial resets the
// failure counter; empty_unexpected is not in escalationError's escalate set). The
// partial status + the error row are the signal that feeds the P5 health cards (red
// when chronically empty).
//
// Scope: FETCH runs only. The scheduled fetch cycle is the steady-state cadence where a
// station has a stable expected play volume; discover/backfill volumes are inherently
// variable (new-show counts, bounded history windows) so a trailing baseline is not
// meaningful for them.

// These four knobs are compile-time consts (like radioCircuitBreakerThreshold), not env:
// they are tuned rarely and deliberately (in code review), not per incident. Promote to
// env (cf. RADIO_AUTO_BACKFILL_DAYS) only if ops ever needs to retune them live.
const (
	// volumeAnomalyFraction — a run is anomalous when its plays_imported falls below
	// this fraction of the trailing mean (< 30% of normal). Conservative by design
	// ("start strict, refine" — PSY-1156): a true collapse (0 vs ~50) always trips it
	// while an ordinary dip does not.
	volumeAnomalyFraction = 0.3

	// volumeAnomalyMinRuns — the minimum number of prior baseline runs before the guard
	// activates. Below this there is no trustworthy baseline, so a new station is never
	// flagged while it accumulates history.
	volumeAnomalyMinRuns = 5

	// volumeAnomalyMinMean — the minimum trailing mean for the guard to apply. A
	// genuinely low-traffic station (mean below this) has no meaningful "normal" to fall
	// below, so it is never flagged. This is the primary false-positive guard.
	volumeAnomalyMinMean = 5.0

	// volumeAnomalyMaxSamples / volumeAnomalyLookback bound the baseline window to the
	// most recent N fetch runs within the trailing period.
	volumeAnomalyMaxSamples = 20
	volumeAnomalyLookback   = 30 * 24 * time.Hour
)

// volumeAnomaly is the pure decision (no DB, unit-tested directly): given the current
// run's plays and the trailing baseline (plays_imported of prior success/partial fetch
// runs), report whether the current run is a volume anomaly plus a human-readable detail
// for the radio_sync_run_errors row.
func volumeAnomaly(currentPlays int, baseline []int) (bool, string) {
	if len(baseline) < volumeAnomalyMinRuns {
		return false, ""
	}
	sum := 0
	for _, p := range baseline {
		sum += p
	}
	mean := float64(sum) / float64(len(baseline))
	if mean < volumeAnomalyMinMean {
		return false, ""
	}
	if float64(currentPlays) >= volumeAnomalyFraction*mean {
		return false, ""
	}
	return true, fmt.Sprintf(
		"plays_imported=%d is below %.0f%% of the trailing mean %.1f over the last %d fetch runs — possible silent fetch failure",
		currentPlays, volumeAnomalyFraction*100, mean, len(baseline),
	)
}

// detectVolumeAnomaly loads the station's trailing fetch baseline and applies
// volumeAnomaly. The current run (still status=running at the call site) is excluded by
// both the status filter and an explicit id guard. A query error degrades to "no
// anomaly" — the guard must never fail a run on its own infrastructure error — and is
// logged.
func (s *RadioService) detectVolumeAnomaly(stationID, currentRunID uint, currentPlays int) (bool, string) {
	var baseline []int
	// Baseline = SUCCESS fetch runs only. partial is deliberately EXCLUDED: a flagged
	// anomaly run is itself written partial with its 0/low play count, so counting
	// partials would let a sustained outage poison its own baseline — after ~MaxSamples
	// zero-runs the mean would fall below MinMean and the guard would self-silence
	// exactly when the outage is longest (the PSY-1126 chronic-failure shape). Success-
	// only keeps the baseline at the station's last known-good volume through a sustained
	// collapse, until those successes age out of the lookback window (by then it is
	// breaker/health-card territory, not a per-run anomaly). The current run is still
	// status=running here; the explicit id guard is belt-and-suspenders.
	err := s.db.Model(&catalogm.RadioSyncRun{}).
		Where("station_id = ? AND run_type = ? AND status = ? AND started_at >= ? AND id <> ?",
			stationID,
			catalogm.RadioSyncRunTypeFetch,
			catalogm.RadioSyncRunStatusSuccess,
			time.Now().Add(-volumeAnomalyLookback),
			currentRunID,
		).
		Order("started_at DESC, id DESC"). // id tie-break: deterministic LIMIT on equal started_at
		Limit(volumeAnomalyMaxSamples).
		Pluck("plays_imported", &baseline).Error
	if err != nil {
		slog.Warn("radio: volume-anomaly baseline query failed; skipping guard",
			"station_id", stationID, "error", err)
		return false, ""
	}
	return volumeAnomaly(currentPlays, baseline)
}
