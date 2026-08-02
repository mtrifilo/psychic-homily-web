package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// The sweep is layer two of the guarantee that lets the show-list partition read
// venues.timezone without validating it per row. Layer one (the PSY-1707 write
// gate) cannot cover tzdata drift, because what pg_timezone_names contains is a
// property of the server's tzdata packaging and changes under image bumps.

func (suite *VenueServiceIntegrationTestSuite) seedVenueWithRawTimezone(name, state, raw string) *catalogm.Venue {
	venue := &catalogm.Venue{Name: name, City: "Testville", State: state}
	suite.Require().NoError(suite.db.Create(venue).Error)
	// Written past the model so the value bypasses the write gate — which is
	// precisely the situation the sweep exists for.
	suite.Require().NoError(suite.db.Table("venues").
		Where("id = ?", venue.ID).Update("timezone", raw).Error)
	return venue
}

func (suite *VenueServiceIntegrationTestSuite) storedTimezone(venueID uint) sql.NullString {
	var tz sql.NullString
	suite.Require().NoError(suite.db.Raw("SELECT timezone FROM venues WHERE id = ?", venueID).Scan(&tz).Error)
	return tz
}

func (suite *VenueServiceIntegrationTestSuite) TestSweepVenueTimezones_ClearsOnlyWhatDrifted() {
	good := suite.seedVenueWithRawTimezone("Sweep Good", "AZ", "America/Phoenix")
	drifted := suite.seedVenueWithRawTimezone("Sweep Drifted", "AZ", "Pacific/Atlantis")
	blank := suite.seedVenueWithRawTimezone("Sweep Blank", "AZ", "   ")
	// A NULL venue is already in the fallback state and must not be counted as
	// drift — otherwise every cycle reports work it did not do.
	untouched := &catalogm.Venue{Name: "Sweep Null", City: "Testville", State: "AZ"}
	suite.Require().NoError(suite.db.Create(untouched).Error)

	report, err := SweepVenueTimezones(context.Background(), suite.db)
	suite.Require().NoError(err)

	suite.Equal(3, report.Scanned, "scanned counts venues with a non-NULL zone")
	suite.Equal(2, report.Cleared)
	suite.Require().Len(report.Drifted, 2)

	suite.True(suite.storedTimezone(good.ID).Valid)
	suite.Equal("America/Phoenix", suite.storedTimezone(good.ID).String)
	suite.False(suite.storedTimezone(drifted.ID).Valid, "an unknown zone must be cleared")
	suite.False(suite.storedTimezone(blank.ID).Valid, "a blank zone raises in AT TIME ZONE just like an unknown one")
	suite.False(suite.storedTimezone(untouched.ID).Valid)

	// The report has to name the casualties, not just count them: this is an
	// operator's only signal that a venue silently lost its zone.
	byName := map[string]string{}
	for _, d := range report.Drifted {
		byName[d.Name] = d.Timezone
	}
	suite.Equal("Pacific/Atlantis", byName["Sweep Drifted"])
	suite.Contains(byName, "Sweep Blank")
}

// Converged steady state: a second cycle finds nothing and writes nothing. A
// sweep that keeps "fixing" the same rows would mean the predicate and the
// update disagree.
func (suite *VenueServiceIntegrationTestSuite) TestSweepVenueTimezones_IsIdempotent() {
	suite.seedVenueWithRawTimezone("Idem Drift", "AZ", "Pacific/Atlantis")
	suite.seedVenueWithRawTimezone("Idem Good", "AZ", "America/Phoenix")

	first, err := SweepVenueTimezones(context.Background(), suite.db)
	suite.Require().NoError(err)
	suite.Equal(1, first.Cleared)

	second, err := SweepVenueTimezones(context.Background(), suite.db)
	suite.Require().NoError(err)
	suite.Equal(0, second.Cleared, "a converged catalog must be a no-op")
	suite.Empty(second.Drifted)
}

// Case and whitespace variants are NOT drift: the write gate canonicalizes, but
// a row restored from an older dump can carry them, and clearing a resolvable
// zone would be the sweep destroying good data.
func (suite *VenueServiceIntegrationTestSuite) TestSweepVenueTimezones_KeepsResolvableVariants() {
	for i, raw := range []string{"america/phoenix", "  America/Phoenix  ", "\tAmerica/Phoenix\n"} {
		v := suite.seedVenueWithRawTimezone(fmt.Sprintf("Variant %d", i), "AZ", raw)
		report, err := SweepVenueTimezones(context.Background(), suite.db)
		suite.Require().NoError(err)
		suite.Equal(0, report.Cleared, "raw %q resolves and must be kept", raw)
		suite.True(suite.storedTimezone(v.ID).Valid, "raw %q was cleared", raw)
	}
}

// The sweep must survive a catalog with nothing to do, and on an empty table.
func (suite *VenueServiceIntegrationTestSuite) TestSweepVenueTimezones_EmptyCatalog() {
	report, err := SweepVenueTimezones(context.Background(), suite.db)
	suite.Require().NoError(err)
	suite.Equal(0, report.Scanned)
	suite.Equal(0, report.Cleared)
	suite.Empty(report.Drifted)
}

func TestSweepVenueTimezones_NilDBIsAnError(t *testing.T) {
	if _, err := SweepVenueTimezones(context.Background(), nil); err == nil {
		t.Fatal("a nil database must be an error, not a silent no-op that looks like a clean sweep")
	}
}
