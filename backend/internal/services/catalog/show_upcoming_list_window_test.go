package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The date-windowed readers of the upcoming partition: GetUpcomingShowsPage and
// GetUpcomingShowMonths.
//
// Every fixture here is anchored on a VENUE's calendar rather than on a UTC
// instant, for the reason the sibling venue-local suites state: the service
// reads Postgres now(), there is no clock seam, and an assertion anchored on the
// wall clock flips depending on what hour CI runs at. The shared fixtures
// (newVenueInZone, newApprovedShowAt, venueLocalInstant,
// requireLocalAndUTCDatesDiffer) come from show_venue_local_test.go.
//
// The month a fixture lands in is DERIVED from the fixture rather than written
// down, so no test here depends on which month the suite runs in.

// venueLocalYMD is the venue-local calendar date of an instant, which is the
// bucket the SQL puts that show in.
func venueLocalYMD(t *testing.T, at time.Time, zone string) (year, month, day int) {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	require.NoError(t, err, "load zone %q", zone)
	local := at.In(loc)
	return local.Year(), int(local.Month()), local.Day()
}

// addMonths walks a (year, month) pair by whole months.
func addMonths(year, month, delta int) (int, int) {
	anchor := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, delta, 0)
	return anchor.Year(), int(anchor.Month())
}

// lastDayOfMonth is the last calendar day of (year, month).
func lastDayOfMonth(year, month int) int {
	return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1).Day()
}

// venueLocalDateAt is the instant at hourLocal on an explicit venue-local date,
// for the fixtures that need a calendar position rather than a day offset.
func venueLocalDateAt(t *testing.T, zone string, year, month, day, hourLocal int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	require.NoError(t, err, "load zone %q", zone)
	return time.Date(year, time.Month(month), day, hourLocal, 0, 0, 0, loc)
}

// calendarPage is what the window assertions compare: the ids on the page and
// the window's filter-aware total, as one comparable value. Comparing the pair
// rather than either half is deliberate, for the reason upcomingPage gives next
// door: the count and the page are separate queries, and a window bug that moved
// one without the other would otherwise slip through.
type calendarPage struct {
	IDs   []uint
	Total int64
}

func (suite *ShowServiceIntegrationTestSuite) calendarWindow(
	query contracts.ShowCalendarQuery,
	filters *contracts.UpcomingShowsFilter,
) calendarPage {
	if query.Limit == 0 {
		query.Limit = 50
	}
	shows, total, err := suite.showService.GetUpcomingShowsPage(query, false, filters)
	suite.Require().NoError(err)
	ids := make([]uint, 0, len(shows))
	for _, s := range shows {
		ids = append(ids, s.ID)
	}
	return calendarPage{IDs: ids, Total: total}
}

func (suite *ShowServiceIntegrationTestSuite) monthWindow(year, month int) calendarPage {
	return suite.calendarWindow(contracts.ShowCalendarQuery{
		ShowCalendarWindow: contracts.ShowCalendarWindow{Year: year, Month: month},
	}, nil)
}

// A month window lists exactly the shows whose VENUE-LOCAL date falls in that
// month, and nothing from the months on either side.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_MonthWindowSelectsOneVenueLocalMonth() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Month Window Room", "AZ", zone, true)
	user := suite.createTestUser()

	type seeded struct {
		id    uint
		year  int
		month int
	}
	var shows []seeded
	for _, dayOffset := range []int{1, 40, 75, 110} {
		at := venueLocalInstant(suite.T(), zone, dayOffset, 20)
		show := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", at)
		year, month, _ := venueLocalYMD(suite.T(), at, zone)
		shows = append(shows, seeded{id: show.ID, year: year, month: month})
	}

	for _, target := range shows {
		var want []uint
		for _, s := range shows {
			if s.year == target.year && s.month == target.month {
				want = append(want, s.id)
			}
		}

		page := suite.monthWindow(target.year, target.month)
		suite.Require().Equal(want, page.IDs, "window %d-%02d", target.year, target.month)
		suite.Require().Equal(int64(len(want)), page.Total, "window %d-%02d total", target.year, target.month)
	}
}

// Last night is not upcoming, so it is absent from the window that contains its
// own date. The window narrows the upcoming partition; it does not reopen it.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_WindowNeverReadmitsLastNight() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Last Night Room", "AZ", zone, true)
	user := suite.createTestUser()

	lastNight := venueLocalInstant(suite.T(), zone, -1, 23)
	requireLocalAndUTCDatesDiffer(suite.T(), lastNight, zone)
	past := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", lastNight)

	tonight := venueLocalInstant(suite.T(), zone, 0, 20)
	upcoming := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", tonight)

	pastYear, pastMonth, pastDay := venueLocalYMD(suite.T(), lastNight, zone)
	suite.Require().NotContains(suite.monthWindow(pastYear, pastMonth).IDs, past.ID)

	tonightYear, tonightMonth, _ := venueLocalYMD(suite.T(), tonight, zone)
	suite.Require().NotContains(suite.monthWindow(tonightYear, tonightMonth).IDs, past.ID)
	suite.Require().Contains(suite.monthWindow(tonightYear, tonightMonth).IDs, upcoming.ID)

	dayPage := suite.calendarWindow(contracts.ShowCalendarQuery{
		ShowCalendarWindow: contracts.ShowCalendarWindow{Year: pastYear, Month: pastMonth, Day: pastDay},
	}, nil)
	suite.Require().Empty(dayPage.IDs)
	suite.Require().Equal(int64(0), dayPage.Total)
}

// A date-only show is stored at 20:00 venue-local (utils.DateOnlyEventHour), and
// on the last day of a month that instant is already the NEXT month in UTC for
// every western zone. The window has to name the month the venue is in.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_DateOnlyEveningOnTheLastDayStaysInThatMonth() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Month End Room", "AZ", zone, true)
	user := suite.createTestUser()

	todayYear, todayMonth, _ := venueLocalYMD(suite.T(), time.Now(), zone)
	year, month := addMonths(todayYear, todayMonth, 1)
	at := venueLocalDateAt(suite.T(), zone, year, month, lastDayOfMonth(year, month), 20)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	show := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", at)

	suite.Require().Equal([]uint{show.ID}, suite.monthWindow(year, month).IDs)

	nextYear, nextMonth := addMonths(year, month, 1)
	suite.Require().Empty(suite.monthWindow(nextYear, nextMonth).IDs)
}

// The mirror, in a positive-offset zone: 01:00 on the 1st is still the previous
// month in UTC, and the show belongs to the month its venue is in.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_AfterMidnightShowLandsInTheNewVenueLocalMonth() {
	const zone = "Asia/Tokyo"
	venue := newVenueInZone(suite.T(), suite.db, "Tokyo Room", "AZ", zone, true)
	user := suite.createTestUser()

	todayYear, todayMonth, _ := venueLocalYMD(suite.T(), time.Now(), zone)
	year, month := addMonths(todayYear, todayMonth, 2)
	at := venueLocalDateAt(suite.T(), zone, year, month, 1, 1)
	requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
	show := suite.createApprovedShowAt(venue.ID, user.ID, "Tokyo", "AZ", at)

	suite.Require().Equal([]uint{show.ID}, suite.monthWindow(year, month).IDs)

	utcYear, utcMonth := at.UTC().Year(), int(at.UTC().Month())
	suite.Require().NotEqual(month, utcMonth, "fixture no longer straddles a month boundary")
	suite.Require().Empty(suite.monthWindow(utcYear, utcMonth).IDs)
}

// Offset pages partition the window: no row appears twice, none is skipped, and
// the order is the list's own. The total is the window's, not the page's.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_OffsetPagesAreDisjointAndOrdered() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Paging Room", "AZ", zone, true)
	user := suite.createTestUser()

	todayYear, todayMonth, _ := venueLocalYMD(suite.T(), time.Now(), zone)
	year, month := addMonths(todayYear, todayMonth, 3)

	var want []uint
	for day := 1; day <= 5; day++ {
		at := venueLocalDateAt(suite.T(), zone, year, month, day, 20)
		want = append(want, suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", at).ID)
	}

	var walked []uint
	for offset := 0; offset < 6; offset += 2 {
		page := suite.calendarWindow(contracts.ShowCalendarQuery{
			ShowCalendarWindow: contracts.ShowCalendarWindow{Year: year, Month: month},
			Limit:              2,
			Offset:             offset,
		}, nil)
		suite.Require().Equal(int64(5), page.Total, "offset %d total", offset)
		suite.Require().LessOrEqual(len(page.IDs), 2, "offset %d page size", offset)
		walked = append(walked, page.IDs...)
	}

	suite.Require().Equal(want, walked)
}

// A day window is one VENUE-LOCAL date, not one UTC date: the 20:00 fixtures
// below sit on the following UTC day.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_DayWindowNarrowsToOneVenueLocalDate() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Day Window Room", "AZ", zone, true)
	user := suite.createTestUser()

	target := venueLocalInstant(suite.T(), zone, 3, 20)
	requireLocalAndUTCDatesDiffer(suite.T(), target, zone)
	wanted := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", target)

	neighbour := venueLocalInstant(suite.T(), zone, 4, 20)
	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", neighbour)

	year, month, day := venueLocalYMD(suite.T(), target, zone)
	page := suite.calendarWindow(contracts.ShowCalendarQuery{
		ShowCalendarWindow: contracts.ShowCalendarWindow{Year: year, Month: month, Day: day},
	}, nil)
	suite.Require().Equal([]uint{wanted.ID}, page.IDs)
	suite.Require().Equal(int64(1), page.Total)
}

// A run window lists its anchor date and the days that follow it, and stops
// there. The edges are the assertion: a run that leaked one day either way would
// still look right in the middle.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_RunWindowSpansConsecutiveVenueLocalDates() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Run Window Room", "AZ", zone, true)
	user := suite.createTestUser()

	// One show per venue-local day, on five consecutive days starting tomorrow.
	// 20:00 local sits on the following UTC day, so a run read on UTC dates would
	// select a different five.
	byOffset := make(map[int]uint, 5)
	for offset := 1; offset <= 5; offset++ {
		at := venueLocalInstant(suite.T(), zone, offset, 20)
		requireLocalAndUTCDatesDiffer(suite.T(), at, zone)
		byOffset[offset] = suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", at).ID
	}

	anchor := venueLocalInstant(suite.T(), zone, 2, 20)
	year, month, day := venueLocalYMD(suite.T(), anchor, zone)
	page := suite.calendarWindow(contracts.ShowCalendarQuery{
		ShowCalendarWindow: contracts.ShowCalendarWindow{
			Year: year, Month: month, Day: day, Days: 3,
		},
	}, nil)

	suite.Require().Equal([]uint{byOffset[2], byOffset[3], byOffset[4]}, page.IDs)
	suite.Require().Equal(int64(3), page.Total)

	// A run of one is the day itself, on the same predicate the bare day window
	// builds, so the two cannot answer differently for the same date.
	single := suite.calendarWindow(contracts.ShowCalendarQuery{
		ShowCalendarWindow: contracts.ShowCalendarWindow{
			Year: year, Month: month, Day: day, Days: 1,
		},
	}, nil)
	suite.Require().Equal([]uint{byOffset[2]}, single.IDs)
	suite.Require().Equal(int64(1), single.Total)
}

// A run anchored at the far edge of the addressable year range is an EMPTY page
// rather than a database error.
//
// The request schema admits years up to 9999, and a run from the last day of
// that year ends in year 10000, which is the one window whose ISO edge carries
// five digits. That string reaches Postgres as a bind parameter, so the question
// is whether the driver and the date type take it; the answer is asserted here
// rather than reasoned about, because the failure mode is a 500 on a URL anyone
// can type.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_RunAtTheEndOfTheYearRangeIsEmptyNotAnError() {
	page := suite.calendarWindow(contracts.ShowCalendarQuery{
		ShowCalendarWindow: contracts.ShowCalendarWindow{
			Year: 9999, Month: 12, Day: 31,
			Days: contracts.ShowCalendarMaxWindowDays,
		},
	}, nil)

	suite.Require().Empty(page.IDs)
	suite.Require().Equal(int64(0), page.Total)
}

// A run crosses a month boundary, which is what separates it from the month
// window it is addressed under.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_RunWindowCrossesAMonthBoundary() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Run Across Months Room", "AZ", zone, true)
	user := suite.createTestUser()

	// The last day of a month far enough ahead that today cannot fall inside the
	// run, so every seeded show is upcoming whatever hour the suite runs at.
	todayYear, todayMonth, _ := venueLocalYMD(suite.T(), time.Now(), zone)
	year, month := addMonths(todayYear, todayMonth, 2)
	lastDay := lastDayOfMonth(year, month)

	last := venueLocalDateAt(suite.T(), zone, year, month, lastDay, 20)
	lastShow := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", last)
	firstOfNext := last.AddDate(0, 0, 1)
	nextShow := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", firstOfNext)

	page := suite.calendarWindow(contracts.ShowCalendarQuery{
		ShowCalendarWindow: contracts.ShowCalendarWindow{
			Year: year, Month: month, Day: lastDay, Days: 2,
		},
	}, nil)
	suite.Require().Equal([]uint{lastShow.ID, nextShow.ID}, page.IDs)
	suite.Require().Equal(int64(2), page.Total)
}

// A month nothing is booked in is an empty page with a real zero, not an error.
// Whether that is a page or a 404 is the route's call, and this is the answer it
// makes the call from.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_EmptyMonthIsAnEmptyPageWithZeroTotal() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Empty Month Room", "AZ", zone, true)
	user := suite.createTestUser()
	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", venueLocalInstant(suite.T(), zone, 1, 20))

	todayYear, todayMonth, _ := venueLocalYMD(suite.T(), time.Now(), zone)
	year, month := addMonths(todayYear, todayMonth, 60)

	page := suite.monthWindow(year, month)
	suite.Require().Empty(page.IDs)
	suite.Require().Equal(int64(0), page.Total)
}

// An absent window is the whole upcoming set, which is what the root list pages
// over, and it agrees with the cursor endpoint it shares a partition with.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_NoWindowMatchesTheCursorList() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Root Paging Room", "AZ", zone, true)
	user := suite.createTestUser()

	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", venueLocalInstant(suite.T(), zone, -2, 20))
	for _, dayOffset := range []int{0, 5, 40, 400} {
		suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", venueLocalInstant(suite.T(), zone, dayOffset, 20))
	}

	cursorIDs, cursorTotal := suite.upcomingShowIDs("UTC")
	page := suite.calendarWindow(contracts.ShowCalendarQuery{}, nil)

	suite.Require().Equal(cursorTotal, page.Total)
	suite.Require().Equal(cursorIDs, page.IDs)
}

// The histogram's bars sum to the list's total for the SAME filters, which is
// the invariant a pager's month labels rest on: a label walk over cumulative
// counts is only correct while the counts describe the list being paged.
//
// Seeded across a gap month, and with one venue carrying NO stored zone, because
// the sum is exactly what a dropped row breaks: the zone lateral is a LEFT JOIN,
// so a row whose venue has no zone still has to reach a bucket.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowMonths_SoonestFirstAndSumsToTheListTotal() {
	const zone = "America/Phoenix"
	zoned := newVenueInZone(suite.T(), suite.db, "Zoned Room", "AZ", zone, true)
	unzoned := newVenueInZone(suite.T(), suite.db, "Unzoned Room", "AZ", "", true)
	user := suite.createTestUser()

	todayYear, todayMonth, _ := venueLocalYMD(suite.T(), time.Now(), zone)
	for _, monthDelta := range []int{0, 0, 1, 3} {
		year, month := addMonths(todayYear, todayMonth, monthDelta)
		day := 28
		if monthDelta == 0 {
			// The current month must be seeded in its FUTURE, or the row is past.
			day = lastDayOfMonth(year, month)
		}
		suite.createApprovedShowAt(zoned.ID, user.ID, "Phoenix", "AZ",
			venueLocalDateAt(suite.T(), zone, year, month, day, 20))
	}
	futureYear, futureMonth := addMonths(todayYear, todayMonth, 5)
	suite.createApprovedShowAt(unzoned.ID, user.ID, "Phoenix", "AZ",
		venueLocalDateAt(suite.T(), zone, futureYear, futureMonth, 15, 20))

	months, err := suite.showService.GetUpcomingShowMonths(false, nil)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(months)

	var sum int64
	for i, bucket := range months {
		sum += bucket.Count
		suite.Require().Positive(bucket.Count, "sparse histogram must not emit empty months")
		if i == 0 {
			continue
		}
		previous := months[i-1]
		suite.Require().True(
			bucket.Year > previous.Year || (bucket.Year == previous.Year && bucket.Month > previous.Month),
			"months must run soonest first: %v then %v", previous, bucket,
		)
	}

	unwindowed := suite.calendarWindow(contracts.ShowCalendarQuery{}, nil)
	suite.Require().Equal(unwindowed.Total, sum)

	// And each bar equals the window it names.
	for _, bucket := range months {
		suite.Require().Equal(bucket.Count, suite.monthWindow(bucket.Year, bucket.Month).Total,
			"bar %d-%02d disagrees with its own window", bucket.Year, bucket.Month)
	}
}

// The same invariant under a multi-city filter, which is the shape the site
// actually requests: the histogram and the list must narrow identically or the
// strip offers a month the list cannot show.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowMonths_SumsToTheListTotalUnderAMultiCityFilter() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Multi City Room", "AZ", zone, true)
	user := suite.createTestUser()

	todayYear, todayMonth, _ := venueLocalYMD(suite.T(), time.Now(), zone)
	for i, city := range []string{"Phoenix", "Mesa", "Tucson"} {
		year, month := addMonths(todayYear, todayMonth, i+1)
		suite.createApprovedShowAt(venue.ID, user.ID, city, "AZ",
			venueLocalDateAt(suite.T(), zone, year, month, 14, 20))
	}

	filters := &contracts.UpcomingShowsFilter{Cities: []contracts.CityStateFilter{
		{City: "Phoenix", State: "AZ"},
		{City: "Mesa", State: "AZ"},
	}}

	months, err := suite.showService.GetUpcomingShowMonths(false, filters)
	suite.Require().NoError(err)

	var sum int64
	for _, bucket := range months {
		sum += bucket.Count
	}

	unwindowed := suite.calendarWindow(contracts.ShowCalendarQuery{}, filters)
	suite.Require().Equal(int64(2), unwindowed.Total)
	suite.Require().Equal(unwindowed.Total, sum)
}

// A malformed window is REFUSED rather than answered. In SQL a half-stated or
// impossible window narrows to nothing, which is indistinguishable from "no
// window requested" and would hand back the whole upcoming catalog; the HTTP
// boundary refuses these first, and this is the guard for a caller that builds
// the query struct directly.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_RefusesMalformedWindows() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Malformed Window Room", "AZ", zone, true)
	user := suite.createTestUser()
	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", venueLocalInstant(suite.T(), zone, 1, 20))

	for _, window := range []contracts.ShowCalendarWindow{
		{Day: 14},
		{Month: 11, Day: 14},
		{Month: 11},
		{Year: 2027},
		{Year: 2027, Month: 13},
		{Year: 2027, Month: 2, Day: 31},
		{Year: 2027, Month: 2, Day: 29},
		// Negatives are the shape the HTTP schema hides: a miscomputed window
		// read as "unset" is how one day turns into the whole catalog.
		{Year: -1},
		{Year: -1, Month: -5},
		{Year: 2027, Month: -1},
		{Year: 2027, Month: 11, Day: -1},
		// A run with no anchor narrows nothing, and a run longer than the bound
		// is a full-catalog scan behind a URL naming a fortnight.
		{Days: 3},
		{Year: 2027, Days: 3},
		{Year: 2027, Month: 11, Days: 3},
		{Year: 2027, Month: 11, Day: 14, Days: -1},
		{Year: 2027, Month: 11, Day: 14, Days: contracts.ShowCalendarMaxWindowDays + 1},
	} {
		shows, total, err := suite.showService.GetUpcomingShowsPage(
			contracts.ShowCalendarQuery{ShowCalendarWindow: window, Limit: 50}, false, nil)
		suite.Require().Errorf(err, "window %+v must be refused, not answered", window)
		suite.Require().Empty(shows, "window %+v", window)
		suite.Require().Zero(total, "window %+v", window)
	}
}

// The window's coarse UTC bounds are a planner hint and must be LOSSLESS with
// respect to the exact venue-local equality, at the extremes of the inhabited
// offset range where they are likeliest to clip a row.
//
// The first instant of a venue-local month at UTC+14 is fourteen hours before
// the month starts in UTC; the last at UTC-12 is twelve hours after it ends. The
// margin is two days, so both must survive, and the last night of the PREVIOUS
// month must stay out.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsPage_ExtremeVenueOffsetSurvivesTheWindowBounds() {
	user := suite.createTestUser()

	for _, zone := range []string{"Etc/GMT+12", "Etc/GMT+11", "Etc/GMT-14", "Etc/GMT-13"} {
		venue := newVenueInZone(suite.T(), suite.db, "Window Edge Room "+zone, "AZ", zone, true)

		todayYear, todayMonth, _ := venueLocalYMD(suite.T(), time.Now(), zone)
		year, month := addMonths(todayYear, todayMonth, 3)
		lastDay := lastDayOfMonth(year, month)
		previousYear, previousMonth := addMonths(year, month, -1)

		firstInstant := suite.createApprovedShowAt(venue.ID, user.ID, "Edgeville", "AZ",
			venueLocalDateAt(suite.T(), zone, year, month, 1, 0))
		lastInstant := suite.createApprovedShowAt(venue.ID, user.ID, "Edgeville", "AZ",
			venueLocalDateAt(suite.T(), zone, year, month, lastDay, 23))
		previousMonthEnd := suite.createApprovedShowAt(venue.ID, user.ID, "Edgeville", "AZ",
			venueLocalDateAt(suite.T(), zone, previousYear, previousMonth,
				lastDayOfMonth(previousYear, previousMonth), 23))

		page := suite.monthWindow(year, month)
		suite.Require().Equal([]uint{firstInstant.ID, lastInstant.ID}, page.IDs,
			"zone %s: both edges of the venue-local month must survive the coarse bounds", zone)
		suite.Require().Equal(int64(2), page.Total, "zone %s", zone)

		// The same two edges at DAY resolution, whose bounds are tighter still.
		firstDay := suite.calendarWindow(contracts.ShowCalendarQuery{
			ShowCalendarWindow: contracts.ShowCalendarWindow{Year: year, Month: month, Day: 1},
		}, nil)
		suite.Require().Equal([]uint{firstInstant.ID}, firstDay.IDs, "zone %s day 1", zone)

		lastDayPage := suite.calendarWindow(contracts.ShowCalendarQuery{
			ShowCalendarWindow: contracts.ShowCalendarWindow{Year: year, Month: month, Day: lastDay},
		}, nil)
		suite.Require().Equal([]uint{lastInstant.ID}, lastDayPage.IDs, "zone %s day %d", zone, lastDay)

		// Each zone starts clean, for the reason the sibling extreme-offset test
		// gives: scoped removal rather than an unscoped wipe.
		suite.removeShows(firstInstant.ID, lastInstant.ID, previousMonthEnd.ID)
	}
}

// A catalog with nothing upcoming returns an EMPTY histogram, not a nil one, so
// the payload serializes as [] and a client can iterate it without a null check.
// The guarantee is the make() in GetUpcomingShowMonths; nothing above the service
// can observe it, because every caller above hands its own slice to the mock.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowMonths_EmptyCatalogIsAnEmptySliceNotNil() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Quiet Room", "AZ", zone, true)
	user := suite.createTestUser()
	// One PAST show, so the table is not empty and the emptiness comes from the
	// upcoming partition rather than from there being no rows at all.
	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", venueLocalInstant(suite.T(), zone, -3, 20))

	months, err := suite.showService.GetUpcomingShowMonths(false, nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(months, "the histogram must serialize as [] rather than null")
	suite.Require().Empty(months)

	// The same for a filter that matches nothing.
	filtered, err := suite.showService.GetUpcomingShowMonths(false, &contracts.UpcomingShowsFilter{
		Cities: []contracts.CityStateFilter{{City: "Nowhere", State: "ZZ"}},
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(filtered)
	suite.Require().Empty(filtered)
}

// mustLoadZone is the zone or a failed test, so a fixture cannot silently fall
// back to UTC and assert against a calendar the service never used.
func mustLoadZone(t *testing.T, zone string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	require.NoError(t, err, "load zone %q", zone)
	return loc
}

// requireMonthInRange asserts a month is addressable, naming both edges when it
// is not so a failure says which end it fell off.
func (suite *ShowServiceIntegrationTestSuite) requireMonthInRange(
	rng contracts.ShowCalendarRange,
	month contracts.ShowCalendarMonth,
	because string,
) {
	suite.T().Helper()
	suite.Require().GreaterOrEqual(showCalendarMonthOrdinal(month), showCalendarMonthOrdinal(rng.FirstMonth),
		"%s: %v is before the first addressable month %v", because, month, rng.FirstMonth)
	suite.Require().LessOrEqual(showCalendarMonthOrdinal(month), showCalendarMonthOrdinal(rng.LastMonth),
		"%s: %v is past the last addressable month %v", because, month, rng.LastMonth)
}

// An empty catalog is still addressable at TODAY, on every clock a reader could
// be asking from.
//
// The sweep is the assertion. The span is stated in months and a month has no
// zone, so the property that matters is not which UTC month the edges landed in
// but that no inhabited zone's own current month falls outside them, which is
// exactly the dead-end a Tonight link would hit on the one night a year the two
// calendars disagree.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsCalendarRange_EmptyCatalogStillAddressesTodayEverywhere() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Empty Range Room", "AZ", zone, true)
	user := suite.createTestUser()
	// One PAST show, so the emptiness is the upcoming partition's rather than the
	// table's, and a past month cannot drag an edge backwards.
	past := venueLocalInstant(suite.T(), zone, -40, 20)
	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", past)

	rng, err := suite.showService.GetUpcomingShowsCalendarRange()
	suite.Require().NoError(err)
	suite.Require().LessOrEqual(showCalendarMonthOrdinal(rng.FirstMonth), showCalendarMonthOrdinal(rng.LastMonth),
		"the span must never be inverted")

	for _, callerZone := range everyCallerOffsetZone() {
		today := time.Now().In(mustLoadZone(suite.T(), callerZone))
		suite.requireMonthInRange(rng,
			contracts.ShowCalendarMonth{Year: today.Year(), Month: int(today.Month())},
			"today in "+callerZone)
	}

	pastYear, pastMonth, _ := venueLocalYMD(suite.T(), past, zone)
	suite.Require().Greater(showCalendarMonthOrdinal(rng.FirstMonth),
		showCalendarMonthOrdinal(contracts.ShowCalendarMonth{Year: pastYear, Month: pastMonth}),
		"a past show must not extend the span backwards")
}

// Every date the quick-window row can address is inside the span, on a catalog
// that holds nothing at all.
//
// The row is arithmetic on a clock: "This weekend" anchors on the coming Friday,
// which is four days out on a Monday, so on a Monday near the end of a month it
// names the NEXT month. Bounded by the shows alone that month would be outside
// the span and the chip would land on a hard 404, which is the dead end the
// whole surface exists to avoid.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsCalendarRange_HoldsEveryQuickWindowAnchor() {
	const zone = "America/Phoenix"
	newVenueInZone(suite.T(), suite.db, "Chip Horizon Room", "AZ", zone, true)

	rng, err := suite.showService.GetUpcomingShowsCalendarRange()
	suite.Require().NoError(err)

	// Four days is the furthest anchor the row produces; the loop walks every
	// shorter one so a rule change that moves an intermediate chip is caught too.
	today := time.Now().UTC()
	for offset := 0; offset <= 4; offset++ {
		anchor := today.AddDate(0, 0, offset)
		suite.requireMonthInRange(rng,
			contracts.ShowCalendarMonth{Year: anchor.Year(), Month: int(anchor.Month())},
			fmt.Sprintf("a quick-window anchor %d days out", offset))
	}
}

// Every month the sitemap announces is a month the span holds open.
//
// The two are computed from different queries: the sitemap restates the
// upcoming partition, the span composes upcomingShowPredicates. They agree today
// because a month with an upcoming show cannot be outside a span that runs to
// the last such month, and this is what says so when either side moves. A
// sitemap entry outside the span is a URL this site announces and then answers
// with a hard 404.
func (suite *ShowServiceIntegrationTestSuite) TestShowsMonthSitemapStaysInsideTheCalendarRange() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Sitemap Range Room", "AZ", zone, true)
	user := suite.createTestUser()

	todayLocal := time.Now().In(mustLoadZone(suite.T(), zone))
	for _, monthsOut := range []int{0, 1, 5} {
		year, month := addMonths(todayLocal.Year(), int(todayLocal.Month()), monthsOut)
		day := 15
		if monthsOut == 0 {
			// The current month's fixture has to be upcoming, so it is anchored on
			// the venue's own tomorrow rather than on a fixed day of the month.
			tomorrow := todayLocal.AddDate(0, 0, 1)
			year, month, day = tomorrow.Year(), int(tomorrow.Month()), tomorrow.Day()
		}
		suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ",
			venueLocalDateAt(suite.T(), zone, year, month, day, 20))
	}

	rng, err := suite.showService.GetUpcomingShowsCalendarRange()
	suite.Require().NoError(err)

	entries, err := NewSitemapService(suite.db).showsMonthEntries(context.Background())
	suite.Require().NoError(err)
	suite.Require().NotEmpty(entries, "the fixtures must produce sitemap months")

	for _, entry := range entries {
		var year, month int
		_, err := fmt.Sscanf(entry.Slug, "%d/%d", &year, &month)
		suite.Require().NoError(err, "sitemap slug %q", entry.Slug)
		suite.requireMonthInRange(rng,
			contracts.ShowCalendarMonth{Year: year, Month: month},
			"sitemap month "+entry.Slug)
	}
}

// The LAST edge is the last month that holds an upcoming show, and every quiet
// month between here and there is inside the span.
//
// The quiet months are the whole point of the range: the histogram would answer
// with the two months that have rows, and a rule built on it would 404 the four
// between them.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsCalendarRange_SpansTheQuietMonthsBeforeTheLastShow() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Far Future Room", "AZ", zone, true)
	user := suite.createTestUser()

	// Four months out, on the first of the month so no venue-local shift can
	// carry the fixture into a neighbouring month.
	todayLocal := time.Now().In(mustLoadZone(suite.T(), zone))
	farYear, farMonth := addMonths(todayLocal.Year(), int(todayLocal.Month()), 4)
	far := venueLocalDateAt(suite.T(), zone, farYear, farMonth, 1, 20)
	suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", far)

	rng, err := suite.showService.GetUpcomingShowsCalendarRange()
	suite.Require().NoError(err)

	suite.Require().Equal(contracts.ShowCalendarMonth{Year: farYear, Month: farMonth}, rng.LastMonth,
		"the last edge must be the last upcoming month")
	for delta := 0; delta <= 4; delta++ {
		year, month := addMonths(todayLocal.Year(), int(todayLocal.Month()), delta)
		suite.requireMonthInRange(rng, contracts.ShowCalendarMonth{Year: year, Month: month},
			"a quiet month inside the span")
	}

	beyondYear, beyondMonth := addMonths(farYear, farMonth, 1)
	suite.Require().Greater(showCalendarMonthOrdinal(contracts.ShowCalendarMonth{Year: beyondYear, Month: beyondMonth}),
		showCalendarMonthOrdinal(rng.LastMonth),
		"the month after the last show must fall outside the span")
}

// A show nobody can see does not make a month addressable. The span is published
// to an anonymous cache, so a pending submission moving an edge would advertise a
// month whose page renders nothing.
func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShowsCalendarRange_NonApprovedShowsDoNotMoveTheEdges() {
	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Pending Room", "AZ", zone, true)
	user := suite.createTestUser()

	todayLocal := time.Now().In(mustLoadZone(suite.T(), zone))
	farYear, farMonth := addMonths(todayLocal.Year(), int(todayLocal.Month()), 6)
	pending := suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ",
		venueLocalDateAt(suite.T(), zone, farYear, farMonth, 1, 20))
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).Where("id = ?", pending.ID).
		Update("status", catalogm.ShowStatusPending).Error)

	rng, err := suite.showService.GetUpcomingShowsCalendarRange()
	suite.Require().NoError(err)
	suite.Require().Less(showCalendarMonthOrdinal(rng.LastMonth),
		showCalendarMonthOrdinal(contracts.ShowCalendarMonth{Year: farYear, Month: farMonth}),
		"a pending show must not extend the span")
	suite.requireMonthInRange(rng,
		contracts.ShowCalendarMonth{Year: todayLocal.Year(), Month: int(todayLocal.Month())},
		"today")
}
