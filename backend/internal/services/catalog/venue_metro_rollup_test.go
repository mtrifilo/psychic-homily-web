package catalog

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
)

// =============================================================================
// Integration: GET /venues?metro_rollup=true  (PSY-1574)
// =============================================================================
//
// The Atlas city rail lists "the venues of the Phoenix scene", and the scene is
// keyed by CBSA metro (PSY-1255) — so a Tempe room counts toward the Phoenix
// scene. Before this, the rail asked for the literal city and never listed it,
// contradicting the scene page beside it. These tests pin the widened scope AND
// the two things that must not move with it: the verified-only gate and the
// street-coordinate privacy gate.

// geoVenueService is the venue service wired to the real offline geocoder. The
// shared suite service deliberately has none (its creates would then start
// writing coordinates and change unrelated assertions), but metro rollup IS the
// geocoder, so these tests need one.
func (suite *VenueServiceIntegrationTestSuite) geoVenueService() *VenueService {
	return &VenueService{db: suite.db, geocoder: geo.Default()}
}

// createMetroVenue makes a verified venue with its denormalized CBSA already
// written, exactly as applyGeocoding writes it on every real create/update.
func (suite *VenueServiceIntegrationTestSuite) createMetroVenue(name, city, state, cbsa string) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: true,
	}
	if cbsa != "" {
		venue.Metro = &cbsa
	}
	suite.Require().NoError(suite.db.Create(venue).Error)
	return venue
}

func venueNames(resp []*contracts.VenueWithShowCountResponse) []string {
	names := make([]string, len(resp))
	for i, v := range resp {
		names[i] = v.Name
	}
	return names
}

// The headline behaviour: Phoenix lists its metro members, and does not reach
// past the metro into a different one.
func (suite *VenueServiceIntegrationTestSuite) TestMetroRollup_ListsMemberCityVenues() {
	const phoenixCBSA = "38060"
	suite.createMetroVenue("Crescent Ballroom", "Phoenix", "AZ", phoenixCBSA)
	suite.createMetroVenue("Yucca Tap Room", "Tempe", "AZ", phoenixCBSA)
	suite.createMetroVenue("Nile Theater", "Mesa", "AZ", phoenixCBSA)
	// Tucson is its OWN CBSA — a rollup that swallowed it would be a proximity
	// rule, not the scene's scope.
	suite.createMetroVenue("Hotel Congress", "Tucson", "AZ", "46060")

	svc := suite.geoVenueService()
	filters := contracts.VenueListFilters{City: "Phoenix", State: "AZ", MetroRollup: true}
	resp, total, err := svc.GetVenuesWithShowCounts(filters, 50, 0)

	suite.Require().NoError(err)
	suite.ElementsMatch(
		[]string{"Crescent Ballroom", "Yucca Tap Room", "Nile Theater"},
		venueNames(resp),
	)
	// The total must be counted over the SAME rows the list pages through —
	// a count query left on the old predicate would report 1 under 3 rows.
	suite.Equal(int64(3), total)
}

// Without the opt-in, the endpoint keeps meaning the literal city: the venue
// browse page's city filter must not silently start returning suburbs.
func (suite *VenueServiceIntegrationTestSuite) TestMetroRollup_OffKeepsPrincipalCityOnly() {
	const phoenixCBSA = "38060"
	suite.createMetroVenue("Crescent Ballroom", "Phoenix", "AZ", phoenixCBSA)
	suite.createMetroVenue("Yucca Tap Room", "Tempe", "AZ", phoenixCBSA)

	svc := suite.geoVenueService()
	filters := contracts.VenueListFilters{City: "Phoenix", State: "AZ"}
	resp, total, err := svc.GetVenuesWithShowCounts(filters, 50, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("Crescent Ballroom", resp[0].Name)
}

// A place with no CBSA (every non-US scene, which is most of the globe) falls
// back to the single city rather than widening to something arbitrary — the
// same fallback the scene itself uses. Case-insensitively, which the plain
// `city = ?` filter it replaces was not.
func (suite *VenueServiceIntegrationTestSuite) TestMetroRollup_NonMetroCityFallsBackToCity() {
	suite.createMetroVenue("Bar Le Ritz", "montreal", "QC", "")
	suite.createMetroVenue("Crescent Ballroom", "Phoenix", "AZ", "38060")

	svc := suite.geoVenueService()
	filters := contracts.VenueListFilters{City: "Montreal", State: "QC", MetroRollup: true}
	resp, total, err := svc.GetVenuesWithShowCounts(filters, 50, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("Bar Le Ritz", resp[0].Name)
}

// The rollup INCLUDES the metro, it does not trade the city away for it.
// `venues.metro` is reconciled by a human-run backfill, so a venue sitting in a
// CBSA city with a NULL metro is a real state — and it must keep listing under
// its own city's rail, exactly as it did before this change.
func (suite *VenueServiceIntegrationTestSuite) TestMetroRollup_KeepsNullMetroPrincipalCityVenues() {
	suite.createMetroVenue("Backfill Gap Room", "Phoenix", "AZ", "")
	suite.createMetroVenue("Yucca Tap Room", "Tempe", "AZ", "38060")
	// A NULL-metro venue in a MEMBER city is genuinely unreachable — it is not
	// in the metro by the column, and it is not the principal city. It belongs
	// to its own fallback scene, which is where it lists.
	suite.createMetroVenue("Unbackfilled Tempe Room", "Tempe", "AZ", "")

	svc := suite.geoVenueService()
	filters := contracts.VenueListFilters{City: "Phoenix", State: "AZ", MetroRollup: true}
	resp, total, err := svc.GetVenuesWithShowCounts(filters, 50, 0)

	suite.Require().NoError(err)
	suite.ElementsMatch([]string{"Backfill Gap Room", "Yucca Tap Room"}, venueNames(resp))
	suite.Equal(int64(2), total)
}

// A bare city with no state can't be resolved to a metro without risking the
// wrong namesake (Pasadena CA vs TX), so the rollup declines rather than
// guessing — and the literal-city filter still applies.
func (suite *VenueServiceIntegrationTestSuite) TestMetroRollup_IgnoredWithoutState() {
	suite.createMetroVenue("Crescent Ballroom", "Phoenix", "AZ", "38060")
	suite.createMetroVenue("Yucca Tap Room", "Tempe", "AZ", "38060")

	svc := suite.geoVenueService()
	filters := contracts.VenueListFilters{City: "Phoenix", MetroRollup: true}
	resp, total, err := svc.GetVenuesWithShowCounts(filters, 50, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("Crescent Ballroom", resp[0].Name)
}

// PRIVACY GATE (PSY-1536, locked user decision). Widening the scope must not
// widen what is served: an unverified member-city venue stays out of the list
// entirely, and a verified one whose street geocode doesn't match its current
// address still pins at the city centroid.
func (suite *VenueServiceIntegrationTestSuite) TestMetroRollup_PrivacyGateHoldsForMemberCities() {
	const phoenixCBSA = "38060"

	// An unverified Tempe house venue — in the metro, but never listed.
	unverified := &catalogm.Venue{
		Name:     "Basement Show HQ",
		City:     "Tempe",
		State:    "AZ",
		Verified: false,
		Metro:    stringPtr(phoenixCBSA),
	}
	suite.Require().NoError(suite.db.Create(unverified).Error)

	// A verified Tempe venue carrying street coordinates whose geocoded_address
	// memo does NOT match its current address — the stale case buildVenueResponse
	// refuses to serve.
	lat, lng := 33.4255, -111.9400
	precision := "rooftop"
	stale := &catalogm.Venue{
		Name:             "Yucca Tap Room",
		City:             "Tempe",
		State:            "AZ",
		Address:          stringPtr("29 W Southern Ave"),
		Verified:         true,
		Metro:            stringPtr(phoenixCBSA),
		Latitude:         &lat,
		Longitude:        &lng,
		StreetLatitude:   &lat,
		StreetLongitude:  &lng,
		GeocodePrecision: &precision,
		GeocodedAddress:  stringPtr("a-different-address-key"),
	}
	suite.Require().NoError(suite.db.Create(stale).Error)

	svc := suite.geoVenueService()
	filters := contracts.VenueListFilters{City: "Phoenix", State: "AZ", MetroRollup: true}
	resp, total, err := svc.GetVenuesWithShowCounts(filters, 50, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total, "the unverified member-city venue must not be listed")
	suite.Require().Len(resp, 1)
	suite.Equal("Yucca Tap Room", resp[0].Name)
	suite.Nil(resp[0].StreetLatitude, "stale street geocode must not be served for a metro member")
	suite.Nil(resp[0].StreetLongitude, "stale street geocode must not be served for a metro member")
	suite.Nil(resp[0].GeocodePrecision)
	// The centroid is still served — a metro-member venue still pins, just
	// coarsely, exactly like a principal-city one.
	suite.Require().NotNil(resp[0].Latitude)
	suite.InDelta(lat, *resp[0].Latitude, 0.0001)
}

// The rollup and the scene must not have two definitions of "this metro": the
// metro half of the predicate has to be the scene's own scope, verbatim, or the
// rail and the scene page will eventually answer differently.
func (suite *VenueServiceIntegrationTestSuite) TestMetroRollup_MetroHalfIsTheSceneScope() {
	svc := suite.geoVenueService()
	pred, args, ok := svc.metroRollupPredicate(
		contracts.VenueListFilters{City: "Tempe", State: "AZ", MetroRollup: true},
	)
	suite.Require().True(ok)

	// A member city resolves to the SAME CBSA as its metro's principal city,
	// which is what makes the scene rollup work at all.
	sceneScoped, sceneArgs := metroScopeFor(geo.Default(), "Phoenix", "AZ").venuePredicate("venues")
	suite.Contains(pred, sceneScoped, "the metro half must be the scene's own predicate")
	suite.Require().NotEmpty(args)
	suite.Equal(sceneArgs[0], args[0], "a member city must resolve to its principal's metro")
}
