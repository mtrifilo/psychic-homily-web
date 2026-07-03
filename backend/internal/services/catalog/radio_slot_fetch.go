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
// runs a single-show scoped fetch for exactly those. Two targeted provider
// requests per show per airing (one as it starts — the row appears while it
// airs, the post-air backfill sweep then grows the playlist — and one after it
// ends, catching DJs who publish the playlist late). Stations without stored
// schedules (KEXP/NTS today) keep riding the interval sweep, which remains the
// backstop for everything.

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
