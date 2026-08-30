package catalog

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// GetShowTimeline returns the archive facts behind the show page's two corridor
// modules: the headliner's adjacent dates, and each billed act's most recent
// prior date in this show's place.
//
// Three decisions worth stating, because each has a plausible-looking wrong
// version:
//
//   - ADJACENCY IS AN INSTANT COMPARISON, not a venue-local one. The neighbours
//     are at other rooms in other zones, so there is no single calendar to
//     order them on; `(event_date, id)` is a total order and every zone agrees
//     on it. The venue-local partition that GetShowsForArtist uses answers a
//     different question ("has this date happened yet"), which is about now.
//   - THE PLACE IS THE METRO when the room has one, and the room's city+state
//     otherwise. A band that played Evanston last year has played this metro,
//     and metro is the grouping every other scene surface uses; the city
//     fallback is what keeps the module working for the non-US and
//     not-yet-backfilled rooms where venues.metro is NULL.
//   - THE HEADLINER IS RESOLVED FOR DISPLAY, matching the rule the page's own
//     heading uses. See ShowTimelineResponse.HeadlinerArtistID for why this is
//     not headlineSlotSQL.
//
// Every module hides itself on its own evidence, so a show with no venue, no
// bill, or no history is a 200 with empty fields rather than an error. Only an
// unknown show is not-found — as is a non-approved one, which this anonymous
// surface must not distinguish from a show that does not exist.
func (s *ShowService) GetShowTimeline(idOrSlug string) (*contracts.ShowTimelineResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	subject, err := s.resolveTimelineSubject(idOrSlug)
	if err != nil {
		return nil, err
	}

	bill, err := s.loadTimelineBill(subject.ShowID)
	if err != nil {
		return nil, err
	}

	timeline := &contracts.ShowTimelineResponse{
		Recurrence: []contracts.ShowTimelineRecurrence{},
	}
	if len(bill) == 0 {
		return timeline, nil
	}

	// Bill order is headliner-first (see loadTimelineBill's ORDER BY), so the
	// spine's act is row zero rather than a second resolution rule.
	timeline.HeadlinerArtistID = bill[0].ArtistID

	previous, next, err := s.loadAdjacentShows(bill[0].ArtistID, subject)
	if err != nil {
		return nil, err
	}
	timeline.Previous = previous
	timeline.Next = next

	timeline.Recurrence, err = s.loadBillRecurrence(bill, subject)
	if err != nil {
		return nil, err
	}
	return timeline, nil
}

// timelineSubject is the subject show reduced to what the timeline needs: its
// identity, its instant, and the place that instant happened in.
type timelineSubject struct {
	ShowID     uint      `gorm:"column:show_id"`
	EventDate  time.Time `gorm:"column:event_date"`
	VenueMetro string    `gorm:"column:venue_metro"`
	VenueCity  string    `gorm:"column:venue_city"`
	VenueState string    `gorm:"column:venue_state"`
}

// hasPlace reports whether the subject sits somewhere the archive can be asked
// about. A show with no venue, or a venue with neither a metro nor a usable
// city+state, has no place to match against, and every recurrence lookup for it
// would be a scan that can only return rows from everywhere.
//
// TrimSpace rather than `== ""` so this agrees with the TRIM() the place
// predicate itself applies: a whitespace-only city would otherwise pass here and
// then match every other blank-city room in the state.
func (t *timelineSubject) hasPlace() bool {
	return strings.TrimSpace(t.VenueMetro) != "" ||
		(strings.TrimSpace(t.VenueCity) != "" && strings.TrimSpace(t.VenueState) != "")
}

// timelineBillArtist is one act on the subject's bill, with the location fields
// the hometown test reads.
type timelineBillArtist struct {
	ArtistID uint   `gorm:"column:artist_id"`
	City     string `gorm:"column:city"`
	State    string `gorm:"column:state"`
	Metro    string `gorm:"column:metro"`
}

// isHometownOf reports whether this act is BASED in the subject's place.
//
// Two INDEPENDENT positive signals, either of which is sufficient: matching
// metros, or a matching city+state. Not a single rule with a fallback, because
// the metro column is populated by a manually-run backfill and is NULL on rows
// it has not reached — requiring metro agreement would call a Chicago band
// playing Chicago a visitor for as long as either row is unbackfilled, and
// requiring a city match would do the same to the Oak Park band the metro
// grouping exists to include.
func (a *timelineBillArtist) isHometownOf(subject *timelineSubject) bool {
	metro := strings.TrimSpace(a.Metro)
	if metro != "" && metro == strings.TrimSpace(subject.VenueMetro) {
		return true
	}
	city, state := strings.TrimSpace(a.City), strings.TrimSpace(a.State)
	if city == "" || state == "" {
		return false
	}
	return strings.EqualFold(city, strings.TrimSpace(subject.VenueCity)) &&
		strings.EqualFold(state, strings.TrimSpace(subject.VenueState))
}

// timelineEntryRow is the scan target shared by the adjacent and recurrence
// queries: one dated row, its room, and the artist it was selected for.
//
// ArtistID and Side are populated by one query each and left zero by the other;
// both queries select the same venue columns, so one row type keeps the
// entry-building rule in one place.
type timelineEntryRow struct {
	ArtistID  uint      `gorm:"column:artist_id"`
	Side      string    `gorm:"column:side"`
	ShowID    uint      `gorm:"column:show_id"`
	ShowSlug  string    `gorm:"column:show_slug"`
	EventDate time.Time `gorm:"column:event_date"`
	VenueName string    `gorm:"column:venue_name"`
	VenueSlug string    `gorm:"column:venue_slug"`
	City      string    `gorm:"column:city"`
	State     string    `gorm:"column:state"`
	Timezone  string    `gorm:"column:timezone"`
}

// entry resolves the row's display clock and returns the public shape.
//
// EventLocation is called here rather than left to the client for the reason
// stated on ShowTimelineEntry: each row names a different room, and its
// precedence (the venue's IANA zone, then the US state map, then the Arizona
// default) is the same one every other surface dates a show on.
func (r *timelineEntryRow) entry() *contracts.ShowTimelineEntry {
	return &contracts.ShowTimelineEntry{
		ShowID:    r.ShowID,
		ShowSlug:  r.ShowSlug,
		EventDate: r.EventDate,
		VenueName: r.VenueName,
		VenueSlug: r.VenueSlug,
		City:      r.City,
		State:     r.State,
		Timezone:  utils.EventLocation(&r.Timezone, r.State).String(),
	}
}

// resolveTimelineSubject loads the subject show by numeric id or slug.
//
// The status filter is in the WHERE clause rather than a check on the loaded
// row so a hidden show and a nonexistent one are indistinguishable from the
// outside, matching the also-tonight rail and the public ICS export.
//
// Two statements rather than `s.id = ? OR s.slug = ?` so the lookups cannot
// cross-match: a purely numeric slug has to keep meaning the id, as it does on
// every other /shows/{show_id} route.
//
// The room is picked PLACEABLE-FIRST, which is the one place this file departs
// from shared.PrimaryVenueLateralSQL's lowest-venue-id rule. A show can be
// billed at more than one room, and this lateral answers "where did this
// happen" rather than "which room owns this show": under the plain id rule an
// unplaceable room ("A Secret Location", no city, no metro) with the lower id
// wins the pick, hasPlace then reports no place, and every act's recurrence is
// dropped on a show whose other room would have answered.
// resolveAlsoTonightSubject demotes the same rows for the same reason.
//
// Only the PLACE comes from here; the room the page NAMES is the venue module's,
// so a differing pick cannot put two venue names on one screen.
//
// The show row is the fallback for a bill with no room at all. `shows.city` and
// `shows.state` are denormalized and can lag an edit, so the venue wins wherever
// both exist, which is the precedence showTimingInput applies on the page.
// Without it a roomless show has no place at all, which silently empties its
// recurrence.
func (s *ShowService) resolveTimelineSubject(idOrSlug string) (*timelineSubject, error) {
	const placeableVenue = `(
			SELECT iv.metro, iv.city, iv.state
			FROM show_venues sv
			JOIN venues iv ON iv.id = sv.venue_id
			WHERE sv.show_id = s.id
			ORDER BY (iv.city IS NULL OR TRIM(iv.city) = '' OR iv.state IS NULL OR TRIM(iv.state) = '') ASC,
			         sv.venue_id ASC
			LIMIT 1
		)`
	selectSubject := `
		SELECT s.id AS show_id,
		       s.event_date,
		       COALESCE(v.metro, '')          AS venue_metro,
		       COALESCE(v.city, s.city, '')   AS venue_city,
		       COALESCE(v.state, s.state, '') AS venue_state
		FROM shows s
		LEFT JOIN LATERAL ` + placeableVenue + ` v ON TRUE
		WHERE `
	const (
		andApproved = ` AND s.status = ? LIMIT 1`
		byID        = "s.id = ?"
		bySlug      = "s.slug = ?"
	)

	// The predicate is one of two compile-time literals; the caller's value is
	// always a bind arg, never interpolated.
	predicate, addressArg := bySlug, any(idOrSlug)
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		predicate, addressArg = byID, any(uint(id))
	}

	var subject timelineSubject
	err := s.db.Raw(selectSubject+predicate+andApproved, addressArg, catalogm.ShowStatusApproved).Scan(&subject).Error
	if err != nil {
		return nil, fmt.Errorf("failed to resolve show: %w", err)
	}
	if subject.ShowID == 0 {
		return nil, apperrors.ErrShowNotFound(0)
	}
	return &subject, nil
}

// loadTimelineBill returns the subject's bill in DISPLAY order — curated
// headliners first, then bill position — with the location fields the hometown
// test reads.
//
// The ordering is the SQL twin of the frontend's splitBill, and deliberately so:
// the spine names an act, and it has to be the act the page's own heading
// printed as the lead. Both prefer a curated 'headliner' and otherwise take the
// lowest position, breaking a tie on artist id.
//
// The set_type read is COALESCEd because the column is nullable at the schema
// level (VARCHAR(50) DEFAULT 'performer', no NOT NULL). A bare
// `set_type = 'headliner' DESC` would sort NULLs first in Postgres and let a
// row that states nothing outrank the real headliner.
func (s *ShowService) loadTimelineBill(showID uint) ([]timelineBillArtist, error) {
	const query = `
		SELECT sa.artist_id,
		       COALESCE(a.city, '')  AS city,
		       COALESCE(a.state, '') AS state,
		       COALESCE(a.metro, '') AS metro
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		WHERE sa.show_id = ?
		ORDER BY CASE WHEN COALESCE(sa.set_type, '') = ? THEN 0 ELSE 1 END,
		         sa.position ASC,
		         sa.artist_id ASC`

	var bill []timelineBillArtist
	if err := s.db.Raw(query, showID, contracts.SetTypeHeadliner).Scan(&bill).Error; err != nil {
		return nil, fmt.Errorf("failed to load show bill: %w", err)
	}
	return bill, nil
}

// loadAdjacentShows returns the headliner's own dates either side of the
// subject, in ONE round trip.
//
// The bound is the row comparison `(event_date, id)`, which is both the
// ordering and the self-exclusion: the subject is neither strictly before nor
// strictly after itself, so no separate `id <> ?` is needed, and two shows
// sharing an instant still have a deterministic side.
//
// The CTE carries `s.city`/`s.state` so a roomless neighbour still resolves a
// place and, through it, a clock. Without them EventLocation sees an empty zone
// AND an empty state and falls to its default, dating that show differently
// here than its own page dates it from the same show row.
func (s *ShowService) loadAdjacentShows(artistID uint, subject *timelineSubject) (previous, next *contracts.ShowTimelineEntry, err error) {
	query := `
		WITH neighbour AS (
			(SELECT s.id, COALESCE(s.slug, '') AS slug, s.event_date, s.city, s.state, 'previous' AS side
			   FROM show_artists sa
			   JOIN shows s ON s.id = sa.show_id
			  WHERE sa.artist_id = ? AND s.status = ?
			    AND (s.event_date, s.id) < (?, ?)
			  ORDER BY s.event_date DESC, s.id DESC
			  LIMIT 1)
			UNION ALL
			(SELECT s.id, COALESCE(s.slug, '') AS slug, s.event_date, s.city, s.state, 'next' AS side
			   FROM show_artists sa
			   JOIN shows s ON s.id = sa.show_id
			  WHERE sa.artist_id = ? AND s.status = ?
			    AND (s.event_date, s.id) > (?, ?)
			  ORDER BY s.event_date ASC, s.id ASC
			  LIMIT 1)
		)
		SELECT n.side,
		       n.id   AS show_id,
		       n.slug AS show_slug,
		       n.event_date,
		       COALESCE(v.name, '')           AS venue_name,
		       COALESCE(v.slug, '')           AS venue_slug,
		       COALESCE(v.city, n.city, '')   AS city,
		       COALESCE(v.state, n.state, '') AS state,
		       COALESCE(v.timezone, '')       AS timezone
		FROM neighbour n
		-- Raw columns, defaulted at the select above: this lateral is LEFT
		-- joined, so a neighbour with no room matches no lateral row at all and
		-- every column arrives NULL regardless of what the lateral coalesces
		-- internally. Anchored on the CTE's id because ` + "`s`" + ` is out of scope here.
		LEFT JOIN LATERAL ` + shared.PrimaryVenueLateralSQL(
		"iv.name, iv.slug, iv.city, iv.state, iv.timezone",
		"n.id",
	) + ` v ON TRUE`

	var rows []timelineEntryRow
	if err := s.db.Raw(query,
		artistID, catalogm.ShowStatusApproved, subject.EventDate, subject.ShowID,
		artistID, catalogm.ShowStatusApproved, subject.EventDate, subject.ShowID,
	).Scan(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to load adjacent shows: %w", err)
	}

	for i := range rows {
		switch rows[i].Side {
		case "previous":
			previous = rows[i].entry()
		case "next":
			next = rows[i].entry()
		}
	}
	return previous, next, nil
}

// loadBillRecurrence returns, for every act with something to say, whether this
// is its home place and its most recent prior date here.
//
// ONE query for the whole bill: a per-artist lookup would be a query per act on
// a page that already fetches a bill, and a five-band bill is the common case.
// DISTINCT ON picks each act's latest row under the same `(event_date, id)`
// order the adjacency uses, so "most recent" means the same thing in both.
//
// Acts with neither fact are dropped rather than emitted empty — an entry that
// states nothing is not a fact the client can render, and filtering here is what
// lets an empty slice mean "the module has nothing to show".
func (s *ShowService) loadBillRecurrence(bill []timelineBillArtist, subject *timelineSubject) ([]contracts.ShowTimelineRecurrence, error) {
	recurrence := make([]contracts.ShowTimelineRecurrence, 0, len(bill))

	lastPlayed := map[uint]*contracts.ShowTimelineEntry{}
	// No place to ask about means no prior-date lookup, but the hometown test
	// below still runs: it can match on metro alone, and a metro-less subject
	// simply fails it.
	if subject.hasPlace() {
		var err error
		if lastPlayed, err = s.loadLastPlayedInPlace(bill, subject); err != nil {
			return nil, err
		}
	}

	for i := range bill {
		isHometown := bill[i].isHometownOf(subject)
		entry := lastPlayed[bill[i].ArtistID]
		if !isHometown && entry == nil {
			continue
		}
		recurrence = append(recurrence, contracts.ShowTimelineRecurrence{
			ArtistID:   bill[i].ArtistID,
			IsHometown: isHometown,
			LastPlayed: entry,
		})
	}
	return recurrence, nil
}

// loadLastPlayedInPlace returns each act's most recent approved date in the
// subject's place, keyed by artist id.
//
// The place predicate is the metro when the subject's room has one and its
// city+state otherwise, chosen here rather than expressed as one OR so the
// planner gets a single indexed equality on venues.metro on the common path
// instead of a predicate it cannot use either arm of. The city arm TRIMs and
// case-folds both sides, because city/state are free text on the venue.
//
// An inner JOIN on the venue lateral, not a LEFT one: a show with no room
// cannot have happened in this place, so it must not survive the join and
// occupy an act's single DISTINCT ON slot.
func (s *ShowService) loadLastPlayedInPlace(bill []timelineBillArtist, subject *timelineSubject) (map[uint]*contracts.ShowTimelineEntry, error) {
	artistIDs := make([]uint, len(bill))
	for i := range bill {
		artistIDs[i] = bill[i].ArtistID
	}

	placeSQL, placeArgs := timelinePlacePredicate(subject)
	query := `
		SELECT DISTINCT ON (sa.artist_id)
		       sa.artist_id,
		       s.id   AS show_id,
		       COALESCE(s.slug, '') AS show_slug,
		       s.event_date,
		       v.name AS venue_name,
		       v.slug AS venue_slug,
		       v.city,
		       v.state,
		       v.timezone
		FROM show_artists sa
		JOIN shows s ON s.id = sa.show_id
		-- Carries metro because the place predicate below reads it. An INNER
		-- join, so a roomless show cannot survive to occupy an act's slot.
		JOIN LATERAL ` + shared.PrimaryVenueLateralSQL(
		"iv.name, COALESCE(iv.slug, '') AS slug, COALESCE(iv.city, '') AS city, COALESCE(iv.state, '') AS state, COALESCE(iv.timezone, '') AS timezone, iv.metro",
		"s.id",
	) + ` v ON TRUE
		WHERE sa.artist_id IN ?
		  AND s.status = ?
		  AND (s.event_date, s.id) < (?, ?)
		  AND ` + placeSQL + `
		ORDER BY sa.artist_id, s.event_date DESC, s.id DESC`

	args := []any{artistIDs, catalogm.ShowStatusApproved, subject.EventDate, subject.ShowID}
	args = append(args, placeArgs...)

	var rows []timelineEntryRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load bill recurrence: %w", err)
	}

	byArtist := make(map[uint]*contracts.ShowTimelineEntry, len(rows))
	for i := range rows {
		byArtist[rows[i].ArtistID] = rows[i].entry()
	}
	return byArtist, nil
}

// timelinePlacePredicate renders "this room is in the subject's place" as a
// bound SQL condition over the venue lateral aliased `v`.
//
// Callers must have checked timelineSubject.hasPlace; without a place this
// would render a predicate matching every room with a blank city.
func timelinePlacePredicate(subject *timelineSubject) (string, []any) {
	if metro := strings.TrimSpace(subject.VenueMetro); metro != "" {
		return "v.metro = ?", []any{metro}
	}
	return "LOWER(TRIM(v.city)) = LOWER(?) AND LOWER(TRIM(v.state)) = LOWER(?)",
		[]any{strings.TrimSpace(subject.VenueCity), strings.TrimSpace(subject.VenueState)}
}
