package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The scene page's named new-bands module (PSY-1781).
//
// TWO definitions of "new to the scene" exist in the product and they do not
// agree. The DIGEST definition — a catalog row created inside the window — is
// the one this serves, because it is the one the blessed mock's copy states
// ("first listed Aug 10"): the row's own listing date is the fact being
// reported. The scene PULSE's new_artists_30d counts bands whose FIRST approved
// show falls in the window, which answers a different question (who started
// playing here) and would make the module's date field a lie.
//
// The choice is enforced structurally, not by convention: this method does not
// re-implement the window at all. It calls GetSceneNewArtistsSince — the
// digest's own query, cap, total and roster scope — and only enriches the rows
// it returns. A future edit cannot silently swap in the pulse definition
// without deleting that call.

// GetSceneNewArtists returns the scene's named new-bands rows for the window
// (`since`, `now`], newest first, capped at `limit`, plus the uncapped TOTAL in
// the window so the caller can render "+N more" exactly as the digest does.
//
// Membership is GetSceneNewArtistsSince's, verbatim. The enrichment is one
// batched query for the bands' shows, so the cost is two queries per request
// regardless of how many bands the window holds.
//
// Like the method it wraps, this does NOT gate on the scene's venue count: a
// scene that temporarily dips below the threshold still has a real roster, and
// a module that can be empty must render empty rather than 404 (the empty list
// is a state of a page that exists, not a missing page).
func (s *SceneService) GetSceneNewArtists(city, state string, since, now time.Time, limit int) ([]contracts.SceneNewArtistRow, int, error) {
	base, total, err := s.GetSceneNewArtistsSince(city, state, since, now, limit)
	if err != nil {
		return nil, 0, err
	}
	if len(base) == 0 {
		return []contracts.SceneNewArtistRow{}, total, nil
	}

	ids := make([]uint, len(base))
	for i, a := range base {
		ids[i] = a.ID
	}
	// The show lookup's failure is NOT swallowed the way the venue rail's
	// cosmetic aggregations are: a nil Show is a claim ("this band has no
	// booking yet"), so degrading a fault into it would publish a falsehood
	// about every row. The names come from the same database, so a failure
	// here is not a partial-data case worth rendering.
	showByArtist, err := s.newArtistShows(ids, now)
	if err != nil {
		return nil, 0, err
	}

	rows := make([]contracts.SceneNewArtistRow, len(base))
	for i, a := range base {
		rows[i] = contracts.SceneNewArtistRow{SceneNewArtist: a}
		if show, ok := showByArtist[a.ID]; ok {
			rows[i].Show = &show
		}
	}
	return rows, total, nil
}

// newArtistShows returns, per artist ID, the ONE show that best answers "where
// can I see this band": its soonest upcoming approved show, or — when it has
// none — its most recent past one, flagged IsUpcoming=false.
//
// The past fallback is deliberate. A band listed this month whose only show has
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
