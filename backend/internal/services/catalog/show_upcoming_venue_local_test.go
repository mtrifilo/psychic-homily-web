package catalog

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Venue-local partitioning for the MAIN /shows feed: GetUpcomingShows and the
// GetShowCities picker counts (PSY-1678).
//
// The shared fixtures live in the sibling show_venue_local_test.go, which also
// carries the reasoning for why every fixture here is anchored on a VENUE's
// calendar rather than on a UTC instant: the service reads Postgres now(), there
// is no clock seam, and any assertion anchored on the wall clock flips depending
// on what hour CI runs at. everyCallerOffsetZone, venueLocalInstant,
// requireLocalAndUTCDatesDiffer and newVenueInZone all come from there.
//
// The list and the picker are tested together on purpose: their counts have to
// be drawn from one partition, or a non-zero city count dead-ends at an empty
// list.

// createApprovedShowAt binds the shared fixture to this suite's db/T. The
// fixture itself lives in show_venue_local_test.go so all three venue-local
// suites partition the same show shape.
func (suite *ShowServiceIntegrationTestSuite) createApprovedShowAt(venueID, userID uint, city, state string, at time.Time) *catalogm.Show {
	return newApprovedShowAt(suite.T(), suite.db, venueID, userID, city, state, at)
}

// upcomingPage is what every assertion below compares: the ids on the page and
// the filter-aware total, as one comparable value. Comparing the pair rather
// than either half is deliberate — a boundary bug that moved the total without
// moving the page (they are separate queries) would otherwise slip through.
type upcomingPage struct {
	IDs   []uint
	Total int64
}

func (suite *ShowServiceIntegrationTestSuite) upcomingShows(callerZone string) upcomingPage {
	shows, _, total, err := suite.showService.GetUpcomingShows(callerZone, "", 50, false, nil)
	suite.Require().NoError(err)
	ids := make([]uint, 0, len(shows))
	for _, s := range shows {
		ids = append(ids, s.ID)
	}
	return upcomingPage{IDs: ids, Total: total}
}

func (suite *ShowServiceIntegrationTestSuite) upcomingShowIDs(callerZone string) ([]uint, int64) {
	page := suite.upcomingShows(callerZone)
	return page.IDs, page.Total
}

// The headline acceptance criterion: a Phoenix date stays listed until midnight
// in Phoenix, however far east the reader is. 23:00 tonight in Phoenix is
// already tomorrow in UTC and in Europe, so the old caller-anchored boundary
// dropped it for a Berlin reader hours before doors.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_PhoenixShowStaysListedForEveryViewerZone() {
	const zone = "America/Phoenix" // UTC-7, no DST
	venue := newVenueInZone(suite.T(), suite.db, "Phoenix Room", "AZ", zone, true)
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), zone, 0, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	show := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", at)

	callerZones := append(everyCallerOffsetZone(), "Europe/Berlin", "Asia/Tokyo", "UTC")
	for _, callerZone := range callerZones {
		ids, total := suite.upcomingShowIDs(callerZone)
		suite.Require().Equal(int64(1), total, "caller zone %q lost tonight's Phoenix show", callerZone)
		suite.Require().Equal([]uint{show.ID}, ids, "caller zone %q", callerZone)
	}
}

// The mirror: venue-local yesterday is gone even while UTC still calls that
// instant today. A UTC-anchored boundary re-admits the previous evening's shows
// for most of the day, which is the "yesterday's US shows read as upcoming" half
// of the bug.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_VenueLocalYesterdayIsGoneEvenWhenUTCDateIsToday() {
	const zone = "Pacific/Honolulu" // UTC-10, no DST
	venue := newVenueInZone(suite.T(), suite.db, "Honolulu Room", "HI", zone, true)
	user := suite.createTestUser()

	// 23:00 yesterday in Honolulu is 09:00 TODAY in UTC.
	at := venueLocalInstant(suite.T(), zone, -1, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	suite.createApprovedShowAt(venue.ID, user.ID, "Honolulu", "HI", at)

	_, total := suite.upcomingShowIDs("UTC")
	suite.Equal(int64(0), total, "UTC's calendar must not pull a finished show back into upcoming")
}

// A show already in progress is still tonight's listing. This is the half a
// future-dated fixture cannot prove, and the half an instant-based boundary
// (event_date > now()) gets wrong.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_AlreadyStartedShowStaysListedUntilVenueMidnight() {
	const zone = "Pacific/Honolulu"
	venue := newVenueInZone(suite.T(), suite.db, "In Progress Room", "HI", zone, true)
	user := suite.createTestUser()

	// The first instant of the venue's today: as far into the venue's past as a
	// show can be while still belonging to today's listing.
	show := suite.createApprovedShowAt(venue.ID, user.ID, "Honolulu", "HI", venueLocalInstant(suite.T(), zone, 0, 0))

	ids, total := suite.upcomingShowIDs("UTC")
	suite.Require().Equal(int64(1), total, "a show already in progress is still an upcoming listing")
	suite.Equal([]uint{show.ID}, ids)
}

// Caller-INDEPENDENCE, swept across the whole inhabited offset range. A show
// seeded half a day back sits inside the window where the old boundary was
// ambiguous, so the pre-PSY-1678 code has to contradict itself here at every
// hour of the day.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_SameAnswerForEveryCallerZone() {
	venue := newVenueInZone(suite.T(), suite.db, "Sweep Room", "AZ", "Asia/Tokyo", true)
	user := suite.createTestUser()
	suite.createApprovedShowAt(venue.ID, user.ID, "Sweepville", "AZ", time.Now().Add(-12*time.Hour))

	assertSameForEveryCallerZone(suite.T(), "GetUpcomingShows", suite.upcomingShows)
}

// Caller-independence alone would also be satisfied by a hardcoded UTC boundary.
// This pins venue-LOCALITY at the extremes of the offset range, where the coarse
// sargable prefilter in shared.VenueLocalDateCondition is likeliest to clip a
// row the exact condition would keep.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_ExtremeVenueOffsetSurvivesCoarsePrefilter() {
	user := suite.createTestUser()

	for _, zone := range []string{"Etc/GMT+12", "Etc/GMT+11", "Etc/GMT-14", "Etc/GMT-13"} {
		venue := newVenueInZone(suite.T(), suite.db, "Edge Room "+zone, "AZ", zone, true)
		midnightToday := suite.createApprovedShowAt(venue.ID, user.ID, "Edgeville", "AZ",
			venueLocalInstant(suite.T(), zone, 0, 0))
		yesterdayLate := suite.createApprovedShowAt(venue.ID, user.ID, "Edgeville", "AZ",
			venueLocalInstant(suite.T(), zone, -1, 23))

		ids, total := suite.upcomingShowIDs("UTC")
		suite.Require().Equal(int64(1), total, "zone %s", zone)
		suite.Require().Equal([]uint{midnightToday.ID}, ids,
			"zone %s: venue-local midnight today in, the last instant of yesterday out", zone)

		// Each zone starts clean, so the assertions above stay exact rather than
		// accumulating every earlier zone's rows. Scoped to the two ids this
		// iteration created rather than truncating the tables: an unscoped wipe
		// would also have to replicate TearDownTest's FK-safe order (bookmarks
		// and show_artists before show_venues), and would silently destroy any
		// suite-level fixture a later edit introduces.
		suite.removeShows(midnightToday.ID, yesterdayLate.ID)
	}
}

// removeShows deletes shows and their venue links by id, children first.
func (suite *ShowServiceIntegrationTestSuite) removeShows(ids ...uint) {
	suite.Require().NoError(
		suite.db.Where("show_id IN ?", ids).Delete(&catalogm.ShowVenue{}).Error)
	suite.Require().NoError(
		suite.db.Where("id IN ?", ids).Delete(&catalogm.Show{}).Error)
}

// A venue predating the timezone backfill still has to partition, and on the
// SAME zone every other surface renders it in (utils.EventLocation's state-map
// arm) rather than UTC.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_NullVenueTimezoneUsesStateMap() {
	// Hawaii, not Arizona: HST is UTC-10, far enough from UTC that a fixture
	// straddling the two calendars is unambiguous.
	venue := suite.createTestVenue("No Zone Room", "Honolulu", "HI", true)
	suite.Require().Nil(venue.Timezone)
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), "Pacific/Honolulu", -1, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, "Pacific/Honolulu")
	suite.createApprovedShowAt(venue.ID, user.ID, "Honolulu", "HI", at)

	_, total := suite.upcomingShowIDs("UTC")
	suite.Equal(int64(0), total, "a NULL venue zone must fall back to the state map, not UTC")
}

// Multi-venue shows resolve on the PRIMARY (lowest venue_id) venue, matching the
// display convention and every other migrated surface. Without it a show booked
// at two venues could read upcoming here and past on the venue page.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_MultiVenueShowUsesPrimaryVenueZone() {
	honolulu := newVenueInZone(suite.T(), suite.db, "First Room", "HI", "Pacific/Honolulu", true)
	tokyo := newVenueInZone(suite.T(), suite.db, "Second Room", "AZ", "Asia/Tokyo", true)
	suite.Require().Less(honolulu.ID, tokyo.ID)
	user := suite.createTestUser()

	// 23:00 yesterday in Honolulu is 18:00 TODAY in Tokyo: past on the primary
	// venue's calendar, still upcoming on the second's.
	at := venueLocalInstant(suite.T(), "Pacific/Honolulu", -1, 23)
	show := suite.createApprovedShowAt(honolulu.ID, user.ID, "Honolulu", "HI", at)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: tokyo.ID}).Error)

	_, total := suite.upcomingShowIDs("UTC")
	suite.Equal(int64(0), total, "the primary venue's zone decides, not the later-added one's")
}

// A show with NO venue row at all must still partition rather than vanish. The
// lateral is a LEFT JOIN precisely so this row survives it; every zone input
// reads NULL, so the state CASE falls through to its ELSE arm. This matters more
// on the main feed than on the artist and venue lists, which can only be reached
// through a relation a venueless show does not have.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_VenuelessShowStillPartitions() {
	user := suite.createTestUser()

	future := &catalogm.Show{
		Title:       "Venueless Upcoming",
		EventDate:   time.Now().Add(48 * time.Hour),
		City:        stringPtr("Nowhere"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.Require().NoError(suite.db.Create(future).Error)

	past := &catalogm.Show{
		Title:       "Venueless Past",
		EventDate:   time.Now().Add(-96 * time.Hour),
		City:        stringPtr("Nowhere"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.Require().NoError(suite.db.Create(past).Error)

	ids, total := suite.upcomingShowIDs("UTC")
	suite.Require().Equal(int64(1), total, "the venueless future show must survive the LEFT JOIN")
	suite.Equal([]uint{future.ID}, ids)

	// And it must be counted by the picker on the same terms.
	cities, err := suite.showService.GetShowCities("UTC")
	suite.Require().NoError(err)
	suite.Require().Len(cities, 1)
	suite.Equal("Nowhere", cities[0].City)
	suite.Equal(1, cities[0].ShowCount)
}

// The city filters have to compose with the venue-local partition rather than
// replace it, and they now run in a query that spans two relations.
//
// This does NOT test column disambiguation, despite where it sits. Postgres
// cannot catch a `shows.`/`venue_tz.` mix-up here: shared.VenueTZJoin projects
// under `venue_tz_*` aliases, so nothing the lateral exposes collides with a
// `shows` column and an unqualified `city`/`state` resolves cleanly. That is
// deliberate, and it is pinned where it is actually enforced —
// shared/show_venue_local_sql_test.go asserts the aliases on both the lateral
// and the state CASE. The `shows.` qualifiers in applyUpcomingFilters are
// readability, not a constraint, and stripping them would leave this test green.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_CityFilterAppliesInsideTheVenueLocalPartition() {
	venue := newVenueInZone(suite.T(), suite.db, "Filter Room", "AZ", "America/Phoenix", true)
	user := suite.createTestUser()
	tonight := venueLocalInstant(suite.T(), "America/Phoenix", 0, 23)
	lastNight := venueLocalInstant(suite.T(), "America/Phoenix", -1, 23)

	wanted := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", tonight)
	suite.createApprovedShowAt(venue.ID, user.ID, "Mesa", "AZ", tonight)
	// Same city as `wanted`, but the venue's yesterday. The filter must not
	// widen the partition to reach it — an AND that became an OR, or a date
	// condition dropped when filters are present, both surface here.
	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", lastNight)

	shows, _, total, err := suite.showService.GetUpcomingShows("UTC", "", 50, false,
		&contracts.UpcomingShowsFilter{Cities: []contracts.CityStateFilter{{City: "Phoenix", State: "AZ"}}})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total, "the filtered total is still bounded by the venue-local partition")
	suite.Require().Len(shows, 1)
	suite.Equal(wanted.ID, shows[0].ID)

	shows, _, total, err = suite.showService.GetUpcomingShows("UTC", "", 50, false,
		&contracts.UpcomingShowsFilter{City: "Mesa", State: "AZ"})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total, "legacy single-city filter")
	suite.Require().Len(shows, 1)
}

// Paging still works with the lateral in the query, and the page rows still
// carry their preloaded relations — `SELECT shows.*` is what keeps venue_tz's
// own columns from being scanned over the show's.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_CursorPagesWithinTheVenueLocalPartition() {
	venue := newVenueInZone(suite.T(), suite.db, "Paging Room", "AZ", "America/Phoenix", true)
	user := suite.createTestUser()
	for day := 0; day < 3; day++ {
		suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ",
			venueLocalInstant(suite.T(), "America/Phoenix", day, 20))
	}

	first, cursor, total, err := suite.showService.GetUpcomingShows("UTC", "", 2, false, nil)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total, "total covers the whole partition, not the page")
	suite.Require().Len(first, 2)
	suite.Require().NotNil(cursor)
	suite.Require().Len(first[0].Venues, 1, "preloads survive the lateral")
	suite.Equal("Paging Room", first[0].Venues[0].Name)

	second, nextCursor, total, err := suite.showService.GetUpcomingShows("UTC", *cursor, 2, false, nil)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Require().Len(second, 1)
	suite.Nil(nextCursor)
	suite.NotEqual(first[0].ID, second[0].ID)
}

// The picker must count the SAME partition the list serves. A count drawn from a
// different boundary offers a city whose link then lands on an empty list.
func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_CountsTheSameVenueLocalPartitionAsTheList() {
	phoenix := newVenueInZone(suite.T(), suite.db, "Phoenix Room", "AZ", "America/Phoenix", true)
	honolulu := newVenueInZone(suite.T(), suite.db, "Honolulu Room", "HI", "Pacific/Honolulu", true)
	user := suite.createTestUser()

	// Tonight in Phoenix: listed, and counted.
	suite.createApprovedShowAt(phoenix.ID, user.ID, "Phoenix", "AZ",
		venueLocalInstant(suite.T(), "America/Phoenix", 0, 23))
	// Last night in Honolulu: neither listed nor counted, even though UTC still
	// calls that instant today.
	suite.createApprovedShowAt(honolulu.ID, user.ID, "Honolulu", "HI",
		venueLocalInstant(suite.T(), "Pacific/Honolulu", -1, 23))

	cities, err := suite.showService.GetShowCities("UTC")
	suite.Require().NoError(err)
	suite.Require().Len(cities, 1, "only cities with a show in the venue-local upcoming partition")
	suite.Equal("Phoenix", cities[0].City)
	suite.Equal("AZ", cities[0].State)
	suite.Equal(1, cities[0].ShowCount)

	// The invariant, stated directly: every city the picker offers has to be
	// reachable in the list it filters.
	for _, city := range cities {
		shows, _, total, err := suite.showService.GetUpcomingShows("UTC", "", 50, false,
			&contracts.UpcomingShowsFilter{Cities: []contracts.CityStateFilter{{City: city.City, State: city.State}}})
		suite.Require().NoError(err)
		suite.Equal(int64(city.ShowCount), total,
			"picker count for %s, %s must equal the filtered list total", city.City, city.State)
		suite.Len(shows, city.ShowCount)
	}
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_SameAnswerForEveryCallerZone() {
	venue := newVenueInZone(suite.T(), suite.db, "Cities Sweep Room", "AZ", "Asia/Tokyo", true)
	user := suite.createTestUser()
	suite.createApprovedShowAt(venue.ID, user.ID, "Sweepville", "AZ", time.Now().Add(-12*time.Hour))

	assertSameForEveryCallerZone(suite.T(), "GetShowCities",
		func(callerZone string) []contracts.ShowCityResponse {
			cities, err := suite.showService.GetShowCities(callerZone)
			suite.Require().NoError(err, "caller zone %q", callerZone)
			return cities
		})
}
