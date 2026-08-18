package catalog

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Offset paging, venue-local year filtering and the year histogram for a
// venue's show list (PSY-1750).
//
// Every fixture here is anchored on a FIXED past year rather than an offset from
// now, which is the opposite of the upcoming/past partitioning tests next door.
// That is deliberate: those tests are about a boundary that moves with the
// clock, these are about calendar buckets that do not, and a "3 years ago"
// fixture would silently change which bucket it lands in as the year turns.

// seedShowsForVenue creates approved shows at the given instants and returns
// their ids in creation order. It is the plural of createApprovedShowAt, which
// already owns the fixture's shape.
func (suite *VenueServiceIntegrationTestSuite) seedShowsForVenue(venueID, userID uint, at ...time.Time) []uint {
	ids := make([]uint, 0, len(at))
	for _, when := range at {
		ids = append(ids, suite.createApprovedShowAt(venueID, userID, when).ID)
	}
	return ids
}

func showIDsOf(shows []*contracts.VenueShowResponse) []uint {
	ids := make([]uint, 0, len(shows))
	for _, s := range shows {
		ids = append(ids, s.ID)
	}
	return ids
}

// fixedUTC builds an instant with no dependence on the current date, so a year
// assertion means the same thing in every calendar year the suite runs in.
func fixedUTC(year int, month time.Month, day, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, time.UTC)
}

// =============================================================================
// Offset paging
// =============================================================================

// Paging must partition the result set: every show reachable exactly once, and
// the union of the pages equal to `total`. A ragged boundary here is invisible
// to a caller: it just looks like the venue has fewer shows than it does.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_OffsetPagesPartitionTheResultSet() {
	venue := suite.createTestVenue("Paging Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.seedShowsForVenue(venue.ID, user.ID,
		fixedUTC(2019, time.March, 1, 20),
		fixedUTC(2019, time.March, 2, 20),
		fixedUTC(2019, time.March, 3, 20),
		fixedUTC(2019, time.March, 4, 20),
		fixedUTC(2019, time.March, 5, 20),
	)

	seen := make([]uint, 0, 5)
	for offset := 0; offset < 6; offset += 2 {
		page, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
			TimeFilter: "all", Limit: 2, Offset: offset,
		})
		suite.Require().NoError(err)
		suite.Equal(int64(5), total, "offset %d: total must not move with the page", offset)
		seen = append(seen, showIDsOf(page)...)
	}

	suite.Len(seen, 5, "pages must cover the whole result set exactly once")
	unique := map[uint]bool{}
	for _, id := range seen {
		suite.False(unique[id], "show %d appeared on two pages", id)
		unique[id] = true
	}
}

// The last page is the one that gets a boundary wrong, so assert its exact
// contents rather than only its size.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_LastPageIsShortNotWrapped() {
	venue := suite.createTestVenue("Short Page Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	ids := suite.seedShowsForVenue(venue.ID, user.ID,
		fixedUTC(2019, time.April, 1, 20),
		fixedUTC(2019, time.April, 2, 20),
		fixedUTC(2019, time.April, 3, 20),
	)

	page, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: 2, Offset: 2,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Equal([]uint{ids[2]}, showIDsOf(page))
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_OffsetPastTotalIsEmptyAndTotalStands() {
	venue := suite.createTestVenue("Overrun Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.seedShowsForVenue(venue.ID, user.ID, fixedUTC(2019, time.May, 1, 20))

	page, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: 20, Offset: 500,
	})
	suite.Require().NoError(err)
	suite.Empty(page, "an offset past the end returns no rows")
	suite.Equal(int64(1), total, "...but still reports what the caller overran")
}

// Negative page bounds are caller bugs, and GORM reads each as a different
// instruction: a negative offset becomes OFFSET -1, which Postgres rejects, and
// a negative limit CANCELS the limit, which succeeds while returning the venue's
// whole history. The second is the dangerous one because it looks like it worked.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_NegativePageBoundsClamp() {
	venue := suite.createTestVenue("Negative Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	ids := suite.seedShowsForVenue(venue.ID, user.ID,
		fixedUTC(2019, time.June, 1, 20),
		fixedUTC(2019, time.June, 2, 20),
	)

	page, _, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: 1, Offset: -5,
	})
	suite.Require().NoError(err)
	suite.Equal([]uint{ids[0]}, showIDsOf(page), "a negative offset must read as the first page")

	page, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: -1,
	})
	suite.Require().NoError(err)
	suite.Empty(page, "a negative limit must not fall through to an unbounded read")
	suite.Equal(int64(2), total, "...and still report the real total, like limit 0")
}

// The whole reason the order carries `shows.id ASC`: shows sharing an event_date
// are exactly where an unstable sort duplicates one row across pages and drops
// another entirely. Paging one at a time maximises the number of boundaries that
// land inside the tied group.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_PagesStayDisjointAcrossATiedEventDate() {
	venue := suite.createTestVenue("Tiebreak Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	sameNight := fixedUTC(2019, time.July, 4, 20)
	suite.seedShowsForVenue(venue.ID, user.ID, sameNight, sameNight, sameNight, sameNight)

	seen := map[uint]bool{}
	for offset := 0; offset < 4; offset++ {
		page, _, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
			TimeFilter: "all", Limit: 1, Offset: offset,
		})
		suite.Require().NoError(err)
		suite.Require().Len(page, 1, "offset %d", offset)
		suite.False(seen[page[0].ID], "show %d repeated at offset %d", page[0].ID, offset)
		seen[page[0].ID] = true
	}
	suite.Len(seen, 4)
}

// =============================================================================
// Venue-local year filter
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_YearNarrowsPageAndTotal() {
	venue := suite.createTestVenue("Archive Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.seedShowsForVenue(venue.ID, user.ID,
		fixedUTC(2018, time.August, 1, 20),
		fixedUTC(2019, time.August, 1, 20),
		fixedUTC(2019, time.September, 1, 20),
		fixedUTC(2020, time.August, 1, 20),
	)

	page, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: 50, Year: 2019,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(2), total, "total must reflect the year filter, not the venue's whole history")
	suite.Len(page, 2)
	for _, show := range page {
		suite.Equal(2019, show.EventDate.UTC().Year())
	}

	_, allTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: 50, Year: 0,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(4), allTotal, "year 0 means every year")
}

// The load-bearing one. A New Year's Eve show in Honolulu is already the NEXT
// year in UTC, and a New Year's Day show at UTC+14 is still the PREVIOUS one, so
// a UTC-bucketed filter files both under the wrong year. Both signs are covered
// because the coarse UTC bounds are widened in both directions and a one-sided
// margin would pass half of this.
//
// It also proves the timezone lateral is joined for timeFilter "all", which
// needs no date condition and would otherwise skip it. Without the join these
// queries do not return the wrong rows, they fail outright.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_YearIsVenueLocalNotUTC() {
	user := suite.createTestUser()

	for _, tc := range []struct {
		zone      string
		state     string
		local     time.Time
		localYear int
		utcYear   int
	}{
		// 23:00 on 2019-12-31 in Honolulu (UTC-10) is 09:00 on 2020-01-01 UTC.
		{"Pacific/Honolulu", "HI", time.Date(2019, time.December, 31, 23, 0, 0, 0, time.FixedZone("HST", -10*3600)), 2019, 2020},
		// The extreme western offset, one further out than Honolulu.
		{"Etc/GMT+12", "AZ", time.Date(2019, time.December, 31, 23, 0, 0, 0, time.FixedZone("W12", -12*3600)), 2019, 2020},
		// 00:30 on 2020-01-01 at UTC+14 is 10:30 on 2019-12-31 UTC.
		{"Etc/GMT-14", "AZ", time.Date(2020, time.January, 1, 0, 30, 0, 0, time.FixedZone("E14", 14*3600)), 2020, 2019},
	} {
		venue := newVenueInZone(suite.T(), suite.db, "New Year Room "+tc.zone, tc.state, tc.zone, true)
		suite.Require().Equal(tc.utcYear, tc.local.UTC().Year(),
			"zone %s: fixture no longer straddles the year boundary", tc.zone)
		suite.seedShowsForVenue(venue.ID, user.ID, tc.local)

		_, localYearTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
			TimeFilter: "all", Limit: 50, Year: tc.localYear,
		})
		suite.Require().NoError(err, "zone %s", tc.zone)
		suite.Equal(int64(1), localYearTotal,
			"zone %s: the show belongs to the venue's %d", tc.zone, tc.localYear)

		_, utcYearTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
			TimeFilter: "all", Limit: 50, Year: tc.utcYear,
		})
		suite.Require().NoError(err, "zone %s", tc.zone)
		suite.Equal(int64(0), utcYearTotal,
			"zone %s: UTC's calendar must not file the show under %d", tc.zone, tc.utcYear)
	}
}

// Year and time filter compose rather than override: a past-only view of one
// year must not resurrect that year's upcoming dates.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_YearComposesWithTimeFilter() {
	venue := suite.createTestVenue("Composed Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	upcomingAt := time.Now().UTC().AddDate(0, 0, 30)
	suite.seedShowsForVenue(venue.ID, user.ID, fixedUTC(2019, time.October, 1, 20), upcomingAt)

	_, pastInOldYear, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "past", Limit: 50, Year: 2019,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastInOldYear)

	_, upcomingInOldYear, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "upcoming", Limit: 50, Year: 2019,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingInOldYear, "a 2019 date cannot be upcoming")

	// The upcoming show's own year, taken in the VENUE's zone rather than
	// assuming it matches UTC's: run this in the last hours of December 31 UTC
	// and a Phoenix venue is still in the previous year.
	phoenix, err := time.LoadLocation("America/Phoenix")
	suite.Require().NoError(err)
	upcomingYear := upcomingAt.In(phoenix).Year()

	_, upcomingInItsYear, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "upcoming", Limit: 50, Year: upcomingYear,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingInItsYear, "year %d", upcomingYear)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_YearWithNoShowsIsEmpty() {
	venue := suite.createTestVenue("Gap Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.seedShowsForVenue(venue.ID, user.ID, fixedUTC(2019, time.November, 1, 20))

	page, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: 50, Year: 1999,
	})
	suite.Require().NoError(err)
	suite.Empty(page)
	suite.Equal(int64(0), total)
}

// =============================================================================
// Projection
// =============================================================================

// The venue page links every row by slug and strikes cancelled dates through, so
// these three fields have to survive the projection. Before PSY-1750 the slug
// was absent and every link fell back to /shows/{id}.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_ProjectionCarriesSlugAndStatusFlags() {
	venue := suite.createTestVenue("Projection Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	show := &catalogm.Show{
		Title:       "Flagged Show",
		Slug:        stringPtr("flagged-show-projection-room-2019"),
		EventDate:   fixedUTC(2019, time.December, 1, 20),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
		IsCancelled: true,
		IsSoldOut:   true,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)

	page, _, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: 10,
	})
	suite.Require().NoError(err)
	suite.Require().Len(page, 1)
	suite.Equal("flagged-show-projection-room-2019", page[0].Slug)
	suite.True(page[0].IsCancelled)
	suite.True(page[0].IsSoldOut)
}

// A slugless show must project an empty string rather than panic on the
// dereference: the column is nullable and older rows predate slugging.
func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_NullSlugProjectsEmptyString() {
	venue := suite.createTestVenue("Slugless Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.seedShowsForVenue(venue.ID, user.ID, fixedUTC(2019, time.December, 2, 20))

	page, _, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: 10,
	})
	suite.Require().NoError(err)
	suite.Require().Len(page, 1)
	suite.Empty(page[0].Slug)
	suite.False(page[0].IsCancelled)
	suite.False(page[0].IsSoldOut)
}

// =============================================================================
// Year histogram
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueShowYears_BucketsNewestFirstSkippingEmptyYears() {
	venue := suite.createTestVenue("Histogram Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	// 2016 and 2018 populated, 2017 deliberately empty.
	suite.seedShowsForVenue(venue.ID, user.ID,
		fixedUTC(2016, time.February, 1, 20),
		fixedUTC(2018, time.February, 1, 20),
		fixedUTC(2018, time.March, 1, 20),
		fixedUTC(2018, time.April, 1, 20),
	)

	years, err := suite.venueService.GetVenueShowYears(venue.ID, "all")
	suite.Require().NoError(err)
	suite.Equal([]contracts.VenueShowYearCount{
		{Year: 2018, Count: 3},
		{Year: 2016, Count: 1},
	}, years, "newest first, and no zero-count 2017 bucket")
}

// The histogram buckets on the same venue-local calendar the list filters on. If
// the two disagreed, picking the year the histogram offered would return an
// empty list.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenueShowYears_BucketsByVenueLocalYear() {
	const zone = "Pacific/Honolulu"
	venue := newVenueInZone(suite.T(), suite.db, "Histogram NYE Room", "HI", zone, true)
	user := suite.createTestUser()

	loc, err := time.LoadLocation(zone)
	suite.Require().NoError(err)
	newYearsEve := time.Date(2019, time.December, 31, 23, 0, 0, 0, loc)
	suite.Require().Equal(2020, newYearsEve.UTC().Year(), "fixture no longer straddles the year boundary")
	suite.seedShowsForVenue(venue.ID, user.ID, newYearsEve)

	years, err := suite.venueService.GetVenueShowYears(venue.ID, "all")
	suite.Require().NoError(err)
	suite.Equal([]contracts.VenueShowYearCount{{Year: 2019, Count: 1}}, years)

	// ...and the year it offers actually returns that show.
	_, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", contracts.VenueShowsQuery{
		TimeFilter: "all", Limit: 10, Year: years[0].Year,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total, "the histogram must not offer a year the list cannot show")
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueShowYears_RespectsTimeFilter() {
	venue := suite.createTestVenue("Filtered Histogram Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.seedShowsForVenue(venue.ID, user.ID,
		fixedUTC(2019, time.February, 1, 20),
		time.Now().UTC().AddDate(0, 0, 45),
	)

	past, err := suite.venueService.GetVenueShowYears(venue.ID, "past")
	suite.Require().NoError(err)
	suite.Equal([]contracts.VenueShowYearCount{{Year: 2019, Count: 1}}, past)

	upcoming, err := suite.venueService.GetVenueShowYears(venue.ID, "upcoming")
	suite.Require().NoError(err)
	suite.Require().Len(upcoming, 1)
	suite.Equal(int64(1), upcoming[0].Count)
	suite.NotEqual(2019, upcoming[0].Year)

	all, err := suite.venueService.GetVenueShowYears(venue.ID, "all")
	suite.Require().NoError(err)
	suite.Len(all, 2)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueShowYears_EmptyVenueReturnsEmptySlice() {
	venue := suite.createTestVenue("Silent Room", "Phoenix", "AZ", true)

	years, err := suite.venueService.GetVenueShowYears(venue.ID, "all")
	suite.Require().NoError(err)
	suite.NotNil(years, "an empty histogram must serialize as [] rather than null")
	suite.Empty(years)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueShowYears_VenueNotFound() {
	_, err := suite.venueService.GetVenueShowYears(99999, "all")
	suite.Require().Error(err)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}

// The histogram is scoped to ONE venue. A count that leaked a neighbour's shows
// would offer years this venue never played.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenueShowYears_ScopedToOneVenue() {
	venue := suite.createTestVenue("Scoped Room", "Phoenix", "AZ", true)
	other := suite.createTestVenue("Neighbour Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.seedShowsForVenue(venue.ID, user.ID, fixedUTC(2015, time.May, 1, 20))
	suite.seedShowsForVenue(other.ID, user.ID, fixedUTC(2014, time.May, 1, 20))

	years, err := suite.venueService.GetVenueShowYears(venue.ID, "all")
	suite.Require().NoError(err)
	suite.Equal([]contracts.VenueShowYearCount{{Year: 2015, Count: 1}}, years)
}

// =============================================================================
// Year-archive existence probe (PSY-1770)
// =============================================================================
//
// The probe answers the SAME question the past histogram does, one bit at a
// time, and these tests are written against that equivalence rather than against
// the SQL: the only reason it may be substituted for the histogram in the
// proxy's existence branch is that a year it says yes to is a year the archive
// has rows for.

func (suite *VenueServiceIntegrationTestSuite) TestHasPastShowsInYear_AgreesWithThePastHistogram() {
	venue := suite.createTestVenue("Probe Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.seedShowsForVenue(venue.ID, user.ID,
		fixedUTC(2016, time.February, 1, 20),
		fixedUTC(2018, time.February, 1, 20),
	)

	years, err := suite.venueService.GetVenueShowYears(venue.ID, "past")
	suite.Require().NoError(err)
	populated := map[int]bool{}
	for _, entry := range years {
		populated[entry.Year] = true
	}
	suite.Require().True(populated[2016] && populated[2018], "fixture no longer seeds both years")

	// 2017 sits BETWEEN two populated years, which is the case a range check
	// would get wrong and a per-year probe must not.
	for _, year := range []int{2015, 2016, 2017, 2018, 2019} {
		exists, err := suite.venueService.HasPastShowsInYear(venue.ID, year)
		suite.Require().NoError(err, "year %d", year)
		suite.Equal(populated[year], exists,
			"year %d: probe and histogram must agree on which years are documents", year)
	}
}

// Past-only by construction: an upcoming date is not an archive, and the page
// 404s that year. A probe that answered yes would turn every one of those URLs
// into a 200 carrying a not-found body.
func (suite *VenueServiceIntegrationTestSuite) TestHasPastShowsInYear_IgnoresUpcomingShows() {
	venue := suite.createTestVenue("Upcoming Probe Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	upcomingAt := time.Now().UTC().AddDate(0, 0, 45)
	suite.seedShowsForVenue(venue.ID, user.ID, upcomingAt)

	phoenix, err := time.LoadLocation("America/Phoenix")
	suite.Require().NoError(err)
	upcomingYear := upcomingAt.In(phoenix).Year()

	exists, err := suite.venueService.HasPastShowsInYear(venue.ID, upcomingYear)
	suite.Require().NoError(err)
	suite.False(exists, "year %d holds only an upcoming show, so it is not an archive", upcomingYear)
}

// The year is the VENUE's, not UTC's — the same rule the list and the histogram
// apply. A probe on UTC's calendar would 404 an archive the page renders.
func (suite *VenueServiceIntegrationTestSuite) TestHasPastShowsInYear_UsesTheVenueLocalYear() {
	user := suite.createTestUser()
	// 23:00 on 2019-12-31 in Honolulu (UTC-10) is 09:00 on 2020-01-01 UTC.
	venue := newVenueInZone(suite.T(), suite.db, "Probe New Year Room", "HI", "Pacific/Honolulu", true)
	local := time.Date(2019, time.December, 31, 23, 0, 0, 0, time.FixedZone("HST", -10*3600))
	suite.Require().Equal(2020, local.UTC().Year(), "fixture no longer straddles the year boundary")
	suite.seedShowsForVenue(venue.ID, user.ID, local)

	localYear, err := suite.venueService.HasPastShowsInYear(venue.ID, 2019)
	suite.Require().NoError(err)
	suite.True(localYear, "the show belongs to the venue's 2019")

	utcYear, err := suite.venueService.HasPastShowsInYear(venue.ID, 2020)
	suite.Require().NoError(err)
	suite.False(utcYear, "UTC's calendar must not file the show under 2020")
}

// Scoped to ONE venue. A leak here would 200 a neighbour's year on this venue's
// archive, which then renders empty.
func (suite *VenueServiceIntegrationTestSuite) TestHasPastShowsInYear_ScopedToOneVenue() {
	venue := suite.createTestVenue("Probe Scoped Room", "Phoenix", "AZ", true)
	other := suite.createTestVenue("Probe Neighbour Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.seedShowsForVenue(other.ID, user.ID, fixedUTC(2014, time.May, 1, 20))

	exists, err := suite.venueService.HasPastShowsInYear(venue.ID, 2014)
	suite.Require().NoError(err)
	suite.False(exists)
}

// An unknown venue is "no", not an error, and the distinction matters at the
// proxy: an error there fails OPEN and soft-404s the page, while "no" is the
// 404 the caller asked for. The handler's slug resolution already 404s the shape
// a real caller sends; this is the numeric-id path.
func (suite *VenueServiceIntegrationTestSuite) TestHasPastShowsInYear_UnknownVenueIsFalse() {
	exists, err := suite.venueService.HasPastShowsInYear(99999, 2019)
	suite.Require().NoError(err)
	suite.False(exists)
}
