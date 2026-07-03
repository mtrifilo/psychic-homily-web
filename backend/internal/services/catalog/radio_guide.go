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
	// guide (a dormant show's slots are stale by definition).
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
		Where("s.schedule IS NOT NULL AND s.lifecycle_state = ? AND st.is_active", catalogm.RadioLifecycleActive).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("loading scheduled shows: %w", err)
	}

	occurrences := make([]guideOccurrence, 0, len(rows)*2)
	for _, r := range rows {
		rawMsg := json.RawMessage(r.Schedule)
		sched, pErr := catalogm.ParseRadioSchedule(&rawMsg)
		if pErr != nil || sched == nil {
			continue // an unparseable schedule never renders a guide row
		}
		loc, lErr := time.LoadLocation(sched.Timezone)
		if lErr != nil {
			continue
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
			best, seen := nextByStation[occ.row.Station.Slug]
			if !seen || occ.start.Before(best.start) {
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

// expandGuideSlots materializes every slot's concrete airings for the
// station-local days [yesterday, today, tomorrow] around `now` — yesterday
// catches an overnight wrap still on the air, tomorrow feeds the UP NEXT
// horizon across midnight. Unlike WindowForDate (which freezes ONE window per
// weekday for a date-only episode), the guide needs every slot: stacked slots
// (two shows splitting a band) and double airings all render.
//
// An End <= Start slot wraps past midnight (same convention as WindowForDate;
// End == Start degenerates to 24h and fails safe by over-covering). The
// spring-forward gap normalization noted on WindowForDate applies here the
// same way — a window can only ever shrink, so a show is never falsely ON NOW
// for longer than its slot.
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
			sh, sm, ok := parseGuideHHMM(sl.Start)
			if !ok {
				continue
			}
			eh, em, ok := parseGuideHHMM(sl.End)
			if !ok {
				continue
			}
			start := time.Date(day.Year(), day.Month(), day.Day(), sh, sm, 0, 0, loc)
			end := time.Date(day.Year(), day.Month(), day.Day(), eh, em, 0, 0, loc)
			if !end.After(start) {
				end = end.AddDate(0, 0, 1)
			}
			out = append(out, timedSlot{start: start, end: end})
		}
	}
	return out
}

// parseGuideHHMM parses a validated "HH:MM" slot time. Slots reaching here
// passed RadioSchedule.Validate, so failure is defensive (skip the slot, never
// error the guide).
func parseGuideHHMM(s string) (hour, minute int, ok bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}
