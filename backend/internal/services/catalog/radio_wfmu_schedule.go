package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// PSY-1159: scrape WFMU's weekly program grid (wfmu.org/table) into per-show recurring
// schedule slots. WFMU episodes carry only a date (no air time), so the lifecycle state
// machine (PSY-1152) falls them back to "aired"; this schedule is the air-time source.
//
// The grid is a single 7-day table: <th class="day">MONDAY..SUNDAY</th> as columns, time
// labels on both edges (<td class="hour">). Each show is a <td class="program" rowspan=N>
// whose contents are structured (not free text): the show title + WFMU program code live
// in <a class="show-title-link" href="/playlists/{CODE}">, and the slot's time range is in
// <span class="program_time"> ("6-9am", "9am-Noon", "9pm-Mid", "Mid-3am"). The day comes
// only from the cell's column, so the rowspan/colspan grid is reconstructed to a matrix.

// wfmuScheduleTimezone is the IANA zone for WFMU's schedule. The table header reads
// "EDT (-0400)" but that is a seasonal snapshot — we store the DST-aware zone so PSY-1152
// stamps episode windows correctly year-round (never the fixed -0400 offset).
const wfmuScheduleTimezone = "America/New_York"

// WFMUScheduleEntry is one show's recurring weekly slots, keyed by the WFMU program code
// (== radio_shows.external_id, so matching is an exact join, not a fuzzy name match).
type WFMUScheduleEntry struct {
	Code  string // program code, e.g. "WA"
	Name  string // show title (diagnostics/logging)
	Slots []catalogm.RadioScheduleSlot
}

// DiscoverSchedule fetches wfmu.org/table and parses the weekly grid into per-show slots.
// WFMU-specific (not part of RadioPlaylistProvider): only the 91.1 broadcast has this grid.
func (p *WFMUProvider) DiscoverSchedule() (entries []WFMUScheduleEntry, skipped int, err error) {
	<-p.rateLimiter.C
	body, err := p.doGet(fmt.Sprintf("%s/table", p.baseURL))
	if err != nil {
		return nil, 0, fmt.Errorf("fetching schedule table: %w", err)
	}
	return parseWFMUScheduleTable(body)
}

// scheduleDiscoverer is the WFMU-only capability the schedule cycle needs; kept as a
// narrow interface so the cycle can be driven by a mock provider in tests. The second
// return is the count of grid cells skipped during parse (observability).
type scheduleDiscoverer interface {
	DiscoverSchedule() ([]WFMUScheduleEntry, int, error)
}

// wfmuFlagshipStationSlug is the seeded radio_stations.slug for WFMU 91.1 — the only
// station whose shows the /table grid describes (the three sub-streams have their own
// rosters; PSY-1127). Schedule writes are scoped to this station's shows.
const wfmuFlagshipStationSlug = "wfmu"

// wfmuScheduleClearMinEntries is the floor a scrape must clear before clear-on-absence
// runs (PSY-1186). The normal /table grid has ~60+ shows; a result far below this means a
// broken parse, and clearing "absent" shows then would wipe nearly every schedule. So
// clear-on-absence is skipped (logged) when a scrape returns fewer than this many shows.
const wfmuScheduleClearMinEntries = 20

// ApplyWFMUSchedule writes each parsed entry's slots onto the matching WFMU 91.1 show,
// matched by external_id (== program code) — an exact join, no fuzzy name match. Returns
// matched/unmatched/cleared counts. A single show's validate/marshal/update failure is
// logged and skipped (one bad row never aborts the batch); an unmatched code is deferred
// (the show may not have a row yet under create-on-first, PSY-1153).
//
// SCRAPE-WINS for unlocked shows (per the owner's "re-scrape weekly for seasonal churn"
// decision): wfmu.org/table is the source of truth, so this overwrites an unlocked
// schedule (that's how churn propagates). schedule_locked shows are SKIPPED — an admin
// curated those by hand (PSY-1186), so the scrape leaves them alone. After writing, a
// guarded clear-on-absence nulls the schedule of unlocked shows that dropped off the grid.
func (s *RadioService) ApplyWFMUSchedule(entries []WFMUScheduleEntry) (matched, unmatched, cleared int, err error) {
	if s.db == nil {
		return 0, 0, 0, fmt.Errorf("database not initialized")
	}
	var station catalogm.RadioStation
	if err := s.db.Where("slug = ?", wfmuFlagshipStationSlug).First(&station).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("wfmu flagship station (slug=%q) not found: %w", wfmuFlagshipStationSlug, err)
	}

	scrapedCodes := make([]string, 0, len(entries))
	for _, e := range entries {
		scrapedCodes = append(scrapedCodes, e.Code)

		var show catalogm.RadioShow
		lookupErr := s.db.Select("id", "schedule", "schedule_locked").
			Where("station_id = ? AND external_id = ?", station.ID, e.Code).
			First(&show).Error
		if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			unmatched++
			slog.Info("wfmu schedule: no show for code, deferred", "code", e.Code, "name", e.Name)
			continue
		}
		if lookupErr != nil {
			slog.Warn("wfmu schedule: show lookup failed", "code", e.Code, "error", lookupErr)
			continue
		}
		if show.ScheduleLocked {
			slog.Info("wfmu schedule: show is schedule_locked, skipped (admin-curated)", "code", e.Code, "show_id", show.ID)
			continue
		}

		sched := catalogm.RadioSchedule{Timezone: wfmuScheduleTimezone, Slots: e.Slots}
		if vErr := sched.Validate(); vErr != nil {
			slog.Warn("wfmu schedule: invalid schedule, skipped", "code", e.Code, "error", vErr)
			continue
		}
		raw, mErr := json.Marshal(sched)
		if mErr != nil {
			slog.Warn("wfmu schedule: marshal failed", "code", e.Code, "error", mErr)
			continue
		}
		rawMsg := json.RawMessage(raw)
		if uErr := s.db.Model(&catalogm.RadioShow{}).
			Where("id = ?", show.ID).
			Update("schedule", &rawMsg).Error; uErr != nil {
			slog.Warn("wfmu schedule: update failed", "code", e.Code, "show_id", show.ID, "error", uErr)
			continue
		}
		matched++
	}

	cleared = s.clearAbsentWFMUSchedules(station.ID, scrapedCodes)
	return matched, unmatched, cleared, nil
}

// clearAbsentWFMUSchedules nulls the schedule of WFMU 91.1 shows that have one but whose
// code was NOT in this scrape (they dropped off the grid) — except schedule_locked shows
// (admin-curated). Guarded: a scrape returning fewer than wfmuScheduleClearMinEntries shows
// is treated as suspect (broken parse) and clears nothing, so a bad scrape can't wipe the
// lineup. Returns the number of schedules cleared.
func (s *RadioService) clearAbsentWFMUSchedules(stationID uint, scrapedCodes []string) int {
	if len(scrapedCodes) < wfmuScheduleClearMinEntries {
		slog.Warn("wfmu schedule: clear-on-absence skipped — scrape returned too few shows (suspect parse)",
			"scraped", len(scrapedCodes), "min", wfmuScheduleClearMinEntries)
		return 0
	}
	res := s.db.Model(&catalogm.RadioShow{}).
		Where("station_id = ? AND schedule IS NOT NULL AND schedule_locked = ? AND external_id NOT IN ?",
			stationID, false, scrapedCodes).
		Update("schedule", nil)
	if res.Error != nil {
		slog.Warn("wfmu schedule: clear-on-absence failed", "station_id", stationID, "error", res.Error)
		return 0
	}
	if res.RowsAffected > 0 {
		slog.Info("wfmu schedule: cleared schedules for shows absent from the grid",
			"cleared", res.RowsAffected, "station_id", stationID)
	}
	return int(res.RowsAffected)
}

// dayNameToWeekday maps the table's day-header text to RadioScheduleSlot.DayOfWeek
// (0=Sunday..6=Saturday).
var dayNameToWeekday = map[string]int{
	"SUNDAY": 0, "MONDAY": 1, "TUESDAY": 2, "WEDNESDAY": 3,
	"THURSDAY": 4, "FRIDAY": 5, "SATURDAY": 6,
}

// parseWFMUScheduleTable reconstructs the grid and returns one entry per program code,
// with a slot for each (day, time-range) cell, plus the count of program cells that were
// SKIPPED (no code, no parseable time, or outside the day columns). A single malformed
// cell never fails the whole scrape — but the skipped count is surfaced + logged so a
// WFMU markup change that breaks N cells doesn't look identical to a healthy run. Known
// skip: a stacked two-show cell (e.g. Monday's "Jim Price (3-3:01)" / "Scott Williams
// (3:01-6)") has two show links + no program_time span; both shows are dropped (tracked
// follow-up PSY-1186). The returned entries are unsorted.
func parseWFMUScheduleTable(body []byte) (entries []WFMUScheduleEntry, skipped int, err error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, 0, fmt.Errorf("parsing schedule HTML: %w", err)
	}

	table := findScheduleTable(doc)
	if table == nil {
		return nil, 0, fmt.Errorf("schedule table not found (no <th class=\"day\">)")
	}

	rows := collectRows(table)
	if len(rows) == 0 {
		return nil, 0, fmt.Errorf("schedule table has no rows")
	}

	// Reconstruct the cell matrix (rowspan/colspan aware) so each cell has a column.
	// colToWeekday is learned from the header row's <th class="day"> column positions.
	colToWeekday := map[int]int{}
	type placedCell struct {
		node    *html.Node
		col     int
		rowHour string // the cell's starting-row hour label ("3pm") — band context for stacked cells
	}
	var programCells []placedCell

	rowspanRemaining := map[int]int{} // col -> further rows still occupied from above
	for _, row := range rows {
		col := 0
		rowHour := "" // this row's left-edge <td class="hour"> label, if any
		for _, cell := range cellChildren(row) {
			for rowspanRemaining[col] > 0 { // skip cols held by a rowspan from above
				col++
			}
			rs := attrInt(cell, "rowspan", 1)
			cs := attrInt(cell, "colspan", 1)

			switch {
			case hasClass(cell, "hour"):
				if rowHour == "" {
					rowHour = strings.TrimSpace(textContent(cell))
				}
			case hasClass(cell, "day"):
				if wd, ok := dayNameToWeekday[strings.ToUpper(strings.TrimSpace(textContent(cell)))]; ok {
					colToWeekday[col] = wd
				}
			case hasClass(cell, "program"):
				programCells = append(programCells, placedCell{node: cell, col: col, rowHour: rowHour})
			}

			for k := 0; k < cs; k++ {
				rowspanRemaining[col+k] = rs // blocks these cols for rs rows (incl. current)
			}
			col += cs
		}
		for c := range rowspanRemaining { // end of row: one row consumed everywhere
			if rowspanRemaining[c] > 0 {
				rowspanRemaining[c]--
			}
		}
	}

	if len(colToWeekday) == 0 {
		return nil, 0, fmt.Errorf("schedule table has no day-column headers")
	}

	// Group slots by program code. Each cell yields one slot (normal) or several (a stacked
	// two-show cell); all slots from a cell share the cell's weekday (its column).
	byCode := map[string]*WFMUScheduleEntry{}
	var order []string
	for _, pc := range programCells {
		weekday, ok := colToWeekday[pc.col]
		if !ok {
			skipped++ // a program cell outside the 7 day columns (defensive)
			continue
		}
		slots := extractCellSlots(pc.node, pc.rowHour)
		if len(slots) == 0 {
			skipped++
			continue
		}
		for _, sl := range slots {
			entry := byCode[sl.code]
			if entry == nil {
				entry = &WFMUScheduleEntry{Code: sl.code, Name: sl.name}
				byCode[sl.code] = entry
				order = append(order, sl.code)
			}
			entry.Slots = append(entry.Slots, catalogm.RadioScheduleSlot{
				DayOfWeek: weekday, Start: sl.start, End: sl.end,
			})
		}
	}

	out := make([]WFMUScheduleEntry, 0, len(order))
	for _, code := range order {
		out = append(out, *byCode[code])
	}
	if skipped > 0 {
		slog.Warn("wfmu schedule: program cells skipped (no code / unparseable time / out of grid)",
			"skipped", skipped, "parsed", len(out))
	}
	return out, skipped, nil
}

// --- DOM helpers (x/net/html) ---

func findScheduleTable(root *html.Node) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "table") && containsDayHeader(n) {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return found
}

func containsDayHeader(n *html.Node) bool {
	var has bool
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if has {
			return
		}
		if x.Type == html.ElementNode && hasClass(x, "day") {
			has = true
			return
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return has
}

// collectRows returns every <tr> descendant of the table in document order.
func collectRows(table *html.Node) []*html.Node {
	var rows []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "tr") {
			rows = append(rows, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(table)
	return rows
}

// cellChildren returns the direct <td>/<th> children of a <tr>, in order.
func cellChildren(row *html.Node) []*html.Node {
	var cells []*html.Node
	for c := row.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (strings.EqualFold(c.Data, "td") || strings.EqualFold(c.Data, "th")) {
			cells = append(cells, c)
		}
	}
	return cells
}

func attrInt(n *html.Node, key string, def int) int {
	if v := getAttr(n, key); v != "" {
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && i > 0 {
			return i
		}
	}
	return def
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			sb.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// findDescendant returns the first descendant element matching pred (depth-first).
func findDescendant(n *html.Node, pred func(*html.Node) bool) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if found != nil {
			return
		}
		if x.Type == html.ElementNode && pred(x) {
			found = x
			return
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}

var (
	kdbProgramIDRe = regexp.MustCompile(`^KDBprogram-([A-Za-z0-9]+)$`)
	playlistCodeRe = regexp.MustCompile(`(?i)/playlists/([A-Za-z0-9]+)`)
)

// extractProgramCodeAndName pulls the WFMU program code + the show title from a program
// cell. The code is read from the favorite-icon span's id="KDBprogram-{CODE}" — the
// canonical, always-present source. The show-title-link href is NOT reliable for the code:
// most are relative /playlists/{CODE} but some are absolute archives URLs (e.g. The Glen
// Jones Radio Programme → https://www.wfmu.org/Playlists/GJ/archives.html), so it is only a
// case-insensitive fallback. The title text always comes from the show-title-link.
func extractProgramCodeAndName(cell *html.Node) (code, name string) {
	if idNode := findDescendant(cell, func(x *html.Node) bool {
		return kdbProgramIDRe.MatchString(getAttr(x, "id"))
	}); idNode != nil {
		code = kdbProgramIDRe.FindStringSubmatch(getAttr(idNode, "id"))[1]
	}

	link := findDescendant(cell, func(x *html.Node) bool {
		return strings.EqualFold(x.Data, "a") && hasClass(x, "show-title-link")
	})
	if link != nil {
		name = strings.Join(strings.Fields(textContent(link)), " ")
		if code == "" { // fallback: relative or absolute /playlists/{CODE} href
			if m := playlistCodeRe.FindStringSubmatch(getAttr(link, "href")); m != nil {
				code = m[1]
			}
		}
	}
	return code, name
}

// parsedSlot is one (show, time) extracted from a program cell — usually one per cell, but
// a stacked two-show cell yields several.
type parsedSlot struct{ code, name, start, end string }

// extractCellSlots returns the slot(s) a program cell contributes (PSY-1186). The common
// case is a single show with a <span class="program_time"> ("3-6pm"). A STACKED cell (two
// shows split across one slot, e.g. Monday's "Jim Price (3-3:01)" / "Scott Williams
// (3:01-6)") has multiple show-title-links + inline parenthesized times and NO program_time
// span — those inline times carry no meridiem, so they are anchored to the cell's band via
// its row's hour label (rowHour, e.g. "3pm"). Returns nil if nothing parseable (caller
// counts it as a skipped cell).
func extractCellSlots(cell *html.Node, rowHour string) []parsedSlot {
	if pt := programTimeText(cell); pt != "" {
		code, name := extractProgramCodeAndName(cell)
		if code == "" {
			return nil
		}
		start, end, ok := parseWFMUTimeRange(pt)
		if !ok {
			return nil
		}
		return []parsedSlot{{code, name, start, end}}
	}

	// Stacked: pair each show (code + title, in document order) with the inline time at the
	// same index. Bare inline times are anchored to the band's meridiem (from rowHour).
	_, bandMer, ok := parseTimeToken(rowHour)
	if !ok || bandMer == "" {
		return nil // no band context → can't disambiguate the meridiem-less inline times
	}
	codes := allProgramCodes(cell)
	names := allShowTitleNames(cell)
	ranges := inlineTimeRanges(cell)
	if len(codes) < 2 || len(codes) != len(names) || len(codes) != len(ranges) {
		return nil // not a recognizable stacked layout
	}
	out := make([]parsedSlot, 0, len(codes))
	for i := range codes {
		start, end, ok := parseWFMUTimeRangeWithDefault(ranges[i], bandMer)
		if !ok {
			return nil
		}
		out = append(out, parsedSlot{codes[i], names[i], start, end})
	}
	return out
}

// allProgramCodes returns every KDBprogram-{CODE} id in the cell, in document order.
func allProgramCodes(cell *html.Node) []string {
	var codes []string
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode {
			if m := kdbProgramIDRe.FindStringSubmatch(getAttr(x, "id")); m != nil {
				codes = append(codes, m[1])
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(cell)
	return codes
}

// allShowTitleNames returns every show-title-link's text in the cell, in document order.
func allShowTitleNames(cell *html.Node) []string {
	var names []string
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode && strings.EqualFold(x.Data, "a") && hasClass(x, "show-title-link") {
			names = append(names, strings.Join(strings.Fields(textContent(x)), " "))
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(cell)
	return names
}

var inlineTimeRe = regexp.MustCompile(`\(([^)]*\d[^)]*)\)`)

// inlineTimeRanges returns the parenthesized "(start - end)" time substrings in the cell's
// text, in document order (a digit must be present, so "(weekly)" is ignored).
func inlineTimeRanges(cell *html.Node) []string {
	var out []string
	for _, m := range inlineTimeRe.FindAllStringSubmatch(textContent(cell), -1) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

// programTimeText returns the trimmed text of the cell's <span class="program_time">.
func programTimeText(cell *html.Node) string {
	span := findDescendant(cell, func(x *html.Node) bool {
		return strings.EqualFold(x.Data, "span") && hasClass(x, "program_time")
	})
	if span == nil {
		return ""
	}
	return strings.Join(strings.Fields(textContent(span)), " ")
}

var timeTokenRe = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?(am|pm)?$`)

// parseWFMUTimeRange parses a slot range like "6-9am", "9am-Noon", "9pm-Mid", "Mid-3am",
// "10pm-Mid", "11am-1pm" into 24-hour "HH:MM" start/end. "Noon"=12:00, "Mid"=00:00. A
// bare numeric start (no am/pm) inherits the end's meridiem ("6-9am" → 06:00/09:00,
// "3-6pm" → 15:00/18:00). end<=start denotes an overnight wrap (RadioSchedule semantics).
func parseWFMUTimeRange(s string) (start, end string, ok bool) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	left, right := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])

	rightMin, rightMer, rok := parseTimeToken(right)
	if !rok {
		return "", "", false
	}
	leftMin, leftMer, lok := parseTimeToken(left)
	if !lok {
		return "", "", false
	}
	if leftMer == "" {
		if rightMer == "" {
			return "", "", false // neither side anchored — ambiguous, skip
		}
		// A bare numeric start is a same-day FORWARD range (overnight slots always carry
		// explicit meridiems, e.g. "9pm-Mid"). Choose the meridiem that keeps start < end:
		// inheriting the end's works for same-meridiem ranges ("1-3pm" → 13:00, "6-9am" →
		// 06:00); when that would reverse the range, the start is the earlier meridiem
		// ("9-3pm" → 09:00 not 21:00; "10-Noon" → 10:00). Inheriting blindly (the prior
		// bug) silently produced an ~18h reversed wrap on AM→PM crossing ranges.
		if inherited := applyMeridiem(leftMin, rightMer); inherited < rightMin {
			leftMin = inherited
		} else {
			leftMin = applyMeridiem(leftMin, otherMeridiem(rightMer))
		}
	}
	return fmtHHMM(leftMin), fmtHHMM(rightMin), true
}

// parseWFMUTimeRangeWithDefault parses a stacked-cell inline range ("3 - 3:01") whose tokens
// carry NO meridiem, anchoring each bare token to the cell's band meridiem (defaultMer, from
// the row's hour label). A token with its own am/pm keeps it. PSY-1186.
func parseWFMUTimeRangeWithDefault(s, defaultMer string) (start, end string, ok bool) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	lMin, lMer, lok := parseTimeToken(strings.TrimSpace(parts[0]))
	rMin, rMer, rok := parseTimeToken(strings.TrimSpace(parts[1]))
	if !lok || !rok {
		return "", "", false
	}
	if lMer == "" {
		lMin = applyMeridiem(lMin, defaultMer)
	}
	if rMer == "" {
		rMin = applyMeridiem(rMin, defaultMer)
	}
	return fmtHHMM(lMin), fmtHHMM(rMin), true
}

func otherMeridiem(m string) string {
	if m == "pm" {
		return "am"
	}
	return "pm"
}

// parseTimeToken returns minutes-since-midnight for a single token. meridiem is "am"/"pm"
// when the token carried one (or was Noon/Mid), else "" so the caller can infer it.
func parseTimeToken(tok string) (minutes int, meridiem string, ok bool) {
	switch strings.ToLower(tok) {
	case "noon":
		return 12 * 60, "pm", true
	case "mid", "midnight":
		return 0, "am", true
	}
	m := timeTokenRe.FindStringSubmatch(strings.ToLower(tok))
	if m == nil {
		return 0, "", false
	}
	h, _ := strconv.Atoi(m[1])
	if h < 1 || h > 12 {
		return 0, "", false
	}
	min := 0
	if m[2] != "" {
		min, _ = strconv.Atoi(m[2])
		if min > 59 {
			return 0, "", false
		}
	}
	mer := m[3]
	if mer == "" {
		return h*60 + min, "", true // meridiem unknown → caller infers
	}
	return hourToMinutes(h, mer) + min, mer, true
}

// applyMeridiem re-anchors a meridiem-less token (parsed as h*60+min) to am/pm.
func applyMeridiem(minutes int, meridiem string) int {
	h, min := minutes/60, minutes%60
	return hourToMinutes(h, meridiem) + min
}

func hourToMinutes(h12 int, meridiem string) int {
	h := h12 % 12 // 12am→0, 12pm→0 (then +12)
	if meridiem == "pm" {
		h += 12
	}
	return h * 60
}

func fmtHHMM(minutes int) string {
	minutes %= 24 * 60
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}
