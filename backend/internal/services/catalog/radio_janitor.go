package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Janitor reconcile operations (PSY-1155). These are the DB-side steps the nightly
// janitor cycle (RadioFetchService.runJanitorCycle) orchestrates. They are pure
// service methods so they can be unit-tested against a real DB without the ticker.

// ReconcileShowLifecycle reconciles every show's lifecycle_state between 'active' and
// 'dormant' from a single signal: whether the show has an episode that aired within
// idleThreshold of now (PSY-1155).
//
//   - active  = aired recently (≥1 episode with air_date within the window).
//   - dormant = inactive/historical — no recent episode (incl. shows with zero
//     episodes, which clears the old discovered-empty-show pollution). Still fully
//     browsable; the distinction is purely "show active shows first, historical ones
//     on a dig-deeper affordance."
//
// The transition is reversible and non-destructive — a returning DJ's next episode
// flips the show back to 'active' on the next nightly run here (and, once PSY-1153
// create-on-first-episode lands, in real time on ingest). is_active and the fetch set
// are intentionally NOT touched, so a dormant show keeps being polled and can
// reactivate naturally; until then the ~24h janitor cadence is the only promotion path.
//
// 'retired' is left untouched: the janitor never auto-retires (the provider can't
// distinguish a leave of absence from an ending — owner decision, 2026-06-21).
// 'retired' is set manually via UpdateShow (PSY-1172) and is never auto-set or cleared
// here — the active↔dormant queries below scope WHERE lifecycle_state = 'active'/'dormant',
// so a retired show is excluded by construction. (reactivateShowIfDormant on import
// likewise only touches 'dormant', so a new episode never resurrects a retired show.) The
// 'active schedule changed → dormant' signal is likewise deferred to PSY-1159 (it
// needs the scraped wfmu.org/table schedule).
//
// Returns the number of shows promoted (dormant→active) and demoted (active→dormant).
func (s *RadioService) ReconcileShowLifecycle(idleThreshold time.Duration, now time.Time) (promoted, demoted int, err error) {
	if s.db == nil {
		return 0, 0, fmt.Errorf("database not initialized")
	}

	cutoff := now.Add(-idleThreshold).Format("2006-01-02")

	// Demote: an active show with no episode aired since the cutoff → dormant. The
	// NOT IN subquery is the set of shows that DO have a recent episode; a show with
	// zero episodes is trivially absent from it and is demoted.
	demote := s.db.Model(&catalogm.RadioShow{}).
		Where("lifecycle_state = ?", catalogm.RadioLifecycleActive).
		Where("id NOT IN (?)", s.db.Model(&catalogm.RadioEpisode{}).
			Select("show_id").Where("air_date >= ?", cutoff)).
		Updates(map[string]any{
			"lifecycle_state": catalogm.RadioLifecycleDormant,
			"updated_at":      now,
		})
	if demote.Error != nil {
		return 0, 0, fmt.Errorf("demoting idle shows to dormant: %w", demote.Error)
	}
	demoted = int(demote.RowsAffected)

	// Promote: a dormant show that has aired since the cutoff → active. (A just-demoted
	// show can't match — it was demoted precisely because it has no recent episode.)
	promote := s.db.Model(&catalogm.RadioShow{}).
		Where("lifecycle_state = ?", catalogm.RadioLifecycleDormant).
		Where("id IN (?)", s.db.Model(&catalogm.RadioEpisode{}).
			Select("show_id").Where("air_date >= ?", cutoff)).
		Updates(map[string]any{
			"lifecycle_state": catalogm.RadioLifecycleActive,
			"updated_at":      now,
		})
	if promote.Error != nil {
		return 0, demoted, fmt.Errorf("promoting returning shows to active: %w", promote.Error)
	}
	promoted = int(promote.RowsAffected)

	return promoted, demoted, nil
}

// ReconcilePlayCounts corrects each episode's denormalized play_count against the
// actual radio_plays row count (PSY-1155; §9 decision 4: "maintained on write +
// nightly reconcile"). radio_plays is append-only (importPlays does ON CONFLICT DO
// NOTHING, never deletes), so the true count only grows; play_count is also kept
// monotonic on write (recordPlaylistOutcome). This catches any residual drift —
// chiefly historical rows written before the monotonic fix.
//
// One set-based statement: the LEFT JOIN yields 0 for episodes with no plays, and the
// `<>` guard writes only the drifted rows, so a steady-state run touches nothing.
// Returns the number of episodes corrected.
//
// Scale note (conscious decision): the aggregate READ is unbounded — it scans all of
// radio_plays nightly regardless of recency — because the residual drift it targets is
// chiefly historical (rows written before play_count became monotonic-on-write), which
// a recency bound would skip. This is cheap now (sub-second on an indexed GROUP BY at
// the current tens-of-shows scale) and the `<>` guard keeps the WRITE empty in steady
// state. If radio_plays ever reaches millions, bound the read to recently-touched
// episodes (e.g. updated_at >= cutoff) and run a one-off full reconcile for history —
// tracked as a follow-up.
func (s *RadioService) ReconcilePlayCounts() (corrected int, err error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	res := s.db.Exec(`
		UPDATE radio_episodes e
		SET play_count = c.cnt, updated_at = NOW()
		FROM (
			SELECT ep.id, COUNT(p.id) AS cnt
			FROM radio_episodes ep
			LEFT JOIN radio_plays p ON p.episode_id = ep.id
			GROUP BY ep.id
		) c
		WHERE e.id = c.id AND e.play_count <> c.cnt
	`)
	if res.Error != nil {
		return 0, fmt.Errorf("reconciling episode play_count: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}
