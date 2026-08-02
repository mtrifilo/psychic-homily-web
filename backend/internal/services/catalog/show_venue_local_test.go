package catalog

import (
	"fmt"
	"testing"
	"time"

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
// Sweeping the WHOLE range is what gives the caller-independence tests teeth.
// The old boundary was start-of-today in the caller's zone, and local midnight
// today is always somewhere in the 24 hours before now — so for a show seeded
// inside that 24-hour window, some zone in this list necessarily says
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
	if err != nil {
		t.Fatalf("load zone %q: %v", zone, err)
	}
	day := time.Now().In(loc).AddDate(0, 0, dayOffset)
	return time.Date(day.Year(), day.Month(), day.Day(), hourLocal, 0, 0, 0, loc)
}

// localAndUTCDatesDiffer asserts the fixture actually exercises the thing under
// test. Without it a zone/offset change could quietly make the venue-local and
// UTC calendars agree, leaving a test that passes for the wrong reason.
func localAndUTCDatesDiffer(t *testing.T, at time.Time, zone string) {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatalf("load zone %q: %v", zone, err)
	}
	localDate := at.In(loc).Format("2006-01-02")
	utcDate := at.UTC().Format("2006-01-02")
	if localDate == utcDate {
		t.Fatalf("fixture no longer straddles a date boundary: %s local == %s UTC", localDate, utcDate)
	}
}

// =============================================================================
// Artist show lists
// =============================================================================

// createVenueInZone makes a venue whose IANA zone is set, which is what the
// partition resolves through. createTestVenue leaves it NULL on purpose (the
// pre-backfill shape) and that case is covered separately below.
func (suite *ArtistServiceIntegrationTestSuite) createVenueInZone(name, zone string) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:     name,
		City:     "Testville",
		State:    "AZ",
		Timezone: stringPtr(zone),
	}
	suite.Require().NoError(suite.db.Create(venue).Error)
	return venue
}

// The regression PSY-1695 exists for: the show's UTC calendar date says today,
// its venue's calendar says yesterday, and the listing must follow the venue.
// A UTC-anchored boundary calls this upcoming for most of the day.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_VenueLocalYesterdayIsPast_EvenWhenUTCDateIsToday() {
	const zone = "Pacific/Honolulu" // UTC-10, no DST
	artist := suite.createTestArtist("Honolulu Artist")
	venue := suite.createVenueInZone("Honolulu Room", zone)
	user := suite.createTestUser()

	// 23:00 yesterday in Honolulu is 09:00 TODAY in UTC.
	at := venueLocalInstant(suite.T(), zone, -1, 23)
	localAndUTCDatesDiffer(suite.T(), at, zone)
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, at)

	past, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "past")
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal, "venue-local yesterday belongs to past")
	suite.Require().Len(past, 1)

	upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingTotal, "UTC's calendar must not pull the show back into upcoming")
	suite.Empty(upcoming)
}

// The mirror image: venue-local today, UTC tomorrow. A late-night show is still
// tonight's listing, and stays upcoming through venue-local midnight.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_VenueLocalTodayIsUpcoming_EvenWhenUTCDateIsTomorrow() {
	const zone = "Pacific/Honolulu"
	artist := suite.createTestArtist("Late Night Artist")
	venue := suite.createVenueInZone("Late Night Room", zone)
	user := suite.createTestUser()

	// 23:00 today in Honolulu is 09:00 TOMORROW in UTC.
	at := venueLocalInstant(suite.T(), zone, 0, 23)
	localAndUTCDatesDiffer(suite.T(), at, zone)
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, at)

	upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal)
	suite.Require().Len(upcoming, 1)

	_, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "past")
	suite.Require().NoError(err)
	suite.Equal(int64(0), pastTotal)
}

// The acceptance criterion itself: the caller's zone no longer moves anything.
// Two callers 25 hours apart must be handed identical partitions.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_PartitionIgnoresCallerTimezone() {
	const zone = "Asia/Tokyo" // UTC+9, no DST
	artist := suite.createTestArtist("Tokyo Artist")
	venue := suite.createVenueInZone("Tokyo Room", zone)
	user := suite.createTestUser()

	yesterday := suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, -1, 12))
	tomorrow := suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, 1, 12))

	// Kiritimati (UTC+14) and Midway (UTC-11) are the extremes of the inhabited
	// offset range; an empty string and an unparseable zone cover the degraded
	// callers the old code fell back to UTC for.
	for _, callerZone := range []string{"Pacific/Kiritimati", "Pacific/Midway", "UTC", "", "Not/AZone"} {
		upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, callerZone, 10, "upcoming")
		suite.Require().NoError(err, "caller zone %q", callerZone)
		suite.Require().Len(upcoming, 1, "caller zone %q", callerZone)
		suite.Equal(tomorrow.ID, upcoming[0].ID, "caller zone %q", callerZone)
		suite.Equal(int64(1), upcomingTotal, "caller zone %q", callerZone)

		past, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, callerZone, 10, "past")
		suite.Require().NoError(err, "caller zone %q", callerZone)
		suite.Require().Len(past, 1, "caller zone %q", callerZone)
		suite.Equal(yesterday.ID, past[0].ID, "caller zone %q", callerZone)
		suite.Equal(int64(1), pastTotal, "caller zone %q", callerZone)
	}
}

// The load-bearing regression test, and the only one here that is guaranteed to
// fail against the pre-PSY-1695 code at EVERY hour of the day.
//
// The show is seeded half a day back, which puts it strictly inside the window
// where the old caller-anchored boundary was ambiguous: local midnight today is
// always less than 24 hours ago, so sweeping every offset zone is guaranteed to
// find one whose midnight falls before this show (upcoming) and one whose
// midnight falls after it (past). The old code therefore HAS to contradict
// itself across this list; the new code cannot, because none of these zones
// reaches the query any more.
//
// It asserts agreement rather than a specific side on purpose: which partition
// a half-day-old show belongs to depends on the venue's calendar and the hour,
// and pinning that would reintroduce exactly the clock dependence this test
// exists to remove.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_SamePartitionForEveryCallerZone() {
	artist := suite.createTestArtist("Sweep Artist")
	venue := suite.createVenueInZone("Sweep Room", "Asia/Tokyo")
	user := suite.createTestUser()

	show := suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().Add(-12*time.Hour))

	var firstZone string
	var wasUpcoming bool
	for i, callerZone := range everyCallerOffsetZone() {
		upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, callerZone, 10, "upcoming")
		suite.Require().NoError(err, "caller zone %q", callerZone)
		past, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, callerZone, 10, "past")
		suite.Require().NoError(err, "caller zone %q", callerZone)

		// Exactly one partition claims it, whichever one that is.
		suite.Require().Equal(int64(1), upcomingTotal+pastTotal, "caller zone %q: show must appear in exactly one partition", callerZone)
		suite.Require().Len(append(append([]*contracts.ArtistShowResponse{}, upcoming...), past...), 1, "caller zone %q", callerZone)

		isUpcoming := upcomingTotal == 1
		if i == 0 {
			firstZone, wasUpcoming = callerZone, isUpcoming
			suite.Require().Equal(show.ID, append(append([]*contracts.ArtistShowResponse{}, upcoming...), past...)[0].ID)
			continue
		}
		suite.Equal(wasUpcoming, isUpcoming,
			"caller zone %q disagrees with %q about the same show: the split is still being made in the CALLER's timezone",
			callerZone, firstZone)
	}
}

// A venue predating the timezone backfill still has to partition, not vanish:
// the lateral's COALESCE degrades a NULL zone to UTC rather than erroring or
// filtering the row out.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_NullVenueTimezoneFallsBackToUTC() {
	artist := suite.createTestArtist("No Zone Artist")
	venue := suite.createTestVenue("No Zone Room", "Phoenix", "AZ") // Timezone NULL
	suite.Require().Nil(venue.Timezone)
	user := suite.createTestUser()

	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), "UTC", 1, 12))
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), "UTC", -1, 12))

	_, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal)

	_, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "past")
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal)
}

// A blank or malformed stored zone is the other pre-backfill shape, and it must
// degrade the same way rather than take the whole query down.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_MalformedVenueTimezoneFallsBackToUTC() {
	artist := suite.createTestArtist("Bad Zone Artist")
	user := suite.createTestUser()

	for i, zone := range []string{"", "   ", "Not/AZone", "EST"} {
		venue := &catalogm.Venue{
			Name:     fmt.Sprintf("Bad Zone Room %d", i),
			City:     "Testville",
			State:    "AZ",
			Timezone: stringPtr(zone),
		}
		suite.Require().NoError(suite.db.Create(venue).Error)
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), "UTC", 1, 12))
	}

	_, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(4), upcomingTotal, "every malformed zone should degrade to UTC, not drop the show")
}

// A show with no venue row at all: the LEFT JOIN LATERAL must keep it, or
// venue-less shows would silently disappear from every artist page.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_VenuelessShowStillPartitions() {
	artist := suite.createTestArtist("Venueless Artist")
	user := suite.createTestUser()

	upcomingShow := &catalogm.Show{
		Title:       "Venueless Upcoming",
		EventDate:   venueLocalInstant(suite.T(), "UTC", 1, 12),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.Require().NoError(suite.db.Create(upcomingShow).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{ShowID: upcomingShow.ID, ArtistID: artist.ID, Position: 0}).Error)

	pastShow := &catalogm.Show{
		Title:       "Venueless Past",
		EventDate:   venueLocalInstant(suite.T(), "UTC", -1, 12),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.Require().NoError(suite.db.Create(pastShow).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{ShowID: pastShow.ID, ArtistID: artist.ID, Position: 0}).Error)

	upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal)
	suite.Require().Len(upcoming, 1)
	suite.Equal(upcomingShow.ID, upcoming[0].ID)

	past, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "past")
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
	venue := suite.createVenueInZone("Paging Room", zone)
	user := suite.createTestUser()

	// 3 venue-local past, 2 venue-local upcoming. Every past show is seeded at
	// 23:00 local, so each one's UTC date is the following day — under the old
	// UTC-anchored boundary some of them would count as upcoming.
	for i := 1; i <= 3; i++ {
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, -i, 23))
	}
	for i := 1; i <= 2; i++ {
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, venueLocalInstant(suite.T(), zone, i, 23))
	}

	upcoming, upcomingTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 1, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(2), upcomingTotal, "total counts the whole venue-local partition")
	suite.Require().Len(upcoming, 1, "limit pages the venue-local partition")

	past, pastTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 1, "past")
	suite.Require().NoError(err)
	suite.Equal(int64(3), pastTotal)
	suite.Require().Len(past, 1)

	// "all" keeps every row and is deliberately not venue-zone aware: there is
	// no boundary to draw.
	_, allTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 50, "all")
	suite.Require().NoError(err)
	suite.Equal(int64(5), allTotal)
}

// =============================================================================
// Venue show lists
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) createVenueInZone(name, zone string) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:     name,
		City:     "Testville",
		State:    "AZ",
		Verified: true,
		Timezone: stringPtr(zone),
	}
	suite.Require().NoError(suite.db.Create(venue).Error)
	return venue
}

func (suite *VenueServiceIntegrationTestSuite) createApprovedShowAt(venueID, userID uint, at time.Time) *catalogm.Show {
	show := &catalogm.Show{
		Title:       fmt.Sprintf("Show-%d", time.Now().UnixNano()),
		EventDate:   at,
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)
	return show
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_VenueLocalYesterdayIsPast_EvenWhenUTCDateIsToday() {
	const zone = "Pacific/Honolulu"
	venue := suite.createVenueInZone("Honolulu Room", zone)
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), zone, -1, 23)
	localAndUTCDatesDiffer(suite.T(), at, zone)
	suite.createApprovedShowAt(venue.ID, user.ID, at)

	past, pastTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", 10, "past")
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal)
	suite.Require().Len(past, 1)

	upcoming, upcomingTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingTotal)
	suite.Empty(upcoming)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_VenueLocalTodayIsUpcoming_EvenWhenUTCDateIsTomorrow() {
	const zone = "Pacific/Honolulu"
	venue := suite.createVenueInZone("Late Night Room", zone)
	user := suite.createTestUser()

	at := venueLocalInstant(suite.T(), zone, 0, 23)
	localAndUTCDatesDiffer(suite.T(), at, zone)
	suite.createApprovedShowAt(venue.ID, user.ID, at)

	upcoming, upcomingTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal)
	suite.Require().Len(upcoming, 1)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_PartitionIgnoresCallerTimezone() {
	const zone = "Asia/Tokyo"
	venue := suite.createVenueInZone("Tokyo Room", zone)
	user := suite.createTestUser()

	yesterday := suite.createApprovedShowAt(venue.ID, user.ID, venueLocalInstant(suite.T(), zone, -1, 12))
	tomorrow := suite.createApprovedShowAt(venue.ID, user.ID, venueLocalInstant(suite.T(), zone, 1, 12))

	for _, callerZone := range []string{"Pacific/Kiritimati", "Pacific/Midway", "UTC", "", "Not/AZone"} {
		upcoming, upcomingTotal, err := suite.venueService.GetShowsForVenue(venue.ID, callerZone, 10, "upcoming")
		suite.Require().NoError(err, "caller zone %q", callerZone)
		suite.Require().Len(upcoming, 1, "caller zone %q", callerZone)
		suite.Equal(tomorrow.ID, upcoming[0].ID, "caller zone %q", callerZone)
		suite.Equal(int64(1), upcomingTotal, "caller zone %q", callerZone)

		past, pastTotal, err := suite.venueService.GetShowsForVenue(venue.ID, callerZone, 10, "past")
		suite.Require().NoError(err, "caller zone %q", callerZone)
		suite.Require().Len(past, 1, "caller zone %q", callerZone)
		suite.Equal(yesterday.ID, past[0].ID, "caller zone %q", callerZone)
		suite.Equal(int64(1), pastTotal, "caller zone %q", callerZone)
	}
}

// Venue-side twin of TestGetShowsForArtist_SamePartitionForEveryCallerZone.
// See that test for why the half-day offset and the full offset sweep are what
// make this fail against the pre-PSY-1695 code at every hour.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_SamePartitionForEveryCallerZone() {
	venue := suite.createVenueInZone("Sweep Room", "Asia/Tokyo")
	user := suite.createTestUser()

	show := suite.createApprovedShowAt(venue.ID, user.ID, time.Now().Add(-12*time.Hour))

	var firstZone string
	var wasUpcoming bool
	for i, callerZone := range everyCallerOffsetZone() {
		upcoming, upcomingTotal, err := suite.venueService.GetShowsForVenue(venue.ID, callerZone, 10, "upcoming")
		suite.Require().NoError(err, "caller zone %q", callerZone)
		past, pastTotal, err := suite.venueService.GetShowsForVenue(venue.ID, callerZone, 10, "past")
		suite.Require().NoError(err, "caller zone %q", callerZone)

		suite.Require().Equal(int64(1), upcomingTotal+pastTotal, "caller zone %q: show must appear in exactly one partition", callerZone)
		combined := append(append([]*contracts.VenueShowResponse{}, upcoming...), past...)
		suite.Require().Len(combined, 1, "caller zone %q", callerZone)

		isUpcoming := upcomingTotal == 1
		if i == 0 {
			firstZone, wasUpcoming = callerZone, isUpcoming
			suite.Require().Equal(show.ID, combined[0].ID)
			continue
		}
		suite.Equal(wasUpcoming, isUpcoming,
			"caller zone %q disagrees with %q about the same show: the split is still being made in the CALLER's timezone",
			callerZone, firstZone)
	}
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_NullVenueTimezoneFallsBackToUTC() {
	venue := suite.createTestVenue("No Zone Room", "Phoenix", "AZ", true) // Timezone NULL
	suite.Require().Nil(venue.Timezone)
	user := suite.createTestUser()

	suite.createApprovedShowAt(venue.ID, user.ID, venueLocalInstant(suite.T(), "UTC", 1, 12))
	suite.createApprovedShowAt(venue.ID, user.ID, venueLocalInstant(suite.T(), "UTC", -1, 12))

	_, upcomingTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal)

	_, pastTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", 10, "past")
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal)
}

// Pins the documented "first venue (lowest venue_id) decides the zone" rule for
// the rare show booked at two venues. The point is that ONE show has ONE
// venue-local date on every surface, so it cannot read upcoming on the venue
// page and past on the artist page. The cost is visible here: queried through
// the Tokyo venue, the show is judged on Honolulu's calendar.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_MultiVenueShowUsesFirstVenueZone() {
	// Created first, so it holds the lower venue_id and owns the zone.
	honolulu := suite.createVenueInZone("First Room", "Pacific/Honolulu")
	tokyo := suite.createVenueInZone("Second Room", "Asia/Tokyo")
	suite.Require().Less(honolulu.ID, tokyo.ID)
	user := suite.createTestUser()

	// 23:00 yesterday in Honolulu is 18:00 TODAY in Tokyo: past on the first
	// venue's calendar, still upcoming on the second's.
	at := venueLocalInstant(suite.T(), "Pacific/Honolulu", -1, 23)
	show := suite.createApprovedShowAt(honolulu.ID, user.ID, at)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: tokyo.ID}).Error)

	upcoming, upcomingTotal, err := suite.venueService.GetShowsForVenue(tokyo.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingTotal, "first venue's zone decides, not the queried venue's")
	suite.Empty(upcoming)

	past, pastTotal, err := suite.venueService.GetShowsForVenue(tokyo.ID, "UTC", 10, "past")
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal)
	suite.Require().Len(past, 1)
	suite.Equal(show.ID, past[0].ID)
}
