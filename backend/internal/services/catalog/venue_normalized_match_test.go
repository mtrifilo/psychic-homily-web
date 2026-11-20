package catalog

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The production defect: "7th St Entry" was created alongside "7th Street Entry"
// in Minneapolis. The exact-match lookup and the unique index on
// (lower(name), lower(city)) both passed, because the two lowercase strings
// genuinely differ. The duplicate venue then silently duplicated ~90 shows,
// since show dedup keys on (artist_id, venue_id, event_date).
func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_MatchesAbbreviatedStreet() {
	_, err := suite.venueService.CreateVenue(&contracts.CreateVenueRequest{
		Name:  "7th Street Entry",
		City:  "Minneapolis",
		State: "MN",
	}, true)
	suite.Require().NoError(err)

	venue, created, err := suite.venueService.FindOrCreateVenue("7th St Entry", "Minneapolis", "MN", nil, nil, nil, false)

	suite.Require().NoError(err)
	suite.False(created, "abbreviated street form must resolve to the existing room, not create a duplicate")
	suite.Equal("7th Street Entry", venue.Name, "the canonical existing record wins")
}

func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_MatchesLeadingArticleAndPunctuation() {
	_, err := suite.venueService.CreateVenue(&contracts.CreateVenueRequest{
		Name:  "Schuba's Tavern",
		City:  "Chicago",
		State: "IL",
	}, true)
	suite.Require().NoError(err)

	venue, created, err := suite.venueService.FindOrCreateVenue("The Schubas Tavern", "Chicago", "IL", nil, nil, nil, false)

	suite.Require().NoError(err)
	suite.False(created, "apostrophe + leading article must not spawn a duplicate")
	suite.Equal("Schuba's Tavern", venue.Name)
}

// The load-bearing negative case. Salt Shed and Salt Shed Fairgrounds are
// genuinely separate rooms at the same complex — normalization must NOT merge
// them. A false merge destroys data; a missed merge only leaves a duplicate.
func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_DoesNotMergeDistinctRooms() {
	_, err := suite.venueService.CreateVenue(&contracts.CreateVenueRequest{
		Name:  "Salt Shed",
		City:  "Chicago",
		State: "IL",
	}, true)
	suite.Require().NoError(err)

	venue, created, err := suite.venueService.FindOrCreateVenue("Salt Shed Fairgrounds", "Chicago", "IL", nil, nil, nil, false)

	suite.Require().NoError(err)
	suite.True(created, "distinct rooms at one complex must stay distinct")
	suite.Equal("Salt Shed Fairgrounds", venue.Name)
}

// Normalization is scoped to a single city, so two rooms that share a name in
// different cities must never collapse into one.
func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_NormalizedMatchIsScopedToCity() {
	_, err := suite.venueService.CreateVenue(&contracts.CreateVenueRequest{
		Name:  "The Metro",
		City:  "Chicago",
		State: "IL",
	}, true)
	suite.Require().NoError(err)

	venue, created, err := suite.venueService.FindOrCreateVenue("Metro", "Baltimore", "MD", nil, nil, nil, false)

	suite.Require().NoError(err)
	suite.True(created, "a same-named room in another city is a different venue")
	suite.Equal("Baltimore", venue.City)
}

// A name that is an existing room's name followed by the room's own city is
// that room, so it resolves to it instead of creating a twin.
func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_NameEndingInItsCityReusesTheTwin() {
	existing, err := suite.venueService.CreateVenue(&contracts.CreateVenueRequest{
		Name:  "Rialto Theatre",
		City:  "Tucson",
		State: "AZ",
	}, true)
	suite.Require().NoError(err)

	venue, created, err := suite.venueService.FindOrCreateVenue("Rialto Theater Tucson", "tucson", "az", nil, nil, nil, true)

	suite.Require().NoError(err)
	suite.False(created, "the city-suffixed spelling must resolve to the existing room")
	suite.Equal(existing.ID, venue.ID)
	suite.Equal("Rialto Theatre", venue.Name, "the existing record keeps its name")
	suite.assertVenueCount(1)
}

// A room whose real name ends in its city, with no twin, is created as named.
func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_NameEndingInItsCityWithNoTwinIsCreated() {
	venue, created, err := suite.venueService.FindOrCreateVenue("House of Blues Chicago", "Chicago", "IL", nil, nil, nil, true)

	suite.Require().NoError(err)
	suite.True(created)
	suite.Equal("House of Blues Chicago", venue.Name, "the name is stored as given, city included")
	suite.Require().NotNil(venue.Slug)
	suite.Equal("house-of-blues-chicago-chicago-il", *venue.Slug)
}

// The twin must be in the same city: "House of Blues" in Las Vegas does not
// make "House of Blues Chicago" in Chicago a duplicate.
func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_CityStrippedMatchIgnoresOtherCities() {
	_, err := suite.venueService.CreateVenue(&contracts.CreateVenueRequest{
		Name:  "House of Blues",
		City:  "Las Vegas",
		State: "NV",
	}, true)
	suite.Require().NoError(err)

	venue, created, err := suite.venueService.FindOrCreateVenue("House of Blues Chicago", "Chicago", "IL", nil, nil, nil, true)

	suite.Require().NoError(err)
	suite.True(created, "a same-named room in another city is a different venue")
	suite.Equal("Chicago", venue.City)
	suite.assertVenueCount(2)
}

// The city-stripped match also requires the state: Portland, Maine is not
// Portland, Oregon, so a suffixed name in one never resolves to a room in the
// other.
func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_CityStrippedMatchRequiresTheState() {
	_, err := suite.venueService.CreateVenue(&contracts.CreateVenueRequest{
		Name:  "State Theatre",
		City:  "Portland",
		State: "OR",
	}, true)
	suite.Require().NoError(err)

	venue, created, err := suite.venueService.FindOrCreateVenue("State Theatre Portland", "Portland", "ME", nil, nil, nil, true)

	suite.Require().NoError(err)
	suite.True(created, "a twin in a same-named city of another state is a different room")
	suite.Equal("ME", venue.State)
	suite.assertVenueCount(2)
}

func (suite *VenueServiceIntegrationTestSuite) assertVenueCount(want int64) {
	var count int64
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Count(&count).Error)
	suite.Equal(want, count)
}
