package catalog

import (
	"fmt"
	"log/slog"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1333: schedule-aware slot fetch.
//
// Episode rows are created only by the incremental listing fetch, so with the
// default 6h cycle a show can air its ENTIRE slot — and sit hours past it —
// before the site shows any trace of it (the user-reported "JA In The AM is
// missing" shape: aired Fri 9–noon ET, imported 2:23pm ET). The slot fetch
// closes that gap for schedule-bearing shows: a fast ticker asks which shows
// had a scheduled slot START or END inside the window since its last tick and
// runs a single-show scoped fetch for exactly those — two extra scoped fetches
// per show per airing (start: the row appears while it airs, the post-air
// backfill sweep then grows the playlist; end: catches DJs who publish late).
// Cost honesty: a scoped fetch is a full incremental fetch for that show, not
// one request — the listing covers the show's fetchSince window (≥ the 45-day
// floor) and importing a listed-but-unpublished episode burns a playlist fetch
// attempt, exactly as when the 6h sweep imports a mid-air show; the delta is
// ~2 such fetches per airing. Scoped runs are BREAKER-NEUTRAL and excluded
// from failure streaks, the volume-anomaly guard/baseline, and the station
// health rates (see RunStationSync + computeStationRates). The boundary work
// list is schedule-driven, so it covers the WFMU family only; KEXP/NTS episode
// rows are created at airtime by the airing-feed ingestion (PSY-1509,
// radio_airing_ingest.go) and then ride the live-refresh work list below. The
// interval sweep remains the backstop for everything.
//
// PSY-1370 (live refresh): the boundary work list alone re-fetches a show only at
// its slot start/end — so a playlist fetched empty before airtime (WFMU pre-publishes
// the page hours early) stays at 0 tracks for the whole live window. Each tick the
// cycle therefore ALSO scoped-fetches every show with an episode airing right now
// whose playlist is still incomplete (ShowsWithLiveIncompleteEpisodes), so tracks
// accumulate during the show. Same neutral scoped-fetch path (all the neutrality
// guarantees above are inherited), same ~one incremental fetch per tick per live
// show; the re-fetch itself is opened by ShouldRefreshLivePlaylist in
// reimportExistingEpisode. Eligibility keys on the live windowed EPISODE, not a
// stored show schedule (PSY-1509 — see ShowsWithLiveIncompleteEpisodes for the
// bounding invariant). Handoff is clean: past ends_at the episode is no longer live
// and the post-air backfill owns the single final fetch → complete. Worst case
// within a window is a persistently-broken live feed re-fetched every tick with no
// backoff — bounded by ends_at + the show's duration (typically 1–3h), and
// breaker-neutral by the scoped-run design.

// slotBoundaryDue reports whether any slot of the schedule has a start or end
// instant inside (from, to]. Half-open on the left so back-to-back ticker
// windows never double-count a boundary; inclusive on the right so a boundary
// landing exactly on the tick still fires. Candidate calendar days span
// from−1d..to in the schedule's zone: a slot materialized on day D can wrap
// past midnight (TimesOnDay), so its end may fall inside a window whose dates
// no longer include D.
func slotBoundaryDue(sched *catalogm.RadioSchedule, from, to time.Time) (bool, error) {
	if sched == nil || !to.After(from) {
		return false, nil
	}
	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		return false, fmt.Errorf("invalid schedule timezone %q: %w", sched.Timezone, err)
	}
	first := from.In(loc).AddDate(0, 0, -1)
	first = time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, loc)
	last := to.In(loc)
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		weekday := int(day.Weekday())
		for i := range sched.Slots {
			if sched.Slots[i].DayOfWeek != weekday {
				continue
			}
			start, end, ok := sched.Slots[i].TimesOnDay(day, loc)
			if !ok {
				continue // defensive; slots are validated at write time
			}
			if (start.After(from) && !start.After(to)) || (end.After(from) && !end.After(to)) {
				return true, nil
			}
		}
	}
	return false, nil
}

// ShowsWithSlotBoundariesIn returns stationID → showIDs for every show whose
// stored schedule has a slot boundary in (from, to] — the slot-fetch ticker's
// work list. The population mirrors the incremental fetch loop's exactly
// (active shows with an external id on active, automated stations), so a
// scoped fetch is never attempted for a show the sweep itself wouldn't fetch.
// A show whose stored schedule fails to parse is skipped with a warning —
// one bad schedule must not starve the rest of the tick.
func (s *RadioService) ShowsWithSlotBoundariesIn(from, to time.Time) (map[uint][]uint, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var shows []catalogm.RadioShow
	err := s.db.
		Joins("JOIN radio_stations ON radio_stations.id = radio_shows.station_id").
		Where("radio_shows.is_active = TRUE AND radio_shows.external_id IS NOT NULL AND radio_shows.external_id != ''").
		Where("radio_shows.schedule IS NOT NULL").
		Where("radio_stations.is_active = TRUE AND radio_stations.playlist_source IS NOT NULL AND radio_stations.playlist_source != '' AND radio_stations.playlist_source != ?", catalogm.PlaylistSourceManual).
		Find(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("querying schedule-bearing shows: %w", err)
	}
	due := make(map[uint][]uint)
	for i := range shows {
		sched, err := catalogm.ParseRadioSchedule(shows[i].Schedule)
		if err != nil || sched == nil {
			if err != nil {
				slog.Default().Warn("radio slot fetch: unparseable stored schedule; skipping show",
					"show_id", shows[i].ID, "show_slug", shows[i].Slug, "error", err)
			}
			continue
		}
		hit, err := slotBoundaryDue(sched, from, to)
		if err != nil {
			slog.Default().Warn("radio slot fetch: schedule evaluation failed; skipping show",
				"show_id", shows[i].ID, "show_slug", shows[i].Slug, "error", err)
			continue
		}
		if hit {
			due[shows[i].StationID] = append(due[shows[i].StationID], shows[i].ID)
		}
	}
	return due, nil
}

// ShowsWithLiveIncompleteEpisodes returns stationID → showIDs for every show that has
// an episode airing RIGHT NOW whose playlist is still incomplete (PSY-1370). This is
// the slot ticker's live-refresh work list, unioned with the boundary work list so an
// on-air show is re-fetched every tick and its playlist grows during the show — not
// only at the boundary.
//
// The show population mirrors ShowsWithSlotBoundariesIn (active shows with an
// external id on active, automated stations) EXCEPT the schedule IS NOT NULL filter,
// which this query deliberately does NOT share (PSY-1509 — the locked eligibility
// decision). Eligibility keys on the EPISODE, not the show: a show is refreshed only
// while it has a genuinely-live windowed episode row (starts_at <= now <= ends_at,
// playlist still pending/partial). Those rows come exclusively from real ingested air
// windows — WFMU's schedule-derived stamping (PSY-1152/1238) or the KEXP/NTS
// airing-feed ingestion (radio_airing_ingest.go).
//
// Load honesty: for a back-to-back 24/7 broadcaster (KEXP) this DOES mean the
// station's current show is, in steady state, continuously in this work list —
// the scenario the pre-PSY-1509 scope comment deferred for its own decision.
// That decision was made (PSY-1509, reviewed with numbers in its PR): ~1 scoped
// fetch per tick ≈ 144/day/station at the 10-min default, bounded per-show by
// ends_at and gated to MATCHED shows with real ingested windows (unmatched or
// rerun broadcasts contribute nothing), with every scoped-run neutrality
// guarantee unchanged. The boundary work list (ShowsWithSlotBoundariesIn)
// STAYS schedule-gated.
//
// "Live" is the bounded-window condition ComputeEpisodeStatus uses (starts_at <= now <=
// ends_at). The SQL bounds match it exactly at the boundary instants: `starts_at <= now`
// ⟺ !now.Before(starts), `ends_at >= now` ⟺ !now.After(ends). A windowless episode (NULL
// bounds) is excluded by construction — it can't be "live".
func (s *RadioService) ShowsWithLiveIncompleteEpisodes(now time.Time) (map[uint][]uint, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	type row struct {
		StationID uint
		ShowID    uint
	}
	var rows []row
	err := s.db.Model(&catalogm.RadioEpisode{}).
		Select("DISTINCT radio_shows.station_id AS station_id, radio_shows.id AS show_id").
		Joins("JOIN radio_shows ON radio_shows.id = radio_episodes.show_id").
		Joins("JOIN radio_stations ON radio_stations.id = radio_shows.station_id").
		Where("radio_shows.is_active = TRUE AND radio_shows.external_id IS NOT NULL AND radio_shows.external_id != ''").
		Where("radio_stations.is_active = TRUE AND radio_stations.playlist_source IS NOT NULL AND radio_stations.playlist_source != '' AND radio_stations.playlist_source != ?", catalogm.PlaylistSourceManual).
		Where("radio_episodes.starts_at IS NOT NULL AND radio_episodes.ends_at IS NOT NULL").
		Where("radio_episodes.starts_at <= ? AND radio_episodes.ends_at >= ?", now, now).
		Where("radio_episodes.playlist_state IN ?", []string{catalogm.RadioPlaylistStatePending, catalogm.RadioPlaylistStatePartial}).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("querying shows with live incomplete episodes: %w", err)
	}
	live := make(map[uint][]uint)
	for _, r := range rows {
		live[r.StationID] = append(live[r.StationID], r.ShowID)
	}
	return live, nil
}
