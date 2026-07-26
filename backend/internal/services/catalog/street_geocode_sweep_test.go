package catalog

import (
	"context"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

// =============================================================================
// Scheduled street-geocode sweep (PSY-1544). Runs inside the existing
// VenueServiceIntegrationTestSuite (real DB), driving the service tick
// directly via RunSweepNow — the same code path the RunTickerLoop ticker
// invokes in the server.
// =============================================================================

// sweepWithStub builds the sweep service against the suite DB and a stub
// geocoder, bypassing NewStreetGeocodeSweep's env-derived knobs where a test
// needs a specific limit.
func (suite *VenueServiceIntegrationTestSuite) sweepWithStub(stub *stubAddressGeocoder, limit int) *StreetGeocodeSweep {
	s := NewStreetGeocodeSweep(suite.db, stub)
	if limit > 0 {
		s.limit = limit
	}
	return s
}

// TestSweep_PopulatesNullStreetPins is the PSY-1544 acceptance path: a venue
// created WITHOUT inline geocoding (FindOrCreateVenue via show submission,
// data-sync import — modeled by a bare row with NULL street columns) gets
// street coordinates from a single scheduled tick, no manual intervention.
func (suite *VenueServiceIntegrationTestSuite) TestSweep_PopulatesNullStreetPins() {
	seeded := suite.seedVenueRow("Sweep Show Submission", "130 N Central Ave")
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)

	suite.sweepWithStub(stub, 0).RunSweepNow(context.Background())

	v := suite.loadVenue(seeded.ID)
	suite.Require().NotNil(v.StreetLatitude, "a scheduled tick must populate NULL street pins")
	suite.InDelta(33.448227, *v.StreetLatitude, 1e-6)
	suite.Require().NotNil(v.StreetLongitude)
	suite.InDelta(-112.073069, *v.StreetLongitude, 1e-6)
	suite.Require().NotNil(v.GeocodePrecision)
	suite.Equal(geo.PrecisionRooftop, *v.GeocodePrecision)
	suite.Require().NotNil(v.GeocodedAddress)
	suite.Equal("130 N Central Ave, Phoenix, AZ", *v.GeocodedAddress)
}

// TestSweep_ReResolvesAfterAddressEdit covers the contribution-edit leak: an
// approved address change clears the street fields without re-resolving
// (street pin silently downgraded to city centroid); the next tick must
// re-geocode the new address.
func (suite *VenueServiceIntegrationTestSuite) TestSweep_ReResolvesAfterAddressEdit() {
	seeded := suite.seedVenueRow("Sweep Address Edit", "130 N Central Ave")
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)
	sweep := suite.sweepWithStub(stub, 0)

	sweep.RunSweepNow(context.Background())
	suite.Equal(1, stub.calls)

	// Approved contribution edit: address changes, street fields cleared,
	// nothing re-resolved (ApprovePendingEdit's exact write shape).
	newAddr := "308 N 2nd Ave"
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", seeded.ID).Updates(map[string]interface{}{
		"address":           newAddr,
		"street_latitude":   (*float64)(nil),
		"street_longitude":  (*float64)(nil),
		"geocode_precision": (*string)(nil),
		"geocoded_address":  (*string)(nil),
	}).Error)

	stub.result = geo.AddressResult{Latitude: 33.451020, Longitude: -112.077228, Precision: geo.PrecisionInterpolated}
	sweep.RunSweepNow(context.Background())
	suite.Equal(2, stub.calls, "the tick after an address edit must re-resolve")

	v := suite.loadVenue(seeded.ID)
	suite.Require().NotNil(v.StreetLatitude)
	suite.InDelta(33.451020, *v.StreetLatitude, 1e-6)
	suite.Require().NotNil(v.GeocodedAddress)
	suite.Equal("308 N 2nd Ave, Phoenix, AZ", *v.GeocodedAddress)
}

// TestSweep_SecondTickIsFree asserts the cheap steady state the daily
// schedule depends on: once reconciled, a tick makes zero network calls.
func (suite *VenueServiceIntegrationTestSuite) TestSweep_SecondTickIsFree() {
	suite.seedVenueRow("Sweep Idempotent", "130 N Central Ave")
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)
	sweep := suite.sweepWithStub(stub, 0)

	sweep.RunSweepNow(context.Background())
	sweep.RunSweepNow(context.Background())

	suite.Equal(1, stub.calls, "a tick over reconciled data must not hit the network")
}

// TestSweep_LimitBoundsNetworkBudget asserts the per-tick limit caps lookups,
// leaving the remainder for the next tick.
func (suite *VenueServiceIntegrationTestSuite) TestSweep_LimitBoundsNetworkBudget() {
	suite.seedVenueRow("Sweep Limit A", "100 First St")
	suite.seedVenueRow("Sweep Limit B", "200 Second St")
	suite.seedVenueRow("Sweep Limit C", "300 Third St")

	stub := hitStub(33.0, -112.0, geo.PrecisionRooftop)
	sweep := suite.sweepWithStub(stub, 2)

	sweep.RunSweepNow(context.Background())
	suite.Equal(2, stub.calls, "a tick must stop at the configured limit")

	sweep.RunSweepNow(context.Background())
	suite.Equal(3, stub.calls, "the next tick picks up where the limit stopped")
}

// TestSweep_CanceledContextAbortsCycle asserts the shutdown property: a
// canceled ctx (main.go cancels before Stop) aborts the cycle between venues
// instead of grinding through the remaining lookups.
func (suite *VenueServiceIntegrationTestSuite) TestSweep_CanceledContextAbortsCycle() {
	seeded := suite.seedVenueRow("Sweep Canceled", "130 N Central Ave")
	stub := hitStub(33.0, -112.0, geo.PrecisionRooftop)
	sweep := suite.sweepWithStub(stub, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sweep.RunSweepNow(ctx)

	suite.Equal(0, stub.calls, "a canceled ctx must abort before any lookup")
	v := suite.loadVenue(seeded.ID)
	suite.Nil(v.StreetLatitude, "an aborted cycle must not have written")
}
