package catalog

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Latest-additions module (PSY-1781, redefined by PSY-1844) — run as part of
// the SceneServiceIntegrationTestSuite (real Postgres, all migrations).

// createArtistListedAt seeds a Phoenix band with an explicit catalog created_at,
// which is the fact the module orders on. GORM sets CreatedAt itself unless the
// field is non-zero, so this cannot go through createArtist.
func (suite *SceneServiceIntegrationTestSuite) createArtistListedAt(name string, listedAt time.Time) *catalogm.Artist {
	slug := name
	artist := &catalogm.Artist{
		Name:      name,
		Slug:      &slug,
		City:      stringPtr("Phoenix"),
		State:     stringPtr("AZ"),
		Metro:     seedMetro("Phoenix", "AZ"),
		CreatedAt: listedAt,
		UpdatedAt: listedAt,
	}
	suite.Require().NoError(suite.db.Create(artist).Error)
	return artist
}

// createPendingShow seeds an unreviewed submission — the status the show pick
// must never surface.
func (suite *SceneServiceIntegrationTestSuite) createPendingShow(title string, venueID, artistID, userID uint, eventDate time.Time) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   eventDate,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusPending,
		SubmittedBy: &userID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artistID, Position: 0}).Error)
	return show
}

// createVenuelessApprovedShow seeds an approved show with NO show_venues row —
// a booking announced before the room is settled.
func (suite *SceneServiceIntegrationTestSuite) createVenuelessApprovedShow(title string, artistID, userID uint, eventDate time.Time) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   eventDate,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artistID, Position: 0}).Error)
	return show
}

// THE DEFINITION PIN.
//
// Three definitions of "new to the scene" have been in play and they disagree.
// The module serves LATEST: the roster ordered by catalog created_at, newest
// first, with NO window. The two fixtures below are chosen so that each of the
// rejected definitions fails this test rather than silently changing what the
// module means:
//
//   - PULSE (MIN(event_date) of the band's approved shows inside a window, see
//     GetSceneDetail's new_artists_30d) would return ONLY "Long Established
//     Band" and would make the first_listed_at printed on the row a lie.
//   - DIGEST/WINDOWED (created_at inside a trailing 30 days, what PSY-1781
//     served and GetSceneNewArtistsSince still serves the weekly email) would
//     return ONLY "Saguaro Teeth" — and returned NOTHING at all on 5 of 6 major
//     scenes in production, which is why PSY-1844 removed the window.
//
// LATEST returns BOTH, most recently listed first.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneLatestArtists_PinsLatestListedNotPulseNotWindow() {
	venue := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	user := suite.createUser()
	now := time.Now().UTC()

	// Listed recently, first played LONG ago: windowed yes, pulse no.
	freshListing := suite.createArtistListedAt("Saguaro Teeth", now.AddDate(0, 0, -5))
	suite.createApprovedShow("old show", venue.ID, freshListing.ID, user.ID, now.AddDate(0, 0, -200))

	// Listed LONG ago, first played recently: pulse yes, windowed no.
	oldListing := suite.createArtistListedAt("Long Established Band", now.AddDate(0, 0, -200))
	suite.createApprovedShow("debut show", venue.ID, oldListing.ID, user.ID, now.AddDate(0, 0, -3))

	rows, err := suite.sceneService.GetSceneLatestArtists("Phoenix", "AZ", now, 10)
	suite.Require().NoError(err)
	suite.Require().Len(rows, 2, "the module is the roster's latest arrivals, not a window and not a debut list")
	suite.Equal("Saguaro Teeth", rows[0].Name, "most recently LISTED first — created_at, not first show")
	suite.Equal("Long Established Band", rows[1].Name)
	suite.WithinDuration(now.AddDate(0, 0, -5), rows[0].FirstListedAt, time.Minute,
		"first_listed_at must be the catalog created_at the ordering selected on")
}

// The regression PSY-1844 fixes, stated directly.
//
// Scene rosters grow in human-run seeding batches, not continuously, so a
// trailing window emptied out between batches: production read 0 new bands on
// 5 of 6 major scenes on 2026-08-18, six days after the last batch aged past
// 30 days. A roster whose every band was listed months ago still has most
// recently listed bands, and this module must name them.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneLatestArtists_RosterListedLongAgoStillReports() {
	now := time.Now().UTC()
	suite.createArtistListedAt("Batch Two", now.AddDate(0, 0, -200))
	suite.createArtistListedAt("Batch One", now.AddDate(0, 0, -400))
	suite.createArtistListedAt("Batch Three", now.AddDate(0, 0, -100))

	rows, err := suite.sceneService.GetSceneLatestArtists("Phoenix", "AZ", now, 10)
	suite.Require().NoError(err)
	suite.Require().Len(rows, 3, "a roster listed entirely outside any trailing window is still a roster")
	suite.Equal([]string{"Batch Three", "Batch Two", "Batch One"},
		[]string{rows[0].Name, rows[1].Name, rows[2].Name}, "newest listing first")
}

// The weekly digest KEEPS its window, and the two surfaces now differ on
// purpose. The digest advances a per-follow cursor to `now` after each send, so
// a band outside its window has already been reported; the page module has no
// cursor and no send. Anyone "fixing" the inconsistency by routing the digest
// through the module's query would start re-sending the whole roster every week.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneLatestArtists_DigestStaysWindowedAndDiffers() {
	now := time.Now().UTC()
	suite.createArtistListedAt("Listed This Week", now.AddDate(0, 0, -2))
	suite.createArtistListedAt("Listed Last Year", now.AddDate(0, 0, -300))

	digest, digestTotal, err := suite.sceneService.GetSceneNewArtistsSince("Phoenix", "AZ", now.AddDate(0, 0, -30), now, 10)
	suite.Require().NoError(err)
	suite.Require().Len(digest, 1, "the digest must still see ONLY the window")
	suite.Equal("Listed This Week", digest[0].Name)
	suite.Equal(1, digestTotal)

	rows, err := suite.sceneService.GetSceneLatestArtists("Phoenix", "AZ", now, 10)
	suite.Require().NoError(err)
	suite.Require().Len(rows, 2, "the module must see the whole roster's latest arrivals")
}

// The cap trims the tail, never the head, and roster scope still bounds
// membership: a band based elsewhere is not this scene's latest addition.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneLatestArtists_CapsToNewestAndKeepsRosterScope() {
	now := time.Now().UTC()
	suite.createArtistListedAt("Band One", now.AddDate(0, 0, -1))
	suite.createArtistListedAt("Band Two", now.AddDate(0, 0, -2))
	suite.createArtistListedAt("Band Three", now.AddDate(0, 0, -3))
	// Based elsewhere — outside the roster scope, however recently listed.
	suite.createArtistIn("Tour Van", "Denver", "CO")

	rows, err := suite.sceneService.GetSceneLatestArtists("Phoenix", "AZ", now, 2)
	suite.Require().NoError(err)
	suite.Require().Len(rows, 2, "the cap applies to the newest end of the roster")
	suite.Equal([]string{"Band One", "Band Two"}, []string{rows[0].Name, rows[1].Name}, "most recently listed first")
}

// A caller that names no limit gets the module's own default rather than the
// whole roster — the section is an index into the roster, not a second copy of
// it. The service owns that number outright (the request struct carries no
// `default:` tag precisely so it cannot be stated twice), so the assertion is
// the LITERAL 5 against a roster deliberately larger than it. Asserting
// sceneLatestArtistsDefaultLimit here instead would compare the constant to
// itself and pass for any value.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneLatestArtists_ZeroLimitUsesDefault() {
	now := time.Now().UTC()
	for i := 0; i < 7; i++ {
		suite.createArtistListedAt(
			"Band "+string(rune('A'+i)),
			now.AddDate(0, 0, -(i+1)),
		)
	}

	rows, err := suite.sceneService.GetSceneLatestArtists("Phoenix", "AZ", now, 0)
	suite.Require().NoError(err)
	suite.Len(rows, 5, "an absent limit lands on the service's own default of 5, not the whole roster")
}

// The show attached to a row: next upcoming when there is one, most recent past
// otherwise, and nil when the band has no approved show at all.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneLatestArtists_AttachesNextThenMostRecentShow() {
	rebel := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	nile := suite.createVerifiedVenue("Nile Theater", "Mesa", "AZ")
	user := suite.createUser()
	now := time.Now().UTC()

	upcoming := suite.createArtistListedAt("Has Upcoming", now.AddDate(0, 0, -1))
	suite.createApprovedShow("past", nile.ID, upcoming.ID, user.ID, now.AddDate(0, 0, -4))
	nextShow := suite.createApprovedShow("next", rebel.ID, upcoming.ID, user.ID, now.AddDate(0, 0, 10))
	suite.createApprovedShow("later", rebel.ID, upcoming.ID, user.ID, now.AddDate(0, 0, 40))
	// A second room on the same bill: the display venue is the alphabetically
	// first one, so this must flip the reported name off The Rebel Lounge.
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: nextShow.ID, VenueID: nile.ID}).Error)
	// An UNAPPROVED show sooner than the real one — a dropped status filter
	// would surface a submission nobody has reviewed.
	suite.createPendingShow("unreviewed", rebel.ID, upcoming.ID, user.ID, now.AddDate(0, 0, 2))

	pastOnly := suite.createArtistListedAt("Played Already", now.AddDate(0, 0, -2))
	suite.createApprovedShow("older", nile.ID, pastOnly.ID, user.ID, now.AddDate(0, 0, -20))
	mostRecent := suite.createApprovedShow("recent", nile.ID, pastOnly.ID, user.ID, now.AddDate(0, 0, -2))

	suite.createArtistListedAt("Not Booked Yet", now.AddDate(0, 0, -3))

	// A booking with no room attached yet: the venue joins must stay LEFT, or
	// the whole show vanishes rather than just its venue name.
	venueless := suite.createArtistListedAt("Venue TBA", now.AddDate(0, 0, -4))
	tba := suite.createVenuelessApprovedShow("tba", venueless.ID, user.ID, now.AddDate(0, 0, 12))

	rows, err := suite.sceneService.GetSceneLatestArtists("Phoenix", "AZ", now, 10)
	suite.Require().NoError(err)
	suite.Require().Len(rows, 4)

	byName := map[string]int{}
	for i, r := range rows {
		byName[r.Name] = i
	}

	withUpcoming := rows[byName["Has Upcoming"]]
	suite.Require().NotNil(withUpcoming.Show)
	suite.Equal(nextShow.ID, withUpcoming.Show.ID, "the SOONEST APPROVED upcoming show, not the later or the pending one")
	suite.True(withUpcoming.Show.IsUpcoming)
	suite.Equal("Nile Theater", withUpcoming.Show.VenueName, "a multi-room bill reports its alphabetically first venue")
	suite.Equal(now.AddDate(0, 0, 10).Format("2006-01-02"), withUpcoming.Show.EventDate)
	suite.WithinDuration(now.AddDate(0, 0, 10), withUpcoming.Show.StartsAt, time.Second,
		"starts_at is the absolute instant, since event_date cannot be parsed back into one")

	withPast := rows[byName["Played Already"]]
	suite.Require().NotNil(withPast.Show)
	suite.Equal(mostRecent.ID, withPast.Show.ID, "the MOST RECENT past show, not the oldest")
	suite.False(withPast.Show.IsUpcoming)
	suite.Equal("Nile Theater", withPast.Show.VenueName)

	suite.Nil(rows[byName["Not Booked Yet"]].Show, "a band with no approved show carries no show")

	withoutVenue := rows[byName["Venue TBA"]]
	suite.Require().NotNil(withoutVenue.Show, "a venueless show still attaches — the venue joins are LEFT")
	suite.Equal(tba.ID, withoutVenue.Show.ID)
	suite.Empty(withoutVenue.Show.VenueName)
}

// A cancelled show is not an answer to "where can I see this band", in either
// direction: the row carries no status badge, so citing one would read as a
// date to turn up to (upcoming) or a gig that happened (past).
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneLatestArtists_SkipsCancelledShows() {
	rebel := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	nile := suite.createVerifiedVenue("Nile Theater", "Mesa", "AZ")
	user := suite.createUser()
	now := time.Now().UTC()

	band := suite.createArtistListedAt("Cancelled Tour", now.AddDate(0, 0, -1))
	played := suite.createApprovedShow("played", nile.ID, band.ID, user.ID, now.AddDate(0, 0, -6))
	cancelled := suite.createApprovedShow("called off", rebel.ID, band.ID, user.ID, now.AddDate(0, 0, 5))
	suite.Require().NoError(suite.db.Model(cancelled).Update("is_cancelled", true).Error)

	onlyCancelled := suite.createArtistListedAt("Nothing Left", now.AddDate(0, 0, -2))
	onlyShow := suite.createApprovedShow("also called off", rebel.ID, onlyCancelled.ID, user.ID, now.AddDate(0, 0, 8))
	suite.Require().NoError(suite.db.Model(onlyShow).Update("is_cancelled", true).Error)

	rows, err := suite.sceneService.GetSceneLatestArtists("Phoenix", "AZ", now, 10)
	suite.Require().NoError(err)
	suite.Require().Len(rows, 2)

	byName := map[string]int{}
	for i, r := range rows {
		byName[r.Name] = i
	}

	fellBack := rows[byName["Cancelled Tour"]]
	suite.Require().NotNil(fellBack.Show)
	suite.Equal(played.ID, fellBack.Show.ID, "the cancelled upcoming show must not outrank a real past one")
	suite.False(fellBack.Show.IsUpcoming)

	suite.Nil(rows[byName["Nothing Left"]].Show, "a band whose only show is cancelled carries no show")
}

// A scene with no bands based in it is an empty module, never an error and
// never a nil slice. With the window gone this is the ONLY way the module comes
// back empty, which is the point: it is now a fact about the scene rather than
// about when the last seeding batch ran.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneLatestArtists_SceneWithNoRosterIsEmptyNotError() {
	suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	// A real band, based somewhere else — it must not stand in for a roster.
	suite.createArtistIn("Tour Van", "Denver", "CO")

	rows, err := suite.sceneService.GetSceneLatestArtists("Phoenix", "AZ", time.Now().UTC(), 10)
	suite.Require().NoError(err)
	suite.NotNil(rows, "an empty module must marshal as [], not null")
	suite.Len(rows, 0)
}
