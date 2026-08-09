package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Venue-local upcoming/past partitioning for artist and venue show lists.
//
// These tests are written to be TRUE AT EVERY HOUR OF THE DAY, which rules out
// the obvious framing ("seed a show 3 hours ago and assert it is past"): the
// service reads Postgres now(), there is no seam to inject a clock, and any
// assertion anchored on the UTC instant flips depending on when CI happens to
// run. So every fixture below is anchored on the VENUE's calendar instead —
// "yesterday at 23:00 in Honolulu" is a fixed number of days from the venue's
// today no matter what the wall clock says, which is exactly the property the
// production code is supposed to have.
//
// Zones are chosen for stability, not realism: Pacific/Honolulu (UTC-10) and
// Asia/Tokyo (UTC+9) observe no DST, so their offsets cannot drift under the
// test. DST-observing zones belong in a test that pins a specific date.

// everyCallerOffsetZone spans the inhabited UTC offset range at one-hour
// granularity. The Etc/GMT zones are POSIX-signed (Etc/GMT+5 is UTC-5) and
// observe no DST, which is what makes them usable as a stable sweep.
//
// Sweeping the WHOLE range is what gives assertSamePartitionForEveryCallerZone
// its teeth. The old boundary was start-of-today in the caller's zone, and local
// midnight today is always somewhere in the 24 hours before now — so for a show
// seeded inside that 24-hour window, some zone in this list necessarily says
// "upcoming" and some necessarily says "past". A test that pinned one or two
// zones would pass or fail depending on what hour CI ran at; this one cannot.
func everyCallerOffsetZone() []string {
	zones := make([]string, 0, 27)
	for offset := 12; offset >= -14; offset-- {
		zones = append(zones, fmt.Sprintf("Etc/GMT%+d", offset))
	}
	return zones
}

// venueLocalInstant returns the instant at hourLocal:00 on the day dayOffset
// days from today, ON THE GIVEN ZONE'S CALENDAR.
func venueLocalInstant(t *testing.T, zone string, dayOffset, hourLocal int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	require.NoError(t, err, "load zone %q", zone)
	day := time.Now().In(loc).AddDate(0, 0, dayOffset)
	return time.Date(day.Year(), day.Month(), day.Day(), hourLocal, 0, 0, 0, loc)
}

// requireLocalAndUTCDatesDiffer asserts the fixture actually exercises the thing
// under test. Without it a zone/offset change could quietly make the venue-local
// and UTC calendars agree, leaving a test that passes for the wrong reason.
func requireLocalAndUTCDatesDiffer(t *testing.T, at time.Time, zone string) {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	require.NoError(t, err, "load zone %q", zone)
	require.NotEqual(t, at.UTC().Format("2006-01-02"), at.In(loc).Format("2006-01-02"),
		"fixture no longer straddles a date boundary")
}

// newVenueInZone creates a venue with an explicit IANA zone. The suites' own
// createTestVenue helpers deliberately leave `timezone` NULL (the pre-backfill
// shape, covered by its own tests below) and their signatures differ, so the
// zone-carrying constructor lives here as one free function instead of once per
// suite.
func newVenueInZone(t *testing.T, db *gorm.DB, name, state, zone string, verified bool) *catalogm.Venue {
	t.Helper()
	venue := &catalogm.Venue{
		Name:     name,
		City:     "Testville",
		State:    state,
		Verified: verified,
	}
	if zone != "" {
		venue.Timezone = stringPtr(zone)
	}
	require.NoError(t, db.Create(venue).Error)
	return venue
}

// listPartition adapts a service's paged list to (show ids, total) so the sweep
// assertion below can be written once for both services.
type listPartition func(callerZone, timeFilter string) (ids []uint, total int64, err error)

// assertSamePartitionForEveryCallerZone is the load-bearing regression
// assertion, and the only one here guaranteed to fail against the pre-PSY-1695
// code at EVERY hour of the day.
//
// The caller seeds one show half a day back, which puts it strictly inside the
// window where the old caller-anchored boundary was ambiguous (see
// everyCallerOffsetZone). The old code therefore HAS to contradict itself across
// the sweep; the new code cannot, because none of these zones reaches the query
// any more.
//
// It asserts agreement rather than a specific side on purpose: which partition a
// half-day-old show belongs to depends on the venue's calendar and the hour, and
// pinning that would reintroduce exactly the clock dependence this exists to
// remove.
func assertSamePartitionForEveryCallerZone(t *testing.T, wantShowID uint, list listPartition) {
	t.Helper()
	var firstZone string
	var wasUpcoming bool
	for i, callerZone := range everyCallerOffsetZone() {
		upcoming, upcomingTotal, err := list(callerZone, "upcoming")
		require.NoError(t, err, "caller zone %q", callerZone)
		past, pastTotal, err := list(callerZone, "past")
		require.NoError(t, err, "caller zone %q", callerZone)

		// Exactly one partition claims it, whichever one that is.
		require.Equal(t, int64(1), upcomingTotal+pastTotal,
			"caller zone %q: show must appear in exactly one partition", callerZone)
		require.Len(t, append(upcoming, past...), 1, "caller zone %q", callerZone)
		require.Equal(t, wantShowID, append(upcoming, past...)[0], "caller zone %q", callerZone)

		isUpcoming := upcomingTotal == 1
		if i == 0 {
			firstZone, wasUpcoming = callerZone, isUpcoming
			continue
		}
		require.Equal(t, wasUpcoming, isUpcoming,
			"caller zone %q disagrees with %q about the same show: the split is still being made in the CALLER's timezone",
			callerZone, firstZone)
	}
}

// =============================================================================
// Artist show lists
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) artistShowIDs(artistID uint) listPartition {
	return func(callerZone, timeFilter string) ([]uint, int64, error) {
		shows, total, err := suite.artistService.GetShowsForArtist(artistID, callerZone, contracts.ArtistShowsQuery{TimeFilter: timeFilter, Limit: 50})
		return artistShowIDsOf(shows), total, err
	}
}

// The regression PSY-1695 exists for: the show's UTC calendar date says today,
// its venue's calendar says yesterday, and the listing must follow the venue.
// A UTC-anchored boundary calls this upcoming for most of the day.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_VenueLocalYesterdayIsPast_EvenWhenUTCDateIsToday() {
	const zone = "Pacific/Honolulu" // UTC-10, no DST
	artist := suite.createTestArtist("Honolulu Artist")
	venue := newVenueInZone(suite.T(), suite.db, "Honolulu Room", "HI", zone, false)
	user := suite.createTestUser()

	// 23:00 yesterday in Honolulu is 09:00 TODAY in UTC.
	at := venueLocalInstant(suite.T(), zone, -1, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, at)

	past, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal, "venue-local yesterday belongs to past")
	suite.Require().Len(past, 1)

	upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingTotal, "UTC's calendar must not pull the show back into upcoming")
	suite.Empty(upcoming)
}

// The mirror image: venue-local today, UTC tomorrow. A late-night show is still
// tonight's listing, and stays upcoming through venue-local midnight.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_VenueLocalTodayIsUpcoming_EvenWhenUTCDateIsTomorrow() {
	const zone = "Pacific/Honolulu"
	artist := suite.createTestArtist("Late Night Artist")
	venue := newVenueInZone(suite.T(), suite.db, "Late Night Room", "HI", zone, false)
	user := suite.createTestUser()

	// 23:00 today in Honolulu is 09:00 TOMORROW in UTC.
	at := venueLocalInstant(suite.T(), zone, 0, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, at)

	upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal)
	suite.Require().Len(upcoming, 1)

	_, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(0), pastTotal)
}

// A show that has ALREADY STARTED is still tonight's listing until venue-local
// midnight. This is the half of the rule a future-dated fixture cannot prove:
// the old caller-anchored boundary was start-of-today in the caller's zone,
// which for a caller east of the venue had already passed venue-local midnight,
// so it filed this show under Past.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_AlreadyStartedShowStaysUpcomingUntilVenueMidnight() {
	const zone = "Pacific/Honolulu"
	artist := suite.createTestArtist("In Progress Artist")
	venue := newVenueInZone(suite.T(), suite.db, "In Progress Room", "HI", zone, false)
	user := suite.createTestUser()

	// The first instant of the venue's today: as far into the venue's past as a
	// show can be while still belonging to today's listing.
	at := venueLocalInstant(suite.T(), zone, 0, 0)
	suite.Require().True(at.Before(time.Now()) || at.Equal(time.Now()) || time.Since(at) > -time.Hour,
		"fixture should be at or before now for most of the venue's day")
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, at)

	_, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal, "a show already in progress is still an upcoming listing")

	_, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(0), pastTotal)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_SamePartitionForEveryCallerZone() {
	artist := suite.createTestArtist("Sweep Artist")
	venue := newVenueInZone(suite.T(), suite.db, "Sweep Room", "AZ", "Asia/Tokyo", false)
	user := suite.createTestUser()

	show := suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().Add(-12*time.Hour))

	assertSamePartitionForEveryCallerZone(suite.T(), show.ID, suite.artistShowIDs(artist.ID))
}

// A venue predating the timezone backfill still has to partition, and must do it
// on the SAME zone every other surface renders it in — utils.EventLocation's
// state-map arm, not UTC. A Phoenix venue judged in UTC would sit 7 hours from
// its own show page.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_NullVenueTimezoneUsesStateMap() {
	artist := suite.createTestArtist("No Zone Artist")
	// Hawaii, not Arizona: HST is UTC-10, so a fixture that straddles the two
	// calendars distinguishes the state map from both UTC and the Phoenix
	// default that an unmatched state would fall to.
	venue := newVenueInZone(suite.T(), suite.db, "No Zone Room", "HI", "", false)
	suite.Require().Nil(venue.Timezone)
	user := suite.createTestUser()

	// 23:00 yesterday in Honolulu is 09:00 today in UTC: past on Hawaii's
	// calendar, and NOT past under a UTC or Phoenix boundary.
	at := venueLocalInstant(suite.T(), "Pacific/Honolulu", -1, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, "Pacific/Honolulu")
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, at)

	_, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal, "a NULL venue zone must resolve through the state map, not UTC")

	_, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingTotal)
}

// The trust-the-column contract, stated as a test rather than left implicit.
//
// The partition reads venues.timezone straight into AT TIME ZONE, so a stored
// zone the server does not carry does NOT degrade -- it raises SQLSTATE 22023
// and takes the listing query down. That is the cost of not paying per-row
// validation (8.1s on a 20k-show venue), and it is only acceptable because two
// layers keep such a value out of the column: the write gate (PSY-1707) and the
// integrity sweep below.
//
// This asserts the raw failure mode so nobody "fixes" it by quietly
// reintroducing a COALESCE that would mask real drift, and then asserts the
// sweep is what actually resolves it.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_DriftedVenueTimezoneRaisesUntilSwept() {
	artist := suite.createTestArtist("Drift Artist")
	venue := newVenueInZone(suite.T(), suite.db, "Drift Room", "HI", "Pacific/Honolulu", false)
	user := suite.createTestUser()
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), "Pacific/Honolulu", 1, 20))

	// Simulate tzdata drift: a zone that WAS valid when written is no longer in
	// the catalog. Written straight to the column, bypassing the write gate,
	// because that is exactly what drift looks like.
	suite.Require().NoError(suite.db.Table("venues").Where("id = ?", venue.ID).
		Update("timezone", "Pacific/Atlantis").Error)

	_, _, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().Error(err, "a drifted zone must surface loudly, not silently mis-partition")
	suite.Contains(err.Error(), "not recognized")

	report, sweepErr := SweepVenueTimezones(context.Background(), suite.db)
	suite.Require().NoError(sweepErr)
	suite.Equal(1, report.Cleared)
	suite.Require().Len(report.Drifted, 1)
	suite.Equal("Pacific/Atlantis", report.Drifted[0].Timezone)

	// After the sweep the venue falls back to the state map and listings work.
	_, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err, "the sweep must restore a queryable state")
	suite.Equal(int64(1), upcomingTotal)
}

// A show with no venue row at all: the LEFT JOIN LATERAL must keep it, or
// venue-less shows would silently disappear from every artist page. With no
// venue there is no state either, so it lands on GetTimezoneForState's default.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_VenuelessShowStillPartitions() {
	artist := suite.createTestArtist("Venueless Artist")
	user := suite.createTestUser()

	newShow := func(title string, at time.Time) *catalogm.Show {
		show := &catalogm.Show{
			Title:       title,
			EventDate:   at,
			Status:      catalogm.ShowStatusApproved,
			SubmittedBy: &user.ID,
		}
		suite.Require().NoError(suite.db.Create(show).Error)
		suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, Position: 0}).Error)
		return show
	}
	upcomingShow := newShow("Venueless Upcoming", venueLocalInstant(suite.T(), "America/Phoenix", 1, 12))
	pastShow := newShow("Venueless Past", venueLocalInstant(suite.T(), "America/Phoenix", -1, 12))

	upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal)
	suite.Require().Len(upcoming, 1)
	suite.Equal(upcomingShow.ID, upcoming[0].ID)

	past, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal)
	suite.Require().Len(past, 1)
	suite.Equal(pastShow.ID, past[0].ID)
}

// limit pages the venue-local partition and total counts it — the property that
// made client-side re-partitioning a non-starter.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_LimitAndTotalApplyToVenueLocalPartition() {
	const zone = "Pacific/Honolulu"
	artist := suite.createTestArtist("Paging Artist")
	venue := newVenueInZone(suite.T(), suite.db, "Paging Room", "HI", zone, false)
	user := suite.createTestUser()

	// 3 venue-local past, 2 venue-local upcoming. Every show is seeded at 23:00
	// local, so each one's UTC date is the following day — under the old
	// UTC-anchored boundary some of them would count as upcoming.
	for i := 1; i <= 3; i++ {
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, -i, 23))
	}
	for i := 1; i <= 2; i++ {
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, i, 23))
	}

	upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 1})
	suite.Require().NoError(err)
	suite.Equal(int64(2), upcomingTotal, "total counts the whole venue-local partition")
	suite.Require().Len(upcoming, 1, "limit pages the venue-local partition")

	past, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "past", Limit: 1})
	suite.Require().NoError(err)
	suite.Equal(int64(3), pastTotal)
	suite.Require().Len(past, 1)

	// "all" keeps every row and is deliberately not venue-zone aware: there is
	// no boundary to draw.
	_, allTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "all", Limit: 50})
	suite.Require().NoError(err)
	suite.Equal(int64(5), allTotal)
}

// The coarse sargable bounds in VenueLocalDateCondition must never clip a row
// the exact venue-local condition would keep. The worst case is a venue at an
// extreme negative offset whose local midnight is nearly a whole day behind
// now: a show at venue-local today 00:00 in Etc/GMT+12 (UTC-12) sits ~24 hours
// in the past as an instant, and must still be upcoming.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_ExtremeVenueOffsetSurvivesCoarsePrefilter() {
	artist := suite.createTestArtist("Edge Offset Artist")
	user := suite.createTestUser()

	for _, zone := range []string{"Etc/GMT+12", "Etc/GMT+11", "Etc/GMT-14", "Etc/GMT-13"} {
		venue := newVenueInZone(suite.T(), suite.db, "Edge Room "+zone, "AZ", zone, false)
		// First instant of the venue's today: the earliest thing "upcoming" can
		// possibly mean for this venue.
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, 0, 0))
		// Last instant of the venue's yesterday: the latest thing "past" can mean.
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, -1, 23))
	}

	_, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 50})
	suite.Require().NoError(err)
	suite.Equal(int64(4), upcomingTotal, "the coarse lower bound must not clip a venue-local midnight at an extreme offset")

	_, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "past", Limit: 50})
	suite.Require().NoError(err)
	suite.Equal(int64(4), pastTotal, "the coarse upper bound must not clip the last instant of venue-local yesterday")

	// Nothing fell through the gap between the two partitions.
	_, allTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "all", Limit: 50})
	suite.Require().NoError(err)
	suite.Equal(int64(8), allTotal)
}

// The graph card's "next show" and the artist page's upcoming list are
// documented as having to agree (PSY-1352), so the card must not name a show the
// list has already filed under Past.
func (suite *ArtistServiceIntegrationTestSuite) TestGetNextShowForArtist_AgreesWithUpcomingList() {
	const zone = "Pacific/Honolulu"
	artist := suite.createTestArtist("Next Show Artist")
	venue := newVenueInZone(suite.T(), suite.db, "Next Show Room", "HI", zone, false)
	user := suite.createTestUser()

	// Venue-local yesterday (UTC today): past on both paths.
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, -1, 23))
	// Venue-local today, already started: still tonight's listing on both.
	tonight := suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, 0, 0))

	upcoming, _, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Require().NotEmpty(upcoming)

	next, err := suite.artistService.GetNextShowForArtist(artist.ID, "UTC")
	suite.Require().NoError(err)
	suite.Require().NotNil(next)
	suite.Equal(tonight.ID, next.ID)
	suite.Equal(upcoming[0].ID, next.ID, "next show must be the first row of the upcoming list")
}

// =============================================================================
// Venue show lists
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) venueShowIDs(venueID uint) listPartition {
	return func(callerZone, timeFilter string) ([]uint, int64, error) {
		shows, total, err := suite.venueService.GetShowsForVenue(venueID, callerZone, contracts.VenueShowsQuery{TimeFilter: timeFilter, Limit: 50})
		ids := make([]uint, 0, len(shows))
		for _, s := range shows {
			ids = append(ids, s.ID)
		}
		return ids, total, err
	}
}

func (suite *VenueServiceIntegrationTestSuite) createApprovedShowAt(venueID, userID uint, at time.Time) *catalogm.Show {
	show := &catalogm.Show{
		Title:       fmt.Sprintf("Show-%d", time.Now().UnixNano()),
		EventDate:   at,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)
	return show
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_VenueLocalYesterdayIsPast_EvenWhenUTCDateIsToday() {
	const zone = "Pacific/Honolulu"
	venue := newVenueInZone(suite.T(), suite.db, "Honolulu Room", "HI", zone, true)
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), zone, -1, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	suite.createApprovedShowAt(venue.ID, user.ID, at)

	past, pastTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal)
	suite.Require().Len(past, 1)

	upcoming, upcomingTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingTotal)
	suite.Empty(upcoming)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_VenueLocalTodayIsUpcoming_EvenWhenUTCDateIsTomorrow() {
	const zone = "Pacific/Honolulu"
	venue := newVenueInZone(suite.T(), suite.db, "Late Night Room", "HI", zone, true)
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), zone, 0, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	suite.createApprovedShowAt(venue.ID, user.ID, at)

	upcoming, upcomingTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal)
	suite.Require().Len(upcoming, 1)

	_, pastTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(0), pastTotal)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_SamePartitionForEveryCallerZone() {
	venue := newVenueInZone(suite.T(), suite.db, "Sweep Room", "AZ", "Asia/Tokyo", true)
	user := suite.createTestUser()

	show := suite.createApprovedShowAt(venue.ID, user.ID, time.Now().Add(-12*time.Hour))

	assertSamePartitionForEveryCallerZone(suite.T(), show.ID, suite.venueShowIDs(venue.ID))
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_NullVenueTimezoneUsesStateMap() {
	venue := newVenueInZone(suite.T(), suite.db, "No Zone Room", "HI", "", true)
	suite.Require().Nil(venue.Timezone)
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), "Pacific/Honolulu", -1, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), at, "Pacific/Honolulu")
	suite.createApprovedShowAt(venue.ID, user.ID, at)

	_, pastTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal, "a NULL venue zone must resolve through the state map, not UTC")

	_, upcomingTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingTotal)
}

// Pins the documented "primary venue (lowest venue_id) decides the zone" rule
// for the rare show booked at two venues. The point is that ONE show has ONE
// venue-local date on every surface, so it cannot read upcoming on the venue
// page and past on the artist page. The cost is visible here: queried through
// the Tokyo venue, the show is judged on Honolulu's calendar.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_MultiVenueShowUsesPrimaryVenueZone() {
	// Created first, so it holds the lower venue_id and owns the zone.
	honolulu := newVenueInZone(suite.T(), suite.db, "First Room", "HI", "Pacific/Honolulu", true)
	tokyo := newVenueInZone(suite.T(), suite.db, "Second Room", "AZ", "Asia/Tokyo", true)
	suite.Require().Less(honolulu.ID, tokyo.ID)
	user := suite.createTestUser()

	// 23:00 yesterday in Honolulu is 18:00 TODAY in Tokyo: past on the primary
	// venue's calendar, still upcoming on the second's.
	at := venueLocalInstant(suite.T(), "Pacific/Honolulu", -1, 23)
	show := suite.createApprovedShowAt(honolulu.ID, user.ID, at)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: tokyo.ID}).Error)

	upcoming, upcomingTotal, err := suite.venueService.GetShowsForVenue(tokyo.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "upcoming", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingTotal, "primary venue's zone decides, not the queried venue's")
	suite.Empty(upcoming)

	past, pastTotal, err := suite.venueService.GetShowsForVenue(tokyo.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal)
	suite.Require().Len(past, 1)
	suite.Equal(show.ID, past[0].ID)
}

// Venue-suite twin of TestGetShowsForArtist_ExtremeVenueOffsetSurvivesCoarsePrefilter.
// Without it the venue path had no test that both proves venue-locality AND
// fails against a caller- or UTC-anchored boundary at every hour of the day:
// SamePartitionForEveryCallerZone proves only caller-INDEPENDENCE, which a
// hardcoded UTC boundary would also satisfy.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_ExtremeVenueOffsetSurvivesCoarsePrefilter() {
	user := suite.createTestUser()

	for _, zone := range []string{"Etc/GMT+12", "Etc/GMT+11", "Etc/GMT-14", "Etc/GMT-13"} {
		venue := newVenueInZone(suite.T(), suite.db, "Edge Room "+zone, "AZ", zone, true)
		suite.createApprovedShowAt(venue.ID, user.ID, venueLocalInstant(suite.T(), zone, 0, 0))
		suite.createApprovedShowAt(venue.ID, user.ID, venueLocalInstant(suite.T(), zone, -1, 23))

		_, upcomingTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "upcoming", Limit: 50})
		suite.Require().NoError(err, "zone %s", zone)
		suite.Equal(int64(1), upcomingTotal, "zone %s: venue-local midnight today must be upcoming", zone)

		_, pastTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "past", Limit: 50})
		suite.Require().NoError(err, "zone %s", zone)
		suite.Equal(int64(1), pastTotal, "zone %s: the last instant of venue-local yesterday must be past", zone)
	}
}

// A venue outside the US must NOT be judged on the US state map: "WA" is
// Western Australia as well as Washington, and the ungated map would put a
// Fremantle venue on America/Los_Angeles, 16 hours away.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_NonUSVenueIgnoresStateMap() {
	venue := &catalogm.Venue{
		Name:     "Fremantle Room",
		City:     "Fremantle",
		State:    "WA",
		Country:  stringPtr("Australia"),
		Verified: true,
	}
	suite.Require().NoError(suite.db.Create(venue).Error)
	suite.Require().Nil(venue.Timezone)
	user := suite.createTestUser()

	// Noon UTC yesterday: past on UTC's calendar (the non-US fallback) and on
	// Perth's, but 05:00 yesterday in America/Los_Angeles, which the ungated
	// state map would have used.
	suite.createApprovedShowAt(venue.ID, user.ID, venueLocalInstant(suite.T(), "UTC", -1, 12))

	_, pastTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{TimeFilter: "past", Limit: 10})
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal, "a non-US venue must fall back to UTC, not the US state map")
}
