package catalog

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// This file finds approved shows that share a room, a venue-local date and a
// headliner, and suggests which copy of each group to keep. It writes nothing:
// a person reviews the suggestions and merges with cmd/reattribute-shows, whose
// merge row names the copy to keep. A matinee and an evening show on one day
// are two shows, so nothing here merges on its own.
//
// Invariants a reader can check against the code below:
//
//   - The venue-local date is read with venueRowLocalDate in the room's zone,
//     the same reading ReattributeShow's merge check makes, so every pair listed
//     here passes that check's same-date test.
//   - The headliner is the act showBillHeadlinerRank ranks first, the rule the
//     slug writer and ReattributeShow use.
//   - A show at two rooms is in both rooms' groups.
//   - A merge is suggested only for a pair; a group of more than two copies, a
//     pair where one copy is cancelled, and a pair where only one copy states
//     a time and the two are a matinee gap apart are left to the reviewer.

// sameLocalDateCandidateWindow bounds the coarse SQL pairing. Two instants on
// one local day in one zone are less than 25 hours apart (a day the clocks go
// back is 25 hours long), so this bound loses no pair; the exact comparison is
// made in Go.
const sameLocalDateCandidateWindow = "26 hours"

// matineeGap is how far apart two times must be for a pair to read as a
// matinee and an evening show.
const matineeGap = 3 * time.Hour

// SameLocalDateRoom is the room a group shares.
type SameLocalDateRoom struct {
	ID    uint
	Slug  string
	Name  string
	City  string
	State string
}

// SameLocalDateCopy is one show in a group.
type SameLocalDateCopy struct {
	ShowID    uint
	Slug      string
	EventDate time.Time
	// LocalTime is event_date's clock in the room's zone, "15:04 MST".
	LocalTime string
	// StartAt is the start the row states: its music time, else its own clock
	// when that is not the date-only hour. Zero when it states none.
	StartAt time.Time
	// DoorsAt is the row's doors time, zero when none.
	DoorsAt time.Time
	// Clock says, for a reviewer, which time the row states and where it came
	// from, in the room's zone.
	Clock string
	// DroppedFields names the fields this row fills that a merge does not carry
	// onto the kept copy, in DroppedByMergeFields order.
	DroppedFields []string
	IsCancelled   bool
	CreatedAt     time.Time
	Source        string
	DataSource    string
	TicketURL     string
	Saves         int64
	Comments      int64
	// RoomCount is how many rooms the show lists.
	RoomCount int
}

// StatesStart is true when the row states a start time.
func (c SameLocalDateCopy) StatesStart() bool { return !c.StartAt.IsZero() }

// statedTime is the time the matinee rule compares: the start, else the doors
// time, zero when the row states neither.
func (c SameLocalDateCopy) statedTime() time.Time {
	if c.StatesStart() {
		return c.StartAt
	}
	return c.DoorsAt
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

// droppedFields is every field of a show row a merge leaves behind on the
// merged-away copy, apart from its times, which Clock and the start rule
// cover. MergeDuplicateShow re-points references and copies no shows column.
var droppedFields = []struct {
	name   string
	filled func(r sameLocalDateRow) bool
}{
	{"ticket link", func(r sameLocalDateRow) bool { return strings.TrimSpace(shared.DerefOrEmpty(r.TicketURL)) != "" }},
	{"price", func(r sameLocalDateRow) bool { return r.Price != nil }},
	{"door price", func(r sameLocalDateRow) bool { return r.DoorPrice != nil }},
	{"age requirement", func(r sameLocalDateRow) bool { return strings.TrimSpace(shared.DerefOrEmpty(r.AgeRequirement)) != "" }},
	{"description", func(r sameLocalDateRow) bool { return strings.TrimSpace(shared.DerefOrEmpty(r.Description)) != "" }},
	{"image", func(r sameLocalDateRow) bool { return strings.TrimSpace(shared.DerefOrEmpty(r.ImageURL)) != "" }},
	{"doors time", func(r sameLocalDateRow) bool { return r.DoorsAt != nil }},
	{"sold out", func(r sameLocalDateRow) bool { return r.IsSoldOut }},
}

// DroppedByMergeFields names droppedFields, in order.
var DroppedByMergeFields = func() []string {
	names := make([]string, len(droppedFields))
	for i, f := range droppedFields {
		names[i] = f.name
	}
	return names
}()

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
			SELECT s.id AS show_id, s.slug, s.event_date, s.music_at, s.doors_at, s.is_cancelled, s.is_sold_out,
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
	clock := func(t time.Time) string { return t.In(loc).Format("15:04 MST") }
	local := r.EventDate.In(loc)
	c := SameLocalDateCopy{
		ShowID:      r.ShowID,
		Slug:        shared.DerefOrEmpty(r.Slug),
		EventDate:   r.EventDate,
		LocalTime:   clock(r.EventDate),
		IsCancelled: r.IsCancelled,
		CreatedAt:   r.CreatedAt,
		Source:      r.Source,
		DataSource:  shared.DerefOrEmpty(r.DataSource),
		TicketURL:   strings.TrimSpace(shared.DerefOrEmpty(r.TicketURL)),
		Saves:       r.Saves,
		Comments:    r.Comments,
		RoomCount:   r.RoomCount,
	}
	if r.DoorsAt != nil {
		c.DoorsAt = *r.DoorsAt
	}
	atDateOnlyHour := local.Hour() == utils.DateOnlyEventHour && local.Minute() == 0 && local.Second() == 0
	switch {
	case r.MusicAt != nil:
		c.StartAt = *r.MusicAt
		c.Clock = "music " + clock(*r.MusicAt)
		if !r.MusicAt.Equal(r.EventDate) {
			c.Clock += ", stored start " + c.LocalTime
		}
	case !atDateOnlyHour:
		c.StartAt = r.EventDate
		c.Clock = "start " + c.LocalTime
	default:
		c.Clock = c.LocalTime + " with no music time stored, so it may be the date-only anchor"
	}
	if r.DoorsAt != nil {
		c.Clock += ", doors " + clock(*r.DoorsAt)
	}
	for _, f := range droppedFields {
		if f.filled(r) {
			c.DroppedFields = append(c.DroppedFields, f.name)
		}
	}
	return c
}

// SameLocalDateAction is what a suggestion proposes for one copy.
type SameLocalDateAction string

const (
	// SameLocalDateMerge: merge the copy into the kept one.
	SameLocalDateMerge SameLocalDateAction = "merge"
	// SameLocalDateKeepBoth: the copy and the kept one look like two shows.
	SameLocalDateKeepBoth SameLocalDateAction = "keep both"
	// SameLocalDateReview: the rule makes no suggestion; a person compares the
	// copies. Every copy of such a group gets a row.
	SameLocalDateReview SameLocalDateAction = "review"
)

// SameLocalDateRuleText states the suggestion rule, for every surface that
// prints suggestions.
var SameLocalDateRuleText = fmt.Sprintf("Suggestions only, for a person to review. A merge moves the merged-away "+
	"copy's saves, comments and other references onto the kept copy, and loses that copy's own fields, its times "+
	"and its URL. So in a pair, the copy to keep is the one stating a start time (a music time, or a clock other "+
	"than the %02d:00 date-only hour), then the one holding more of the fields a merge drops (%s), then the older "+
	"row. Two copies that both state a start or doors time %s or more apart look like a matinee and an evening "+
	"show: keep both. A group of more than two copies, a pair where one copy is cancelled, and a pair %s or more "+
	"apart where only one copy states a time are marked review, with no copy suggested.",
	utils.DateOnlyEventHour, strings.Join(DroppedByMergeFields, ", "), formatGap(matineeGap), formatGap(matineeGap))

// SameLocalDateSuggestion is the suggestion for one copy. Keep is the copy to
// merge into on a merge row, the other copy on a keep-both row, and on a
// review row the copy it is compared with.
type SameLocalDateSuggestion struct {
	Copy   SameLocalDateCopy
	Keep   SameLocalDateCopy
	Action SameLocalDateAction
	Reason string
}

// SuggestSameLocalDateKeep says, for the copies of a group, which to keep and
// whether to merge each other copy into it. keep is the copy suggested to keep,
// or nil when the group is marked review, in which case every copy has a row.
// See SameLocalDateRuleText.
func SuggestSameLocalDateKeep(g SameLocalDateGroup) (*SameLocalDateCopy, []SameLocalDateSuggestion) {
	ranked := append([]SameLocalDateCopy(nil), g.Copies...)
	sort.SliceStable(ranked, func(i, j int) bool { return keepRanksAhead(ranked[i], ranked[j]) })

	if len(g.Copies) > 2 {
		slugs := make([]string, len(g.Copies))
		for i, gc := range g.Copies {
			slugs[i] = gc.Slug
		}
		reason := fmt.Sprintf("this group holds %d copies (%s); compare them all by hand, since two of them "+
			"may be separate shows that day", len(g.Copies), strings.Join(slugs, ", "))
		return nil, reviewRows(g.Copies, ranked, reason)
	}

	keep, c := ranked[0], ranked[1]
	if reason, review := pairNeedsReview(keep, c); review {
		return nil, reviewRows(g.Copies, ranked, reason)
	}
	return &keep, []SameLocalDateSuggestion{suggestFor(keep, c)}
}

// reviewRows gives every copy a review row, compared with the copy ranked
// first, or with the second for the first itself.
func reviewRows(copies, ranked []SameLocalDateCopy, reason string) []SameLocalDateSuggestion {
	out := make([]SameLocalDateSuggestion, 0, len(copies))
	for _, c := range copies {
		other := ranked[0]
		if c.ShowID == other.ShowID {
			other = ranked[1]
		}
		out = append(out, SameLocalDateSuggestion{Copy: c, Keep: other, Action: SameLocalDateReview, Reason: reason})
	}
	return out
}

func pairNeedsReview(a, b SameLocalDateCopy) (string, bool) {
	if a.IsCancelled != b.IsCancelled {
		return "one copy is cancelled and the other is not; a merge keeps the kept copy's status, so decide by hand", true
	}
	aTime, bTime := a.statedTime(), b.statedTime()
	if aTime.IsZero() != bTime.IsZero() {
		if gap := a.EventDate.Sub(b.EventDate).Abs(); gap >= matineeGap {
			return fmt.Sprintf("the copies are %s apart and only one states a time; the other's %02d:00 may be the "+
				"date-only anchor or a real %02d:00 start (the web form's default time), so these may be two shows: "+
				"decide by hand", formatGap(gap), utils.DateOnlyEventHour, utils.DateOnlyEventHour), true
		}
	}
	return "", false
}

// keepRanksAhead orders copies by the keep rule: a stated start, then more of
// the fields a merge drops, then the older row, then the lower id.
func keepRanksAhead(a, b SameLocalDateCopy) bool {
	if a.StatesStart() != b.StatesStart() {
		return a.StatesStart()
	}
	if na, nb := len(a.DroppedFields), len(b.DroppedFields); na != nb {
		return na > nb
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ShowID < b.ShowID
}

func suggestFor(keep, c SameLocalDateCopy) SameLocalDateSuggestion {
	s := SameLocalDateSuggestion{Copy: c, Keep: keep}
	kt, ct := keep.statedTime(), c.statedTime()
	if !kt.IsZero() && !ct.IsZero() {
		if gap := kt.Sub(ct).Abs(); gap >= matineeGap {
			s.Action = SameLocalDateKeepBoth
			s.Reason = fmt.Sprintf("both state a time (this copy %s; the other %s), %s apart: they look like a "+
				"matinee and an evening show", c.Clock, keep.Clock, formatGap(gap))
			return s
		}
	}

	s.Action = SameLocalDateMerge
	var why string
	switch {
	case keep.StatesStart() && !c.StatesStart():
		why = fmt.Sprintf("it states a start time (%s), and this copy does not (%s)", keep.Clock, c.Clock)
	case len(keep.DroppedFields) != len(c.DroppedFields):
		why = fmt.Sprintf("it holds more of the fields a merge drops (%s) than this copy (%s)",
			fieldList(keep.DroppedFields), fieldList(c.DroppedFields))
	case !keep.CreatedAt.Equal(c.CreatedAt):
		why = fmt.Sprintf("it is the older row (created %s), so its URL has existed longer",
			keep.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"))
	default:
		why = "both rows were created at the same instant; it has the lower id"
	}
	parts := []string{fmt.Sprintf("keep %s: %s", keep.Slug, why)}
	if keep.StatesStart() && c.StatesStart() && !keep.StartAt.Equal(c.StartAt) {
		parts = append(parts, fmt.Sprintf("the two state different times (kept copy %s; this copy %s) and the "+
			"merge keeps only the kept copy's: if this copy's is right, set it by hand after the merge",
			keep.Clock, c.Clock))
	}
	if lost := notCarried(keep, c); len(lost) > 0 {
		parts = append(parts, fmt.Sprintf("this copy holds %s, which the kept copy lacks and the merge drops: "+
			"copy it over by hand before or after the merge", strings.Join(lost, ", ")))
	}
	if c.Saves > 0 || c.Comments > 0 {
		parts = append(parts, fmt.Sprintf("this copy's %d saves and %d visible comments move to the kept copy",
			c.Saves, c.Comments))
	}
	if c.Source == string(catalogm.ShowSourceDiscovery) {
		parts = append(parts, "this copy came from the discovery scraper, which can create it again on its next "+
			"run: the merge carries neither its source event id nor its time")
	}
	if c.RoomCount != 1 || keep.RoomCount != 1 {
		parts = append(parts, "a copy lists more than one room, and reattribute-shows merges only shows with exactly one")
	}
	s.Reason = strings.Join(parts, "; ")
	return s
}

// notCarried names the fields the merged-away copy fills that the kept copy
// leaves empty.
func notCarried(keep, c SameLocalDateCopy) []string {
	kept := map[string]bool{}
	for _, f := range keep.DroppedFields {
		kept[f] = true
	}
	var lost []string
	for _, f := range c.DroppedFields {
		if !kept[f] {
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
