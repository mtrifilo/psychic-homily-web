package catalog

import (
	"fmt"
	"log/slog"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Atlas city-view rail enrichment for GET /venues.
//
// The rail renders one dense row per venue — name, upcoming count, and a meta
// line "NEXT <date> · <bill> · <genre family>" — plus the header's filter
// chips. Everything here exists to fill that row for a PAGE of venues in a
// fixed number of queries: four batched scans keyed by the page's venue IDs,
// never one query per venue.
//
// Every aggregation is BEST EFFORT. The rail's reason to exist is the venue
// list; a missing meta line degrades a row, a failed list degrades the page.
// So each helper's error is logged and swallowed by the caller, leaving the
// corresponding fields zero — the same "cosmetic data must not blank the list"
// rule ListScenes applies to its genre tint.

// venueNextShow is the soonest upcoming approved show at one venue.
type venueNextShow struct {
	// Date is the show's instant; the caller renders it in the venue's zone.
	Date time.Time
	// Title is the show's own title, empty for most shows.
	Title string
	// Artists is the bill in position order — the display name fallback.
	Artists []string
}

// venueUpcomingWeekCounts returns, per venue ID, how many of its upcoming
// approved shows fall inside the rolling next-7-days window. Same window
// length the scene list uses (sceneThisWeekDays), so a venue row's "Next 7
// days" chip and its scene's pulse agree on what the window is.
//
// Rolling from now, not Monday-to-Sunday, which is why the rail says "next 7
// days" rather than "this week" (PSY-1732).
func (s *VenueService) venueUpcomingWeekCounts(venueIDs []uint, now time.Time) (map[uint]int, error) {
	type row struct {
		VenueID uint `gorm:"column:venue_id"`
		Count   int  `gorm:"column:count"`
	}
	var rows []row
	err := s.db.Raw(`
		SELECT sv.venue_id AS venue_id, COUNT(DISTINCT s.id) AS count
		FROM show_venues sv
		JOIN shows s ON s.id = sv.show_id
		WHERE sv.venue_id IN ?
		  AND s.status = ?
		  AND s.event_date >= ?
		  AND s.event_date < ?
		GROUP BY sv.venue_id
	`, venueIDs, catalogm.ShowStatusApproved, now, now.AddDate(0, 0, sceneThisWeekDays)).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count this-week shows for venues: %w", err)
	}

	out := make(map[uint]int, len(rows))
	for _, r := range rows {
		out[r.VenueID] = r.Count
	}
	return out, nil
}

// venueNextShows returns, per venue ID, the soonest upcoming approved show.
//
// Two queries, both batched: DISTINCT ON picks one show per venue, then a
// single scan pulls those shows' bills. The (event_date, id) ordering makes
// the pick deterministic when a venue has two shows on the same instant.
func (s *VenueService) venueNextShows(venueIDs []uint, now time.Time) (map[uint]venueNextShow, error) {
	type showRow struct {
		VenueID   uint      `gorm:"column:venue_id"`
		ShowID    uint      `gorm:"column:show_id"`
		EventDate time.Time `gorm:"column:event_date"`
		Title     string    `gorm:"column:title"`
	}
	var showRows []showRow
	err := s.db.Raw(`
		SELECT DISTINCT ON (sv.venue_id)
		       sv.venue_id AS venue_id,
		       s.id        AS show_id,
		       s.event_date,
		       COALESCE(s.title, '') AS title
		FROM show_venues sv
		JOIN shows s ON s.id = sv.show_id
		WHERE sv.venue_id IN ?
		  AND s.status = ?
		  AND s.event_date >= ?
		ORDER BY sv.venue_id, s.event_date ASC, s.id ASC
	`, venueIDs, catalogm.ShowStatusApproved, now).Scan(&showRows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get next shows for venues: %w", err)
	}
	if len(showRows) == 0 {
		return map[uint]venueNextShow{}, nil
	}

	showIDs := make([]uint, 0, len(showRows))
	for _, r := range showRows {
		showIDs = append(showIDs, r.ShowID)
	}

	type artistRow struct {
		ShowID uint   `gorm:"column:show_id"`
		Name   string `gorm:"column:name"`
	}
	var artistRows []artistRow
	err = s.db.Raw(`
		SELECT sa.show_id AS show_id, a.name AS name
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		WHERE sa.show_id IN ?
		ORDER BY sa.show_id, sa.position ASC, a.name ASC
	`, showIDs).Scan(&artistRows).Error
	if err != nil {
		// The bill is the nicer half of the line, but the date still reads on
		// its own — degrade to dateless-bill rather than dropping the show.
		slog.Default().Error("venue next-show bill lookup failed; rendering dates without bills", "error", err)
		artistRows = nil
	}
	billByShow := make(map[uint][]string, len(showIDs))
	for _, r := range artistRows {
		billByShow[r.ShowID] = append(billByShow[r.ShowID], r.Name)
	}

	out := make(map[uint]venueNextShow, len(showRows))
	for _, r := range showRows {
		out[r.VenueID] = venueNextShow{
			Date:    r.EventDate,
			Title:   r.Title,
			Artists: billByShow[r.ShowID],
		}
	}
	return out, nil
}

// venueGenreWindowMonths bounds how far back the dominant-genre mass reaches.
//
// The window exists for two reasons at once. It answers the question the rail
// actually asks — "what does this venue book?", present tense, not what it
// booked when it had a different owner — and it caps the query: an all-time
// mass would grow without limit as the catalogue ages, on a join across a
// whole city's venues (a code-review finding). Two years is wide enough to
// clear the confident-dominance floor that an upcoming-only slice would miss
// on most venues, and narrow enough that a venue that changed its booking
// identity reads as what it is now.
//
// Consequence to know about: the venue page's own genre profile
// (GetVenueGenreProfile) has no window, so a venue with a long and changed
// history can show a different top genre there than the family the rail
// tints. That is two questions at two time scales, not a bug.
const venueGenreWindowMonths = 24

// venueDominantGenres returns, per venue ID, the genre family that confidently
// dominates the venue's recent bookings — or no entry when none does.
//
// The dominance RULE is shared with scenes verbatim (dominantGenreFamily): one
// confident-share test, one set of family keys, one place to retune. Only the
// MASS is venue-scoped: the distinct tagged artists on the venue's approved
// shows within venueGenreWindowMonths, upcoming ones included.
func (s *VenueService) venueDominantGenres(venueIDs []uint, now time.Time) (map[uint]string, error) {
	type row struct {
		VenueID uint   `gorm:"column:venue_id"`
		Slug    string `gorm:"column:slug"`
		Count   int    `gorm:"column:count"`
	}
	var rows []row
	err := s.db.Raw(`
		SELECT sv.venue_id AS venue_id, t.slug AS slug, COUNT(DISTINCT sa.artist_id) AS count
		FROM show_artists sa
		JOIN show_venues sv ON sv.show_id = sa.show_id
		JOIN shows s ON s.id = sa.show_id
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = sa.artist_id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE sv.venue_id IN ? AND s.status = ? AND s.event_date >= ?
		GROUP BY sv.venue_id, t.slug
	`, venueIDs, catalogm.ShowStatusApproved, now.AddDate(0, -venueGenreWindowMonths, 0)).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate venue genres: %w", err)
	}

	countsByVenue := make(map[uint][]contracts.GenreCount)
	for _, r := range rows {
		countsByVenue[r.VenueID] = append(countsByVenue[r.VenueID], contracts.GenreCount{Slug: r.Slug, Count: r.Count})
	}
	out := make(map[uint]string, len(countsByVenue))
	for venueID, counts := range countsByVenue {
		if fam := dominantGenreFamily(counts); fam != "" {
			out[venueID] = fam
		}
	}
	return out, nil
}

// venueHostsAllAges returns the subset of the given venue IDs that carry the
// canonical all-ages tag (catalogm.TagSlugAllAges) — the data behind the rail's
// "All-ages shows" chip (PSY-1573).
//
// Read off entity_tags rather than a venues column, by user decision: this is a
// crowd-maintained claim about a room, which is what the tag system is for, and
// it needed no migration. Deliberately NOT derived from venues.age_policy — see
// TagSlugAllAges — because the column is the HOUSE DEFAULT and the tag is
// "sometimes", so a 21+ room that books all-ages matinees is invisible to the
// column and visible here. That is the whole point of the chip.
//
// A venue absent from the returned set is UNTAGGED, which means "nobody has
// said", never "this room is 21+". The rail's empty state has to carry that
// distinction; the boolean on the wire cannot.
func (s *VenueService) venueHostsAllAges(venueIDs []uint) (map[uint]bool, error) {
	// LOWER(t.slug), not a bare `t.slug =`, so this agrees with ApplyTagFilter's
	// `LOWER(tags.slug) IN ?`. Those are the two read paths over the same tag —
	// this one and `GET /venues?tags=all-ages` — and a case-sensitive comparison
	// here would let them disagree about the same venue.
	var taggedIDs []uint
	err := s.db.Raw(`
		SELECT et.entity_id
		FROM entity_tags et
		JOIN tags t ON t.id = et.tag_id
		WHERE et.entity_type = ?
		  AND et.entity_id IN ?
		  AND LOWER(t.slug) = ?
	`, catalogm.TagEntityVenue, venueIDs, catalogm.TagSlugAllAges).Scan(&taggedIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to look up all-ages venue tags: %w", err)
	}

	out := make(map[uint]bool, len(taggedIDs))
	for _, id := range taggedIDs {
		out[id] = true
	}
	return out, nil
}

// venueLocalDate renders an instant as the ISO calendar date a person standing
// at the venue would call it. A show at 9pm Friday in Austin is stored as a
// Saturday-morning UTC timestamp; rendering it in UTC would put "NEXT Sat" on a
// Friday show. Unknown or unloadable zones fall back to UTC.
func venueLocalDate(t time.Time, tz *string) string {
	if tz != nil && *tz != "" {
		if loc, err := time.LoadLocation(*tz); err == nil {
			return t.In(loc).Format("2006-01-02")
		}
	}
	return t.UTC().Format("2006-01-02")
}

// enrichVenueRailFields fills the rail payload on an already-built page of
// venue responses, in place. Best effort throughout: any aggregation that
// fails is logged and skipped, leaving those fields zero.
func (s *VenueService) enrichVenueRailFields(responses []*contracts.VenueWithShowCountResponse, now time.Time) {
	if len(responses) == 0 {
		return
	}
	venueIDs := make([]uint, 0, len(responses))
	for _, r := range responses {
		venueIDs = append(venueIDs, r.ID)
	}

	weekCounts, err := s.venueUpcomingWeekCounts(venueIDs, now)
	if err != nil {
		slog.Default().Error("venue this-week counts failed; rail renders without them", "error", err)
		weekCounts = nil
	}
	nextShows, err := s.venueNextShows(venueIDs, now)
	if err != nil {
		slog.Default().Error("venue next-show lookup failed; rail renders without next dates", "error", err)
		nextShows = nil
	}
	genres, err := s.venueDominantGenres(venueIDs, now)
	if err != nil {
		slog.Default().Error("venue genre aggregation failed; rail renders untinted", "error", err)
		genres = nil
	}
	allAges, allAgesErr := s.venueHostsAllAges(venueIDs)
	if allAgesErr != nil {
		// Best effort like the rest, but NOT interchangeable with them: a
		// missing genre tints a row, while a missing all-ages answer would be
		// read as "no venue here is tagged", which is a claim about the data.
		// So this failure leaves HostsAllAges NIL rather than false, and the
		// client degrades to "we don't know" instead of asserting an absence
		// off a query that never ran.
		slog.Default().Error("venue all-ages tag lookup failed; rail reports the tag as undetermined", "error", allAgesErr)
	}

	for _, r := range responses {
		r.ShowsThisWeek = weekCounts[r.ID]
		r.DominantGenre = genres[r.ID]
		if allAgesErr == nil {
			tagged := allAges[r.ID]
			r.HostsAllAges = &tagged
		}
		if next, ok := nextShows[r.ID]; ok {
			r.NextShowDate = venueLocalDate(next.Date, r.Timezone)
			r.NextShowTitle = next.Title
			r.NextShowArtists = next.Artists
		}
	}
}
