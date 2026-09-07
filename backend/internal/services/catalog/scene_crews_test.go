package catalog

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Scene-scoped crew tags (PSY-1884) — run as part of the
// SceneServiceIntegrationTestSuite (real Postgres, all migrations).

// crewScene seeds the minimum verified-venue count so GetSceneCrews clears its
// existence gate, and returns the two Phoenix rooms to hang shows on.
func (suite *SceneServiceIntegrationTestSuite) crewScene() (*catalogm.Venue, *catalogm.Venue) {
	return suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ"),
		suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
}

// tagShow tags a show, the edge the crew query reads.
func (suite *SceneServiceIntegrationTestSuite) tagShow(showID, tagID, userID uint) {
	suite.tagEntity(catalogm.TagEntityShow, showID, tagID, userID)
}

func (suite *SceneServiceIntegrationTestSuite) crewSlugs(city, state string) []string {
	crews, err := suite.sceneService.GetSceneCrews(city, state)
	suite.Require().NoError(err)
	slugs := make([]string, 0, len(crews))
	for _, c := range crews {
		slugs = append(slugs, c.Slug)
	}
	return slugs
}

// The two properties that define the row: the count is scene-LOCAL, and the
// ranking is count desc then name asc. The out-of-metro show is the pin that
// separates "this crew books here" from "this crew exists".
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCrews_ScopesToSceneAndRanksByLocalShowCount() {
	rebel, valley := suite.crewScene()
	tucson := suite.createVerifiedVenue("Club Congress", "Tucson", "AZ")
	user := suite.createUser()
	artist := suite.createArtist("Crew Booked Band")
	when := time.Now().UTC().AddDate(0, 0, -30)

	busy := suite.createTagInCategory("Rubber Brother Records", "rubber-brother-records", catalogm.TagCategoryCrew)
	quiet := suite.createTagInCategory("Ascetic House", "ascetic-house", catalogm.TagCategoryCrew)
	elsewhere := suite.createTagInCategory("Tucson Crew", "tucson-crew", catalogm.TagCategoryCrew)

	for i, venue := range []*catalogm.Venue{rebel, valley} {
		show := suite.createApprovedShow("busy crew night", venue.ID, artist.ID, user.ID, when.AddDate(0, 0, i))
		suite.tagShow(show.ID, busy, user.ID)
	}
	quietShow := suite.createApprovedShow("quiet crew night", rebel.ID, artist.ID, user.ID, when.AddDate(0, 0, 3))
	suite.tagShow(quietShow.ID, quiet, user.ID)

	// Same crew, a room outside the metro: invisible to Phoenix.
	outOfScope := suite.createApprovedShow("tucson night", tucson.ID, artist.ID, user.ID, when.AddDate(0, 0, 4))
	suite.tagShow(outOfScope.ID, busy, user.ID)
	suite.tagShow(outOfScope.ID, elsewhere, user.ID)

	crews, err := suite.sceneService.GetSceneCrews("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(crews, 2)

	suite.Equal("rubber-brother-records", crews[0].Slug)
	suite.Equal("Rubber Brother Records", crews[0].Name)
	suite.Equal(2, crews[0].ShowCount)

	suite.Equal("ascetic-house", crews[1].Slug)
	suite.Equal(1, crews[1].ShowCount)
}

// Equal counts order by NAME ascending. The slugs are deliberately not sorted
// in the expected sequence, and the seeding order is a third permutation, so
// ordering by slug (either direction) or by insertion fails this.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCrews_TiesBreakOnNameAscending() {
	rebel, _ := suite.crewScene()
	user := suite.createUser()
	artist := suite.createArtist("Tie Break Band")
	when := time.Now().UTC().AddDate(0, 0, -20)

	seeded := []struct{ name, slug string }{
		{"Zebra Collective", "c-crew"},
		{"Mojave Bookings", "a-crew"},
		{"Ajo Way Presents", "b-crew"},
	}
	for i, s := range seeded {
		tagID := suite.createTagInCategory(s.name, s.slug, catalogm.TagCategoryCrew)
		show := suite.createApprovedShow("tie night", rebel.ID, artist.ID, user.ID, when.AddDate(0, 0, i))
		suite.tagShow(show.ID, tagID, user.ID)
	}

	suite.Equal([]string{"b-crew", "a-crew", "c-crew"}, suite.crewSlugs("Phoenix", "AZ"))
}

// show_venues is many-to-many. A show booked into two rooms of the same scene
// joins twice, and COUNT(*) would credit its crew with two nights.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCrews_MultiRoomShowCountsOnce() {
	rebel, valley := suite.crewScene()
	user := suite.createUser()
	artist := suite.createArtist("Two Room Band")

	crew := suite.createTagInCategory("Two Room Crew", "two-room-crew", catalogm.TagCategoryCrew)
	show := suite.createApprovedShow("two room night", rebel.ID, artist.ID, user.ID, time.Now().UTC().AddDate(0, 0, -5))
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: valley.ID}).Error)
	suite.tagShow(show.ID, crew, user.ID)

	crews, err := suite.sceneService.GetSceneCrews("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(crews, 1)
	suite.Equal(1, crews[0].ShowCount)
}

// Four exclusions the published number depends on, each seeded so that dropping
// its predicate changes the answer: a non-crew category is not a crew, an
// unapproved show is not a night anyone can read, and the edge is the show, so a
// crew tag sitting on an artist or on a venue contributes nothing.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCrews_ExcludesOtherCategoriesUnapprovedShowsAndOtherEdges() {
	rebel, valley := suite.crewScene()
	user := suite.createUser()
	artist := suite.createArtist("Excluded Band")
	when := time.Now().UTC().AddDate(0, 0, -15)

	counted := suite.createTagInCategory("Counted Crew", "counted-crew", catalogm.TagCategoryCrew)
	genre := suite.createTagInCategory("Desert Rock", "desert-rock", catalogm.TagCategoryGenre)
	pendingOnly := suite.createTagInCategory("Pending Crew", "pending-crew", catalogm.TagCategoryCrew)
	artistOnly := suite.createTagInCategory("Artist Edge Crew", "artist-edge-crew", catalogm.TagCategoryCrew)
	venueOnly := suite.createTagInCategory("Venue Edge Crew", "venue-edge-crew", catalogm.TagCategoryCrew)

	approved := suite.createApprovedShow("counted night", rebel.ID, artist.ID, user.ID, when)
	suite.tagShow(approved.ID, counted, user.ID)
	suite.tagShow(approved.ID, genre, user.ID)

	pending := suite.createPendingShow("pending night", rebel.ID, artist.ID, user.ID, when.AddDate(0, 0, 1))
	suite.tagShow(pending.ID, pendingOnly, user.ID)

	// The two non-show edges, on entities that ARE in the scene, so only the
	// entity_type pin can be what excludes them. A room mis-tagged with its
	// resident promoter is the likelier of the two in practice.
	suite.tagArtist(artist.ID, artistOnly, user.ID)
	suite.tagEntity(catalogm.TagEntityVenue, valley.ID, venueOnly, user.ID)

	suite.Equal([]string{"counted-crew"}, suite.crewSlugs("Phoenix", "AZ"))
}

// The scene's rooms are every room in scope, NOT the verified ones the rooms
// leaderboard publishes. A crew that books only unverified DIY spaces is
// precisely what this list exists to surface, so swapping venuePredicate for
// trackedVenuePredicate has to fail here.
//
// The scene still clears its existence gate on the two verified rooms, which is
// what makes this an assertion about the bare predicate rather than a 404 test.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCrews_CountsUnverifiedRooms() {
	suite.crewScene()
	diy := suite.createUnverifiedVenue("The Trunk Space Basement", "Phoenix", "AZ")
	user := suite.createUser()
	artist := suite.createArtist("DIY Band")

	crew := suite.createTagInCategory("Basement Bookings", "basement-bookings", catalogm.TagCategoryCrew)
	show := suite.createApprovedShow("diy night", diy.ID, artist.ID, user.ID, time.Now().UTC().AddDate(0, 0, -8))
	suite.tagShow(show.ID, crew, user.ID)

	crews, err := suite.sceneService.GetSceneCrews("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(crews, 1)
	suite.Equal("basement-bookings", crews[0].Slug)
	suite.Equal(1, crews[0].ShowCount)
}

// A scene with no crew tag gets an empty slice, never an error and never nil,
// so the handler is not the layer that has to invent [].
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCrews_EmptyWhenNoCrewTags() {
	suite.crewScene()

	crews, err := suite.sceneService.GetSceneCrews("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.NotNil(crews)
	suite.Empty(crews)
}

// The existence gate. A real place below the verified-venue threshold is not a
// scene, and an empty crew row would otherwise publish "nobody books here"
// about a town this site does not cover.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCrews_BelowVenueThresholdIsSceneNotFound() {
	suite.createVerifiedVenue("Lone Room", "Flagstaff", "AZ")

	_, err := suite.sceneService.GetSceneCrews("Flagstaff", "AZ")
	suite.Require().Error(err)

	var sceneErr *apperrors.SceneError
	suite.Require().ErrorAs(err, &sceneErr)
	suite.Equal(apperrors.CodeSceneNotFound, sceneErr.Code)
}
