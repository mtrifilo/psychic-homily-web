package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/contracts"
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
	suite.Equal(1, cities[0].ShowCount)
}

// yearOfShowAtNewYearBoundary is the discriminator the OVER-rejection tests are
// built on, and it exists because the obvious framing does not work.
//
// Asserting "the show is still listed as upcoming" cannot tell an ACCEPTED zone
// from a REJECTED one: the state map and any nearby zone usually agree about
// which side of today an instant falls on, so such a test passes whatever the
// guard decides. It would be a tautology dressed as a canary — and this is the
// canary that matters, because an empty or stale snapshot re-dates every show
// in the catalogue onto its state's zone with no error and no log.
//
// So this pins the resolved zone DIRECTLY, through the venue-local year: it
// seeds one past show 30 minutes after New Year in the venue's STATE-MAP zone,
// which is still 31 December in a zone further west. The two candidate answers
// are then different YEARS, and the year histogram reports which one the guard
// actually used. Being a fixed historical instant, it is also true at every hour
// of the day, which is the property show_venue_local_test.go's fixtures exist to
// preserve.
func (suite *ShowServiceIntegrationTestSuite) yearOfShowAtNewYearBoundary(venueID uint, city, state string) []contracts.VenueShowYearCount {
	suite.T().Helper()
	phoenix, err := time.LoadLocation("America/Phoenix") // what AZ maps to
	suite.Require().NoError(err)

	// 00:30 on 1 Jan 2025 in Phoenix is 21:30 on 31 Dec 2024 in Honolulu.
	at := time.Date(2025, time.January, 1, 0, 30, 0, 0, phoenix)
	user := suite.createTestUser()
	suite.createApprovedShowAt(venueID, user.ID, city, state, at)

	years, err := NewVenueService(suite.db).GetVenueShowYears(venueID, "past")
	suite.Require().NoError(err)
	return years
}

// The canary for the guard OVER-rejecting a zone the catalog does carry. The
// venue is in AZ but keeps its clock in Honolulu; if the membership test
// wrongly rejected Pacific/Honolulu the show would bucket as 2025 (the AZ state
// map) instead of 2024.
func (suite *ShowServiceIntegrationTestSuite) TestVenueLocalYear_KnownVenueZoneStillBeatsTheStateMap() {
	venue := newVenueInZone(suite.T(), suite.db, "Honolulu In Arizona", "AZ", "Pacific/Honolulu", true)

	years := suite.yearOfShowAtNewYearBoundary(venue.ID, "Honolulu", "HI")

	suite.Require().Len(years, 1)
	suite.Equal(2024, years[0].Year,
		"a zone the catalog carries must still win over the state map; 2025 means the guard rejected it")
}

// The mirror, so the pair cannot both pass by accident: a zone the catalog does
// NOT carry must land on the state map, which buckets the same instant as 2025.
func (suite *ShowServiceIntegrationTestSuite) TestVenueLocalYear_UnknownVenueZoneUsesTheStateMap() {
	venue := newVenueInZone(suite.T(), suite.db, "Atlantis In Arizona", "AZ", "Pacific/Honolulu", true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "Pacific/Atlantis")

	years := suite.yearOfShowAtNewYearBoundary(venue.ID, "Phoenix", "AZ")

	suite.Require().Len(years, 1)
	suite.Equal(2025, years[0].Year,
		"an unresolvable zone must fall through to the AZ state map, not raise and not keep 2024")
}

// AT TIME ZONE resolves a name case-insensitively and so does the drift sweep,
// so the read guard has to as well. A stricter guard would send a venue the
// sweep calls healthy to the state map and mis-date its shows with nothing
// logged anywhere — which the year bucket, unlike an upcoming/past assertion,
// can actually detect.
func (suite *ShowServiceIntegrationTestSuite) TestVenueLocalYear_ZoneIsMatchedCaseInsensitively() {
	venue := newVenueInZone(suite.T(), suite.db, "Lowercase Zone", "AZ", "Pacific/Honolulu", true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "pacific/honolulu")

	years := suite.yearOfShowAtNewYearBoundary(venue.ID, "Honolulu", "HI")

	suite.Require().Len(years, 1)
	suite.Equal(2024, years[0].Year,
		"a differently-cased spelling of a real zone must be accepted, not silently downgraded")
}

// A migrated database must already carry a populated snapshot: the read guard
// depends on it, so populating it belongs to the schema rather than to a
// background job an environment might not run. An empty table is the one
// failure mode that produces no error anywhere — it re-dates every show onto
// its state's zone in silence — so it gets its own assertion.
//
// This asserts the INVARIANT, not the mechanism. The suite runs migrations and
// nothing else seeds the table, so a green run here means a migrated database
// is a correct one; it deliberately does not try to prove which statement did
// it, because a sibling test in this file refreshes the table and testify
// gives no ordering guarantee between them.
func (suite *ShowServiceIntegrationTestSuite) TestTimezoneNamesSnapshot_MatchesTheServerCatalog() {
	var snapshot, live int64
	suite.Require().NoError(suite.db.Raw("SELECT count(*) FROM timezone_names_snapshot").Scan(&snapshot).Error)
	suite.Require().NoError(suite.db.Raw("SELECT count(*) FROM pg_timezone_names").Scan(&live).Error)

	suite.Require().Positive(snapshot, "an empty snapshot silently re-dates every show onto the state map")
	suite.Equal(live, snapshot, "the snapshot carries exactly the zones this server resolves")
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

// EVERY OTHER CONSUMER of the venue-local fragments, against the same poisoned
// row. These could be argued redundant now that the guard lives in one shared
// fragment. They are not: each surface reaches that fragment through a
// different join shape — through show_artists, through show_venues, through a
// GROUP BY on the venue-local year — and the guard is a subquery the planner is
// free to place differently in each. A regression that broke only one join
// shape would otherwise ship green.

// The artist show list, reached through show_artists rather than show_venues,
// plus its year facet, which dereferences the zone a third time.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_UnknownVenueZoneStillAnswers() {
	venue := newVenueInZone(suite.T(), suite.db, "Poisoned Artist Room", "AZ", "America/Phoenix", true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "Pacific/Atlantis")
	user := suite.createTestUser()
	artist := suite.createTestArtist("Poisoned Bill")

	at := venueLocalInstant(suite.T(), "America/Phoenix", 0, 23)
	show := newApprovedShowAt(suite.T(), suite.db, venue.ID, user.ID, "Phoenix", "AZ", at)
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO show_artists (show_id, artist_id, position) VALUES (?, ?, 0)", show.ID, artist.ID).Error)

	shows, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC",
		contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 20})
	suite.Require().NoError(err, "an unresolvable venue zone must not break the artist page")
	suite.Require().Equal(int64(1), total)
	suite.Require().Len(shows, 1)

	years, err := suite.artistService.GetArtistShowYears(artist.ID, "upcoming")
	suite.Require().NoError(err, "an unresolvable venue zone must not break the year facet")
	suite.Require().Len(years, 1)
}

// The venue show list: the surface a poisoned row belongs to, and the only one
// that broke before PSY-1678 widened the blast radius to the whole feed.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_UnknownVenueZoneStillAnswers() {
	venue := newVenueInZone(suite.T(), suite.db, "Poisoned Venue Page", "AZ", "America/Phoenix", true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "Pacific/Atlantis")
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), "America/Phoenix", 0, 23)
	newApprovedShowAt(suite.T(), suite.db, venue.ID, user.ID, "Phoenix", "AZ", at)

	shows, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC",
		contracts.VenueShowsQuery{TimeFilter: "upcoming", Limit: 20})
	suite.Require().NoError(err, "an unresolvable venue zone must not break the venue page")
	suite.Require().Equal(int64(1), total)
	suite.Require().Len(shows, 1)
}

// The sitemap's venue-year archive: the most exposed shape in the repo, because
// it dereferences the zone in its SELECT, its GROUP BY and its WHERE. It also
// runs with no request behind it, so a raise here surfaces as a silently broken
// sitemap rather than as a 500 anyone is watching.
func (suite *VenueServiceIntegrationTestSuite) TestSitemapVenueYears_UnknownVenueZoneStillAnswers() {
	venue := newVenueInZone(suite.T(), suite.db, "Poisoned Archive", "AZ", "America/Phoenix", true)
	poisonVenueTimezone(suite.T(), suite.db, venue.ID, "Pacific/Atlantis")
	suite.Require().NoError(suite.db.Table("venues").
		Where("id = ?", venue.ID).Update("slug", "poisoned-archive").Error)
	user := suite.createTestUser()

	phoenix, err := time.LoadLocation("America/Phoenix")
	suite.Require().NoError(err)
	newApprovedShowAt(suite.T(), suite.db, venue.ID, user.ID, "Phoenix", "AZ",
		time.Date(2025, time.January, 1, 0, 30, 0, 0, phoenix))

	entries, err := NewSitemapService(suite.db).Entries(context.Background(), "venue_years")
	suite.Require().NoError(err, "an unresolvable venue zone must not break sitemap generation")
	suite.Require().Len(entries.VenueYears, 1)
	suite.Equal("poisoned-archive/shows/2025", entries.VenueYears[0].Slug,
		"the archive year comes from the AZ state-map fallback, not from the rejected zone")
}
