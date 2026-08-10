package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// Venue-local partitioning for the TAG PAGE (PSY-1760).
//
// A tag page used to answer "is this show upcoming" three different ways at
// once: the entity gate said start-of-today UTC, the entity cards' printed
// upcoming_show_count said event_date >= NOW() with no status filter at all,
// and the /shows list a reader clicks through to said the show's own
// venue-local calendar day. Every test below seeds ONE fixture and asserts all
// three now answer the same thing.
//
// Fixtures are anchored on the VENUE's calendar rather than a UTC instant, for
// the reason spelled out at the top of show_venue_local_test.go: the boundary
// is evaluated by Postgres per row, there is no clock seam, and an assertion
// anchored on the wall clock would flip depending on the hour CI runs at. The
// shared fixtures (newVenueInZone, newApprovedShowAt, venueLocalInstant,
// requireLocalAndUTCDatesDiffer) come from that file.

// ── the boundaries this replaced, run against the same rows ────────

// legacyStartOfTodayUTCShowCount is the gate intersectBaseQuery used before
// PSY-1760, expressed in SQL so a test can show — rather than assert on faith —
// that the old and new boundaries disagree about the fixture in front of them.
// A test whose fixture both boundaries answer identically would pass against
// the code this ticket replaced.
func (s *TagIntersectionIntegrationTestSuite) legacyStartOfTodayUTCShowCount() int64 {
	var n int64
	s.Require().NoError(s.db.Raw(`
		SELECT COUNT(*) FROM shows
		WHERE status = ?
		  AND event_date >= (date_trunc('day', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC')
	`, catalogm.ShowStatusApproved).Scan(&n).Error)
	return n
}

// legacyInstantUpcomingShowCount is the predicate enrichArtists and enrichVenues
// printed on their cards before PSY-1760: an instant bound, and no status
// filter, so a pending show inflated a public count and a show that started
// earlier today had already left it.
func (s *TagIntersectionIntegrationTestSuite) legacyInstantUpcomingShowCount() int64 {
	var n int64
	s.Require().NoError(s.db.Raw(`
		SELECT COUNT(*) FROM shows WHERE event_date >= NOW()
	`).Scan(&n).Error)
	return n
}

// ── reading the three answers ──────────────────────────────────────

// tagPageAnswers is what a reader of /tags/{slug} is shown about "upcoming":
// the show group's count, the counts printed on the artist and venue cards, and
// the ids in the group's preview.
type tagPageAnswers struct {
	ShowCount        int64
	ShowPreviewIDs   []uint
	ArtistCardCount  int
	VenueCardCount   int
	ArtistCardsFound int
	VenueCardsFound  int
}

func (s *TagIntersectionIntegrationTestSuite) tagPage(slug string) tagPageAnswers {
	resp, err := s.tagService.IntersectEntitiesByTags([]string{slug}, false, 10)
	s.Require().NoError(err)

	out := tagPageAnswers{ShowPreviewIDs: []uint{}}
	for _, g := range resp.Groups {
		switch g.EntityType {
		case catalogm.TagEntityShow:
			out.ShowCount = g.Count
			for _, p := range g.Preview {
				out.ShowPreviewIDs = append(out.ShowPreviewIDs, p.EntityID)
			}
		case catalogm.TagEntityArtist:
			for _, p := range g.Preview {
				s.Require().NotNil(p.UpcomingShowCount, "artist card must carry a count")
				out.ArtistCardCount += *p.UpcomingShowCount
				out.ArtistCardsFound++
			}
		case catalogm.TagEntityVenue:
			for _, p := range g.Preview {
				s.Require().NotNil(p.UpcomingShowCount, "venue card must carry a count")
				out.VenueCardCount += *p.UpcomingShowCount
				out.VenueCardsFound++
			}
		}
	}
	return out
}

// showsListForTag is the list the tag page's show count links into: the main
// /shows feed narrowed by the same transitive artist-tag filter. Its total is
// the number the count has to equal, or a non-zero count dead-ends on an empty
// page.
func (s *TagIntersectionIntegrationTestSuite) showsListForTag(slug string) ([]uint, int64) {
	shows, _, total, err := NewShowService(s.db).GetUpcomingShows("UTC", "", 50, false,
		&contracts.UpcomingShowsFilter{TagSlugs: []string{slug}})
	s.Require().NoError(err)
	ids := make([]uint, 0, len(shows))
	for _, sh := range shows {
		ids = append(ids, sh.ID)
	}
	return ids, total
}

// seedShowAt creates a show at an exact instant with an explicit status, booked
// at one venue. newApprovedShowAt covers the approved case; this exists for the
// pending row that only PSY-1760's status filter excludes.
func (s *TagIntersectionIntegrationTestSuite) seedShowAt(venueID uint, at time.Time, status catalogm.ShowStatus) uint {
	show := &catalogm.Show{
		Title:       "Pending-" + at.Format(time.RFC3339Nano),
		EventDate:   at,
		City:        stringPtr("Testville"),
		State:       stringPtr("HI"),
		Status:      status,
		SubmittedBy: &s.user.ID,
	}
	s.Require().NoError(s.db.Create(show).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)
	return show.ID
}

// ── tests ──────────────────────────────────────────────────────────

// A show that started earlier on its venue's own calendar day is still tonight's
// listing. The card count this replaced was instant-bounded (event_date >=
// NOW()), and venue-local midnight today is ALWAYS in the past as an instant, so
// the legacy guard below holds at every hour of the day rather than only when
// CI happens to run.
func (s *TagIntersectionIntegrationTestSuite) TestTagPage_AlreadyStartedShowIsCountedAndListed() {
	const zone = "Pacific/Honolulu" // UTC-10, no DST
	venue := newVenueInZone(s.T(), s.db, "Tag Page Room", "HI", zone, true)
	artist := s.seedArtist("BoundaryArtist")
	s.tag("artist", artist, "shoegaze")
	s.tag("venue", venue.ID, "shoegaze")

	show := newApprovedShowAt(s.T(), s.db, venue.ID, s.user.ID, "Honolulu", "HI",
		venueLocalInstant(s.T(), zone, 0, 0))
	s.addArtistToShow(show.ID, artist, 0)

	s.Require().Equal(int64(0), s.legacyInstantUpcomingShowCount(),
		"fixture no longer distinguishes the instant-bounded card count")

	page := s.tagPage("shoegaze")
	listIDs, listTotal := s.showsListForTag("shoegaze")

	s.Equal(int64(1), page.ShowCount, "entity gate keeps the show until venue-local midnight")
	s.Equal([]uint{show.ID}, page.ShowPreviewIDs)
	s.Equal(1, page.ArtistCardsFound)
	s.Equal(1, page.ArtistCardCount, "the artist card prints the same partition")
	s.Equal(1, page.VenueCardsFound)
	s.Equal(1, page.VenueCardCount, "the venue card prints the same partition")
	s.Equal(int64(1), listTotal, "the /shows list the count links into agrees")
	s.Equal([]uint{show.ID}, listIDs)
	s.Equal(page.ShowCount, listTotal, "count and list must be drawn from one boundary")
}

// The headline regression, stated as the two boundaries themselves rather than
// as a hand-picked instant: seed a show in the gap BETWEEN start-of-today UTC
// and venue-local midnight, where the old gate and the new one necessarily
// disagree. Whichever boundary is earlier flips over the course of a day, so the
// test derives the direction instead of assuming one — which is what makes it
// true at every hour, and what makes both directions of the bug reachable.
//
// Left of the gap the old gate counted a show /shows had already dropped (the
// dead-end link this ticket exists to close); right of it the old gate dropped a
// show /shows still lists.
func (s *TagIntersectionIntegrationTestSuite) TestTagPage_CountsFollowVenueLocalWhereItDisagreesWithUTC() {
	const zone = "Pacific/Honolulu" // UTC-10: its local midnight can never coincide with UTC's
	venue := newVenueInZone(s.T(), s.db, "Boundary Gap Room", "HI", zone, true)
	artist := s.seedArtist("GapArtist")
	s.tag("artist", artist, "shoegaze")
	s.tag("venue", venue.ID, "shoegaze")

	now := time.Now().UTC()
	utcMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	venueMidnight := venueLocalInstant(s.T(), zone, 0, 0)
	s.Require().NotEqual(utcMidnight, venueMidnight.UTC(), "the two boundaries must differ for the gap to exist")

	// The earlier boundary is the inclusive start of the gap: a show there is
	// upcoming by that boundary and past by the other.
	at, oldCounts := venueMidnight, false
	if venueMidnight.After(utcMidnight) {
		at, oldCounts = utcMidnight, true
	}
	show := newApprovedShowAt(s.T(), s.db, venue.ID, s.user.ID, "Honolulu", "HI", at)
	s.addArtistToShow(show.ID, artist, 0)

	legacy := s.legacyStartOfTodayUTCShowCount()
	page := s.tagPage("shoegaze")
	listIDs, listTotal := s.showsListForTag("shoegaze")

	if oldCounts {
		s.Require().Equal(int64(1), legacy, "the old UTC gate must count this row")
		s.Equal(int64(0), page.ShowCount, "a show its venue already calls yesterday must not be counted")
		s.Equal(0, page.ArtistCardCount)
		s.Equal(0, page.VenueCardCount)
		s.Equal(int64(0), listTotal)
	} else {
		s.Require().Equal(int64(0), legacy, "the old UTC gate must drop this row")
		s.Equal(int64(1), page.ShowCount, "a show still on its venue's calendar day must be counted")
		s.Equal([]uint{show.ID}, page.ShowPreviewIDs)
		s.Equal(1, page.ArtistCardCount)
		s.Equal(1, page.VenueCardCount)
		s.Equal(int64(1), listTotal)
		s.Equal([]uint{show.ID}, listIDs)
	}
	s.NotEqual(legacy, page.ShowCount, "the boundaries must disagree, or the fixture proves nothing")
	s.Equal(page.ShowCount, listTotal, "count and list must be drawn from one boundary")
}

// The card counts carried no status filter at all, so a pending or rejected
// submission inflated a public number. Nothing else on the page ever counted it.
func (s *TagIntersectionIntegrationTestSuite) TestTagPage_CardCountsExcludeNonApprovedShows() {
	const zone = "Pacific/Honolulu"
	venue := newVenueInZone(s.T(), s.db, "Pending Room", "HI", zone, true)
	artist := s.seedArtist("PendingArtist")
	s.tag("artist", artist, "shoegaze")
	s.tag("venue", venue.ID, "shoegaze")

	pending := s.seedShowAt(venue.ID, venueLocalInstant(s.T(), zone, 1, 20), catalogm.ShowStatusPending)
	s.addArtistToShow(pending, artist, 0)

	s.Require().Equal(int64(1), s.legacyInstantUpcomingShowCount(),
		"fixture no longer exercises the missing status filter")

	page := s.tagPage("shoegaze")
	_, listTotal := s.showsListForTag("shoegaze")

	s.Equal(0, page.ArtistCardCount, "a pending show must not inflate a public card count")
	s.Equal(0, page.VenueCardCount, "a pending show must not inflate a public card count")
	s.Equal(int64(0), page.ShowCount)
	s.Equal(int64(0), listTotal)
}

// The preview's busiest-first ORDER BY sorts by the very number the cards print,
// so it has to be the same quantity. Before PSY-1760 the sort was instant-based
// and the printed count status-blind, which could render "1 show" above
// "2 shows".
func (s *TagIntersectionIntegrationTestSuite) TestTagPage_ArtistPreviewOrderMatchesThePrintedCounts() {
	const zone = "Pacific/Honolulu"
	venue := newVenueInZone(s.T(), s.db, "Order Room", "HI", zone, true)

	// `busy` plays one show that only the venue-local boundary counts; `quiet`
	// plays one that only the status-blind predicate counted. Under the old
	// pair the order and the printed counts contradicted each other.
	busy := s.seedArtist("BusyLocal")
	quiet := s.seedArtist("QuietPending")
	s.tag("artist", busy, "shoegaze")
	s.tag("artist", quiet, "shoegaze")

	started := newApprovedShowAt(s.T(), s.db, venue.ID, s.user.ID, "Honolulu", "HI",
		venueLocalInstant(s.T(), zone, 0, 0))
	s.addArtistToShow(started.ID, busy, 0)

	pending := s.seedShowAt(venue.ID, venueLocalInstant(s.T(), zone, 1, 20), catalogm.ShowStatusPending)
	s.addArtistToShow(pending, quiet, 0)

	resp, err := s.tagService.IntersectEntitiesByTags([]string{"shoegaze"}, false, 10)
	s.Require().NoError(err)

	var names []string
	var counts []int
	for _, g := range resp.Groups {
		if g.EntityType != catalogm.TagEntityArtist {
			continue
		}
		for _, p := range g.Preview {
			names = append(names, p.Name)
			s.Require().NotNil(p.UpcomingShowCount)
			counts = append(counts, *p.UpcomingShowCount)
		}
	}
	s.Require().Len(names, 2)
	s.Equal([]string{"BusyLocal", "QuietPending"}, names, "busiest artist leads the preview")
	s.Equal([]int{1, 0}, counts, "the printed counts must descend with the order")
}

// ── fragment-level pins (no database) ──────────────────────────────

// The one-boundary property, asserted structurally rather than only through
// fixtures: the tag page's count renderer must build FROM the shared fragments,
// so a future edit to the boundary reaches it automatically. A restated
// predicate would keep every integration test above green while drifting the
// moment services/shared changes.
func TestVenueLocalUpcomingCountSQL_BuildsThroughTheSharedFragments(t *testing.T) {
	for _, tc := range []struct{ junction, entityFK string }{
		{"show_artists", "artist_id"},
		{"show_venues", "venue_id"},
	} {
		sql := venueLocalUpcomingCountSQL(tc.junction, tc.entityFK, "")
		require.Contains(t, sql, shared.VenueTZJoin, "%s must resolve zones through the shared lateral", tc.junction)
		require.Contains(t, sql, shared.VenueLocalDateCondition("upcoming"),
			"%s must partition through the shared condition", tc.junction)
		require.Contains(t, sql, "shows.status = 'approved'", "%s must count approved shows only", tc.junction)
		require.NotContains(t, sql, "NOW()", "%s must not reintroduce an instant bound", tc.junction)
		// The shared lateral correlates on a bare `shows.id`, so the table
		// cannot be aliased inside this subquery.
		require.Contains(t, sql, "JOIN shows ON shows.id = j.show_id")
		require.NotContains(t, sql, "?", "the unrestricted form must stay parameter-free")

		restricted := venueLocalUpcomingCountSQL(tc.junction, tc.entityFK, "j."+tc.entityFK+" IN ?")
		require.Contains(t, restricted, "AND j."+tc.entityFK+" IN ?",
			"the restriction must land inside the aggregate, not outside it")
	}
}

// venueLocalUpcomingCountSQL interpolates the status constant rather than
// binding it, which keeps the fragment parameter-free and composable into both
// GORM's Joins and Raw. That is safe only while the constant stays a bare word.
func TestShowStatusApproved_IsSafeToInterpolate(t *testing.T) {
	require.Regexp(t, `^[a-z_]+$`, string(catalogm.ShowStatusApproved))
}
