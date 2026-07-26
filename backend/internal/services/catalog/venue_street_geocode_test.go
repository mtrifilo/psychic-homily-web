package catalog

import (
	"context"
	"errors"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
)

// =============================================================================
// Street-level geocoding (PSY-1536) — write paths, privacy gate, backfill.
// Runs inside the existing VenueServiceIntegrationTestSuite (real DB).
// =============================================================================

// stubAddressGeocoder is an in-memory geo.AddressGeocoder for tests.
type stubAddressGeocoder struct {
	result    geo.AddressResult
	ok        bool
	err       error
	calls     int
	lastQuery geo.AddressQuery
}

func (s *stubAddressGeocoder) GeocodeAddress(_ context.Context, q geo.AddressQuery) (geo.AddressResult, bool, error) {
	s.calls++
	s.lastQuery = q
	return s.result, s.ok, s.err
}

func hitStub(lat, lng float64, precision string) *stubAddressGeocoder {
	return &stubAddressGeocoder{result: geo.AddressResult{Latitude: lat, Longitude: lng, Precision: precision}, ok: true}
}

// streetService returns a VenueService wired to the given stub, sharing the
// suite's DB.
func (suite *VenueServiceIntegrationTestSuite) streetService(stub *stubAddressGeocoder) *VenueService {
	return &VenueService{db: suite.db, addressGeocoder: stub}
}

func (suite *VenueServiceIntegrationTestSuite) loadVenue(id uint) *catalogm.Venue {
	var v catalogm.Venue
	suite.Require().NoError(suite.db.First(&v, id).Error)
	return &v
}

// --- Create path ---

func (suite *VenueServiceIntegrationTestSuite) TestCreateVenue_StreetGeocoded() {
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)
	svc := suite.streetService(stub)

	resp, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name:    "Street Geocoded",
		City:    "Phoenix",
		State:   "AZ",
		Address: stringPtr("130 N Central Ave"),
		Zipcode: stringPtr("85004"),
	}, true) // admin = verified

	suite.Require().NoError(err)
	suite.Equal(1, stub.calls)
	suite.Equal("130 N Central Ave", stub.lastQuery.Street)
	suite.Equal("Phoenix", stub.lastQuery.City)

	v := suite.loadVenue(resp.ID)
	suite.Require().NotNil(v.StreetLatitude)
	suite.InDelta(33.448227, *v.StreetLatitude, 1e-6)
	suite.Require().NotNil(v.StreetLongitude)
	suite.InDelta(-112.073069, *v.StreetLongitude, 1e-6)
	suite.Require().NotNil(v.GeocodePrecision)
	suite.Equal(geo.PrecisionRooftop, *v.GeocodePrecision)
	suite.Require().NotNil(v.GeocodedAddress)
	suite.Equal("130 N Central Ave, Phoenix, AZ, 85004", *v.GeocodedAddress)

	// Verified venue with a fresh geocode → exposed in the response.
	suite.Require().NotNil(resp.StreetLatitude)
	suite.Require().NotNil(resp.StreetLongitude)
	suite.Require().NotNil(resp.GeocodePrecision)
}

func (suite *VenueServiceIntegrationTestSuite) TestCreateVenue_GeocodeFailureNeverBlocksWrite() {
	stub := &stubAddressGeocoder{err: errors.New("nominatim: status 503")}
	svc := suite.streetService(stub)

	resp, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name:    "Geocode Down",
		City:    "Phoenix",
		State:   "AZ",
		Address: stringPtr("130 N Central Ave"),
	}, true)

	suite.Require().NoError(err, "geocoding failure must never fail the venue write")
	v := suite.loadVenue(resp.ID)
	suite.Nil(v.StreetLatitude)
	suite.Nil(v.StreetLongitude)
	suite.Nil(v.GeocodePrecision)
	suite.Nil(v.GeocodedAddress)
}

func (suite *VenueServiceIntegrationTestSuite) TestCreateVenue_NoAddressNoGeocodeCall() {
	stub := hitStub(1, 2, geo.PrecisionRooftop)
	svc := suite.streetService(stub)

	_, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name: "No Address", City: "Phoenix", State: "AZ",
	}, true)

	suite.Require().NoError(err)
	suite.Equal(0, stub.calls, "no street address — no network call")
}

// --- Update path ---

func (suite *VenueServiceIntegrationTestSuite) TestUpdateVenue_AddressChangeRegeocodes() {
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)
	svc := suite.streetService(stub)
	created, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name: "Moving Venue", City: "Phoenix", State: "AZ", Address: stringPtr("130 N Central Ave"),
	}, true)
	suite.Require().NoError(err)
	suite.Equal(1, stub.calls)

	stub.result = geo.AddressResult{Latitude: 33.451020, Longitude: -112.077228, Precision: geo.PrecisionInterpolated}
	_, err = svc.UpdateVenue(created.ID, &contracts.UpdateVenueRequest{Address: stringPtr("308 N 2nd Ave")})
	suite.Require().NoError(err)
	suite.Equal(2, stub.calls)

	v := suite.loadVenue(created.ID)
	suite.Require().NotNil(v.StreetLatitude)
	suite.InDelta(33.451020, *v.StreetLatitude, 1e-6)
	suite.Require().NotNil(v.GeocodedAddress)
	suite.Equal("308 N 2nd Ave, Phoenix, AZ", *v.GeocodedAddress)
	suite.Require().NotNil(v.GeocodePrecision)
	suite.Equal(geo.PrecisionInterpolated, *v.GeocodePrecision)
}

func (suite *VenueServiceIntegrationTestSuite) TestUpdateVenue_NonAddressFieldSkipsGeocode() {
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)
	svc := suite.streetService(stub)
	created, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name: "Renamed Venue", City: "Phoenix", State: "AZ", Address: stringPtr("130 N Central Ave"),
	}, true)
	suite.Require().NoError(err)

	_, err = svc.UpdateVenue(created.ID, &contracts.UpdateVenueRequest{Name: stringPtr("Renamed Venue 2")})
	suite.Require().NoError(err)
	suite.Equal(1, stub.calls, "a non-address update must not re-geocode")

	v := suite.loadVenue(created.ID)
	suite.NotNil(v.StreetLatitude, "existing geocode must survive a non-address update")
}

func (suite *VenueServiceIntegrationTestSuite) TestUpdateVenue_SameAddressShortCircuits() {
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)
	svc := suite.streetService(stub)
	created, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name: "Same Address", City: "Phoenix", State: "AZ", Address: stringPtr("130 N Central Ave"),
	}, true)
	suite.Require().NoError(err)

	_, err = svc.UpdateVenue(created.ID, &contracts.UpdateVenueRequest{Address: stringPtr("130 N Central Ave")})
	suite.Require().NoError(err)
	suite.Equal(1, stub.calls, "an unchanged address key must not make a network call")

	v := suite.loadVenue(created.ID)
	suite.NotNil(v.StreetLatitude)
	suite.NotNil(v.GeocodedAddress)
}

func (suite *VenueServiceIntegrationTestSuite) TestUpdateVenue_GeocodeFailureWritesNullsNotStaleCoords() {
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)
	svc := suite.streetService(stub)
	created, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name: "Fail On Move", City: "Phoenix", State: "AZ", Address: stringPtr("130 N Central Ave"),
	}, true)
	suite.Require().NoError(err)

	stub.ok = false
	stub.err = errors.New("nominatim: status 503")
	_, err = svc.UpdateVenue(created.ID, &contracts.UpdateVenueRequest{Address: stringPtr("308 N 2nd Ave")})
	suite.Require().NoError(err, "geocoding failure must never fail the venue update")

	v := suite.loadVenue(created.ID)
	suite.Nil(v.StreetLatitude, "old address's coords must not survive an address change")
	suite.Nil(v.StreetLongitude)
	suite.Nil(v.GeocodePrecision)
	suite.Nil(v.GeocodedAddress)
}

// --- Privacy gate + freshness gate (buildVenueResponse) ---

func (suite *VenueServiceIntegrationTestSuite) TestBuildVenueResponse_UnverifiedHidesStreetCoords() {
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)
	svc := suite.streetService(stub)

	resp, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name:    "House Show Spot",
		City:    "Phoenix",
		State:   "AZ",
		Address: stringPtr("1234 Secret House St"),
	}, false) // non-admin = unverified

	suite.Require().NoError(err)
	// Stored (ready for when the venue is verified)…
	v := suite.loadVenue(resp.ID)
	suite.NotNil(v.StreetLatitude)
	// …but NEVER exposed while unverified — mirrors the address/zipcode
	// redaction protecting DIY/house venues.
	suite.Nil(resp.StreetLatitude, "street latitude must be hidden for unverified venues")
	suite.Nil(resp.StreetLongitude, "street longitude must be hidden for unverified venues")
	suite.Nil(resp.GeocodePrecision, "geocode precision must be hidden for unverified venues")

	// And it stays hidden on read paths too.
	got, err := svc.GetVenue(resp.ID)
	suite.Require().NoError(err)
	suite.Nil(got.StreetLatitude)
	suite.Nil(got.StreetLongitude)
	suite.Nil(got.GeocodePrecision)
}

func (suite *VenueServiceIntegrationTestSuite) TestBuildVenueResponse_StaleGeocodeHidden() {
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)
	svc := suite.streetService(stub)
	created, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name: "Stale Venue", City: "Phoenix", State: "AZ", Address: stringPtr("130 N Central Ave"),
	}, true)
	suite.Require().NoError(err)
	suite.NotNil(created.StreetLatitude)

	// Simulate a writer that changes the address WITHOUT re-geocoding (the
	// class of bug the freshness gate exists for).
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).
		Where("id = ?", created.ID).
		Update("address", "999 Somewhere Else Rd").Error)

	got, err := svc.GetVenue(created.ID)
	suite.Require().NoError(err)
	suite.Nil(got.StreetLatitude, "stale street coords (address changed since geocode) must not be served")
	suite.Nil(got.StreetLongitude)
	suite.Nil(got.GeocodePrecision)
}

// --- Backfill reconciler ---

func (suite *VenueServiceIntegrationTestSuite) seedVenueRow(name, address string) *catalogm.Venue {
	v := &catalogm.Venue{
		Name:  name,
		City:  "Phoenix",
		State: "AZ",
	}
	if address != "" {
		v.Address = &address
	}
	suite.Require().NoError(suite.db.Create(v).Error)
	return v
}

func (suite *VenueServiceIntegrationTestSuite) TestBackfill_DryRunGeocodesButWritesNothing() {
	seeded := suite.seedVenueRow("Backfill Dry", "130 N Central Ave")
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)

	report, err := BackfillVenueStreetGeocodes(suite.db, stub, StreetGeocodeOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal(1, report.Scanned)
	suite.Equal(1, report.Set)
	suite.Equal(1, stub.calls, "dry-run performs the real lookup so printed results are real")
	suite.Equal(1, report.PrecisionCounts[geo.PrecisionRooftop])
	suite.Require().Len(report.Changes, 1)
	suite.Equal(StreetGeocodeSet, report.Changes[0].Action)
	suite.Equal("130 N Central Ave, Phoenix, AZ", report.Changes[0].Key)

	v := suite.loadVenue(seeded.ID)
	suite.Nil(v.StreetLatitude, "dry-run must not write")
	suite.Nil(v.GeocodedAddress)
}

func (suite *VenueServiceIntegrationTestSuite) TestBackfill_ConfirmWritesAndSecondRunSkips() {
	seeded := suite.seedVenueRow("Backfill Live", "130 N Central Ave")
	stub := hitStub(33.448227, -112.073069, geo.PrecisionRooftop)

	report, err := BackfillVenueStreetGeocodes(suite.db, stub, StreetGeocodeOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.Set)

	v := suite.loadVenue(seeded.ID)
	suite.Require().NotNil(v.StreetLatitude)
	suite.InDelta(33.448227, *v.StreetLatitude, 1e-6)
	suite.Require().NotNil(v.GeocodedAddress)
	suite.Equal("130 N Central Ave, Phoenix, AZ", *v.GeocodedAddress)

	// Idempotent: a clean second run makes no network calls and no changes.
	report2, err := BackfillVenueStreetGeocodes(suite.db, stub, StreetGeocodeOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report2.Unchanged)
	suite.Equal(0, report2.Set)
	suite.Equal(1, stub.calls, "second run over unchanged data must not hit the network")
}

func (suite *VenueServiceIntegrationTestSuite) TestBackfill_MissClearsStaleGeocode() {
	seeded := suite.seedVenueRow("Backfill Stale Miss", "999 Unresolvable Ln")
	oldKey := "130 N Central Ave, Phoenix, AZ"
	lat, lng, prec := 33.0, -112.0, geo.PrecisionRooftop
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", seeded.ID).Updates(map[string]interface{}{
		"street_latitude": lat, "street_longitude": lng,
		"geocode_precision": prec, "geocoded_address": oldKey,
	}).Error)

	stub := &stubAddressGeocoder{ok: false}
	report, err := BackfillVenueStreetGeocodes(suite.db, stub, StreetGeocodeOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.Missed)

	v := suite.loadVenue(seeded.ID)
	suite.Nil(v.StreetLatitude, "a stored geocode for a different address must not survive a miss")
	suite.Nil(v.GeocodedAddress)
}

func (suite *VenueServiceIntegrationTestSuite) TestBackfill_AddressRemovedCleared() {
	seeded := suite.seedVenueRow("Backfill Cleared", "")
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", seeded.ID).Updates(map[string]interface{}{
		"street_latitude": 33.0, "street_longitude": -112.0,
		"geocode_precision": geo.PrecisionRooftop, "geocoded_address": "130 N Central Ave, Phoenix, AZ",
	}).Error)

	stub := hitStub(1, 2, geo.PrecisionRooftop)
	report, err := BackfillVenueStreetGeocodes(suite.db, stub, StreetGeocodeOptions{})
	suite.Require().NoError(err)
	suite.Equal(1, report.Cleared)
	suite.Equal(0, stub.calls, "an address-less venue must not hit the network")

	v := suite.loadVenue(seeded.ID)
	suite.Nil(v.StreetLatitude)
	suite.Nil(v.GeocodedAddress)
}

func (suite *VenueServiceIntegrationTestSuite) TestBackfill_LimitCapsNetworkCalls() {
	suite.seedVenueRow("Limit A", "100 First St")
	suite.seedVenueRow("Limit B", "200 Second St")
	suite.seedVenueRow("Limit C", "300 Third St")

	stub := hitStub(33.0, -112.0, geo.PrecisionRooftop)
	report, err := BackfillVenueStreetGeocodes(suite.db, stub, StreetGeocodeOptions{Limit: 2})
	suite.Require().NoError(err)
	suite.Equal(2, stub.calls)
	suite.Equal(2, report.Set)
	suite.True(report.LimitHit)
}

func (suite *VenueServiceIntegrationTestSuite) TestBackfill_ErrorReportedRowLeftAsIs() {
	seeded := suite.seedVenueRow("Backfill Error", "130 N Central Ave")
	stub := &stubAddressGeocoder{err: errors.New("nominatim: status 503")}

	report, err := BackfillVenueStreetGeocodes(suite.db, stub, StreetGeocodeOptions{})
	suite.Require().NoError(err, "per-row errors must not abort the run")
	suite.Require().Len(report.Errors, 1)
	suite.Require().Len(report.Changes, 1)
	suite.Equal(StreetGeocodeError, report.Changes[0].Action)

	v := suite.loadVenue(seeded.ID)
	suite.Nil(v.StreetLatitude)
}
