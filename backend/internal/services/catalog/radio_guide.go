package catalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1053: the /radio hub's ON NOW / UP NEXT program guide, computed from
// radio_shows.schedule (the PSY-1159/1322 scraped weekly slots) in each
// station's own timezone. Stations without schedule data (KEXP, NTS) simply
// contribute no rows — the guide's honesty contract is "scheduled programming
// only", not a claim about every stream.
//
// This is SCHEDULE-derived, deliberately distinct from the live now-playing
// path (PSY-1022/1239, which gates "on air" on a live-DJ playlist link): a
// guide row says "the schedule has this show in this slot", never "a live DJ
// is confirmed on the stream". The two surfaces complement each other on the
// hub — the dial strips carry the live claim, the guide carries the schedule.

// radioGuideUpNextHorizon bounds how far ahead an UP NEXT row may start. A
// day covers every overnight gap in a real weekly schedule (WFMU's family
// broadcasts around the clock; the widest observed gap is hours), while
// keeping "up next" honest — a slot 6 days out is next week's schedule, not
// "up next".
const radioGuideUpNextHorizon = 24 * time.Hour

// guideOccurrence is one concrete airing of one show's slot, materialized
// around `now`.
type guideOccurrence struct {
	row   contracts.RadioGuideRow
	start time.Time
	end   time.Time
}

// GetRadioGuide computes the dial-wide guide at `now`: every scheduled show
// currently inside one of its slots (ON NOW), and each station's next
// upcoming slot (UP NEXT, earliest start within radioGuideUpNextHorizon).
func (s *RadioService) GetRadioGuide(now time.Time) (*contracts.RadioGuideResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Every active scheduled show on an active station, with its station in
	// one round trip. Retired/dormant shows keep their schedule out of the
	// guide (a dormant show's slots are stale by definition), and the legacy
	// admin-toggleable show is_active is honored too — an admin turning a
	// show off must remove it from the public guide, whatever the lifecycle
	// reconciler thinks.
	var rows []struct {
		ShowID      uint
		ShowSlug    string
		ShowName    string
		HostName    *string
		Schedule    []byte
		StationSlug string
		StationName string
	}
	err := s.db.Table("radio_shows s").
		Select(`s.id AS show_id, s.slug AS show_slug, s.name AS show_name, s.host_name,
			s.schedule, st.slug AS station_slug, st.name AS station_name`).
		Joins("JOIN radio_stations st ON st.id = s.station_id").
		Where("s.schedule IS NOT NULL AND s.lifecycle_state = ? AND s.is_active AND st.is_active", catalogm.RadioLifecycleActive).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("loading scheduled shows: %w", err)
	}

	occurrences := make([]guideOccurrence, 0, len(rows)*2)
	// One LoadLocation per distinct zone per request — the stdlib call can
	// hit the zoneinfo filesystem, and every WFMU-family row shares a zone.
	locs := map[string]*time.Location{}
	for _, r := range rows {
		rawMsg := json.RawMessage(r.Schedule)
		sched, pErr := catalogm.ParseRadioSchedule(&rawMsg)
		if pErr != nil || sched == nil {
			continue // an unparseable schedule never renders a guide row
		}
		loc, cached := locs[sched.Timezone]
		if !cached {
			var lErr error
			loc, lErr = time.LoadLocation(sched.Timezone)
			if lErr != nil {
				continue
			}
			locs[sched.Timezone] = loc
		}
		for _, occ := range expandGuideSlots(sched.Slots, loc, now) {
			occurrences = append(occurrences, guideOccurrence{
				row: contracts.RadioGuideRow{
					Station:         contracts.RadioGuideStationRef{Slug: r.StationSlug, Name: r.StationName},
					Show:            contracts.RadioGuideShowRef{ID: r.ShowID, Slug: r.ShowSlug, Name: r.ShowName, HostName: r.HostName},
					StartsAt:        occ.start,
					EndsAt:          occ.end,
					StationTimezone: sched.Timezone,
				},
				start: occ.start,
				end:   occ.end,
			})
		}
	}

	resp := &contracts.RadioGuideResponse{GeneratedAt: now}
	nextByStation := map[string]guideOccurrence{}
	for _, occ := range occurrences {
		switch {
		case !occ.start.After(now) && occ.end.After(now):
			resp.OnNow = append(resp.OnNow, occ.row)
		case occ.start.After(now) && occ.start.Before(now.Add(radioGuideUpNextHorizon)):
			// Earliest start wins; on an exact tie (stacked slots sharing a
			// boundary) the show slug breaks it deterministically — Scan row
			// order is unordered and must not decide a public payload.
			best, seen := nextByStation[occ.row.Station.Slug]
			if !seen || occ.start.Before(best.start) ||
				(occ.start.Equal(best.start) && occ.row.Show.Slug < best.row.Show.Slug) {
				nextByStation[occ.row.Station.Slug] = occ
			}
		}
	}
	for _, occ := range nextByStation {
		resp.UpNext = append(resp.UpNext, occ.row)
	}

	// Deterministic presentation: ON NOW by station name (a dial in order),
	// UP NEXT soonest-first (the tune-in-next list).
	sort.Slice(resp.OnNow, func(i, j int) bool {
		a, b := resp.OnNow[i], resp.OnNow[j]
		if a.Station.Name != b.Station.Name {
			return a.Station.Name < b.Station.Name
		}
		return a.StartsAt.Before(b.StartsAt)
	})
	sort.Slice(resp.UpNext, func(i, j int) bool {
		a, b := resp.UpNext[i], resp.UpNext[j]
		if !a.StartsAt.Equal(b.StartsAt) {
			return a.StartsAt.Before(b.StartsAt)
		}
		return a.Station.Name < b.Station.Name
	})
	return resp, nil
}

// timedSlot is one concrete (start, end) instant pair for a weekly slot.
type timedSlot struct{ start, end time.Time }

// guideMaxSlotSpan is the trust ceiling for a guide window, matching the
// frontend's parseWindow guard (stationOverview.ts MAX_WINDOW_MS = 12h): a
// slot that expands to ≥12h is corrupt data (the End == Start degenerate
// 24h case, or a garbled scrape), and rendering it would claim a day-long
// ON NOW that the frontend then shows with NO time range at all (both range
// formatters reject ≥12h windows). The two halves of the feature must agree
// on what's trustworthy — corrupt slots are dropped here, not half-rendered.
const guideMaxSlotSpan = 12 * time.Hour

// expandGuideSlots materializes every slot's concrete airings for the
// station-local days [yesterday, today, tomorrow] around `now` — yesterday
// catches an overnight wrap still on the air, tomorrow feeds the UP NEXT
// horizon across midnight. Unlike WindowForDate (which freezes ONE window per
// weekday for a date-only episode), the guide needs every slot: stacked slots
// (two shows splitting a band) and double airings all render. The slot-time
// semantics themselves (HH:MM parse, midnight wrap, DST-gap normalization)
// are the shared RadioScheduleSlot.TimesOnDay — one definition, never forked
// from the episode air-window path.
func expandGuideSlots(slots []catalogm.RadioScheduleSlot, loc *time.Location, now time.Time) []timedSlot {
	local := now.In(loc)
	out := make([]timedSlot, 0, len(slots))
	for dayOffset := -1; dayOffset <= 1; dayOffset++ {
		day := local.AddDate(0, 0, dayOffset)
		weekday := int(day.Weekday())
		for _, sl := range slots {
			if sl.DayOfWeek != weekday {
				continue
			}
			start, end, ok := sl.TimesOnDay(day, loc)
			if !ok || end.Sub(start) >= guideMaxSlotSpan {
				continue
			}
			out = append(out, timedSlot{start: start, end: end})
		}
	}
	return out
}
