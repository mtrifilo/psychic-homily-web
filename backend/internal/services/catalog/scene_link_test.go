package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
)

func TestSceneLinkPlace_IsUSPlace(t *testing.T) {
	tests := []struct {
		name  string
		place sceneLinkPlace
		want  bool
	}{
		{"US state, no country", sceneLinkPlace{City: "Phoenix", State: "AZ"}, true},
		{"US state, blank country", sceneLinkPlace{City: "Phoenix", State: "AZ", Country: stringPtr("  ")}, true},
		{"US state, USA spelled out", sceneLinkPlace{City: "Phoenix", State: "az", Country: stringPtr("United States")}, true},
		{"US state, US code", sceneLinkPlace{City: "Phoenix", State: "AZ", Country: stringPtr("US")}, true},
		{"non-US state", sceneLinkPlace{City: "Leeds", State: "England"}, false},
		{"Canadian province", sceneLinkPlace{City: "Toronto", State: "ON"}, false},
		{"US state code with a non-US country", sceneLinkPlace{City: "Perth", State: "WA", Country: stringPtr("Australia")}, false},
		{"US state code with an unrecognised country", sceneLinkPlace{City: "Perth", State: "WA", Country: stringPtr("Nowhereland")}, false},
		{"no city", sceneLinkPlace{City: " ", State: "AZ"}, false},
		{"no state", sceneLinkPlace{City: "Phoenix"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.place.isUSPlace())
		})
	}
}

func TestArtistHasAnyLocation(t *testing.T) {
	assert.False(t, artistHasAnyLocation(&catalogm.Artist{}))
	assert.False(t, artistHasAnyLocation(&catalogm.Artist{City: stringPtr(""), State: stringPtr(" "), Country: stringPtr("")}))
	assert.True(t, artistHasAnyLocation(&catalogm.Artist{City: stringPtr("Phoenix")}))
	assert.True(t, artistHasAnyLocation(&catalogm.Artist{State: stringPtr("AZ")}))
	assert.True(t, artistHasAnyLocation(&catalogm.Artist{Country: stringPtr("Japan")}))
}

func TestArtistOwnPlace_NormalisesASpelledOutState(t *testing.T) {
	place := artistOwnPlace(&catalogm.Artist{City: stringPtr("Phoenix"), State: stringPtr(" Arizona ")})
	assert.Equal(t, "AZ", place.State)
	assert.True(t, place.isUSPlace())

	kept := artistOwnPlace(&catalogm.Artist{City: stringPtr("Leeds"), State: stringPtr("England")})
	assert.Equal(t, "England", kept.State, "a value the state map does not know is kept as stored")
	assert.False(t, kept.isUSPlace())
}

func TestIsKnownNonUSPlace(t *testing.T) {
	g := geo.Default()
	tests := []struct {
		name  string
		place sceneLinkPlace
		want  bool
	}{
		{"non-US country field", sceneLinkPlace{City: "Perth", State: "WA", Country: stringPtr("Australia")}, true},
		{"US country field", sceneLinkPlace{City: "Phoenix", State: "Arizonaa", Country: stringPtr("USA")}, false},
		{"a UK city with a region for a state", sceneLinkPlace{City: "Leeds", State: "England"}, true},
		{"a state that is another country's name", sceneLinkPlace{State: "Japan"}, true},
		{"a US city with a misspelled state", sceneLinkPlace{City: "Phoenix", State: "Arizonaa"}, false},
		{"nothing to place", sceneLinkPlace{State: "Nowhere"}, false},
		{"a city the geocoder misses, with a non-US country", sceneLinkPlace{City: "Kiev", Country: stringPtr("Ukraine")}, true},
		{"the geocoder places a city and province in Canada", sceneLinkPlace{City: "Toronto", State: "ON"}, true},
		{"the geocoder places a bare city abroad", sceneLinkPlace{City: "Paris"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isKnownNonUSPlace(g, tt.place))
		})
	}
}

// =============================================================================
// INTEGRATION (runs inside SceneServiceIntegrationTestSuite for its teardown
// and fixtures)
// =============================================================================

// sceneLink resolves a place's link and, whenever one comes back, proves the
// invariant every link carries: its slug passes the /scenes soft-404 gate and
// GET /scenes/{slug} renders a page whose own slug is the link's.
func (suite *SceneServiceIntegrationTestSuite) sceneLink(city, state string, country *string) *contracts.SceneLinkResponse {
	link, err := servedSceneLink(suite.db, geo.Default(), sceneLinkPlace{City: city, State: state, Country: country})
	suite.Require().NoError(err)
	if link != nil {
		suite.requireLinkServes(link)
	}
	return link
}

func (suite *SceneServiceIntegrationTestSuite) requireLinkServes(link *contracts.SceneLinkResponse) {
	exists, err := NewEntityExistenceService(suite.db, suite.sceneService).sceneExists(link.Slug)
	suite.Require().NoError(err)
	suite.Require().True(exists, "the soft-404 gate must pass %q", link.Slug)

	pageCity, pageState, err := suite.sceneService.ParseSceneSlug(link.Slug)
	suite.Require().NoError(err)
	page, err := suite.sceneService.GetSceneDetail(pageCity, pageState)
	suite.Require().NoError(err, "GET /scenes/%s must render", link.Slug)
	suite.Equal(link.Slug, page.Slug)
	suite.Equal(link.City, page.City)
	suite.Equal(link.State, page.State)
}

func (suite *SceneServiceIntegrationTestSuite) TestServedSceneLink_USMetroPrincipal() {
	suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")

	link := suite.sceneLink("Phoenix", "AZ", nil)
	suite.Require().NotNil(link)
	suite.Equal(contracts.SceneLinkResponse{Slug: "phoenix-az", City: "Phoenix", State: "AZ"}, *link)
}

func (suite *SceneServiceIntegrationTestSuite) TestServedSceneLink_MetroMemberNamesThePrincipal() {
	suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createVerifiedVenue("Yucca Tap Room", "Tempe", "AZ")

	link := suite.sceneLink("Tempe", "AZ", nil)
	suite.Require().NotNil(link)
	suite.Equal("phoenix-az", link.Slug, "a member city links to its metro's canonical scene")
	suite.Equal("Phoenix", link.City)
}

func (suite *SceneServiceIntegrationTestSuite) TestServedSceneLink_BelowTheVenueFloorIsNil() {
	suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createUnverifiedVenue("House Show", "Phoenix", "AZ")

	suite.Nil(suite.sceneLink("Phoenix", "AZ", nil), "one verified room is not a scene; the page would 404")
}

func (suite *SceneServiceIntegrationTestSuite) TestServedSceneLink_NonUSIsNilEvenWhenThePageServes() {
	suite.createVerifiedVenue("Brudenell Social Club", "Leeds", "England")
	suite.createVerifiedVenue("Belgrave Music Hall", "Leeds", "England")

	// The scene page itself renders: the omission is this link's own rule.
	city, state, err := suite.sceneService.ParseSceneSlug("leeds-england")
	suite.Require().NoError(err)
	_, err = suite.sceneService.GetSceneDetail(city, state)
	suite.Require().NoError(err)

	suite.Nil(suite.sceneLink("Leeds", "England", nil))
}

func (suite *SceneServiceIntegrationTestSuite) TestServedSceneLink_USPlaceWithNoCBSAUsesItsVerifiedVenues() {
	suite.Require().Nil(seedMetro("Marfa", "TX"), "fixture must be a US place outside every CBSA")
	suite.createVerifiedVenue("Ballroom Marfa", "Marfa", "TX")
	suite.createVerifiedVenue("Lost Horse Saloon", "Marfa", "TX")

	link := suite.sceneLink("Marfa", "TX", nil)
	suite.Require().NotNil(link)
	suite.Equal(contracts.SceneLinkResponse{Slug: "marfa-tx", City: "Marfa", State: "TX"}, *link)
}

func (suite *SceneServiceIntegrationTestSuite) TestServedSceneLink_UnknownPlaceIsNil() {
	suite.Nil(suite.sceneLink("Marfa", "TX", nil), "no verified room at all")
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_OwnLocation() {
	suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	artist := suite.createArtistIn("Local Band", "Phoenix", "AZ")

	resp, err := NewArtistService(suite.db).GetArtist(artist.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp.Scene)
	suite.requireLinkServes(resp.Scene)
	suite.Equal("phoenix-az", resp.Scene.Slug)
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_NoLocationTakesTheLatestShowsVenue() {
	user := suite.createUser()
	valleyBar := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	recordBar := suite.createVerifiedVenue("Record Bar", "Kansas City", "MO")
	suite.createVerifiedVenue("Lemonade Park", "Kansas City", "MO")

	artist := &catalogm.Artist{Name: "Nowhere Band"}
	suite.Require().NoError(suite.db.Create(artist).Error)
	suite.createApprovedShow("Older", valleyBar.ID, artist.ID, user.ID, time.Now().AddDate(0, -6, 0))
	suite.createApprovedShow("Latest", recordBar.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))

	resp, err := NewArtistService(suite.db).GetArtistBySlug(suite.slugOf(artist))
	suite.Require().NoError(err)
	suite.Require().NotNil(resp.Scene)
	suite.requireLinkServes(resp.Scene)
	suite.Equal("kansas-city-mo", resp.Scene.Slug)
	suite.Equal("Kansas City", resp.Scene.City)
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_NoLocationIgnoresUnapprovedShows() {
	user := suite.createUser()
	valleyBar := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	recordBar := suite.createVerifiedVenue("Record Bar", "Kansas City", "MO")
	suite.createVerifiedVenue("Lemonade Park", "Kansas City", "MO")

	artist := &catalogm.Artist{Name: "Pending Band"}
	suite.Require().NoError(suite.db.Create(artist).Error)
	suite.createApprovedShow("Approved", valleyBar.ID, artist.ID, user.ID, time.Now().AddDate(0, -6, 0))
	pending := suite.createApprovedShow("Pending", recordBar.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))
	suite.Require().NoError(suite.db.Model(pending).Update("status", catalogm.ShowStatusPending).Error)

	resp, err := NewArtistService(suite.db).GetArtist(artist.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp.Scene)
	suite.requireLinkServes(resp.Scene)
	suite.Equal("phoenix-az", resp.Scene.Slug)
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_NoLocationNoShowsIsNil() {
	suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	artist := &catalogm.Artist{Name: "Unplaced Band"}
	suite.Require().NoError(suite.db.Create(artist).Error)

	resp, err := NewArtistService(suite.db).GetArtist(artist.ID)
	suite.Require().NoError(err)
	suite.Nil(resp.Scene)
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_NonUSLocationDoesNotBorrowAShow() {
	user := suite.createUser()
	valleyBar := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	artist := suite.createArtistIn("Leeds Band", "Leeds", "England")
	suite.createApprovedShow("Tour stop", valleyBar.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))

	resp, err := NewArtistService(suite.db).GetArtist(artist.ID)
	suite.Require().NoError(err)
	suite.Nil(resp.Scene, "an artist based abroad keeps its own location; a US show does not relabel it")
}

// seedTwoScenes makes Phoenix and Kansas City both serve and returns one room
// in each.
func (suite *SceneServiceIntegrationTestSuite) seedTwoScenes() (phoenix, kansasCity *catalogm.Venue) {
	phoenix = suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	kansasCity = suite.createVerifiedVenue("Record Bar", "Kansas City", "MO")
	suite.createVerifiedVenue("Lemonade Park", "Kansas City", "MO")
	return phoenix, kansasCity
}

func (suite *SceneServiceIntegrationTestSuite) unplacedArtist(name string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name}
	suite.Require().NoError(suite.db.Create(artist).Error)
	return artist
}

func (suite *SceneServiceIntegrationTestSuite) artistSceneSlug(artist *catalogm.Artist) string {
	resp, err := NewArtistService(suite.db).GetArtist(artist.ID)
	suite.Require().NoError(err)
	if resp.Scene == nil {
		return ""
	}
	suite.requireLinkServes(resp.Scene)
	return resp.Scene.Slug
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_PastShowBeatsAnUpcomingOne() {
	user := suite.createUser()
	phoenix, kansasCity := suite.seedTwoScenes()
	artist := suite.unplacedArtist("Touring Band")
	suite.createApprovedShow("Played", phoenix.ID, artist.ID, user.ID, time.Now().AddDate(0, -3, 0))
	suite.createApprovedShow("Booked", kansasCity.ID, artist.ID, user.ID, time.Now().AddDate(0, 1, 0))

	suite.Equal("phoenix-az", suite.artistSceneSlug(artist),
		"the most recent show dated up to today wins over any upcoming one")
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_OnlyUpcomingTakesTheNearest() {
	user := suite.createUser()
	phoenix, kansasCity := suite.seedTwoScenes()
	artist := suite.unplacedArtist("New Band")
	suite.createApprovedShow("Later", kansasCity.ID, artist.ID, user.ID, time.Now().AddDate(0, 2, 0))
	suite.createApprovedShow("Sooner", phoenix.ID, artist.ID, user.ID, time.Now().AddDate(0, 1, 0))

	suite.Equal("phoenix-az", suite.artistSceneSlug(artist))
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_CancelledShowsDoNotCount() {
	user := suite.createUser()
	phoenix, kansasCity := suite.seedTwoScenes()
	artist := suite.unplacedArtist("Cancelled Band")
	suite.createApprovedShow("Older", phoenix.ID, artist.ID, user.ID, time.Now().AddDate(0, -6, 0))
	cancelled := suite.createApprovedShow("Called off", kansasCity.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))
	suite.Require().NoError(suite.db.Model(cancelled).Update("is_cancelled", true).Error)

	suite.Equal("phoenix-az", suite.artistSceneSlug(artist), "a more recent cancelled show is skipped")
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_OnlyCancelledShowsIsNil() {
	user := suite.createUser()
	phoenix, _ := suite.seedTwoScenes()
	artist := suite.unplacedArtist("Never Played")
	cancelled := suite.createApprovedShow("Called off", phoenix.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))
	suite.Require().NoError(suite.db.Model(cancelled).Update("is_cancelled", true).Error)

	suite.Equal("", suite.artistSceneSlug(artist))
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_SpelledOutStateIsNormalised() {
	user := suite.createUser()
	_, kansasCity := suite.seedTwoScenes()
	artist := suite.createArtistInNullMetro("Spelled Out", "Phoenix", "Arizona")
	suite.createApprovedShow("Tour", kansasCity.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))

	suite.Equal("phoenix-az", suite.artistSceneSlug(artist),
		"the artist's own normalised state wins; its show elsewhere is not consulted")
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_UnknownStateFallsBackToTheLatestShow() {
	user := suite.createUser()
	_, kansasCity := suite.seedTwoScenes()
	artist := suite.createArtistInNullMetro("Typo Band", "Phoenix", "Arizonaa")
	suite.createApprovedShow("Tour", kansasCity.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))

	suite.Equal("kansas-city-mo", suite.artistSceneSlug(artist))
}

// A city with no state names no scene slug and is not known to be outside
// the US, so it takes the fallback rather than a population-guessed state.
func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_CityAloneFallsBackToTheLatestShow() {
	user := suite.createUser()
	_, kansasCity := suite.seedTwoScenes()
	artist := &catalogm.Artist{Name: "City Only", City: stringPtr("Phoenix")}
	suite.Require().NoError(suite.db.Create(artist).Error)
	suite.createApprovedShow("Tour", kansasCity.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))

	suite.Equal("kansas-city-mo", suite.artistSceneSlug(artist))
}

func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_NonUSCountryNeverBorrowsAShow() {
	user := suite.createUser()
	phoenix, _ := suite.seedTwoScenes()
	artist := &catalogm.Artist{Name: "Kyiv Band", City: stringPtr("Kiev"), Country: stringPtr("Ukraine")}
	suite.Require().NoError(suite.db.Create(artist).Error)
	suite.createApprovedShow("US tour", phoenix.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))

	suite.Equal("", suite.artistSceneSlug(artist))
}

// Decided by the geocoder alone: no country field, a province code rather
// than a country name.
func (suite *SceneServiceIntegrationTestSuite) TestArtistSceneLink_GeocodedAbroadNeverBorrowsAShow() {
	user := suite.createUser()
	phoenix, _ := suite.seedTwoScenes()
	artist := suite.createArtistInNullMetro("Toronto Band", "Toronto", "ON")
	suite.createApprovedShow("US tour", phoenix.ID, artist.ID, user.ID, time.Now().AddDate(0, -1, 0))

	suite.Equal("", suite.artistSceneSlug(artist))
}

func (suite *SceneServiceIntegrationTestSuite) TestVenueAndShowDetailCarryTheScene() {
	user := suite.createUser()
	valleyBar := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	artist := suite.createArtist("Local Band")
	show := suite.createApprovedShow("Show", valleyBar.ID, artist.ID, user.ID, time.Now().AddDate(0, 0, 7))

	venueResp, err := NewVenueService(suite.db).GetVenueDetail(suite.venueSlugOf(valleyBar))
	suite.Require().NoError(err)
	suite.Require().NotNil(venueResp.Scene)
	suite.requireLinkServes(venueResp.Scene)
	suite.Equal("phoenix-az", venueResp.Scene.Slug)

	showResp, err := NewShowService(suite.db).GetShow(show.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(showResp.Scene)
	suite.requireLinkServes(showResp.Scene)
	suite.Equal("phoenix-az", showResp.Scene.Slug)
}

func (suite *SceneServiceIntegrationTestSuite) TestVenueAndShowDetailOmitANonUSScene() {
	user := suite.createUser()
	brudenell := suite.createVerifiedVenue("Brudenell Social Club", "Leeds", "England")
	suite.createVerifiedVenue("Belgrave Music Hall", "Leeds", "England")
	artist := suite.createArtistIn("Leeds Band", "Leeds", "England")
	show := suite.createApprovedShow("Show", brudenell.ID, artist.ID, user.ID, time.Now().AddDate(0, 0, 7))

	venueResp, err := NewVenueService(suite.db).GetVenueDetail(suite.venueSlugOf(brudenell))
	suite.Require().NoError(err)
	suite.Nil(venueResp.Scene)

	showResp, err := NewShowService(suite.db).GetShow(show.ID)
	suite.Require().NoError(err)
	suite.Nil(showResp.Scene)
}

func (suite *SceneServiceIntegrationTestSuite) slugOf(artist *catalogm.Artist) string {
	var slug string
	suite.Require().NoError(suite.db.Raw("SELECT COALESCE(slug, '') FROM artists WHERE id = ?", artist.ID).Scan(&slug).Error)
	if slug == "" {
		slug = "scene-link-artist"
		suite.Require().NoError(suite.db.Exec("UPDATE artists SET slug = ? WHERE id = ?", slug, artist.ID).Error)
	}
	return slug
}

func (suite *SceneServiceIntegrationTestSuite) venueSlugOf(venue *catalogm.Venue) string {
	var slug string
	suite.Require().NoError(suite.db.Raw("SELECT COALESCE(slug, '') FROM venues WHERE id = ?", venue.ID).Scan(&slug).Error)
	if slug == "" {
		slug = "scene-link-venue"
		suite.Require().NoError(suite.db.Exec("UPDATE venues SET slug = ? WHERE id = ?", slug, venue.ID).Error)
	}
	return slug
}
