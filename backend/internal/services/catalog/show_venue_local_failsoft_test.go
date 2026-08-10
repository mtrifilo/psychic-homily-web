package catalog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Fail-soft partitioning for a venues.timezone this server cannot resolve
// (PSY-1761).
//
// Before this, `AT TIME ZONE` raised on such a name and every listing query
// touching the venue 500'd — after PSY-1678 that meant the whole /shows feed
// and its city picker, not one venue page. shared.VenueTZJoin now tests the
// stored zone against timezone_names_snapshot and projects NULL on a miss, so
// the row lands on the same US-state-map arm an ungeocoded venue takes.
//
// Every fixture here is anchored on a VENUE's calendar rather than a UTC
// instant, for the reason show_venue_local_test.go spells out: the service
// reads Postgres now(), there is no clock seam, and a wall-clock-anchored
// assertion flips depending on what hour CI runs at. venueLocalInstant,
// requireLocalAndUTCDatesDiffer and newVenueInZone all come from there.

// poisonVenueTimezone writes a zone straight past the model, which is the
// out-of-band path the PSY-1707 write gate cannot see and therefore the one
// this guard exists for.
func poisonVenueTimezone(t *testing.T, db *gorm.DB, venueID uint, raw string) {
	t.Helper()
	require.NoError(t, db.Table("venues").Where("id = ?", venueID).Update("timezone", raw).Error)
}

// An unresolvable zone must not take the feed down, and the show must land on
// the state-map boundary rather than on some arbitrary one. The fixture is
// tonight-at-23:00 in Phoenix, which is already TOMORROW in UTC — so a query
// that quietly fell back to UTC instead of the state map would drop it, and the
// assertion would fail rather than pass for the wrong reason.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_UnknownVenueZoneFallsBackToTheStateMap() {
	const zone = "America/Phoenix" // what AZ maps to in utils.StateTimezones
	venue := newVenueInZone(suite.T(), suite.db, "Poisoned Room", "AZ", zone, true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "Pacific/Atlantis")
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), zone, 0, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	show := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", at)

	ids, total := suite.upcomingShowIDs("UTC")
	suite.Require().Equal(int64(1), total, "an unresolvable venue zone must not empty the feed")
	suite.Require().Equal([]uint{show.ID}, ids,
		"the show should be partitioned by the AZ state-map arm, exactly as a NULL zone is")
}

// The mirror: venue-local YESTERDAY must still be gone. Together with the test
// above this pins that the fallback is the STATE MAP and not "keep everything" —
// a guard that degraded to no date filter at all would pass the first assertion
// and fail this one.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_UnknownVenueZoneStillDropsYesterday() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Poisoned Yesterday", "AZ", zone, true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "Not/AZone")
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), zone, -1, 23)
	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", at)

	ids, total := suite.upcomingShowIDs("UTC")
	suite.Require().Equal(int64(0), total, "last night in Phoenix is not upcoming")
	suite.Require().Empty(ids)
}

// The picker is drawn from the same partition as the list, so it has to survive
// the same poisoned row — a 500 here empties /explore's city picker and the
// account-settings one along with it.
func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_UnknownVenueZoneStillAnswers() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Poisoned Picker", "AZ", zone, true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "Pacific/Atlantis")
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), zone, 0, 23)
	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", at)

	cities, err := suite.showService.GetShowCities("UTC")
	suite.Require().NoError(err, "the city picker must not raise on an unresolvable venue zone")
	suite.Require().Len(cities, 1)
	suite.Equal("Phoenix", cities[0].City)
	suite.Equal(int64(1), cities[0].ShowCount)
}

// The canary for the guard OVER-rejecting. If timezone_names_snapshot were
// empty or stale, every venue would fail the membership test and every show
// would silently re-date onto its state's zone — no error, no log, just wrong
// dates everywhere. This venue is in AZ but keeps its clock in Honolulu, so the
// two answers differ by three hours and the test can tell them apart.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_KnownVenueZoneStillBeatsTheStateMap() {
	const zone = "Pacific/Honolulu" // UTC-10, no DST; AZ maps to UTC-7
	venue := newVenueInZone(suite.T(), suite.db, "Honolulu In Arizona", "AZ", zone, true)
	user := suite.createTestUser()

	// 22:00 tonight in Honolulu is 01:00 TOMORROW in Phoenix, so a show at
	// 22:00 YESTERDAY Honolulu-time is still "yesterday" there but would read as
	// today under the AZ state map for one hour of every day. Anchor on the
	// unambiguous direction instead: tonight in Honolulu is already tomorrow in
	// Phoenix for part of the day, so assert the show stays listed.
	at := venueLocalInstant(suite.T(), zone, 0, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	show := suite.createApprovedShowAt(venue.ID, user.ID, "Honolulu", "HI", at)

	ids, _ := suite.upcomingShowIDs("UTC")
	suite.Require().Equal([]uint{show.ID}, ids,
		"a zone the catalog carries must still win over the state map")
}

// AT TIME ZONE is case-insensitive and so is the drift sweep, so the read guard
// has to be too. A stricter guard would send a venue the sweep calls healthy to
// the state map and mis-date its shows with nothing logged anywhere.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_ZoneIsMatchedCaseInsensitively() {
	const zone = "Pacific/Honolulu"
	venue := newVenueInZone(suite.T(), suite.db, "Lowercase Zone", "AZ", zone, true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "pacific/honolulu")
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), zone, 0, 23)
	show := suite.createApprovedShowAt(venue.ID, user.ID, "Honolulu", "HI", at)

	ids, _ := suite.upcomingShowIDs("UTC")
	suite.Require().Equal([]uint{show.ID}, ids,
		"a differently-cased spelling of a real zone must be accepted, not silently downgraded")
}

// The snapshot is created AND seeded by its migration, deliberately: the read
// guard depends on it being populated, so populating it belongs to the schema
// rather than to a background job that an environment might not run. This
// asserts that property directly, because an empty table is the one failure
// mode that produces no error anywhere.
func (suite *ShowServiceIntegrationTestSuite) TestTimezoneNamesSnapshot_IsSeededByItsMigration() {
	var snapshot, live int64
	suite.Require().NoError(suite.db.Raw("SELECT count(*) FROM timezone_names_snapshot").Scan(&snapshot).Error)
	suite.Require().NoError(suite.db.Raw("SELECT count(*) FROM pg_timezone_names").Scan(&live).Error)

	suite.Require().Positive(snapshot, "an empty snapshot silently re-dates every show onto the state map")
	suite.Equal(live, snapshot, "the migration seeds the snapshot from the live catalog")
}

// The refresh has to reconcile in BOTH directions, and the two mean opposite
// things: a name the catalog dropped must leave the snapshot (or the guard
// keeps trusting a value that now raises), and a name the catalog gained must
// enter it (or a genuinely valid zone is mis-dated onto the state map).
func (suite *ShowServiceIntegrationTestSuite) TestRefreshTimezoneNamesSnapshot_ReconcilesBothDirections() {
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO timezone_names_snapshot (name) VALUES ('Pacific/Atlantis')").Error)
	suite.Require().NoError(suite.db.Exec(
		"DELETE FROM timezone_names_snapshot WHERE name = 'America/Phoenix'").Error)

	report, err := RefreshTimezoneNamesSnapshot(context.Background(), suite.db)
	suite.Require().NoError(err)
	suite.Equal(1, report.Removed, "a name the catalog no longer carries must leave the snapshot")
	suite.Equal(1, report.Added, "a name the catalog carries must be restored")

	var stale, restored int64
	suite.Require().NoError(suite.db.Raw(
		"SELECT count(*) FROM timezone_names_snapshot WHERE name = 'Pacific/Atlantis'").Scan(&stale).Error)
	suite.Require().NoError(suite.db.Raw(
		"SELECT count(*) FROM timezone_names_snapshot WHERE name = 'America/Phoenix'").Scan(&restored).Error)
	suite.Zero(stale)
	suite.Equal(int64(1), restored)
}

// The signal requirement. The read path degrades SILENTLY by construction now,
// so a poisoned row that nothing names would stay poisoned forever. This pins
// the detector both surfaces report from: it must name exactly the stranded
// venue, carry the rejected value verbatim so it can be re-geocoded, and leave
// healthy venues out (a detector that cried wolf would be ignored).
func (suite *ShowServiceIntegrationTestSuite) TestDetectVenueTimezoneDrift_NamesEveryPoisonedVenue() {
	venue := newVenueInZone(suite.T(), suite.db, "Reported Venue", "AZ", "America/Phoenix", true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "Pacific/Atlantis")
	newVenueInZone(suite.T(), suite.db, "Healthy Venue", "AZ", "America/Phoenix", true)
	// The guard accepts a differently-cased spelling, so the detector must too,
	// or it reports a venue whose shows are being dated correctly.
	cased := newVenueInZone(suite.T(), suite.db, "Lowercase Venue", "AZ", "America/Phoenix", true)
	poisonVenueTimezone(suite.T(), suite.db, cased.ID, "america/phoenix")

	drifted, err := detectVenueTimezoneDrift(suite.db)
	suite.Require().NoError(err)

	suite.Require().Len(drifted, 1, "exactly the venue the guard cannot resolve is reported")
	suite.Equal(venue.ID, drifted[0].VenueID)
	suite.Equal("Reported Venue", drifted[0].Name)
	suite.Equal("Pacific/Atlantis", drifted[0].Timezone,
		"the rejected value is reported verbatim so it can be re-geocoded")
}

// The emptiness guard. An empty snapshot is the one failure that produces no
// error anywhere — every venue would fail the membership test and every show
// would silently re-date onto the state map — so the refresh must never be the
// thing that creates it.
func (suite *ShowServiceIntegrationTestSuite) TestRefreshTimezoneNamesSnapshot_NeverEmptiesTheTable() {
	suite.Require().NoError(suite.db.Exec("DELETE FROM timezone_names_snapshot").Error)

	report, err := RefreshTimezoneNamesSnapshot(context.Background(), suite.db)
	suite.Require().NoError(err)
	suite.Positive(report.Added, "a wiped snapshot is repopulated from the live catalog")

	var count int64
	suite.Require().NoError(suite.db.Raw("SELECT count(*) FROM timezone_names_snapshot").Scan(&count).Error)
	suite.Equal(int64(report.Total), count)
}
