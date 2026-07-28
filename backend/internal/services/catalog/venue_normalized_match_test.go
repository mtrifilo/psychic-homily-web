package catalog

import (
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
