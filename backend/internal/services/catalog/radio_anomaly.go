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

	// volumeAnomalyStreakRuns — how many CONSECUTIVE below-threshold station sweeps are
	// required before flagging (PSY-1555). A single low sweep is normal: sweeps run on a
	// 6-hourly cadence and most new plays now arrive via show-scoped slot fetches
	// (PSY-1333), so any individual sweep can legitimately come back near-empty.
	//
	// Requiring a streak also breaks a latch the per-run rule created. Flagging
	// downgrades a run to partial, and the baseline is success-only — so under the old
	// rule the first flag excluded that run from the baseline, the mean froze at the
	// pre-shift volume, and every subsequent sweep was judged against a number the
	// station would never reach again. Measured on production 2026-07-26: 73% of all
	// station sweeps flagged (100% for three stations) while those same stations
	// imported 17,000+ plays; WFMU's last success-status sweep was 2026-07-16, nine days
	// of runs judged against a frozen mean of 203.3. Letting isolated dips pass keeps
	// them in the baseline, so a genuine downward shift in normal is learnable again.
	//
	// Tuned empirically by replaying 10 days / 387 real production sweeps (PSY-1555).
	// Flags per station at each streak length — NTS was the only genuinely stalled
	// station; every other station imported thousands of plays in the same window:
	//
	//   streak |  GtD | KEXP |  NTS | R'n'S | Sheena | WFMU
	//        1 |   30 |   22 |   33 |    39 |     38 |    9   <- the old per-run rule
	//        3 |    6 |   11 |   20 |     8 |     16 |    0
	//        6 |    0 |    3 |   12 |     0 |      8 |    0   <- chosen
	//
	// Sweeps land ~6.5x/day/station, so 6 consecutive empties is roughly a full day of
	// silence before we call it — late enough to be trustworthy, early enough to matter
	// for an observational guard that feeds health cards rather than paging anyone.
	//
	// Known residual: Sheena's Jungle Room still flags (8) while healthy. It is a bursty
	// sub-stream whose sweeps are legitimately empty most of the time, so a MEAN-based
	// threshold is the wrong statistic for it — no streak length fixes that. Tracked
	// separately; a "time since last successful import" signal suits bursty stations
	// better than per-sweep volume.
	volumeAnomalyStreakRuns = 6
)

// trailingMean returns the baseline mean and whether it is trustworthy enough to judge
// against. Both guards are false-positive protection: too few samples means no baseline,
// and a mean below MinMean means the station has no meaningful "normal" to fall below.
func trailingMean(baseline []int) (float64, bool) {
	if len(baseline) < volumeAnomalyMinRuns {
		return 0, false
	}
	sum := 0
	for _, p := range baseline {
		sum += p
	}
	mean := float64(sum) / float64(len(baseline))
	if mean < volumeAnomalyMinMean {
		return 0, false
	}
	return mean, true
}

// belowThreshold reports whether a play count sits under the anomaly fraction of the
// trailing mean.
func belowThreshold(plays int, mean float64) bool {
	return float64(plays) < volumeAnomalyFraction*mean
}

// volumeAnomaly reports whether the current run, TOGETHER WITH the preceding sweeps,
// forms a below-threshold streak. `priorPlays` is the play count of the most recent
// prior station sweeps, newest first, regardless of their status — status is deliberately
// ignored here because a flagged run is written partial, and reading only successes is
// what let the old per-run rule latch (see volumeAnomalyStreakRuns).
func volumeAnomaly(currentPlays int, baseline []int, priorPlays []int) (bool, string) {
	mean, ok := trailingMean(baseline)
	if !ok {
		return false, ""
	}
	if !belowThreshold(currentPlays, mean) {
		return false, ""
	}
	// Not enough history to establish a streak — stay quiet rather than flag on a
	// single observation.
	if len(priorPlays) < volumeAnomalyStreakRuns-1 {
		return false, ""
	}
	for _, p := range priorPlays[:volumeAnomalyStreakRuns-1] {
		if !belowThreshold(p, mean) {
			return false, ""
		}
	}
	return true, fmt.Sprintf(
		"plays_imported=%d is below %.0f%% of the trailing mean %.1f for %d consecutive station sweeps (baseline: last %d runs) — possible silent fetch failure",
		currentPlays, volumeAnomalyFraction*100, mean, volumeAnomalyStreakRuns, len(baseline),
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
	// show_id IS NULL: show-SCOPED fetch runs (PSY-1333 slot fetch) import single-show
	// volumes; letting them into the baseline would drag the station-sweep mean toward
	// zero and blind the guard.
	err := s.db.Model(&catalogm.RadioSyncRun{}).
		Where("show_id IS NULL").
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

	// The streak window reads the immediately preceding sweeps regardless of status.
	// Unlike the baseline it MUST include partials: a flagged run is written partial, so
	// a success-only read could never observe two low runs in a row and the streak would
	// never form.
	var priorPlays []int
	if err := s.db.Model(&catalogm.RadioSyncRun{}).
		Where("show_id IS NULL").
		Where("station_id = ? AND run_type = ? AND started_at >= ? AND id <> ? AND finished_at IS NOT NULL",
			stationID,
			catalogm.RadioSyncRunTypeFetch,
			time.Now().Add(-volumeAnomalyLookback),
			currentRunID,
		).
		Order("started_at DESC, id DESC").
		Limit(volumeAnomalyStreakRuns-1).
		Pluck("plays_imported", &priorPlays).Error; err != nil {
		slog.Warn("radio: volume-anomaly streak query failed; skipping guard",
			"station_id", stationID, "error", err)
		return false, ""
	}

	return volumeAnomaly(currentPlays, baseline, priorPlays)
}
