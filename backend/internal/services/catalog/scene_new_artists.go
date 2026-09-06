package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The scene page's latest-additions module (PSY-1781, redefined by PSY-1844).
//
// THREE definitions of "new to the scene" have now been in play, and the module
// serves the third:
//
//  1. PULSE — bands whose FIRST approved show falls in a trailing window
//     (GetSceneDetail's new_artists_30d). Answers "who started playing here",
//     which makes a "first listed" date on the row a lie. Still rejected.
//  2. DIGEST — bands whose catalog row was CREATED inside a trailing window
//     (GetSceneNewArtistsSince). PSY-1781 chose this and pinned it by test.
//  3. LATEST — the N most recently listed bands on the scene's roster, with no
//     window at all. What this file now serves.
//
// (2) was measured empty on 5 of 6 major scenes and abandoned for a reason that
// constrains any future edit here: scene rosters do not grow continuously.
// Locally-based artists arrive in human-run seeding batches, so ANY trailing
// window on created_at reports on when someone last ran a seeding CLI rather
// than on the scene, and reads zero for whatever stretch separates two batches.
// Do not reintroduce one.
//
// Without the window the module is empty only when the scene genuinely has no
// bands based in it. The date the row prints is still the fact the ordering
// selected on — first_listed_at is what the sort key reads — which is the
// property PSY-1781 was protecting and the reason definition (1) is still
// refused.
//
// This DELIBERATELY no longer delegates to GetSceneNewArtistsSince. PSY-1781
// routed through it so the module and the weekly digest could not disagree; the
// two now differ by design, because the digest must stay windowed (it advances
// a per-follow cursor to `now` after each send, so a band outside the window has
// already been reported or will be) while a page module has no cursor and no
// send. Sharing one query would force one of the two surfaces to be wrong.

// sceneLatestArtistsDefaultLimit is how many bands the module names when the
// caller asks for no particular number. The section is an index into the
// roster, not the roster itself — the full list is the scene page's own
// bands-based-here module — so this stays small enough to read at a glance.
const sceneLatestArtistsDefaultLimit = 5

// GetSceneLatestArtists returns the scene's most recently listed bands, newest
// first, capped at `limit` — the scene page's latest-additions module.
//
// Membership is the roster scope (artistPredicate), the same set
// GetActiveArtists paginates, so the module can only ever name a band the
// scene page also lists. Ordering is the catalog row's own created_at, which
// the caller publishes as first_listed_at: the row states the fact it was
// selected on. Created-at rather than updated-at, unchanged from PSY-1342 —
// updated-at would re-surface a long-established band on any edit.
//
// Like the windowed method it replaces, this does NOT gate on the scene's venue
// count: a scene that dips below the threshold still has a real roster, and a
// module that can be empty must render empty rather than 404 (the empty list is
// a state of a page that exists, not a missing page).
//
// The cost is two queries regardless of roster size: the bands, then one
// batched lookup for their shows.
func (s *SceneService) GetSceneLatestArtists(city, state string, now time.Time, limit int) ([]contracts.SceneNewArtistRow, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if limit <= 0 {
		limit = sceneLatestArtistsDefaultLimit
	}

	scope, err := s.scopeFor(city, state)
	if err != nil {
		return nil, err
	}
	ap, aargs := s.artistPredicate(scope, "a")

	type row struct {
		ID        uint      `gorm:"column:id"`
		Slug      string    `gorm:"column:slug"`
		Name      string    `gorm:"column:name"`
		CreatedAt time.Time `gorm:"column:created_at"`
	}
	// TWIN QUERY: GetSceneNewArtistsSince (scene.go) selects the same columns in
	// the same order from the same roster predicate, differing only by its
	// created_at window. The projection, the COALESCE on the nullable slug, the
	// ordering and the created_at -> FirstListedAt mapping must move together in
	// both places; only the window is allowed to differ. They are deliberately
	// not one query — see the file header — but they are one shape.
	//
	// id DESC is the deterministic tiebreak, and it is load-bearing rather than
	// tidy: a seeding batch writes many rows inside the same second, so
	// created_at alone leaves the cap free to return a different subset of one
	// batch on every request.
	args := append(append([]any{}, aargs...), limit)
	var rows []row
	if err := s.db.Raw(`
		SELECT a.id, COALESCE(a.slug, '') AS slug, a.name, a.created_at
		FROM artists a
		WHERE `+ap+`
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT ?
	`, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get scene latest artists: %w", err)
	}
	if len(rows) == 0 {
		return []contracts.SceneNewArtistRow{}, nil
	}

	ids := make([]uint, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	// The show lookup's failure is NOT swallowed the way the venue rail's
	// cosmetic aggregations are: a nil Show is a claim ("this band has no
	// booking yet"), so degrading a fault into it would publish a falsehood
	// about every row. The names come from the same database, so a failure
	// here is not a partial-data case worth rendering.
	showByArtist, err := s.newArtistShows(ids, now)
	if err != nil {
		return nil, err
	}

	out := make([]contracts.SceneNewArtistRow, len(rows))
	for i, r := range rows {
		out[i] = contracts.SceneNewArtistRow{
			SceneNewArtist: contracts.SceneNewArtist{
				ID:            r.ID,
				Slug:          r.Slug,
				Name:          r.Name,
				FirstListedAt: r.CreatedAt,
			},
		}
		if show, ok := showByArtist[r.ID]; ok {
			out[i].Show = &show
		}
	}
	return out, nil
}

// newArtistShows returns, per artist ID, the ONE show that best answers "where
// can I see this band": its soonest upcoming approved show, or — when it has
// none — its most recent past one, flagged IsUpcoming=false.
//
// The past fallback is deliberate. A recently listed band whose only show has
// already happened is the common case in a sparse scene, and dropping the show
// entirely would leave the row a bare name; naming the room it played is still
// the local fact the module exists to carry. IsUpcoming lets the client choose
// its tense rather than guessing from the date.
//
// CANCELLED shows are excluded outright, in both halves. This row is a single
// unlabelled line ("Nile Theater, Aug 20"), not a show listing with status
// badges, so a cancelled show here would read as a date you can turn up to and
// a past cancelled one as a gig that happened. A band whose only show is
// cancelled falls back to its previous real show, or to no show at all. Sold-out
// is NOT filtered — a sold-out show is still a true statement about where the
// band plays, where a cancelled one is not.
//
// One DISTINCT ON per artist, ordered so upcoming beats past, then soonest
// among upcoming and latest among past, with the show id as the deterministic
// tiebreak. The venue is the alphabetically-first room on the bill, matching
// how GetSceneShowsInRange picks a display venue for a multi-room show.
func (s *SceneService) newArtistShows(artistIDs []uint, now time.Time) (map[uint]contracts.SceneNewArtistShow, error) {
	type showRow struct {
		ArtistID      uint      `gorm:"column:artist_id"`
		ID            uint      `gorm:"column:id"`
		Slug          string    `gorm:"column:slug"`
		EventDate     time.Time `gorm:"column:event_date"`
		VenueName     string    `gorm:"column:venue_name"`
		VenueSlug     string    `gorm:"column:venue_slug"`
		VenueTimezone string    `gorm:"column:venue_timezone"`
	}
	var rows []showRow
	// `now` is passed twice: once to split upcoming from past, once to order
	// within each half. Both must be the SAME instant or a show could sort as
	// upcoming and then be ranked as past.
	//
	// The SQL split is on the START INSTANT, while the IsUpcoming the caller
	// reads is on the venue's own calendar (below). They differ only for a show
	// in progress right now, and only in PREFERENCE: such a show sorts into the
	// past half, so a band that also has a future booking gets the future one.
	// The label is never wrong either way. The instant is used here rather than
	// shared.VenueLocalDateCondition because that fragment requires
	// VenueTZJoin's lateral, whose primary-venue pick would disagree with this
	// query's alphabetical display-venue pick — two venue choices in one row is
	// a worse defect than a ranking preference in a rare tie.
	err := s.db.Raw(`
		SELECT DISTINCT ON (sa.artist_id)
		       sa.artist_id,
		       s.id,
		       COALESCE(s.slug, '') AS slug,
		       s.event_date,
		       COALESCE(v.name, '') AS venue_name,
		       COALESCE(v.slug, '') AS venue_slug,
		       COALESCE(v.timezone, '') AS venue_timezone
		FROM show_artists sa
		JOIN shows s ON s.id = sa.show_id
		LEFT JOIN show_venues sv ON sv.show_id = s.id
		LEFT JOIN venues v ON v.id = sv.venue_id
		WHERE sa.artist_id IN ?
		  AND s.status = ?
		  AND s.is_cancelled = false
		ORDER BY sa.artist_id,
		         (s.event_date >= ?) DESC,
		         CASE WHEN s.event_date >= ? THEN s.event_date END ASC NULLS LAST,
		         s.event_date DESC,
		         s.id ASC,
		         v.name ASC,
		         v.id ASC
	`, artistIDs, catalogm.ShowStatusApproved, now, now).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get shows for scene new artists: %w", err)
	}

	out := make(map[uint]contracts.SceneNewArtistShow, len(rows))
	for _, r := range rows {
		var tz *string
		if r.VenueTimezone != "" {
			tz = &r.VenueTimezone
		}
		// Both dates are rendered through the SAME zone resolution, so the
		// comparison is the site-wide listing boundary — a show graduates to
		// past at venue-local midnight, not at its start instant — and the flag
		// cannot contradict the EventDate printed next to it. ISO dates compare
		// correctly as strings.
		eventDate := venueLocalDate(r.EventDate, tz)
		out[r.ArtistID] = contracts.SceneNewArtistShow{
			ID:         r.ID,
			Slug:       r.Slug,
			EventDate:  eventDate,
			StartsAt:   r.EventDate.UTC(),
			VenueName:  r.VenueName,
			VenueSlug:  r.VenueSlug,
			IsUpcoming: eventDate >= venueLocalDate(now, tz),
		}
	}
	return out, nil
}
