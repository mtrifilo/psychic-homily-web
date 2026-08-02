package catalog

import (
	"database/sql"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
)

// PSY-1707: whatever lands in venues.timezone must be a name Postgres itself
// recognizes, because readers resolve it with AT TIME ZONE and an unknown name
// raises rather than degrading.
//
// The geocoder is stubbed rather than exercised for real, deliberately. The
// production geocoder currently emits only canonical IANA zones, so a test
// against it would pass whether or not the guard existed — it would pin the
// GeoNames dataset's current contents, not this code's behavior. The point of
// the guard is what happens WHEN that stops being true.

// fixedZoneGeocoder resolves every location to one canned timezone.
type fixedZoneGeocoder struct{ zone string }

func (g fixedZoneGeocoder) Resolve(city, state, country string) (geo.Result, bool) {
	return geo.Result{Latitude: 33.45, Longitude: -112.07, Timezone: g.zone}, true
}
func (g fixedZoneGeocoder) ResolveMetro(city, state, country string) (geo.Metro, bool) {
	return geo.Metro{}, false
}
func (g fixedZoneGeocoder) ResolveUSState(city string) (string, geo.USStateStatus) {
	return "", geo.USStateNotFound
}

func (suite *VenueServiceIntegrationTestSuite) TestApplyGeocoding_HoldsTimezoneInvariant() {
	cases := []struct {
		name string
		zone string
		want *string // nil == expect NULL
	}{
		{name: "canonical zone is kept", zone: "America/Phoenix", want: stringPtr("America/Phoenix")},
		{name: "non-canonical spelling is canonicalized", zone: "america/phoenix", want: stringPtr("America/Phoenix")},
		{name: "surrounding whitespace is trimmed", zone: "  America/Phoenix  ", want: stringPtr("America/Phoenix")},
		{name: "empty zone becomes NULL", zone: "", want: nil},

		// The cases that matter: a zone Postgres cannot resolve must never be
		// persisted, or every venue-local query touching this row raises.
		{name: "junk zone becomes NULL", zone: "Not/AZone", want: nil},
		{name: "Go-only abbreviation becomes NULL", zone: "EST", want: nil},
		{name: "Go-only alias becomes NULL", zone: "Local", want: nil},
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			svc := &VenueService{db: suite.db, geocoder: fixedZoneGeocoder{zone: tc.zone}}
			venue := &catalogm.Venue{Name: "Invariant Room", City: "Phoenix", State: "AZ"}

			svc.applyGeocoding(venue)

			switch {
			case tc.want == nil && venue.Timezone != nil:
				suite.Failf("invariant broken", "expected NULL, got %q", *venue.Timezone)
			case tc.want != nil && venue.Timezone == nil:
				suite.Failf("invariant broken", "expected %q, got NULL", *tc.want)
			case tc.want != nil && venue.Timezone != nil:
				suite.Equal(*tc.want, *venue.Timezone)
			}

			// Whatever survived must actually be usable by a reader. This is the
			// assertion that would have caught the bug the guard prevents.
			if venue.Timezone != nil {
				var ok bool
				suite.Require().NoError(suite.db.Raw(
					"SELECT EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = ?)", *venue.Timezone,
				).Scan(&ok).Error)
				suite.True(ok, "persisted zone %q is not in pg_timezone_names", *venue.Timezone)
			}
		})
	}
}

// The end-to-end shape: a venue created through the service holds the invariant
// in the DATABASE, not just in the struct.
func (suite *VenueServiceIntegrationTestSuite) TestCreateVenue_PersistsOnlyResolvableTimezones() {
	svc := &VenueService{db: suite.db, geocoder: fixedZoneGeocoder{zone: "Not/AZone"}}

	created, err := svc.CreateVenue(&contracts.CreateVenueRequest{
		Name:  "Bad Zone Room",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	suite.Require().NoError(err)

	var stored sql.NullString
	suite.Require().NoError(suite.db.Raw(
		"SELECT timezone FROM venues WHERE id = ?", created.ID).Scan(&stored).Error)
	suite.False(stored.Valid, "an unresolvable geocoder zone must be stored as NULL, never persisted (got %q)", stored.String)
}
