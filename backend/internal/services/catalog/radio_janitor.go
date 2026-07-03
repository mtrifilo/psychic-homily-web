package catalog

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Janitor reconcile operations (PSY-1155). These are the DB-side steps the nightly
// janitor cycle (RadioFetchService.runJanitorCycle) orchestrates. They are pure
// service methods so they can be unit-tested against a real DB without the ticker.

// ReconcileShowLifecycle reconciles every show's lifecycle_state between 'active' and
// 'dormant'. The signal depends on the station class (PSY-1155, PSY-1348):
//
//   - Schedule-authoritative stations (any station with ≥1 scrape-maintained schedule
//     — the WFMU family, whose grids are scraped and churn-maintained by
//     ApplyWFMUSchedule / clearAbsentWFMUSchedules): active ⟺ on the current grid.
//     Episode recency is ignored: a fill-in that aired yesterday is still dormant
//     (owner directive, 2026-07-03 — the station page must count only the real
//     lineup), and a scheduled show is active even before its first tracked episode.
//     schedule_locked shows are exempt from this rule entirely (see the grid queries
//     below): their lifecycle is whatever the admin set.
//   - All other stations (KEXP, NTS — no schedule source): active = aired recently
//     (≥1 episode with air_date within idleThreshold); dormant otherwise (incl.
//     zero-episode shows, which clears the old discovered-empty-show pollution).
//
// Dormant is non-destructive either way — still fully browsable; the distinction is
// purely "show active shows first, historical ones on a dig-deeper affordance." The
// transition is reversible: a show scraped back onto the grid promotes at the janitor
// run FOLLOWING that scrape (the scrape itself never writes lifecycle_state); on
// recency stations a new episode promotes via the janitor or reactivateShowIfDormant
// on ingest. Known transient: a show newly ADDED to the grid mid-season reads dormant
// until the next weekly scrape stamps its schedule (disclosed on PSY-1348). is_active
// and the fetch set are intentionally NOT touched, so a dormant show keeps being
// polled.
//
// 'retired' is left untouched: the janitor never auto-retires (the provider can't
// distinguish a leave of absence from an ending — owner decision, 2026-06-21).
// 'retired' is set manually via UpdateShow (PSY-1172) and is never auto-set or cleared
// here — every query below scopes WHERE lifecycle_state = 'active'/'dormant', so a
// retired show is excluded by construction. (reactivateShowIfDormant on import
// likewise only touches 'dormant', so a new episode never resurrects a retired show.)
//
// Returns the number of shows promoted (dormant→active) and demoted (active→dormant).
func (s *RadioService) ReconcileShowLifecycle(idleThreshold time.Duration, now time.Time) (promoted, demoted int, err error) {
	if s.db == nil {
		return 0, 0, fmt.Errorf("database not initialized")
	}

	cutoff := now.Add(-idleThreshold).Format("2006-01-02")

	// Observability: a station silently entering/leaving grid semantics is the one
	// residual failure mode of the data-inferred authority signal — log the computed
	// set every run so a flip shows up next to the demote/promote counts.
	var authStationIDs []uint
	if err := s.scrapeMaintainedScheduleRows().
		Distinct().Pluck("radio_shows.station_id", &authStationIDs).Error; err == nil {
		slog.Info("radio janitor: schedule-authoritative stations", "station_ids", authStationIDs)
	}

	// Grid rule — schedule-authoritative stations only. schedule_locked shows are
	// exempt in BOTH directions: the lock means an admin curates that show by hand
	// (the scrape must not touch it), so the janitor must not auto-flip its
	// lifecycle either — the admin's setting stands until the admin changes it.
	// Demote: on an authoritative station but not on the current grid → dormant.
	// Code-less rows (NULL or empty external_id) are exempt for the same reason
	// clearAbsentWFMUSchedules exempts them: the scrape matches by code, so it can
	// never stamp them — the grid is not authoritative FOR them (stage has zero such
	// rows; belt-and-braces). The reactivation guard carries the same exemption.
	gridDemote := s.db.Model(&catalogm.RadioShow{}).
		Where("lifecycle_state = ?", catalogm.RadioLifecycleActive).
		Where("station_id IN (?)", s.scheduleAuthoritativeStations()).
		Where("schedule IS NULL AND NOT schedule_locked AND external_id IS NOT NULL AND external_id <> ''").
		Updates(map[string]any{
			"lifecycle_state": catalogm.RadioLifecycleDormant,
			"updated_at":      now,
		})
	if gridDemote.Error != nil {
		return 0, 0, fmt.Errorf("demoting off-grid shows to dormant: %w", gridDemote.Error)
	}
	demoted = int(gridDemote.RowsAffected)

	// Promote: on the current grid (scrape-maintained, i.e. unlocked) → active,
	// regardless of recency.
	gridPromote := s.db.Model(&catalogm.RadioShow{}).
		Where("lifecycle_state = ?", catalogm.RadioLifecycleDormant).
		Where("station_id IN (?)", s.scheduleAuthoritativeStations()).
		Where("schedule IS NOT NULL AND NOT schedule_locked").
		Updates(map[string]any{
			"lifecycle_state": catalogm.RadioLifecycleActive,
			"updated_at":      now,
		})
	if gridPromote.Error != nil {
		return 0, demoted, fmt.Errorf("promoting on-grid shows to active: %w", gridPromote.Error)
	}
	promoted = int(gridPromote.RowsAffected)

	// Recency rule — everything else. Demote: an active show with no episode aired
	// since the cutoff → dormant. The NOT IN subquery is the set of shows that DO
	// have a recent episode; a show with zero episodes is trivially absent from it
	// and is demoted.
	demote := s.db.Model(&catalogm.RadioShow{}).
		Where("lifecycle_state = ?", catalogm.RadioLifecycleActive).
		Where("station_id NOT IN (?)", s.scheduleAuthoritativeStations()).
		Where("id NOT IN (?)", s.db.Model(&catalogm.RadioEpisode{}).
			Select("show_id").Where("air_date >= ?", cutoff)).
		Updates(map[string]any{
			"lifecycle_state": catalogm.RadioLifecycleDormant,
			"updated_at":      now,
		})
	if demote.Error != nil {
		return promoted, demoted, fmt.Errorf("demoting idle shows to dormant: %w", demote.Error)
	}
	demoted += int(demote.RowsAffected)

	// Promote: a dormant show that has aired since the cutoff → active. (A just-demoted
	// show can't match — it was demoted precisely because it has no recent episode.)
	promote := s.db.Model(&catalogm.RadioShow{}).
		Where("lifecycle_state = ?", catalogm.RadioLifecycleDormant).
		Where("station_id NOT IN (?)", s.scheduleAuthoritativeStations()).
		Where("id IN (?)", s.db.Model(&catalogm.RadioEpisode{}).
			Select("show_id").Where("air_date >= ?", cutoff)).
		Updates(map[string]any{
			"lifecycle_state": catalogm.RadioLifecycleActive,
			"updated_at":      now,
		})
	if promote.Error != nil {
		return promoted, demoted, fmt.Errorf("promoting returning shows to active: %w", promote.Error)
	}
	promoted += int(promote.RowsAffected)

	return promoted, demoted, nil
}

// scheduleScrapedPlaylistSources lists the playlist sources whose providers maintain
// radio_shows.schedule from a scraped grid (ApplyWFMUSchedule /
// ApplyWFMUSubstreamSchedule). Grid lifecycle semantics only make sense where a scrape
// maintains the grid, so authoritativeness is HARD-GATED on this list: no admin write
// on a KEXP/NTS show — schedule, lock bit, anything — can flip those stations to grid
// semantics (adversarial-review HIGH ×2: the auto-lock inference alone was bypassable
// via UpdateShow's documented explicit schedule_locked=false). Add a source here when
// its provider gains a schedule scraper.
var scheduleScrapedPlaylistSources = []string{catalogm.PlaylistSourceWFMU}

// scheduleAuthoritativeStations returns a fresh subquery of station IDs whose show
// lifecycle is grid-driven rather than recency-driven (PSY-1348): a station whose
// playlist source has a schedule scraper AND that currently has at least one
// scrape-maintained schedule ("scrape-maintained" = schedule present AND not
// schedule_locked; UpdateShow auto-locks admin structured-schedule edits, PSY-1186).
// The ≥1-unlocked-schedule leg is the degradation valve: if a station's scrape source
// dies and its schedules get cleared, the station falls back to recency semantics
// instead of grid-demoting its whole roster; the scrape's suspect-parse floor
// (clearAbsentWFMUSchedules) protects a real lineup from a partial wipe. Residual
// known cliff (accepted, disclosed on PSY-1348): if admins ever locked EVERY schedule
// on a WFMU station it would exit grid semantics — implausible at the current ~65
// scraped rows; the janitor logs the computed set each run so a flip is observable.
// Fresh per call: GORM subquery builders are not safely reusable across statements.
func (s *RadioService) scheduleAuthoritativeStations() *gorm.DB {
	return s.scrapeMaintainedScheduleRows().Select("DISTINCT radio_shows.station_id")
}

// scrapeMaintainedScheduleRows is THE single definition of the authoritativeness
// predicate (shows whose schedule is scrape-maintained, on a scrape-capable station).
// Every consumer — the janitor subquery, the create-path point lookup, the ingest
// reactivation guard — derives from this builder so the paths cannot drift apart.
func (s *RadioService) scrapeMaintainedScheduleRows() *gorm.DB {
	return s.db.Model(&catalogm.RadioShow{}).
		Joins("JOIN radio_stations ON radio_stations.id = radio_shows.station_id").
		Where("radio_stations.playlist_source IN ?", scheduleScrapedPlaylistSources).
		Where("radio_shows.schedule IS NOT NULL AND NOT radio_shows.schedule_locked")
}

// stationIsScheduleAuthoritative reports whether one station is in the
// scheduleAuthoritativeStations set — same predicate, point lookup (used by the
// create-on-first path to pick a new row's initial lifecycle_state).
func (s *RadioService) stationIsScheduleAuthoritative(stationID uint) (bool, error) {
	var count int64
	err := s.scrapeMaintainedScheduleRows().
		Where("radio_shows.station_id = ?", stationID).
		Count(&count).Error
	return count > 0, err
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
