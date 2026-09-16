package catalog

import (
	"fmt"
	"strings"
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
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

// TestVenueListOrderBy_QuietBlockIsGuardedUnderEverySort pins the structural
// property the two-block order depends on: the quiet-room term and every
// active-block term are CASE-guarded on the same count expression, so neither
// block can order the other.
func TestVenueListOrderBy_QuietBlockIsGuardedUnderEverySort(t *testing.T) {
	quietLead := "(" + venueListCountSQL + " = 0) ASC, "
	quietTerm := "CASE WHEN " + venueListCountSQL + " = 0 THEN last_show.event_date END DESC NULLS LAST"
	for _, sort := range contracts.VenueListSortValues {
		order, err := venueListOrderBy(sort)
		if err != nil {
			t.Fatalf("venueListOrderBy(%q): %v", sort, err)
		}
		if !strings.HasPrefix(order, quietLead) {
			t.Errorf("sort=%q order %q does not lead with the quiet-room split %q", sort, order, quietLead)
		}
		if !strings.Contains(order, quietTerm) {
			t.Errorf("sort=%q order %q does not carry the guarded last-show term", sort, order)
		}
		if !strings.HasSuffix(order, "venues.id ASC") {
			t.Errorf("sort=%q order %q does not end in a total key; offset pages could overlap", sort, order)
		}
	}
}

// =============================================================================
// Integration: GET /venues row fields
// =============================================================================

// createListShow makes one approved show at a venue with a slug and a title, so
// the next/last projections can be asserted on more than a date.
func (suite *VenueServiceIntegrationTestSuite) createListShow(
	venueID, userID uint, title string, eventDate time.Time,
) *catalogm.Show {
	slug := fmt.Sprintf("psy2076-%d", time.Now().UnixNano())
	show := &catalogm.Show{
		Title:       title,
		Slug:        &slug,
		EventDate:   eventDate,
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)
	return show
}

// TestGetVenuesWithShowCounts_CountHoldsAShowUnderWay is the ticket's headline
// acceptance: a set that started an hour ago is still an upcoming listing, and
// the /venues number equals the venue page's own upcoming total for it.
//
// The two are drawn on different bounds (night here, venue-local midnight
// there) and agree everywhere except between midnight and NightStartHour, where
// this one is the wider set. The row used here is on TODAY's venue-local date,
// which both bounds keep whatever the hour, so the assertion does not depend on
// when the suite runs.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_CountHoldsAShowUnderWay() {
	venue := suite.createTestVenue("Under Way Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.createListShow(venue.ID, user.ID, "Doors Open", time.Now().UTC().Add(-time.Hour))

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Under Way Room")
	suite.Equal(1, row.UpcomingShowCount, "a set already under way is still an upcoming listing")

	_, venuePageTotal, err := suite.venueService.GetShowsForVenue(venue.ID, "", contracts.VenueShowsQuery{
		TimeFilter: "upcoming",
		Limit:      10,
	})
	suite.Require().NoError(err)
	suite.Equal(venuePageTotal, int64(row.UpcomingShowCount),
		"the directory count and the venue page's own upcoming total must agree")
}

// TestGetVenuesWithShowCounts_NextAndLastPartitionTheRoom pins the invariant the
// whole row depends on: one boundary decides both picks, so the count is above
// zero exactly when next_show is set, and no show is ever both.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_NextAndLastPartitionTheRoom() {
	active := suite.createTestVenue("Active Room", "Phoenix", "AZ", true)
	quiet := suite.createTestVenue("Quiet Room", "Phoenix", "AZ", true)
	suite.createTestVenue("Empty Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	soon := suite.createListShow(active.ID, user.ID, "Soonest", time.Now().UTC().AddDate(0, 0, 3))
	suite.createListShow(active.ID, user.ID, "Later", time.Now().UTC().AddDate(0, 0, 30))
	activePast := suite.createListShow(active.ID, user.ID, "Old Night", time.Now().UTC().AddDate(0, 0, -40))
	quietRecent := suite.createListShow(quiet.ID, user.ID, "Last Night Here", time.Now().UTC().AddDate(0, 0, -10))
	suite.createListShow(quiet.ID, user.ID, "Ancient", time.Now().UTC().AddDate(0, 0, -400))

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
		suite.createListShow(busiest.ID, user.ID, "B", time.Now().UTC().AddDate(0, 0, 10+i))
	}
	suite.createListShow(soonest.ID, user.ID, "S", time.Now().UTC().AddDate(0, 0, 2))
	suite.createListShow(alpha.ID, user.ID, "A", time.Now().UTC().AddDate(0, 0, 20))
	suite.createListShow(quietRecent.ID, user.ID, "QR", time.Now().UTC().AddDate(0, 0, -5))
	suite.createListShow(quietOld.ID, user.ID, "QO", time.Now().UTC().AddDate(0, 0, -50))

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
	suite.createListShow(few.ID, user.ID, "F", time.Now().UTC().AddDate(0, 0, 5))
	suite.createListShow(many.ID, user.ID, "M1", time.Now().UTC().AddDate(0, 0, 6))
	suite.createListShow(many.ID, user.ID, "M2", time.Now().UTC().AddDate(0, 0, 7))

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
	suite.createListShow(suite.createTestVenue("Twin", "Phoenix", "AZ", true).ID, user.ID, "P", time.Now().UTC().AddDate(0, 0, 4))
	suite.createListShow(suite.createTestVenue("Twin", "Tucson", "AZ", true).ID, user.ID, "T", time.Now().UTC().AddDate(0, 0, 4))
	for i := 0; i < 3; i++ {
		v := suite.createTestVenue(fmt.Sprintf("Quiet %d", i), "Phoenix", "AZ", true)
		suite.createListShow(v.ID, user.ID, "Q", time.Now().UTC().AddDate(0, 0, -(i+1)))
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

// TestVenueLocalNightConditions_PartitionEveryShow pins the complement claim
// the next/last pair rests on, in Postgres rather than by reading the strings:
// every approved show satisfies exactly one of the two conditions.
func (suite *VenueServiceIntegrationTestSuite) TestVenueLocalNightConditions_PartitionEveryShow() {
	venue := suite.createTestVenue("Partition Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	for _, offset := range []time.Duration{
		-400 * 24 * time.Hour, -36 * time.Hour, -time.Hour, time.Hour, 36 * time.Hour, 400 * 24 * time.Hour,
	} {
		suite.createListShow(venue.ID, user.ID, "P", time.Now().UTC().Add(offset))
	}

	count := func(condition string) int64 {
		var n int64
		suite.Require().NoError(suite.db.Table("show_venues").
			Joins("JOIN shows ON shows.id = show_venues.show_id").
			Joins(shared.VenueTZJoin).
			Where("show_venues.venue_id = ? AND shows.status = ?", venue.ID, catalogm.ShowStatusApproved).
			Where(condition).
			Count(&n).Error)
		return n
	}

	upcoming := count(shared.VenueLocalNightDateCondition)
	past := count(shared.VenueLocalNightPastDateCondition)
	suite.Equal(int64(6), upcoming+past, "the night condition and its complement must cover every show exactly once")
	suite.Positive(upcoming)
	suite.Positive(past)
}
