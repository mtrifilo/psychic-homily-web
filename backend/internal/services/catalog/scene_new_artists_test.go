package catalog

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Named new-bands module (PSY-1781) — run as part of the
// SceneServiceIntegrationTestSuite (real Postgres, all migrations).

// createArtistListedAt seeds a Phoenix band with an explicit catalog created_at,
// which is the fact the digest definition selects on. GORM sets CreatedAt itself
// unless the field is non-zero, so this cannot go through createArtist.
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

// The definition pin. Two definitions of "new to the scene" exist and they
// disagree; PSY-1781 chose the DIGEST one (catalog row created in the window)
// over the PULSE one (MIN(event_date) of the band's approved shows in the
// window, see GetSceneDetail's new_artists_30d). The two fixtures below are
// each returned by exactly ONE definition, so swapping the implementation back
// to the pulse query fails this test in both directions rather than silently
// changing what the module means.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneNewArtists_PinsDigestDefinitionNotPulse() {
	venue := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	user := suite.createUser()
	now := time.Now().UTC()
	since := now.AddDate(0, 0, -30)

	// Listed inside the window, first played LONG before it: digest yes, pulse no.
	freshListing := suite.createArtistListedAt("Saguaro Teeth", now.AddDate(0, 0, -5))
	suite.createApprovedShow("old show", venue.ID, freshListing.ID, user.ID, now.AddDate(0, 0, -200))

	// Listed long ago, first played inside the window: pulse yes, digest no.
	oldListing := suite.createArtistListedAt("Long Established Band", now.AddDate(0, 0, -200))
	suite.createApprovedShow("debut show", venue.ID, oldListing.ID, user.ID, now.AddDate(0, 0, -3))

	rows, total, err := suite.sceneService.GetSceneNewArtists("Phoenix", "AZ", since, now, 10)
	suite.Require().NoError(err)
	suite.Require().Len(rows, 1, "only the band LISTED in the window belongs in the module")
	suite.Equal("Saguaro Teeth", rows[0].Name)
	suite.Equal(1, total)
	suite.WithinDuration(now.AddDate(0, 0, -5), rows[0].FirstListedAt, time.Minute,
		"first_listed_at must be the catalog created_at the window selected on")
}

// The rows carry the same membership and cap as the digest's own method, which
// is the structural guarantee that the two surfaces cannot disagree.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneNewArtists_AgreesWithDigestMethod() {
	now := time.Now().UTC()
	since := now.AddDate(0, 0, -30)
	suite.createArtistListedAt("Band One", now.AddDate(0, 0, -1))
	suite.createArtistListedAt("Band Two", now.AddDate(0, 0, -2))
	suite.createArtistListedAt("Band Three", now.AddDate(0, 0, -3))
	// Based elsewhere — outside the roster scope for both methods.
	suite.createArtistIn("Tour Van", "Denver", "CO")

	digest, digestTotal, err := suite.sceneService.GetSceneNewArtistsSince("Phoenix", "AZ", since, now, 2)
	suite.Require().NoError(err)
	rows, total, err := suite.sceneService.GetSceneNewArtists("Phoenix", "AZ", since, now, 2)
	suite.Require().NoError(err)

	suite.Equal(digestTotal, total, "the uncapped total must be the digest's")
	suite.Equal(3, total)
	suite.Require().Len(rows, 2, "the cap applies to the rows, not the total")
	names := []string{rows[0].Name, rows[1].Name}
	suite.Equal([]string{digest[0].Name, digest[1].Name}, names)
	suite.Equal([]string{"Band One", "Band Two"}, names, "most recently listed first")
}

// The show attached to a row: next upcoming when there is one, most recent past
// otherwise, and nil when the band has no approved show at all.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneNewArtists_AttachesNextThenMostRecentShow() {
	rebel := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	nile := suite.createVerifiedVenue("Nile Theater", "Mesa", "AZ")
	user := suite.createUser()
	now := time.Now().UTC()
	since := now.AddDate(0, 0, -30)

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

	rows, _, err := suite.sceneService.GetSceneNewArtists("Phoenix", "AZ", since, now, 10)
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
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneNewArtists_SkipsCancelledShows() {
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

	rows, _, err := suite.sceneService.GetSceneNewArtists("Phoenix", "AZ", now.AddDate(0, 0, -30), now, 10)
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

// A quiet scene is an empty module, never an error and never a nil slice.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneNewArtists_QuietSceneIsEmptyNotError() {
	suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	// Based here, but listed well before the window opens.
	now := time.Now().UTC()
	suite.createArtistListedAt("Old Timer", now.AddDate(0, 0, -400))

	rows, total, err := suite.sceneService.GetSceneNewArtists("Phoenix", "AZ", now.AddDate(0, 0, -30), now, 10)
	suite.Require().NoError(err)
	suite.NotNil(rows)
	suite.Len(rows, 0)
	suite.Equal(0, total)
}
