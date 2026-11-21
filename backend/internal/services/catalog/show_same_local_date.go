package catalog

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

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

// sameLocalDateCandidateWindow bounds the coarse SQL pairing. Two instants on
// one local day in one zone are less than 25 hours apart (a day the clocks go
// back is 25 hours long), so this bound loses no pair; the exact comparison is
// made in Go.
const sameLocalDateCandidateWindow = "26 hours"

// matineeGap is how far apart two stated times must be for a pair to read as a
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
	// LocalTime is the show's clock in the room's zone, "15:04 MST".
	LocalTime string
	// DateOnlyAnchor is true when the show stores no music time and its clock
	// reads exactly 20:00 venue-local, the time a show listed with a date and no
	// time is written at. Such a clock may also be a real 8pm start; nothing in
	// the row tells the two apart.
	DateOnlyAnchor bool
	CreatedAt      time.Time
	Source         string
	DataSource     string
	TicketURL      string
	Saves          int64
	Comments       int64
	// RoomCount is how many rooms the show lists.
	RoomCount int
}

// statesTime is the opposite of DateOnlyAnchor.
func (c SameLocalDateCopy) statesTime() bool { return !c.DateOnlyAnchor }

func (c SameLocalDateCopy) engaged() bool { return c.Saves > 0 || c.Comments > 0 }

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
	ShowID       uint
	Slug         *string
	EventDate    time.Time
	MusicAt      *time.Time
	CreatedAt    time.Time
	Source       string
	DataSource   *string
	TicketURL    *string
	RoomID       uint
	RoomSlug     *string
	RoomName     string
	RoomCity     string
	RoomState    string
	RoomTimezone *string
	ArtistID     uint
	ArtistName   string
	RoomCount    int
	Saves        int64
	Comments     int64
}

// FindSameLocalDateGroups lists every group of approved shows that share a
// room, a venue-local date and a headliner, latest date first. It reads inside
// a READ ONLY transaction, so the server refuses any write it might attempt.
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
			SELECT s.id AS show_id, s.slug, s.event_date, s.music_at, s.created_at,
			       s.source, s.data_source, s.ticket_url,
			       v.id AS room_id, v.slug AS room_slug, v.name AS room_name, v.city AS room_city,
			       v.state AS room_state, v.timezone AS room_timezone,
			       c.artist_id, c.artist_name,
			       (SELECT COUNT(*) FROM show_venues x WHERE x.show_id = s.id) AS room_count,
			       (SELECT COUNT(*) FROM user_bookmarks ub
			         WHERE ub.entity_type = 'show' AND ub.entity_id = s.id AND ub.action = 'save') AS saves,
			       (SELECT COUNT(*) FROM comments cm
			         WHERE cm.entity_type = 'show' AND cm.entity_id = s.id) AS comments
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
		local := r.EventDate.In(utils.EventLocation(r.RoomTimezone, r.RoomState))
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
		g.Copies = append(g.Copies, SameLocalDateCopy{
			ShowID:    r.ShowID,
			Slug:      shared.DerefOrEmpty(r.Slug),
			EventDate: r.EventDate,
			LocalTime: local.Format("15:04 MST"),
			DateOnlyAnchor: r.MusicAt == nil && local.Hour() == utils.DateOnlyEventHour &&
				local.Minute() == 0 && local.Second() == 0,
			CreatedAt:  r.CreatedAt,
			Source:     r.Source,
			DataSource: shared.DerefOrEmpty(r.DataSource),
			TicketURL:  strings.TrimSpace(shared.DerefOrEmpty(r.TicketURL)),
			Saves:      r.Saves,
			Comments:   r.Comments,
			RoomCount:  r.RoomCount,
		})
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

// SameLocalDateAction is what a suggestion proposes for one copy.
type SameLocalDateAction string

const (
	// SameLocalDateMerge: merge the copy into the kept one.
	SameLocalDateMerge SameLocalDateAction = "merge"
	// SameLocalDateKeepBoth: the copy and the kept one look like two shows.
	SameLocalDateKeepBoth SameLocalDateAction = "keep both"
)

// SameLocalDateRuleText states the suggestion rule, for every surface that
// prints suggestions.
const SameLocalDateRuleText = "Suggestions only, for a person to review. The copy to keep is the one with " +
	"saves or comments, then the one with a stated time over the 20:00 date-only anchor, then the older row. " +
	"Two copies that both state a time three or more hours apart look like a matinee and an evening show: keep both."

// SameLocalDateSuggestion is the suggestion for one copy of a group other than
// the one suggested to keep.
type SameLocalDateSuggestion struct {
	Copy   SameLocalDateCopy
	Keep   SameLocalDateCopy
	Action SameLocalDateAction
	Reason string
}

// SuggestSameLocalDateKeep picks the copy of a group to keep and says, for
// every other copy, whether to merge it into that one and why. See
// SameLocalDateRuleText.
func SuggestSameLocalDateKeep(g SameLocalDateGroup) (SameLocalDateCopy, []SameLocalDateSuggestion) {
	ranked := append([]SameLocalDateCopy(nil), g.Copies...)
	sort.SliceStable(ranked, func(i, j int) bool { return keepRanksAhead(ranked[i], ranked[j]) })
	keep := ranked[0]

	var out []SameLocalDateSuggestion
	for _, c := range g.Copies {
		if c.ShowID == keep.ShowID {
			continue
		}
		out = append(out, suggestFor(keep, c))
	}
	return keep, out
}

// keepRanksAhead orders copies by the keep rule: saves or comments, then a
// stated time, then the older row, then the lower id.
func keepRanksAhead(a, b SameLocalDateCopy) bool {
	if a.engaged() != b.engaged() {
		return a.engaged()
	}
	if a.statesTime() != b.statesTime() {
		return a.statesTime()
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ShowID < b.ShowID
}

func suggestFor(keep, c SameLocalDateCopy) SameLocalDateSuggestion {
	s := SameLocalDateSuggestion{Copy: c, Keep: keep}
	gap := keep.EventDate.Sub(c.EventDate).Abs()
	if keep.statesTime() && c.statesTime() && gap >= matineeGap {
		s.Action = SameLocalDateKeepBoth
		s.Reason = fmt.Sprintf("both state a time, %s and %s, %s apart: they look like a matinee and an evening show",
			c.LocalTime, keep.LocalTime, formatGap(gap))
		return s
	}

	s.Action = SameLocalDateMerge
	var why string
	switch {
	case keep.engaged() && !c.engaged():
		why = fmt.Sprintf("it has %d saves and %d comments, and this copy has none", keep.Saves, keep.Comments)
	case keep.statesTime() && !c.statesTime():
		why = fmt.Sprintf("it states a time (%s), and this copy is at the 20:00 date-only anchor", keep.LocalTime)
	case !keep.CreatedAt.Equal(c.CreatedAt):
		why = fmt.Sprintf("it is the older row (created %s)", keep.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"))
	default:
		why = "both rows were created at the same instant; it has the lower id"
	}
	parts := []string{fmt.Sprintf("keep %s: %s", keep.Slug, why)}
	if keep.engaged() && c.engaged() {
		parts = append(parts, "both copies have saves or comments; the merge moves this copy's onto the kept one")
	}
	if c.statesTime() && !keep.statesTime() {
		parts = append(parts, fmt.Sprintf("this copy's time (%s) is not carried by the merge; the kept copy stays at %s",
			c.LocalTime, keep.LocalTime))
	}
	if c.TicketURL != "" && keep.TicketURL == "" {
		parts = append(parts, "this copy's ticket link is not carried by the merge")
	}
	if c.RoomCount != 1 || keep.RoomCount != 1 {
		parts = append(parts, "a copy lists more than one room, and reattribute-shows merges only shows with exactly one")
	}
	s.Reason = strings.Join(parts, "; ")
	return s
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
