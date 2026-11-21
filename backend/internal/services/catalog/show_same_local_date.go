package catalog

import (
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// This file finds approved shows that share a room, a venue-local date and a
// headliner, and says for each group whether its two copies can be merged. It
// writes nothing: a person reviews the result and merges with
// cmd/reattribute-shows, whose merge row names the copy to keep.
//
// The rule is conservative on purpose. A pair is "merge" only when every
// condition in mergeConditions holds; anything else is "review", with each
// failed condition named. Two shows the same day (a matinee and an evening
// show, an early and a late set) fail the clock test and are never merged here.
//
// Invariants a reader can check against the code below:
//
//   - The venue-local date is read with venueRowLocalDate in the room's zone,
//     the same reading ReattributeShow's merge check makes.
//   - The headliner is the act showBillHeadlinerRank ranks first, the rule the
//     slug writer and ReattributeShow use.
//   - A show at two rooms is in both rooms' groups.
//   - SameLocalDateRuleText is built from mergeConditions and the same
//     constants the checks use.

// sameLocalDateCandidateWindow bounds the coarse SQL pairing. Two instants on
// one local day in one zone are less than 25 hours apart (a day the clocks go
// back is 25 hours long), so this bound loses no pair; the exact comparison is
// made in Go.
const sameLocalDateCandidateWindow = "26 hours"

// SameShowClockSpan is the widest spread of clocks two copies of one show can
// carry: the reach of one event's doors and start, and shorter than the gap
// between an early and a late show.
const SameShowClockSpan = 90 * time.Minute

// twoShowsClockSpan is the spread at which a review row says the copies may be
// two shows that day.
const twoShowsClockSpan = 3 * time.Hour

// SameLocalDateRoom is the room a group shares.
type SameLocalDateRoom struct {
	ID    uint
	Slug  string
	Name  string
	City  string
	State string
}

// SameLocalDateClock is one time a copy stores, in the room's zone.
type SameLocalDateClock struct {
	// Label is "stored start", "music" or "doors".
	Label string
	At    time.Time
	// Local is At as "15:04 MST" in the room's zone.
	Local string
}

// SameLocalDateCopy is one show in a group.
type SameLocalDateCopy struct {
	ShowID    uint
	Slug      string
	Title     string
	EventDate time.Time
	// Clocks are every time the row stores: its stored start (event_date), and
	// its music and doors times when set.
	Clocks []SameLocalDateClock
	// StatesStart is true when the row stores a music time, or a stored start
	// other than the date-only hour.
	StatesStart bool
	// DroppedFields names the fields this row fills that a merge does not carry
	// onto the kept copy, in DroppedByMergeFields order; FieldValues holds each
	// one's value as compared.
	DroppedFields []string
	FieldValues   map[string]string
	// SourceEventID is the scraper's event id, "" for a row it did not write.
	SourceEventID string
	// SubmittedBy is the submitting user's id, 0 when none; a merge does not
	// carry it.
	SubmittedBy uint
	IsCancelled bool
	CreatedAt   time.Time
	Source      string
	DataSource  string
	Saves       int64
	Comments    int64
	// RoomCount is how many rooms the show lists.
	RoomCount int
}

// ClockText lists the copy's clocks with their labels.
func (c SameLocalDateCopy) ClockText() string {
	parts := make([]string, len(c.Clocks))
	for i, k := range c.Clocks {
		parts[i] = k.Label + " " + k.Local
	}
	return strings.Join(parts, ", ")
}

// SameLocalDateGroup is two or more approved shows at one room on one
// venue-local date with one headliner, ordered by event_date then id.
type SameLocalDateGroup struct {
	Room          SameLocalDateRoom
	LocalDate     string
	HeadlinerID   uint
	HeadlinerName string
	Copies        []SameLocalDateCopy
}

type sameLocalDateRow struct {
	ShowID         uint
	Slug           *string
	Title          string
	SourceEventID  *string
	SubmittedBy    *uint
	EventDate      time.Time
	MusicAt        *time.Time
	DoorsAt        *time.Time
	IsCancelled    bool
	IsSoldOut      bool
	Price          *float64
	DoorPrice      *float64
	AgeRequirement *string
	Description    *string
	ImageURL       *string
	CreatedAt      time.Time
	Source         string
	DataSource     *string
	TicketURL      *string
	RoomID         uint
	RoomSlug       *string
	RoomName       string
	RoomCity       string
	RoomState      string
	RoomTimezone   *string
	ArtistID       uint
	ArtistName     string
	RoomCount      int
	Saves          int64
	Comments       int64
}

// droppedFields are the optional fields of a show row a merge leaves behind on
// the merged-away copy, apart from its times (Clocks) and its title, scraper
// event id and submitter, which have fields of their own. MergeDuplicateShow
// re-points references and copies no shows column.
var droppedFields = []struct {
	name string
	// value is the field as compared, "" when the row leaves it empty.
	value func(r sameLocalDateRow) string
}{
	{"ticket link", func(r sameLocalDateRow) string { return ticketLinkKey(trimmed(r.TicketURL)) }},
	{"price", func(r sameLocalDateRow) string { return money(r.Price) }},
	{"door price", func(r sameLocalDateRow) string { return money(r.DoorPrice) }},
	{"age requirement", func(r sameLocalDateRow) string { return trimmed(r.AgeRequirement) }},
	{"description", func(r sameLocalDateRow) string { return trimmed(r.Description) }},
	{"image", func(r sameLocalDateRow) string { return trimmed(r.ImageURL) }},
	{"sold out", func(r sameLocalDateRow) string {
		if r.IsSoldOut {
			return "sold out"
		}
		return ""
	}},
}

// DroppedByMergeFields names droppedFields, in order.
var DroppedByMergeFields = func() []string {
	names := make([]string, len(droppedFields))
	for i, f := range droppedFields {
		names[i] = f.name
	}
	return names
}()

func trimmed(s *string) string { return strings.TrimSpace(shared.DerefOrEmpty(s)) }

func money(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *p)
}

// ticketLinkKey is a ticket URL reduced to its lower-cased host and its path
// without a trailing slash, so two links to one sale page compare equal.
func ticketLinkKey(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.ToLower(u.Host) + strings.TrimSuffix(u.Path, "/")
}

// FindSameLocalDateGroups lists every group of approved shows that share a
// room, a venue-local date and a headliner, latest date first. It reads inside
// a READ ONLY transaction, so the server refuses any write it might attempt.
// Comments are counted when visible.
func FindSameLocalDateGroups(db *gorm.DB) ([]SameLocalDateGroup, error) {
	var rows []sameLocalDateRow
	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Raw(`
			WITH head AS (
				SELECT DISTINCT ON (show_artists.show_id)
				       show_artists.show_id, show_artists.artist_id, artists.name AS artist_name
				FROM show_artists
				JOIN artists ON artists.id = show_artists.artist_id
				JOIN shows ON shows.id = show_artists.show_id AND shows.status = 'approved'
				ORDER BY show_artists.show_id, ` + showBillHeadlinerRank + `
			), placed AS (
				SELECT shows.id AS show_id, shows.event_date, show_venues.venue_id,
				       head.artist_id, head.artist_name
				FROM shows
				JOIN head ON head.show_id = shows.id
				JOIN show_venues ON show_venues.show_id = shows.id
			), candidates AS (
				SELECT a.* FROM placed a
				WHERE EXISTS (
					SELECT 1 FROM placed b
					WHERE b.venue_id = a.venue_id AND b.artist_id = a.artist_id AND b.show_id <> a.show_id
					  AND b.event_date > a.event_date - interval '` + sameLocalDateCandidateWindow + `'
					  AND b.event_date < a.event_date + interval '` + sameLocalDateCandidateWindow + `'
				)
			)
			SELECT s.id AS show_id, s.slug, s.title, s.source_event_id, s.submitted_by,
			       s.event_date, s.music_at, s.doors_at, s.is_cancelled, s.is_sold_out,
			       s.price, s.door_price, s.age_requirement, s.description, s.image_url,
			       s.created_at, s.source, s.data_source, s.ticket_url,
			       v.id AS room_id, v.slug AS room_slug, v.name AS room_name, v.city AS room_city,
			       v.state AS room_state, v.timezone AS room_timezone,
			       c.artist_id, c.artist_name,
			       (SELECT COUNT(*) FROM show_venues x WHERE x.show_id = s.id) AS room_count,
			       (SELECT COUNT(*) FROM user_bookmarks ub
			         WHERE ub.entity_type = 'show' AND ub.entity_id = s.id AND ub.action = 'save') AS saves,
			       (SELECT COUNT(*) FROM comments cm
			         WHERE cm.entity_type = 'show' AND cm.entity_id = s.id AND cm.visibility = 'visible') AS comments
			FROM candidates c
			JOIN shows s ON s.id = c.show_id
			JOIN venues v ON v.id = c.venue_id
			ORDER BY s.event_date, s.id, v.id
		`).Scan(&rows).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("find shows sharing a room, a venue-local date and a headliner: %w", err)
	}
	return groupSameLocalDateRows(rows), nil
}

func groupSameLocalDateRows(rows []sameLocalDateRow) []SameLocalDateGroup {
	type key struct {
		room, artist uint
		date         string
	}
	groups := map[key]*SameLocalDateGroup{}
	var order []key
	for _, r := range rows {
		k := key{room: r.RoomID, artist: r.ArtistID, date: venueRowLocalDate(r.EventDate, r.RoomTimezone, r.RoomState)}
		g, ok := groups[k]
		if !ok {
			g = &SameLocalDateGroup{
				Room: SameLocalDateRoom{ID: r.RoomID, Slug: shared.DerefOrEmpty(r.RoomSlug), Name: r.RoomName,
					City: r.RoomCity, State: r.RoomState},
				LocalDate: k.date, HeadlinerID: r.ArtistID, HeadlinerName: r.ArtistName,
			}
			groups[k] = g
			order = append(order, k)
		}
		g.Copies = append(g.Copies, copyFromRow(r))
	}

	var out []SameLocalDateGroup
	for _, k := range order {
		if g := groups[k]; len(g.Copies) >= 2 {
			out = append(out, *g)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.LocalDate != b.LocalDate {
			return a.LocalDate > b.LocalDate
		}
		if a.Room.ID != b.Room.ID {
			return a.Room.ID < b.Room.ID
		}
		return a.HeadlinerID < b.HeadlinerID
	})
	return out
}

func copyFromRow(r sameLocalDateRow) SameLocalDateCopy {
	loc := utils.EventLocation(r.RoomTimezone, r.RoomState)
	clock := func(label string, t time.Time) SameLocalDateClock {
		return SameLocalDateClock{Label: label, At: t, Local: t.In(loc).Format("15:04 MST")}
	}
	local := r.EventDate.In(loc)
	c := SameLocalDateCopy{
		ShowID:        r.ShowID,
		Slug:          shared.DerefOrEmpty(r.Slug),
		Title:         strings.TrimSpace(r.Title),
		EventDate:     r.EventDate,
		Clocks:        []SameLocalDateClock{clock("stored start", r.EventDate)},
		SourceEventID: trimmed(r.SourceEventID),
		IsCancelled:   r.IsCancelled,
		CreatedAt:     r.CreatedAt,
		Source:        r.Source,
		DataSource:    shared.DerefOrEmpty(r.DataSource),
		Saves:         r.Saves,
		Comments:      r.Comments,
		RoomCount:     r.RoomCount,
		FieldValues:   map[string]string{},
	}
	if r.SubmittedBy != nil {
		c.SubmittedBy = *r.SubmittedBy
	}
	if r.MusicAt != nil {
		c.Clocks = append(c.Clocks, clock("music", *r.MusicAt))
	}
	if r.DoorsAt != nil {
		c.Clocks = append(c.Clocks, clock("doors", *r.DoorsAt))
	}
	atDateOnlyHour := local.Hour() == utils.DateOnlyEventHour && local.Minute() == 0 && local.Second() == 0
	c.StatesStart = r.MusicAt != nil || !atDateOnlyHour
	for _, f := range droppedFields {
		if v := f.value(r); v != "" {
			c.DroppedFields = append(c.DroppedFields, f.name)
			c.FieldValues[f.name] = v
		}
	}
	return c
}

// SameLocalDateAction is what a suggestion proposes for a copy.
type SameLocalDateAction string

const (
	// SameLocalDateMerge: merge the copy into the kept one.
	SameLocalDateMerge SameLocalDateAction = "merge"
	// SameLocalDateReview: at least one merge condition failed; a person
	// decides. Every copy of such a group gets a row.
	SameLocalDateReview SameLocalDateAction = "review"
)

// mergeCondition is one condition a pair must meet to be suggested as a merge.
// check returns "" when the pair meets it, else what failed, in plain words.
type mergeCondition struct {
	text  string
	check func(copies []SameLocalDateCopy) string
}

// mergeConditions are the conditions of the merge rule, in the order they are
// printed. Every one must hold for a merge.
var mergeConditions = []mergeCondition{
	{"the group has exactly two copies, both with a slug", func(cs []SameLocalDateCopy) string {
		if len(cs) != 2 {
			return fmt.Sprintf("the group holds %d copies", len(cs))
		}
		for _, c := range cs {
			if c.Slug == "" {
				return fmt.Sprintf("show %d has no slug", c.ShowID)
			}
		}
		return ""
	}},
	{"neither copy is cancelled, or both are", func(cs []SameLocalDateCopy) string {
		if cs[0].IsCancelled != cs[1].IsCancelled {
			return "one copy is cancelled and the other is not"
		}
		return ""
	}},
	{"their titles match (ignoring case and surrounding space)", func(cs []SameLocalDateCopy) string {
		if !strings.EqualFold(strings.TrimSpace(cs[0].Title), strings.TrimSpace(cs[1].Title)) {
			return fmt.Sprintf("the titles differ (%q and %q)", cs[0].Title, cs[1].Title)
		}
		return ""
	}},
	{"they are not two different scraper events", func(cs []SameLocalDateCopy) string {
		a, b := cs[0].SourceEventID, cs[1].SourceEventID
		if a != "" && b != "" && a != b {
			return fmt.Sprintf("the scraper listed them as two events (%s and %s)", a, b)
		}
		return ""
	}},
	{"no field is filled on both with different values", func(cs []SameLocalDateCopy) string {
		var differ []string
		for _, name := range DroppedByMergeFields {
			a, aok := cs[0].FieldValues[name]
			b, bok := cs[1].FieldValues[name]
			if aok && bok && a != b {
				differ = append(differ, fmt.Sprintf("%s %s and %s", name, a, b))
			}
		}
		if len(differ) > 0 {
			return "fields differ: " + strings.Join(differ, "; ")
		}
		return ""
	}},
	{fmt.Sprintf("all their clocks (stored start, music, doors) fall within %d minutes", int(SameShowClockSpan/time.Minute)),
		func(cs []SameLocalDateCopy) string {
			if span := clockSpan(cs); span > SameShowClockSpan {
				return fmt.Sprintf("the clocks span %d minutes", int(span/time.Minute))
			}
			return ""
		}},
}

// clockSpan is the latest clock either copy stores minus the earliest.
func clockSpan(cs []SameLocalDateCopy) time.Duration {
	var first, last time.Time
	for _, c := range cs {
		for _, k := range c.Clocks {
			if first.IsZero() || k.At.Before(first) {
				first = k.At
			}
			if k.At.After(last) {
				last = k.At
			}
		}
	}
	return last.Sub(first)
}

// SameLocalDateRuleText states the rule, for every surface that prints
// suggestions. It is built from mergeConditions.
var SameLocalDateRuleText = func() string {
	conds := make([]string, len(mergeConditions))
	for i, c := range mergeConditions {
		conds[i] = fmt.Sprintf("(%d) %s", i+1, c.text)
	}
	return "Suggestions only, for a person to review. A pair is suggested as a merge only when all of these hold: " +
		strings.Join(conds, "; ") + ". Anything else is marked review, with the failed conditions named. " +
		"On a merge, the copy to keep is the one stating a start (a music time, or a stored start other than " +
		fmt.Sprintf("%02d:00", utils.DateOnlyEventHour) + "), then the one filling more of the fields a merge drops (" +
		strings.Join(DroppedByMergeFields, ", ") + "), then the older row. A merge moves saves, comments and other " +
		"references onto the kept copy and loses the merged-away copy's own fields and its URL."
}()

// SameLocalDateSuggestion is the suggestion for one copy. On a merge row, Keep
// is the copy to merge into; on a review row, Keep is the copy it is compared
// with. Span is the spread of the group's clocks.
type SameLocalDateSuggestion struct {
	Copy   SameLocalDateCopy
	Keep   SameLocalDateCopy
	Action SameLocalDateAction
	Reason string
	Span   time.Duration
	// Conditions is each condition's result, numbered as in the rule text:
	// "(n) holds" or "(n) fails: why". A group of other than two copies is
	// judged on the first condition only.
	Conditions []string
}

// SuggestSameLocalDateKeep applies the rule to a group. keep is the copy
// suggested to keep, or nil when the group is marked review, in which case
// every copy has a row.
func SuggestSameLocalDateKeep(g SameLocalDateGroup) (*SameLocalDateCopy, []SameLocalDateSuggestion) {
	if len(g.Copies) < 2 {
		return nil, nil
	}
	ranked := append([]SameLocalDateCopy(nil), g.Copies...)
	sort.SliceStable(ranked, func(i, j int) bool { return keepRanksAhead(ranked[i], ranked[j]) })
	span := clockSpan(g.Copies)
	results, failed := judgeConditions(g.Copies)

	if len(failed) > 0 {
		reason := "conditions not met: " + strings.Join(failed, "; ")
		if span >= twoShowsClockSpan {
			reason = fmt.Sprintf("the clocks span %s; these may be two shows. ", formatGap(span)) + reason
		}
		out := make([]SameLocalDateSuggestion, 0, len(g.Copies))
		for _, c := range g.Copies {
			other := ranked[0]
			if c.ShowID == other.ShowID {
				other = ranked[1]
			}
			out = append(out, SameLocalDateSuggestion{Copy: c, Keep: other, Action: SameLocalDateReview,
				Reason: reason, Span: span, Conditions: results})
		}
		return nil, out
	}

	keep, c := ranked[0], ranked[1]
	return &keep, []SameLocalDateSuggestion{{Copy: c, Keep: keep, Action: SameLocalDateMerge,
		Reason: mergeReason(keep, c), Span: span, Conditions: results}}
}

// judgeConditions returns every condition's result and, separately, the
// failed ones, numbered as in the rule text. Conditions after the first are
// judged only for a pair.
func judgeConditions(cs []SameLocalDateCopy) (results, failed []string) {
	for i, cond := range mergeConditions {
		if i > 0 && len(cs) != 2 {
			break
		}
		if why := cond.check(cs); why != "" {
			results = append(results, fmt.Sprintf("(%d) fails: %s", i+1, why))
			failed = append(failed, fmt.Sprintf("(%d) %s", i+1, why))
		} else {
			results = append(results, fmt.Sprintf("(%d) holds", i+1))
		}
	}
	return results, failed
}

// keepRanksAhead orders copies by the keep rule: a stated start, then more of
// the fields a merge drops, then the older row, then the lower id.
func keepRanksAhead(a, b SameLocalDateCopy) bool {
	if a.StatesStart != b.StatesStart {
		return a.StatesStart
	}
	if na, nb := len(a.DroppedFields), len(b.DroppedFields); na != nb {
		return na > nb
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ShowID < b.ShowID
}

func mergeReason(keep, c SameLocalDateCopy) string {
	var why string
	switch {
	case keep.StatesStart && !c.StatesStart:
		why = fmt.Sprintf("it states a start (%s), and this copy does not (%s)", keep.ClockText(), c.ClockText())
	case len(keep.DroppedFields) != len(c.DroppedFields):
		why = fmt.Sprintf("it fills more of the fields a merge drops (%s) than this copy (%s)",
			fieldList(keep.DroppedFields), fieldList(c.DroppedFields))
	case !keep.CreatedAt.Equal(c.CreatedAt):
		why = fmt.Sprintf("it is the older row (created %s), so its URL has existed longer",
			keep.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"))
	default:
		why = "both rows were created at the same instant; it has the lower id"
	}
	parts := []string{fmt.Sprintf("all %d conditions hold; keep %s: %s", len(mergeConditions), keep.Slug, why)}
	if lost := notCarried(keep, c); len(lost) > 0 {
		parts = append(parts, fmt.Sprintf("this copy fills %s, which the kept copy lacks and the merge drops: "+
			"copy it over by hand before or after the merge", strings.Join(lost, ", ")))
	}
	if c.Saves > 0 || c.Comments > 0 {
		parts = append(parts, fmt.Sprintf("this copy's %d saves and %d visible comments move to the kept copy",
			c.Saves, c.Comments))
	}
	if c.SubmittedBy != 0 && c.SubmittedBy != keep.SubmittedBy {
		parts = append(parts, fmt.Sprintf("this copy was submitted by user %d, whose credit for it the merge drops",
			c.SubmittedBy))
	}
	if c.Source == string(catalogm.ShowSourceDiscovery) {
		parts = append(parts, "this copy came from the discovery scraper, which can create it again on its next "+
			"run: the merge carries neither its source event id nor its time")
	}
	if c.RoomCount != 1 || keep.RoomCount != 1 {
		parts = append(parts, "a copy lists more than one room, and reattribute-shows merges only shows with exactly one")
	}
	return strings.Join(parts, "; ")
}

// notCarried names what the merged-away copy fills that the kept copy leaves
// empty: its music and doors times, then its fields. A merge row has no field
// both fill differently (condition 5).
func notCarried(keep, c SameLocalDateCopy) []string {
	keptLabels := map[string]bool{}
	for _, k := range keep.Clocks {
		keptLabels[k.Label] = true
	}
	var lost []string
	for _, k := range c.Clocks {
		if k.Label != "stored start" && !keptLabels[k.Label] {
			lost = append(lost, fmt.Sprintf("%s time %s", k.Label, k.Local))
		}
	}
	for _, f := range c.DroppedFields {
		if _, ok := keep.FieldValues[f]; !ok {
			lost = append(lost, f)
		}
	}
	return lost
}

func fieldList(fields []string) string {
	if len(fields) == 0 {
		return "none"
	}
	return strings.Join(fields, ", ")
}

// formatGap prints a gap as "4h30m" or "3h".
func formatGap(d time.Duration) string {
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%02dm", h, m)
}
