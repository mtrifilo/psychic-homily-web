package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The city facet's ordinal premise, one test per endpoint: under a given filter
// set, each city's count is the total its list reports for that city, so the SUM
// over every city is the total for no city at all.
//
// It is the premise the bottom sheet's apply button is derived from ("Show N
// venues" is the sum of the ticked rows' counts), so a facet counted on a wider
// set than the list it filters puts a number on that button the page then
// contradicts.
//
// Every fixture below is tagged with a slug unique to its test, and every
// assertion is made under that tag. That is what makes the sum exact rather than
// approximate: the suites share one database across their tests, so an
// unfiltered sum would also count rows seeded by neighbours.
//
// The invariant has ONE documented exemption, exercised by the third case here:
// a row carrying no city or state belongs to the list and to no facet row,
// because there is no place to file it under.

// tagSlugFor mints a slug no other test in the package uses, so a fixture is
// only ever counted by the test that seeded it.
func tagSlugFor(name string) string {
	return fmt.Sprintf("facet-%s-%d", name, time.Now().UnixNano())
}

func sumVenueCityCounts(cities []*contracts.VenueCityResponse) int64 {
	var sum int64
	for _, c := range cities {
		sum += int64(c.VenueCount)
	}
	return sum
}

func sumShowCityCounts(cities []contracts.ShowCityResponse) int64 {
	var sum int64
	for _, c := range cities {
		sum += int64(c.ShowCount)
	}
	return sum
}

func sumArtistCityCounts(cities []*contracts.ArtistCityResponse) int64 {
	var sum int64
	for _, c := range cities {
		sum += int64(c.ArtistCount)
	}
	return sum
}

// createArtistInCity places an artist, which createTestArtist deliberately does
// not: a facet keyed on (city, state) needs both columns set, and the empty pair
// is the exemption this file documents.
func (suite *ArtistServiceIntegrationTestSuite) createArtistInCity(name, city, state string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name}
	if city != "" {
		artist.City = &city
	}
	if state != "" {
		artist.State = &state
	}
	suite.Require().NoError(suite.db.Create(artist).Error)
	return artist
}

// createTaggedArtist is the shows suite's tag fixture. A show is not tagged
// directly: the filter reaches a bill transitively through its artists, so the
// tag has to hang on an artist for the show to match. An empty slug leaves the
// artist untagged, which is what the control rows need.
func (suite *ShowServiceIntegrationTestSuite) createTaggedArtist(name, slug string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: fmt.Sprintf("%s %d", name, time.Now().UnixNano())}
	suite.Require().NoError(suite.db.Create(artist).Error)
	if slug == "" {
		return artist
	}

	var tag catalogm.Tag
	if err := suite.db.Where("slug = ?", slug).First(&tag).Error; err != nil {
		tag = catalogm.Tag{Name: slug, Slug: slug, Category: catalogm.TagCategoryGenre}
		suite.Require().NoError(suite.db.Create(&tag).Error)
	}
	suite.Require().NoError(suite.db.Create(&catalogm.EntityTag{
		TagID:         tag.ID,
		EntityType:    catalogm.TagEntityArtist,
		EntityID:      artist.ID,
		AddedByUserID: suite.createTestUser().ID,
	}).Error)
	return artist
}

func (suite *ShowServiceIntegrationTestSuite) billArtist(showID, artistID uint) {
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: showID, ArtistID: artistID, Position: 0,
	}).Error)
}

// =============================================================================
// GET /venues/cities
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueCities_ScopedSumEqualsTheListTotal() {
	user := suite.createTestUser()
	slug := tagSlugFor("venues")
	tagID := suite.createGenreTag(slug, slug)

	tagged := []*catalogm.Venue{
		suite.createTestVenue("Facet PHX One", "Phoenix", "AZ", true),
		suite.createTestVenue("Facet PHX Two", "Phoenix", "AZ", true),
		suite.createTestVenue("Facet TUC One", "Tucson", "AZ", true),
	}
	for _, v := range tagged {
		suite.tagVenue(v.ID, tagID, user.ID)
	}
	// Untagged and unverified neighbours: the first must not be counted by the
	// filter, the second by anything.
	suite.createTestVenue("Facet PHX Untagged", "Phoenix", "AZ", true)
	unverified := suite.createTestVenue("Facet MES Unverified", "Mesa", "AZ", false)
	suite.tagVenue(unverified.ID, tagID, user.ID)

	filters := contracts.VenueListFilters{Sort: "upcoming", TagSlugs: []string{slug}}

	cities, err := suite.venueService.GetVenueCities(filters)
	suite.Require().NoError(err)

	_, total, err := suite.venueService.GetVenuesWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(3), total, "the tag should select exactly the three verified fixtures")
	suite.Equal(total, sumVenueCityCounts(cities),
		"the sum of the scoped city counts is the list total under the same filters")

	// The per-city halves, because a sum can agree while the split is wrong.
	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.VenueCount
	}
	suite.Equal(2, byCity["Phoenix"])
	suite.Equal(1, byCity["Tucson"])
	suite.NotContains(byCity, "Mesa", "an unverified venue is outside the browse gate")

	// Each row is also the total the list reports for that city alone.
	phoenixOnly := filters
	phoenixOnly.Cities = []contracts.CityStateFilter{{City: "Phoenix", State: "AZ"}}
	_, phoenixTotal, err := suite.venueService.GetVenuesWithShowCounts(phoenixOnly, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(byCity["Phoenix"]), phoenixTotal)
}

// The unfiltered read is the one every other surface already depends on, so it
// has to answer exactly as it did before the endpoint took filters at all.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenueCities_UnfilteredIgnoresTheTagFixtures() {
	user := suite.createTestUser()
	slug := tagSlugFor("venues-unfiltered")
	tagID := suite.createGenreTag(slug, slug)

	tagged := suite.createTestVenue("Facet Unfiltered Tagged", "Flagstaff", "AZ", true)
	suite.tagVenue(tagged.ID, tagID, user.ID)
	suite.createTestVenue("Facet Unfiltered Plain", "Flagstaff", "AZ", true)

	cities, err := suite.venueService.GetVenueCities(contracts.VenueListFilters{})
	suite.Require().NoError(err)

	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.VenueCount
	}
	suite.Equal(2, byCity["Flagstaff"], "an unfiltered facet counts tagged and untagged rooms alike")
}

// =============================================================================
// GET /shows/cities
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_ScopedSumEqualsTheListTotal() {
	user := suite.createTestUser()
	slug := tagSlugFor("shows")

	phoenix := suite.createTestVenue("Facet Show PHX Room", "Phoenix", "AZ", true)
	tucson := suite.createTestVenue("Facet Show TUC Room", "Tucson", "AZ", true)

	tagged := suite.createTaggedArtist("Facet Show Band", slug)
	untagged := suite.createTaggedArtist("Facet Show Other Band", "")

	at := time.Now().UTC().AddDate(0, 0, 7)
	suite.billArtist(suite.createApprovedShowAt(phoenix.ID, user.ID, "Phoenix", "AZ", at).ID, tagged.ID)
	suite.billArtist(suite.createApprovedShowAt(phoenix.ID, user.ID, "Phoenix", "AZ", at).ID, tagged.ID)
	suite.billArtist(suite.createApprovedShowAt(tucson.ID, user.ID, "Tucson", "AZ", at).ID, tagged.ID)
	// A show on the same nights whose bill does not carry the tag.
	suite.billArtist(suite.createApprovedShowAt(phoenix.ID, user.ID, "Phoenix", "AZ", at).ID, untagged.ID)

	filters := &contracts.UpcomingShowsFilter{TagSlugs: []string{slug}}

	cities, err := suite.showService.GetShowCities("UTC", filters, contracts.ShowCalendarWindow{})
	suite.Require().NoError(err)

	_, total, err := suite.showService.GetUpcomingShowsPage(
		contracts.ShowCalendarQuery{Limit: 50}, false, filters,
	)
	suite.Require().NoError(err)

	suite.Equal(int64(3), total, "the tag should select exactly the three tagged bills")
	suite.Equal(total, sumShowCityCounts(cities),
		"the sum of the scoped city counts is the list total under the same filters")

	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.ShowCount
	}
	suite.Equal(2, byCity["Phoenix"])
	suite.Equal(1, byCity["Tucson"])
}

// The window narrows the facet exactly as it narrows the list, which is the half
// the tag case cannot show: a window is applied to the venue-local calendar day
// rather than to a row's own columns.
func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_ScopedSumEqualsTheListTotalInAWindow() {
	user := suite.createTestUser()
	slug := tagSlugFor("shows-window")

	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Facet Window Room", "AZ", zone, true)
	tagged := suite.createTaggedArtist("Facet Window Band", slug)

	inWindow := venueLocalInstant(suite.T(), zone, 7, 20)
	outOfWindow := venueLocalInstant(suite.T(), zone, 30, 20)
	loc, err := time.LoadLocation(zone)
	suite.Require().NoError(err)
	local := inWindow.In(loc)

	suite.billArtist(suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", inWindow).ID, tagged.ID)
	suite.billArtist(suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", inWindow).ID, tagged.ID)
	suite.billArtist(suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", outOfWindow).ID, tagged.ID)

	filters := &contracts.UpcomingShowsFilter{TagSlugs: []string{slug}}
	window := contracts.ShowCalendarWindow{
		Year:  local.Year(),
		Month: int(local.Month()),
		Day:   local.Day(),
	}

	cities, err := suite.showService.GetShowCities("UTC", filters, window)
	suite.Require().NoError(err)

	_, total, err := suite.showService.GetUpcomingShowsPage(
		contracts.ShowCalendarQuery{ShowCalendarWindow: window, Limit: 50}, false, filters,
	)
	suite.Require().NoError(err)

	suite.Equal(int64(2), total, "only the two shows on the addressed venue-local day")
	suite.Equal(total, sumShowCityCounts(cities),
		"the sum of the scoped city counts is the list total under the same window")
}

// A half-stated window narrows to nothing in SQL, so the facet refuses it rather
// than answering for the whole upcoming catalog.
func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_RefusesAHalfStatedWindow() {
	_, err := suite.showService.GetShowCities("UTC", nil, contracts.ShowCalendarWindow{Month: 6})
	suite.Require().Error(err)
}

// =============================================================================
// GET /artists/cities
// =============================================================================

// The artists case carries the extra half: a tag filter drops the activity gate
// (PSY-495), so the facet has to drop it too or it counts the gated set under a
// filter that lists the evergreen one.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistCities_ScopedSumEqualsTheListTotal() {
	slug := tagSlugFor("artists")
	venue := suite.createTestVenue("Facet Artist Room", "Phoenix", "AZ")
	user := suite.createTestUser()

	active := suite.createArtistInCity("Facet Artist Active", "Phoenix", "AZ")
	suite.createApprovedShowWithArtist(active.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.tagArtistForBrowse(active.ID, slug)

	// No upcoming show: outside the gated set, inside the evergreen one.
	quiet := suite.createArtistInCity("Facet Artist Quiet", "Tucson", "AZ")
	suite.tagArtistForBrowse(quiet.ID, slug)

	// Tagged but placeless: the documented exemption. It is in the list total and
	// in no facet row.
	placeless := suite.createArtistInCity("Facet Artist Placeless", "", "")
	suite.tagArtistForBrowse(placeless.ID, slug)

	// Untagged neighbour with a show, to prove the tag is doing the selecting.
	other := suite.createArtistInCity("Facet Artist Untagged", "Phoenix", "AZ")
	suite.createApprovedShowWithArtist(other.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	filters := map[string]interface{}{
		"tag_filter":         TagFilter{TagSlugs: []string{slug}},
		"skip_active_filter": true,
	}

	cities, err := suite.artistService.GetArtistCities(filters)
	suite.Require().NoError(err)

	_, total, err := suite.artistService.GetArtistsWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(3), total, "the evergreen list carries the quiet and the placeless artist")
	suite.Equal(total-1, sumArtistCityCounts(cities),
		"the sum is the list total less the one artist that names no place")

	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.ArtistCount
	}
	suite.Equal(1, byCity["Phoenix"], "the untagged Phoenix artist is outside the filter")
	suite.Equal(1, byCity["Tucson"], "a quiet artist is counted because the tag drops the activity gate")
}

// The unfiltered facet keeps the activity gate, which is what the /artists
// landing page's counts have always meant.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistCities_UnfilteredKeepsTheActivityGate() {
	venue := suite.createTestVenue("Facet Gate Room", "Phoenix", "AZ")
	user := suite.createTestUser()

	active := suite.createArtistInCity("Facet Gate Active", "Sedona", "AZ")
	suite.createApprovedShowWithArtist(active.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.createArtistInCity("Facet Gate Quiet", "Sedona", "AZ")

	cities, err := suite.artistService.GetArtistCities(nil)
	suite.Require().NoError(err)

	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.ArtistCount
	}
	suite.Equal(1, byCity["Sedona"], "an artist with no upcoming show is outside the gated facet")
}
