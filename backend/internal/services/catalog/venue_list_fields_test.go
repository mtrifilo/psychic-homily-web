package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// =============================================================================
// Unit: the ORDER BY key
// =============================================================================

// TestVenueListOrderBy_RejectsUnknownSort pins the service-side half of the
// sort contract: an unrecognized key is an error, never the default order.
func TestVenueListOrderBy_RejectsUnknownSort(t *testing.T) {
	if _, err := venueListOrderBy("bogus"); err == nil {
		t.Fatal("venueListOrderBy(\"bogus\") returned no error; an unknown sort must never fall through to the default")
	}
	for _, sort := range append([]string{""}, contracts.VenueListSortValues...) {
		if _, err := venueListOrderBy(sort); err != nil {
			t.Errorf("venueListOrderBy(%q) errored: %v", sort, err)
		}
	}
}

// =============================================================================
// Integration: GET /venues row fields
// =============================================================================

// venueLocalZone is the zone an AZ venue created by createTestVenue resolves
// to: the row carries no timezone, so both the SQL rules and their Go twin fall
// through to the state map. Phoenix keeps no DST, so wall-clock arithmetic over
// it is exact, which is what lets the boundary tests below pin an edge.
func venueLocalZone(t require.TestingT) *time.Location {
	loc, err := time.LoadLocation("America/Phoenix")
	require.NoError(t, err)
	return loc
}

// TestGetVenuesWithShowCounts_CountHoldsAShowUnderWay is the ticket's headline
// acceptance: a set that started an hour ago is still an upcoming listing here.
//
// It then states the relation to the venue PAGE's own upcoming total, which is
// bounded at venue-local midnight rather than at the night in progress. The two
// answers differ only while the local clock is inside the first hour of a new
// local date, when an hour-old show is still on the PREVIOUS one, so the test
// asserts both sides of that split rather than skipping the awkward hour.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_CountHoldsAShowUnderWay() {
	venue := suite.createTestVenue("Under Way Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	// ONE clock read for both the fixture and the branch it selects below. Two
	// reads either side of the queries would disagree across local midnight and
	// assert the wrong half of the split.
	loc := venueLocalZone(suite.T())
	now := time.Now().In(loc)
	started := now.Add(-time.Hour)
	suite.createRailShow(venue.ID, user.ID, "Doors Open", started)

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Under Way Room")
	suite.Equal(1, row.UpcomingShowCount, "a set already under way is still an upcoming listing")
	suite.Require().NotNil(row.NextShow, "the room has an upcoming show, so next_show must be set")

	_, venuePageTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "", contracts.VenueShowsQuery{
		TimeFilter: "upcoming",
		Limit:      10,
	})
	suite.Require().NoError(err)

	sameLocalDate := started.Format("2006-01-02") == now.Format("2006-01-02")
	if sameLocalDate {
		suite.Equal(int64(1), venuePageTotal,
			"the show is on today's venue-local date, which both boundaries keep")
	} else {
		suite.Equal(int64(0), venuePageTotal,
			"the show is on the previous venue-local date, which only the night boundary keeps")
	}
}

// TestGetVenuesWithShowCounts_NightBoundaryEdge pins the exact instant the two
// picks change places, at whatever hour the suite runs.
//
// Local midnight on the night-start date is the earliest instant the night
// condition keeps; one second earlier is the latest instant its complement
// keeps. A rule that drifted by an hour, a day or a DST offset would move one
// of these two shows to the wrong side.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_NightBoundaryEdge() {
	venue := suite.createTestVenue("Edge Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	// The night-start date comes from Postgres, which is the clock the
	// conditions are evaluated against, rather than from a second reading of
	// this process's clock.
	loc := venueLocalZone(suite.T())
	var nightStartDate string
	suite.Require().NoError(suite.db.Raw(
		"SELECT (((now() AT TIME ZONE ?) - make_interval(hours => ?))::date)::text",
		loc.String(), shared.NightStartHour).Scan(&nightStartDate).Error)
	edge, err := time.ParseInLocation("2006-01-02", nightStartDate, loc)
	suite.Require().NoError(err)

	first := suite.createRailShow(venue.ID, user.ID, "First Of The Night", edge)
	last := suite.createRailShow(venue.ID, user.ID, "Night Before", edge.Add(-time.Second))

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Edge Room")

	suite.Equal(1, row.UpcomingShowCount, "local midnight of the night-start date is inside the night")
	suite.Require().NotNil(row.NextShow)
	suite.Equal(*first.Slug, row.NextShow.Slug)
	suite.Equal(*last.Slug, row.LastShow.Slug, "one second earlier is the previous night")
}

// TestGetVenuesWithShowCounts_RailNextShowCanNameADifferentShow pins the one
// deliberate disagreement on the row: the rail's next_show_date is bounded at
// the request instant, so it drops a set already under way that next_show keeps.
//
// Written as an assertion rather than left to the contract comments, because a
// client rendering both fields would otherwise learn about it from a bug report.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_RailNextShowCanNameADifferentShow() {
	venue := suite.createTestVenue("Rail Split Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	underWay := suite.createRailShow(venue.ID, user.ID, "Under Way", time.Now().UTC().Add(-time.Hour))
	suite.createRailShow(venue.ID, user.ID, "Next Week", time.Now().UTC().AddDate(0, 0, 7))

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{IncludeRailFields: true}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Rail Split Room")

	suite.Require().NotNil(row.NextShow)
	suite.Equal(*underWay.Slug, row.NextShow.Slug, "the night boundary keeps the set under way")
	suite.Equal("Next Week", row.NextShowTitle, "the rail's instant boundary has already moved on")
}

// TestGetVenuesWithShowCounts_ShowWithoutASlugIsUnlinkable pins the contract
// VenueListShowRef states to clients: a show with no slug still appears, with an
// empty slug, and it is the caller's job to render it unlinked.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_ShowWithoutASlugIsUnlinkable() {
	venue := suite.createTestVenue("Slugless Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	// createApprovedShow leaves Slug nil, which is the column's real state for
	// a name GenerateSlug cannot render.
	suite.createApprovedShow(venue.ID, user.ID)

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Slugless Room")

	suite.Equal(1, row.UpcomingShowCount)
	suite.Require().NotNil(row.NextShow, "a slugless show is still the room's next show")
	suite.Empty(row.NextShow.Slug, "a NULL slug reaches the client as an empty string, not a null object")
}

// TestGetVenuesWithShowCounts_ProjectsTheStreetAddress pins the address line the
// directory prints under the room name: the column the venue already stores,
// reaching the row rather than needing a field of its own.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_ProjectsTheStreetAddress() {
	venue := suite.createTestVenue("Addressed Room", "Phoenix", "AZ", true)
	street := "1 Street Address Way"
	suite.Require().NoError(suite.db.Model(venue).Update("address", street).Error)

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Addressed Room")
	suite.Require().NotNil(row.Address)
	suite.Equal(street, *row.Address)
}

// TestGetVenuesWithShowCounts_NextAndLastPartitionTheRoom pins the invariant the
// whole row depends on: one boundary decides both picks, so the count is above
// zero exactly when next_show is set, and no show is ever both.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_NextAndLastPartitionTheRoom() {
	active := suite.createTestVenue("Active Room", "Phoenix", "AZ", true)
	quiet := suite.createTestVenue("Quiet Room", "Phoenix", "AZ", true)
	suite.createTestVenue("Empty Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	soon := suite.createRailShow(active.ID, user.ID, "Soonest", time.Now().UTC().AddDate(0, 0, 3))
	suite.createRailShow(active.ID, user.ID, "Later", time.Now().UTC().AddDate(0, 0, 30))
	activePast := suite.createRailShow(active.ID, user.ID, "Old Night", time.Now().UTC().AddDate(0, 0, -40))
	quietRecent := suite.createRailShow(quiet.ID, user.ID, "Last Night Here", time.Now().UTC().AddDate(0, 0, -10))
	suite.createRailShow(quiet.ID, user.ID, "Ancient", time.Now().UTC().AddDate(0, 0, -400))

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)

	activeRow := suite.findVenueResponse(resp, "Active Room")
	suite.Equal(2, activeRow.UpcomingShowCount)
	suite.Require().NotNil(activeRow.NextShow, "a room with an upcoming show must carry next_show")
	suite.Equal("Soonest", activeRow.NextShow.Title)
	suite.Equal(*soon.Slug, activeRow.NextShow.Slug)
	suite.Require().NotNil(activeRow.LastShow)
	suite.Equal(*activePast.Slug, activeRow.LastShow.Slug, "last_show is the most recent past show, not the oldest")

	quietRow := suite.findVenueResponse(resp, "Quiet Room")
	suite.Equal(0, quietRow.UpcomingShowCount)
	suite.Nil(quietRow.NextShow, "a room with nothing booked must carry a null next_show")
	suite.Require().NotNil(quietRow.LastShow)
	suite.Equal(*quietRecent.Slug, quietRow.LastShow.Slug)

	emptyRow := suite.findVenueResponse(resp, "Empty Room")
	suite.Nil(emptyRow.NextShow)
	suite.Nil(emptyRow.LastShow, "a room that never had a show carries null on both sides")
}

// TestGetVenuesWithShowCounts_SortsRankActiveRoomsAndSinkQuietOnes covers all
// three sorts in one fixture, because what distinguishes them is which room
// leads the SAME set.
//
// Busiest is the one with the most booked, Soonest the one with the earliest
// next show, and Aaa First the alphabetical lead. The quiet block is the same
// two rooms in the same order under all three.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_SortsRankActiveRoomsAndSinkQuietOnes() {
	user := suite.createTestUser()
	busiest := suite.createTestVenue("Busiest", "Phoenix", "AZ", true)
	soonest := suite.createTestVenue("Soonest", "Phoenix", "AZ", true)
	alpha := suite.createTestVenue("Aaa First", "Phoenix", "AZ", true)
	quietRecent := suite.createTestVenue("Quiet Recent", "Phoenix", "AZ", true)
	quietOld := suite.createTestVenue("Quiet Old", "Phoenix", "AZ", true)

	for i := 1; i <= 3; i++ {
		suite.createRailShow(busiest.ID, user.ID, "B", time.Now().UTC().AddDate(0, 0, 10+i))
	}
	suite.createRailShow(soonest.ID, user.ID, "S", time.Now().UTC().AddDate(0, 0, 2))
	suite.createRailShow(alpha.ID, user.ID, "A", time.Now().UTC().AddDate(0, 0, 20))
	suite.createRailShow(quietRecent.ID, user.ID, "QR", time.Now().UTC().AddDate(0, 0, -5))
	suite.createRailShow(quietOld.ID, user.ID, "QO", time.Now().UTC().AddDate(0, 0, -50))

	cases := []struct {
		sort string
		want []string
	}{
		{contracts.VenueListSortUpcoming, []string{"Busiest", "Aaa First", "Soonest", "Quiet Recent", "Quiet Old"}},
		{contracts.VenueListSortName, []string{"Aaa First", "Busiest", "Soonest", "Quiet Recent", "Quiet Old"}},
		{contracts.VenueListSortNext, []string{"Soonest", "Busiest", "Aaa First", "Quiet Recent", "Quiet Old"}},
	}
	for _, tc := range cases {
		resp, total, err := suite.venueService.GetVenuesWithShowCounts(
			contracts.VenueListFilters{Sort: tc.sort}, 10, 0)
		suite.Require().NoErrorf(err, "sort=%s", tc.sort)
		suite.Equalf(int64(5), total, "sort=%s", tc.sort)
		got := make([]string, 0, len(resp))
		for _, r := range resp {
			got = append(got, r.Name)
		}
		suite.Equalf(tc.want, got, "sort=%s row order", tc.sort)
	}
}

// TestGetVenuesWithShowCounts_SortDefaultsAndRejects pins both ends of the
// sort contract at the service: "" is the documented default order, and an
// unknown key is refused rather than silently defaulted.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_SortDefaultsAndRejects() {
	user := suite.createTestUser()
	few := suite.createTestVenue("Few", "Phoenix", "AZ", true)
	many := suite.createTestVenue("Many", "Phoenix", "AZ", true)
	suite.createRailShow(few.ID, user.ID, "F", time.Now().UTC().AddDate(0, 0, 5))
	suite.createRailShow(many.ID, user.ID, "M1", time.Now().UTC().AddDate(0, 0, 6))
	suite.createRailShow(many.ID, user.ID, "M2", time.Now().UTC().AddDate(0, 0, 7))

	blank, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	explicit, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{Sort: contracts.VenueListSortUpcoming}, 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(blank, 2)
	suite.Equal("Many", blank[0].Name)
	suite.Equal(blank[0].ID, explicit[0].ID, "an absent sort is the upcoming sort")

	_, _, err = suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{Sort: "bogus"}, 10, 0)
	suite.Require().Error(err, "an unknown sort must not be served as the default order")
}

// TestGetVenuesWithShowCounts_PagesAreDisjointUnderEverySort pins the reason
// every ordering key ends in venues.id: names are unique only per city, so a
// name-only key could repeat or drop a room across an offset boundary.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_PagesAreDisjointUnderEverySort() {
	user := suite.createTestUser()
	// Two rooms of the SAME name in different cities, which the unique index
	// allows, plus enough quiet rooms to force a second page.
	suite.createRailShow(suite.createTestVenue("Twin", "Phoenix", "AZ", true).ID, user.ID, "P", time.Now().UTC().AddDate(0, 0, 4))
	suite.createRailShow(suite.createTestVenue("Twin", "Tucson", "AZ", true).ID, user.ID, "T", time.Now().UTC().AddDate(0, 0, 4))
	for i := 0; i < 3; i++ {
		v := suite.createTestVenue(fmt.Sprintf("Quiet %d", i), "Phoenix", "AZ", true)
		suite.createRailShow(v.ID, user.ID, "Q", time.Now().UTC().AddDate(0, 0, -(i+1)))
	}

	for _, sort := range contracts.VenueListSortValues {
		seen := map[uint]bool{}
		for offset := 0; offset < 6; offset += 2 {
			page, _, err := suite.venueService.GetVenuesWithShowCounts(
				contracts.VenueListFilters{Sort: sort}, 2, offset)
			suite.Require().NoErrorf(err, "sort=%s offset=%d", sort, offset)
			for _, r := range page {
				suite.Falsef(seen[r.ID], "sort=%s: venue %d appeared on two pages", sort, r.ID)
				seen[r.ID] = true
			}
		}
		suite.Lenf(seen, 5, "sort=%s: paging did not cover every room exactly once", sort)
	}
}

// TestVenueLocalNightConditions_PartitionEveryShow pins the complement claim the
// next/last pair rests on, in Postgres rather than by reading the strings: the
// two counts sum to the number of shows, so no row satisfies both conditions and
// none satisfies neither.
//
// It says nothing about WHERE the boundary falls; that is
// TestGetVenuesWithShowCounts_NightBoundaryEdge, which anchors two shows either
// side of the exact instant.
func (suite *VenueServiceIntegrationTestSuite) TestVenueLocalNightConditions_PartitionEveryShow() {
	venue := suite.createTestVenue("Partition Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	for _, offset := range []time.Duration{
		-400 * 24 * time.Hour, -36 * time.Hour, -time.Hour, time.Hour, 36 * time.Hour, 400 * 24 * time.Hour,
	} {
		suite.createRailShow(venue.ID, user.ID, "P", time.Now().UTC().Add(offset))
	}

	// The production gate, not a hand-written status comparison, so a divergence
	// between the two would show up here.
	count := func(conditions ...string) int64 {
		var n int64
		q := suite.db.Table("show_venues").
			Joins("JOIN shows ON shows.id = show_venues.show_id").
			Joins(shared.VenueTZJoin).
			Where("show_venues.venue_id = ?", venue.ID).
			Where(shared.PublicShowPredicateSQL("shows"))
		for _, c := range conditions {
			q = q.Where(c)
		}
		suite.Require().NoError(q.Count(&n).Error)
		return n
	}

	night := shared.VenueLocalNightDateCondition
	past := shared.VenueLocalNightPastDateCondition
	// Three assertions, because a sum alone is satisfied by one row counted
	// twice beside one row counted by neither.
	suite.Equal(int64(0), count("("+night+") AND ("+past+")"), "no show may satisfy both conditions")
	suite.Equal(int64(0), count("NOT ("+night+") AND NOT ("+past+")"), "no show may satisfy neither")
	suite.Equal(int64(6), count(night)+count(past), "together they cover every show")
	suite.Positive(count(night))
	suite.Positive(count(past))
}
