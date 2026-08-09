package catalog

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Offset paging, venue-local year filtering and the year histogram for an
// artist's show list (PSY-1751), the artist half of the contract PSY-1750 built
// for venues.
//
// Every fixture here is anchored on a FIXED past year rather than an offset from
// now, which is the opposite of the upcoming/past partitioning tests next door.
// That is deliberate: those tests are about a boundary that moves with the
// clock, these are about calendar buckets that do not, and a "3 years ago"
// fixture would silently change which bucket it lands in as the year turns.
// fixedUTC is shared with the venue suite for the same reason.

// seedShowsForArtist creates approved shows at the given instants, all at one
// venue, and returns their ids in creation order.
func (suite *ArtistServiceIntegrationTestSuite) seedShowsForArtist(artistID, venueID, userID uint, at ...time.Time) []uint {
	ids := make([]uint, 0, len(at))
	for _, when := range at {
		ids = append(ids, suite.createApprovedShowWithArtist(artistID, venueID, userID, when).ID)
	}
	return ids
}

func artistShowIDsOf(shows []*contracts.ArtistShowResponse) []uint {
	ids := make([]uint, 0, len(shows))
	for _, s := range shows {
		ids = append(ids, s.ID)
	}
	return ids
}

// seedFlaggedShow creates an approved show that is slugged, cancelled AND sold
// out, so a projection assertion fails on a field that was dropped rather than
// on one that merely happens to be false.
func (suite *ArtistServiceIntegrationTestSuite) seedFlaggedShow(artistID, venueID, userID uint, slug string, at time.Time) *catalogm.Show {
	show := &catalogm.Show{
		Title:       slug,
		Slug:        stringPtr(slug),
		EventDate:   at,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
		IsCancelled: true,
		IsSoldOut:   true,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artistID, Position: 0}).Error)
	return show
}

// =============================================================================
// Offset paging
// =============================================================================

// Paging must partition the result set: every show reachable exactly once, and
// the union of the pages equal to `total`. A ragged boundary here is invisible
// to a caller: it just looks like the artist played fewer shows than they did.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_OffsetPagesPartitionTheResultSet() {
	artist := suite.createTestArtist("Paging Artist Offsets")
	venue := suite.createTestVenue("Paging Offsets Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID,
		fixedUTC(2019, time.March, 1, 20),
		fixedUTC(2019, time.March, 2, 20),
		fixedUTC(2019, time.March, 3, 20),
		fixedUTC(2019, time.March, 4, 20),
		fixedUTC(2019, time.March, 5, 20),
	)

	seen := make([]uint, 0, 5)
	for offset := 0; offset < 6; offset += 2 {
		page, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
			TimeFilter: "all", Limit: 2, Offset: offset,
		})
		suite.Require().NoError(err)
		suite.Equal(int64(5), total, "offset %d: total must not move with the page", offset)
		seen = append(seen, artistShowIDsOf(page)...)
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
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_LastPageIsShortNotWrapped() {
	artist := suite.createTestArtist("Short Page Artist")
	venue := suite.createTestVenue("Short Page Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	ids := suite.seedShowsForArtist(artist.ID, venue.ID, user.ID,
		fixedUTC(2019, time.April, 1, 20),
		fixedUTC(2019, time.April, 2, 20),
		fixedUTC(2019, time.April, 3, 20),
	)

	page, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 2, Offset: 2,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Equal([]uint{ids[2]}, artistShowIDsOf(page))
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_OffsetPastTotalIsEmptyAndTotalStands() {
	artist := suite.createTestArtist("Overrun Artist")
	venue := suite.createTestVenue("Overrun Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID, fixedUTC(2019, time.May, 1, 20))

	page, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 20, Offset: 500,
	})
	suite.Require().NoError(err)
	suite.Empty(page, "an offset past the end returns no rows")
	suite.Equal(int64(1), total, "...but still reports what the caller overran")
}

// Negative page bounds are caller bugs, and GORM reads each as a different
// instruction: a negative offset becomes OFFSET -1, which Postgres rejects, and
// a negative limit CANCELS the limit, which succeeds while returning the
// artist's whole history. The second is the dangerous one because it looks like
// it worked.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_NegativePageBoundsClamp() {
	artist := suite.createTestArtist("Negative Artist")
	venue := suite.createTestVenue("Negative Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	ids := suite.seedShowsForArtist(artist.ID, venue.ID, user.ID,
		fixedUTC(2019, time.June, 1, 20),
		fixedUTC(2019, time.June, 2, 20),
	)

	page, _, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 1, Offset: -5,
	})
	suite.Require().NoError(err)
	suite.Equal([]uint{ids[0]}, artistShowIDsOf(page), "a negative offset must read as the first page")

	page, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: -1,
	})
	suite.Require().NoError(err)
	suite.Empty(page, "a negative limit must not fall through to an unbounded read")
	suite.Equal(int64(2), total, "...and still report the real total, like limit 0")
}

// The whole reason the order carries `shows.id ASC`: shows sharing an event_date
// are exactly where an unstable sort duplicates one row across pages and drops
// another entirely. An artist double-booked on one night (an early set and a
// late one) is the ordinary way that happens here. Paging one at a time
// maximises the number of boundaries that land inside the tied group.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_PagesStayDisjointAcrossATiedEventDate() {
	artist := suite.createTestArtist("Tiebreak Artist")
	venue := suite.createTestVenue("Tiebreak Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	sameNight := fixedUTC(2019, time.July, 4, 20)
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID, sameNight, sameNight, sameNight, sameNight)

	seen := map[uint]bool{}
	for offset := 0; offset < 4; offset++ {
		page, _, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
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

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_YearNarrowsPageAndTotal() {
	artist := suite.createTestArtist("Archive Artist")
	venue := suite.createTestVenue("Archive Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID,
		fixedUTC(2018, time.August, 1, 20),
		fixedUTC(2019, time.August, 1, 20),
		fixedUTC(2019, time.September, 1, 20),
		fixedUTC(2020, time.August, 1, 20),
	)

	page, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 50, Year: 2019,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(2), total, "total must reflect the year filter, not the artist's whole history")
	suite.Len(page, 2)
	for _, show := range page {
		suite.Equal(2019, show.EventDate.UTC().Year())
	}

	_, allTotal, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 50, Year: 0,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(4), allTotal, "year 0 means every year")
}

// The load-bearing one, and the case the artist page has that the venue page
// does not: ONE artist, two venues, opposite sides of the date line.
//
// The New Year's Eve set in Honolulu (UTC-10) is already 2020 in UTC, and the
// New Year's Day set at UTC+14 is still 2019 in UTC. Bucketing by UTC would file
// each show under the OTHER one's year, so this fixture fails in both directions
// at once rather than merely returning a wrong count. It also proves the
// timezone lateral is joined for timeFilter "all", which needs no date condition
// and would otherwise skip it: without the join these queries do not return the
// wrong rows, they fail outright.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_YearIsVenueLocalNotUTC() {
	artist := suite.createTestArtist("Touring Artist")
	user := suite.createTestUser()

	honolulu := newVenueInZone(suite.T(), suite.db, "NYE Honolulu Room", "HI", "Pacific/Honolulu", false)
	dateLine := newVenueInZone(suite.T(), suite.db, "NYD Date Line Room", "AZ", "Etc/GMT-14", false)

	// 23:00 on 2019-12-31 in Honolulu is 09:00 on 2020-01-01 UTC.
	nyeLocal := time.Date(2019, time.December, 31, 23, 0, 0, 0, time.FixedZone("HST", -10*3600))
	// 00:30 on 2020-01-01 at UTC+14 is 10:30 on 2019-12-31 UTC.
	nydLocal := time.Date(2020, time.January, 1, 0, 30, 0, 0, time.FixedZone("E14", 14*3600))
	suite.Require().Equal(2020, nyeLocal.UTC().Year(), "fixture no longer straddles the year boundary")
	suite.Require().Equal(2019, nydLocal.UTC().Year(), "fixture no longer straddles the year boundary")

	nyeShow := suite.createApprovedShowWithArtist(artist.ID, honolulu.ID, user.ID, nyeLocal)
	nydShow := suite.createApprovedShowWithArtist(artist.ID, dateLine.ID, user.ID, nydLocal)

	page2019, total2019, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 50, Year: 2019,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total2019)
	suite.Equal([]uint{nyeShow.ID}, artistShowIDsOf(page2019),
		"the Honolulu set belongs to ITS venue's 2019, not UTC's 2020")

	page2020, total2020, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 50, Year: 2020,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total2020)
	suite.Equal([]uint{nydShow.ID}, artistShowIDsOf(page2020),
		"the date-line set belongs to ITS venue's 2020, not UTC's 2019")
}

// Year and time filter compose rather than override: a past-only view of one
// year must not resurrect that year's upcoming dates.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_YearComposesWithTimeFilter() {
	artist := suite.createTestArtist("Composed Artist")
	venue := suite.createTestVenue("Composed Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	upcomingAt := time.Now().UTC().AddDate(0, 0, 30)
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID, fixedUTC(2019, time.October, 1, 20), upcomingAt)

	_, pastInOldYear, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "past", Limit: 50, Year: 2019,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastInOldYear)

	_, upcomingInOldYear, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "upcoming", Limit: 50, Year: 2019,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(0), upcomingInOldYear, "a 2019 date cannot be upcoming")

	// The upcoming show's own year, taken in the VENUE's zone rather than
	// assuming it matches UTC's: this venue's timezone column is NULL, so it
	// resolves through the US state map to America/Phoenix, and in the last
	// hours of December 31 UTC that is still the previous year.
	phoenix, err := time.LoadLocation("America/Phoenix")
	suite.Require().NoError(err)
	upcomingYear := upcomingAt.In(phoenix).Year()

	_, upcomingInItsYear, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "upcoming", Limit: 50, Year: upcomingYear,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingInItsYear, "year %d", upcomingYear)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_YearWithNoShowsIsEmpty() {
	artist := suite.createTestArtist("Gap Artist")
	venue := suite.createTestVenue("Gap Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID, fixedUTC(2019, time.November, 1, 20))

	page, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 50, Year: 1999,
	})
	suite.Require().NoError(err)
	suite.Empty(page)
	suite.Equal(int64(0), total)
}

// =============================================================================
// Projection
// =============================================================================

// The artist page links every row by slug and strikes cancelled dates through,
// so these three fields have to survive the projection. Before PSY-1751 the
// artist response carried none of them and every link fell back to /shows/{id}.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_ProjectionCarriesSlugAndStatusFlags() {
	artist := suite.createTestArtist("Projection Artist")
	venue := suite.createTestVenue("Projection Room", "Phoenix", "AZ")
	user := suite.createTestUser()

	suite.seedFlaggedShow(artist.ID, venue.ID, user.ID,
		"flagged-show-projection-artist-2019", fixedUTC(2019, time.December, 1, 20))

	page, _, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 10,
	})
	suite.Require().NoError(err)
	suite.Require().Len(page, 1)
	suite.Equal("flagged-show-projection-artist-2019", page[0].Slug)
	suite.True(page[0].IsCancelled)
	suite.True(page[0].IsSoldOut)
}

// A slugless show must project an empty string rather than panic on the
// dereference: the column is nullable and older rows predate slugging.
func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_NullSlugProjectsEmptyString() {
	artist := suite.createTestArtist("Slugless Artist Shows")
	venue := suite.createTestVenue("Slugless Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID, fixedUTC(2019, time.December, 2, 20))

	page, _, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 10,
	})
	suite.Require().NoError(err)
	suite.Require().Len(page, 1)
	suite.Empty(page[0].Slug)
	suite.False(page[0].IsCancelled)
	suite.False(page[0].IsSoldOut)
}

// The graph card builds the SAME response type from its own query, so widening
// the projection without widening that producer would leave a cancelled show
// rendering as a live one on the card while the list below it strikes it
// through. Zero values are not "unset" for a bool.
func (suite *ArtistServiceIntegrationTestSuite) TestGetNextShowForArtist_CarriesSlugAndStatusFlags() {
	artist := suite.createTestArtist("Next Show Projection Artist")
	venue := suite.createTestVenue("Next Show Projection Room", "Phoenix", "AZ")
	user := suite.createTestUser()

	suite.seedFlaggedShow(artist.ID, venue.ID, user.ID,
		"flagged-next-show-projection", time.Now().UTC().AddDate(0, 0, 21))

	next, err := suite.artistService.GetNextShowForArtist(artist.ID, "UTC")
	suite.Require().NoError(err)
	suite.Require().NotNil(next)
	suite.Equal("flagged-next-show-projection", next.Slug)
	suite.True(next.IsCancelled)
	suite.True(next.IsSoldOut)
}

// =============================================================================
// Year histogram
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistShowYears_BucketsNewestFirstSkippingEmptyYears() {
	artist := suite.createTestArtist("Histogram Artist")
	venue := suite.createTestVenue("Histogram Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	// 2016 and 2018 populated, 2017 deliberately empty.
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID,
		fixedUTC(2016, time.February, 1, 20),
		fixedUTC(2018, time.February, 1, 20),
		fixedUTC(2018, time.March, 1, 20),
		fixedUTC(2018, time.April, 1, 20),
	)

	years, err := suite.artistService.GetArtistShowYears(artist.ID, "all")
	suite.Require().NoError(err)
	suite.Equal([]contracts.ArtistShowYearCount{
		{Year: 2018, Count: 3},
		{Year: 2016, Count: 1},
	}, years, "newest first, and no zero-count 2017 bucket")
}

// The histogram buckets on the same venue-local calendar the list filters on. If
// the two disagreed, picking the year the histogram offered would return an
// empty list.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistShowYears_BucketsByVenueLocalYear() {
	const zone = "Pacific/Honolulu"
	artist := suite.createTestArtist("Histogram NYE Artist")
	venue := newVenueInZone(suite.T(), suite.db, "Histogram NYE Room", "HI", zone, false)
	user := suite.createTestUser()

	loc, err := time.LoadLocation(zone)
	suite.Require().NoError(err)
	newYearsEve := time.Date(2019, time.December, 31, 23, 0, 0, 0, loc)
	suite.Require().Equal(2020, newYearsEve.UTC().Year(), "fixture no longer straddles the year boundary")
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID, newYearsEve)

	years, err := suite.artistService.GetArtistShowYears(artist.ID, "all")
	suite.Require().NoError(err)
	suite.Equal([]contracts.ArtistShowYearCount{{Year: 2019, Count: 1}}, years)

	// ...and the year it offers actually returns that show.
	_, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", contracts.ArtistShowsQuery{
		TimeFilter: "all", Limit: 10, Year: years[0].Year,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total, "the histogram must not offer a year the list cannot show")
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistShowYears_RespectsTimeFilter() {
	artist := suite.createTestArtist("Filtered Histogram Artist")
	venue := suite.createTestVenue("Filtered Histogram Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID,
		fixedUTC(2019, time.February, 1, 20),
		time.Now().UTC().AddDate(0, 0, 45),
	)

	past, err := suite.artistService.GetArtistShowYears(artist.ID, "past")
	suite.Require().NoError(err)
	suite.Equal([]contracts.ArtistShowYearCount{{Year: 2019, Count: 1}}, past)

	upcoming, err := suite.artistService.GetArtistShowYears(artist.ID, "upcoming")
	suite.Require().NoError(err)
	suite.Require().Len(upcoming, 1)
	suite.Equal(int64(1), upcoming[0].Count)
	suite.NotEqual(2019, upcoming[0].Year)

	all, err := suite.artistService.GetArtistShowYears(artist.ID, "all")
	suite.Require().NoError(err)
	suite.Len(all, 2)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistShowYears_NoShowsReturnsEmptySlice() {
	artist := suite.createTestArtist("Silent Artist")

	years, err := suite.artistService.GetArtistShowYears(artist.ID, "all")
	suite.Require().NoError(err)
	suite.NotNil(years, "an empty histogram must serialize as [] rather than null")
	suite.Empty(years)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistShowYears_ArtistNotFound() {
	_, err := suite.artistService.GetArtistShowYears(99999, "all")
	suite.Require().Error(err)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

// The histogram is scoped to ONE artist. A count that leaked a bandmate's other
// project would offer years this artist never played.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistShowYears_ScopedToOneArtist() {
	artist := suite.createTestArtist("Scoped Artist")
	other := suite.createTestArtist("Neighbour Artist")
	venue := suite.createTestVenue("Scoped Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.seedShowsForArtist(artist.ID, venue.ID, user.ID, fixedUTC(2015, time.May, 1, 20))
	suite.seedShowsForArtist(other.ID, venue.ID, user.ID, fixedUTC(2014, time.May, 1, 20))

	years, err := suite.artistService.GetArtistShowYears(artist.ID, "all")
	suite.Require().NoError(err)
	suite.Equal([]contracts.ArtistShowYearCount{{Year: 2015, Count: 1}}, years)
}
