package catalog

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/net/html"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1322: scrape the three WFMU sub-stream schedule pages into per-show
// recurring slots, the way PSY-1159 does for the 91.1 flagship grid. Without
// this the sub-stream shows have no radio_shows.schedule, so PSY-1238 stamps
// no air windows — no viewer-local time blocks, no window ordering, and no
// ON NOW / UP NEXT eligibility on those station tabs.
//
// The source pages (wfmu.org/drummer, /sheena, /rocknsoulradio — linked as
// "Schedule" from /audiostream.shtml) are a DIFFERENT shape from /table: a
// rolling 7-day "Upcoming schedule" list, not a rowspan grid. Day groups are
// delimited by <div class="upcoming_dow">Thursday</div> rows; each slot row
// has a time-range cell ("12-3pm", "10pm-12am") and an
// <a href="/playlists/{CODE}">Show Name</a>. Days are literal calendar days —
// the /table broadcast-day off-by-one (PSY-1283) does NOT apply here.
// (wfmu.org/table?channel=drummer serves the flagship page verbatim — the
// query string is ignored upstream; these list pages are the real source.)
//
// THE PARTIAL-TODAY RULE (load-bearing): the listing starts TODAY in the
// stream's local time and today's group carries only the not-yet-aired slots;
// a weekly show that already aired today appears NOWHERE in the rolling
// window (its next occurrence sits just past the last full day). So a
// weekday's slots are authoritative only when that weekday is a FULL day of
// the listing — every weekday except the scrape day. The apply step updates
// the six full weekdays and PRESERVES each show's existing scrape-day slots
// untouched; a daily-or-faster cadence converges on the whole week.

// wfmuSubstreamSchedulePages routes each sub-stream station slug (seeded in
// seeddata/radio.go) to its wfmu.org schedule-page path. In-code constants
// only — the scrape URL is never derived from DB or user input (SSRF posture,
// same as the flagship /table scrape).
var wfmuSubstreamSchedulePages = map[string]string{
	"wfmu-drummer":        "drummer",
	"wfmu-sheena":         "sheena",
	"wfmu-rocknsoulradio": "rocknsoulradio",
}

// wfmuSubstreamClearMinEntries is the recognized-shows floor before a
// sub-stream apply may clear anything (the PSY-1186 suspect-parse guard,
// scaled down: a sub-stream week carries ~25-35 distinct shows vs the
// flagship grid's ~60+, so a third of a healthy lineup is ~10).
const wfmuSubstreamClearMinEntries = 10

// DiscoverSubstreamSchedule fetches one sub-stream schedule page (a path
// under wfmu.org, from wfmuSubstreamSchedulePages) and parses its rolling
// 7-day listing into per-show slots. WFMU-specific, like DiscoverSchedule.
func (p *WFMUProvider) DiscoverSubstreamSchedule(pagePath string) (entries []WFMUScheduleEntry, skipped int, err error) {
	<-p.rateLimiter.C
	body, err := p.doGet(fmt.Sprintf("%s/%s", p.baseURL, pagePath))
	if err != nil {
		return nil, 0, fmt.Errorf("fetching sub-stream schedule %s: %w", pagePath, err)
	}
	return parseWFMUSubstreamSchedule(body)
}

// substreamScheduleDiscoverer is the narrow capability the schedule cycle
// needs for sub-streams, mirroring scheduleDiscoverer so tests can drive the
// cycle with a mock provider.
type substreamScheduleDiscoverer interface {
	DiscoverSubstreamSchedule(pagePath string) ([]WFMUScheduleEntry, int, error)
}

// parseWFMUSubstreamSchedule walks the rolling-week listing: a <div
// class="upcoming_dow"> row sets the current weekday; each subsequent row
// whose first cell parses as a time range and which carries a
// /playlists/{CODE} link becomes one slot for that weekday. Rows before the
// first day header (the "now playing" chrome) are ignored; a slot-shaped row
// that fails time parsing or has no program code is counted in skipped, so a
// markup change doesn't look identical to a healthy run. Entries are keyed by
// code (a show airing several days accumulates several slots), unsorted.
func parseWFMUSubstreamSchedule(body []byte) (entries []WFMUScheduleEntry, skipped int, err error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, 0, fmt.Errorf("parsing sub-stream schedule HTML: %w", err)
	}

	byCode := map[string]*WFMUScheduleEntry{}
	order := []string{}
	currentDay := -1

	var rows []*html.Node
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			rows = append(rows, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(doc)

	for _, row := range rows {
		// Day-header row: <div class="upcoming_dow">Friday</div>.
		if dow := findDescendant(row, func(n *html.Node) bool {
			return n.Type == html.ElementNode && n.Data == "div" &&
				strings.Contains(attrVal(n, "class"), "upcoming_dow")
		}); dow != nil {
			day, ok := dayNameToWeekday[strings.ToUpper(strings.TrimSpace(textContent(dow)))]
			if !ok {
				currentDay = -1 // unrecognized header — don't attribute slots to a stale day
				skipped++
				continue
			}
			currentDay = day
			continue
		}
		if currentDay < 0 {
			continue // pre-header chrome (page banner, "now playing" block)
		}

		cells := cellChildren(row)
		if len(cells) < 2 {
			continue // spacer/border rows inside the table
		}
		start, end, ok := parseWFMUTimeRange(strings.TrimSpace(textContent(cells[0])))
		if !ok {
			continue // not a slot row (nested layout rows land here too)
		}
		code, name := extractSubstreamProgramLink(cells[1])
		if code == "" {
			skipped++ // slot-shaped row with no recognizable program link
			continue
		}
		e, seen := byCode[code]
		if !seen {
			e = &WFMUScheduleEntry{Code: code, Name: name}
			byCode[code] = e
			order = append(order, code)
		}
		e.Slots = append(e.Slots, catalogm.RadioScheduleSlot{DayOfWeek: currentDay, Start: start, End: end})
	}

	entries = make([]WFMUScheduleEntry, 0, len(order))
	for _, code := range order {
		entries = append(entries, *byCode[code])
	}
	return entries, skipped, nil
}

// attrVal returns an attribute's value ("" when absent).
func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// extractSubstreamProgramLink pulls code + name from a sub-stream slot cell.
// The grid extractor (extractProgramCodeAndName) keys on id="KDBprogram-…" /
// class="show-title-link", and sub-stream anchors carry NEITHER — they are
// bare <a href="/playlists/{CODE}">Show Name</a>. The FIRST such anchor is the
// show link (the expandable description below it can mention other shows'
// /playlists/ URLs, so first-match order is load-bearing).
func extractSubstreamProgramLink(cell *html.Node) (code, name string) {
	link := findDescendant(cell, func(x *html.Node) bool {
		return x.Type == html.ElementNode && strings.EqualFold(x.Data, "a") &&
			playlistCodeRe.MatchString(attrVal(x, "href"))
	})
	if link == nil {
		return "", ""
	}
	code = playlistCodeRe.FindStringSubmatch(attrVal(link, "href"))[1]
	name = strings.Join(strings.Fields(textContent(link)), " ")
	return code, name
}

// wfmuLocalWeekday returns the current weekday in WFMU's schedule timezone —
// the scrape day whose listing group is partial (see the partial-today rule).
// The UTC fallback mirrors wfmuTodayCap's defensive posture: production always
// has the IANA db; being wrong here only widens the preserved weekday by the
// UTC/ET gap for a few hours, it never corrupts a slot.
func wfmuLocalWeekday(now time.Time) int {
	loc, err := time.LoadLocation(wfmuScheduleTimezone)
	if err != nil {
		loc = time.UTC
	}
	return int(now.In(loc).Weekday())
}

// ApplyWFMUSubstreamSchedule writes the parsed rolling-week entries onto the
// station's shows (matched exact-by-code within THAT station — the PSY-1127
// family scoping; a code never updates a sibling station's row). It differs
// from the flagship ApplyWFMUSchedule in one way: the partial-today rule.
// excludeWeekday (the scrape day) is never trusted from this scrape — each
// show's new schedule is the scraped slots for the six full weekdays plus its
// EXISTING slots on excludeWeekday, preserved verbatim. Clearing therefore
// also happens per-show through the same merge: a show whose merged slot set
// comes out empty (dropped from all full days, nothing preserved) has its
// schedule nulled — gated on the recognized-shows floor, and always skipping
// schedule_locked shows (admin-curated, PSY-1186).
func (s *RadioService) ApplyWFMUSubstreamSchedule(stationSlug string, entries []WFMUScheduleEntry, excludeWeekday int) (matched, unmatched, cleared int, err error) {
	if s.db == nil {
		return 0, 0, 0, fmt.Errorf("database not initialized")
	}
	var station catalogm.RadioStation
	if err := s.db.Where("slug = ?", stationSlug).First(&station).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("sub-stream station (slug=%q) not found: %w", stationSlug, err)
	}

	var shows []catalogm.RadioShow
	if err := s.db.Select("id", "external_id", "schedule", "schedule_locked").
		Where("station_id = ? AND external_id IS NOT NULL", station.ID).
		Find(&shows).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("loading shows for %s: %w", stationSlug, err)
	}
	showByCode := make(map[string]*catalogm.RadioShow, len(shows))
	for i := range shows {
		showByCode[*shows[i].ExternalID] = &shows[i]
	}

	scraped := make(map[string][]catalogm.RadioScheduleSlot, len(entries))
	recognized, lockedSkipped := 0, 0
	for _, e := range entries {
		scraped[e.Code] = e.Slots
		if _, ok := showByCode[e.Code]; ok {
			recognized++
		} else {
			unmatched++
			slog.Info("wfmu substream schedule: no show for code, deferred",
				"station", stationSlug, "code", e.Code, "name", e.Name)
		}
	}
	clearAllowed := recognized >= wfmuSubstreamClearMinEntries
	if !clearAllowed {
		slog.Warn("wfmu substream schedule: too few shows recognized — clears disabled this run (suspect parse)",
			"station", stationSlug, "recognized", recognized, "min", wfmuSubstreamClearMinEntries)
	}

	for code, show := range showByCode {
		if show.ScheduleLocked {
			if _, inScrape := scraped[code]; inScrape {
				lockedSkipped++
			}
			continue
		}

		// Merge: full-day slots from the scrape + the show's existing
		// scrape-day slots. A show absent from the scrape contributes only
		// its preserved slots — which is exactly how a today-only show
		// survives the day it airs.
		newSlots := make([]catalogm.RadioScheduleSlot, 0, 8)
		for _, sl := range scraped[code] {
			if sl.DayOfWeek != excludeWeekday {
				newSlots = append(newSlots, sl)
			}
		}
		hadSchedule := show.Schedule != nil
		if hadSchedule {
			existing, pErr := catalogm.ParseRadioSchedule(show.Schedule)
			if pErr != nil {
				slog.Warn("wfmu substream schedule: existing schedule unparseable, treating as empty",
					"station", stationSlug, "code", code, "show_id", show.ID, "error", pErr)
			} else if existing != nil {
				for _, sl := range existing.Slots {
					if sl.DayOfWeek == excludeWeekday {
						newSlots = append(newSlots, sl)
					}
				}
			}
		}

		if len(newSlots) == 0 {
			if !hadSchedule || !clearAllowed {
				continue
			}
			if uErr := s.db.Model(&catalogm.RadioShow{}).
				Where("id = ?", show.ID).
				Update("schedule", nil).Error; uErr != nil {
				slog.Warn("wfmu substream schedule: clear failed",
					"station", stationSlug, "code", code, "show_id", show.ID, "error", uErr)
				continue
			}
			cleared++
			slog.Info("wfmu substream schedule: cleared schedule for show absent from all full days",
				"station", stationSlug, "code", code, "show_id", show.ID)
			continue
		}

		sched := catalogm.RadioSchedule{Timezone: wfmuScheduleTimezone, Slots: newSlots}
		if vErr := sched.Validate(); vErr != nil {
			slog.Warn("wfmu substream schedule: invalid schedule, skipped",
				"station", stationSlug, "code", code, "error", vErr)
			continue
		}
		raw, mErr := json.Marshal(sched)
		if mErr != nil {
			slog.Warn("wfmu substream schedule: marshal failed",
				"station", stationSlug, "code", code, "error", mErr)
			continue
		}
		rawMsg := json.RawMessage(raw)
		if uErr := s.db.Model(&catalogm.RadioShow{}).
			Where("id = ?", show.ID).
			Update("schedule", &rawMsg).Error; uErr != nil {
			slog.Warn("wfmu substream schedule: update failed",
				"station", stationSlug, "code", code, "show_id", show.ID, "error", uErr)
			continue
		}
		matched++
	}
	if lockedSkipped > 0 {
		slog.Info("wfmu substream schedule: skipped schedule_locked shows",
			"station", stationSlug, "locked_skipped", lockedSkipped)
	}
	return matched, unmatched, cleared, nil
}

// applySubstreamScheduleForStation is the per-station unit the schedule cycle
// loops over: discover the page, apply with the partial-today rule. A station
// not seeded in this environment surfaces as gorm.ErrRecordNotFound (the apply
// wraps it with %w) — the cycle logs that quietly instead of as an error.
func (s *RadioService) applySubstreamScheduleForStation(sd substreamScheduleDiscoverer, stationSlug, pagePath string, now time.Time) (matched, unmatched, cleared, skipped int, err error) {
	entries, skipped, err := sd.DiscoverSubstreamSchedule(pagePath)
	if err != nil {
		return 0, 0, 0, skipped, err
	}
	matched, unmatched, cleared, err = s.ApplyWFMUSubstreamSchedule(stationSlug, entries, wfmuLocalWeekday(now))
	return matched, unmatched, cleared, skipped, err
}
