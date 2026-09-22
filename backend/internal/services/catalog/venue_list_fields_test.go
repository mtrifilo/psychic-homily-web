package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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

// TestVenueListShowRef_CarriesTheScannedCancelledFlag pins the projection path
// that the integration cases below cannot reach: they assert false, which is
// also the field's zero value, so they would still pass if the pick stopped
// selecting the column. This one fails if the scanned flag stops reaching the
// wire shape.
func TestVenueListShowRef_CarriesTheScannedCancelledFlag(t *testing.T) {
	date := time.Now().UTC()
	slug, title := "a-show", "A Show"
	cancelled := true

	ref := venueListShowRef(&date, &slug, &title, &cancelled)
	if ref == nil {
		t.Fatal("venueListShowRef returned nil for a row that matched")
	}
	if !ref.IsCancelled {
		t.Error("venueListShowRef dropped the scanned is_cancelled flag")
	}

	// A NULL column is the scan-safety case, and false is the answer a client
	// can act on: absent means the pick said nothing, never "cancelled".
	if ref := venueListShowRef(&date, &slug, &title, nil); ref.IsCancelled {
		t.Error("a NULL is_cancelled column must reach the wire as false")
	}
}

// =============================================================================
// Integration: GET /venues row fields
// =============================================================================

// venueLocalZone is the zone an AZ venue created by createTestVenue resolves
// to: the row carries no timezone, so the SQL boundary and utils.EventLocation
// both fall through to the state map. Phoenix keeps no DST, so wall-clock
// arithmetic over it is exact, which is what lets the boundary tests below pin
// an edge.
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
	suite.Require().NotNil(row.LastShow)
	suite.Equal(*last.Slug, row.LastShow.Slug, "one second earlier is the previous night")
}

// TestGetVenuesWithShowCounts_RailAgreesWithTheRow pins the property a rail row
// has to have: the count and the show printed beside it name the same night.
//
// The fixture is the degenerate case, a room whose ONLY booking is a set already
// under way. A rail that picked its own next show on the request instant would
// leave this row saying "1 upcoming" with no date at all, which the Atlas rail
// renders as "nothing on the calendar".
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_RailAgreesWithTheRow() {
	venue := suite.createTestVenue("Rail Row", "Phoenix", "AZ", true)
	loc := venueLocalZone(suite.T())
	suite.Require().NoError(suite.db.Model(venue).Update("timezone", loc.String()).Error)
	user := suite.createTestUser()
	started := time.Now().In(loc).Add(-time.Hour)
	underWay := suite.createRailShow(venue.ID, user.ID, "Under Way", started, "Opening Band", "Headliner")

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{IncludeRailFields: true}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Rail Row")

	suite.Equal(1, row.UpcomingShowCount)
	suite.Require().NotNil(row.NextShow)
	suite.Equal(*underWay.Slug, row.NextShow.Slug)
	suite.Equal(started.Format("2006-01-02"), row.NextShowDate,
		"the rail date is the row's own pick, rendered in the venue's zone")
	suite.Equal("Under Way", row.NextShowTitle)
	suite.Equal([]string{"Opening Band", "Headliner"}, row.NextShowArtists,
		"the bill is the bill of the show the row picked, in position order")
}

// TestGetVenuesWithShowCounts_RailDatesAZonelessRoomLikeItsBoundary covers the
// rooms production actually has trouble with: the ones with no geocoded zone.
//
// The boundary that picks the show resolves those through the US state map, so
// the date printed beside the pick has to resolve them the same way. Rendering
// them in UTC would put an evening show on the following day for every western
// room whose zone was never filled in.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_RailDatesAZonelessRoomLikeItsBoundary() {
	// createTestVenue leaves timezone NULL, which is the state eight of
	// production's rooms are in.
	venue := suite.createTestVenue("Zoneless Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	loc := venueLocalZone(suite.T())
	// 21:00 tonight in Phoenix is tomorrow's date in UTC.
	y, m, d := time.Now().In(loc).Date()
	tonight := time.Date(y, m, d, 21, 0, 0, 0, loc)
	suite.createRailShow(venue.ID, user.ID, "Tonight", tonight)

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{IncludeRailFields: true}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Zoneless Room")

	suite.Require().Nil(row.Timezone, "the fixture is a room with no stored zone")
	suite.Equal(1, row.UpcomingShowCount)
	suite.Equal(tonight.Format("2006-01-02"), row.NextShowDate,
		"the printed date names the venue-local day, not the UTC one")
}

// TestGetVenuesWithShowCounts_RailIsSilentWithoutAnUpcomingShow is the other
// half: a quiet room carries no rail date rather than an empty-looking one.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_RailIsSilentWithoutAnUpcomingShow() {
	venue := suite.createTestVenue("Rail Quiet Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.createRailShow(venue.ID, user.ID, "Long Gone", time.Now().UTC().AddDate(0, 0, -30), "Some Band")

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{IncludeRailFields: true}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Rail Quiet Room")

	suite.Equal(0, row.UpcomingShowCount)
	suite.Nil(row.NextShow)
	suite.Empty(row.NextShowDate)
	suite.Empty(row.NextShowTitle)
	suite.Empty(row.NextShowArtists)
	suite.Require().NotNil(row.LastShow, "a quiet room still names the night it last had one")
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
		resp, totals, err := suite.venueService.GetVenuesWithShowCounts(
			contracts.VenueListFilters{Sort: tc.sort}, 10, 0)
		suite.Require().NoErrorf(err, "sort=%s", tc.sort)
		suite.Equalf(int64(5), totals.Venues, "sort=%s", tc.sort)
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

	// Reads the production gate rather than restating it, so what this counts is
	// what the endpoint counts.
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

// =============================================================================
// Integration: cancelled nights
// =============================================================================

// cancelShow marks a created show cancelled, the state a promoter's
// cancellation leaves the row in.
func (suite *VenueServiceIntegrationTestSuite) cancelShow(show *catalogm.Show) {
	suite.Require().NoError(suite.db.Model(show).Update("is_cancelled", true).Error)
}

// TestGetVenuesWithShowCounts_NextShowSkipsACancelledNight is the ticket's
// headline acceptance: the column answers when the room's next show will
// happen, so a cancelled night is passed over for the one behind it.
//
// It also pins the count beside it, which is drawn on the same filtered set: a
// row that skipped a night must not still be counting it.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_NextShowSkipsACancelledNight() {
	venue := suite.createTestVenue("Cancelled Next Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.cancelShow(suite.createRailShow(venue.ID, user.ID, "Called Off", time.Now().UTC().AddDate(0, 0, 3)))
	standing := suite.createRailShow(venue.ID, user.ID, "Still On", time.Now().UTC().AddDate(0, 0, 10))

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Cancelled Next Room")

	suite.Require().NotNil(row.NextShow)
	suite.Equal(*standing.Slug, row.NextShow.Slug, "the soonest show that will happen is the next show")
	suite.False(row.NextShow.IsCancelled, "the pick selects from uncancelled shows only")
	suite.Equal(1, row.UpcomingShowCount, "the cancelled night is outside the count as well as the pick")
}

// TestGetVenuesWithShowCounts_RoomWithOnlyACancelledNightIsQuiet is the other
// half of the count rule: with nothing left that will happen, the room falls
// into the quiet block rather than reading one upcoming show with nothing to
// name.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_RoomWithOnlyACancelledNightIsQuiet() {
	venue := suite.createTestVenue("Only Cancelled Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.cancelShow(suite.createRailShow(venue.ID, user.ID, "Called Off", time.Now().UTC().AddDate(0, 0, 5)))
	played := suite.createRailShow(venue.ID, user.ID, "Played", time.Now().UTC().AddDate(0, 0, -20))

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Only Cancelled Room")

	suite.Equal(0, row.UpcomingShowCount)
	suite.Nil(row.NextShow, "next_show is absent exactly when the count is zero")
	suite.Require().NotNil(row.LastShow)
	suite.Equal(*played.Slug, row.LastShow.Slug, "a quiet room still names the night it last had one")
}

// TestGetVenuesWithShowCounts_LastShowSkipsACancelledNight covers the quiet
// block's column: LAST SHOW names the night the room last had a show, and a
// cancelled night was never one.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_LastShowSkipsACancelledNight() {
	venue := suite.createTestVenue("Cancelled Last Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.cancelShow(suite.createRailShow(venue.ID, user.ID, "Called Off", time.Now().UTC().AddDate(0, 0, -5)))
	played := suite.createRailShow(venue.ID, user.ID, "Played", time.Now().UTC().AddDate(0, 0, -30))

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Cancelled Last Room")

	suite.Require().NotNil(row.LastShow)
	suite.Equal(*played.Slug, row.LastShow.Slug, "the most recent night that happened is the last show")
	suite.False(row.LastShow.IsCancelled, "the pick selects from uncancelled shows only")
}

// TestGetVenuesWithShowCounts_CancelledNightLosesToItsOwnDate covers the
// tie-break path: two shows on the SAME instant, one of them called off. The
// order alone cannot separate them, so only the filter can.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_CancelledNightLosesToItsOwnDate() {
	venue := suite.createTestVenue("Same Night Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	when := time.Now().UTC().AddDate(0, 0, 6)
	// Created FIRST, so it also holds the lower id that the tie-break reads.
	suite.cancelShow(suite.createRailShow(venue.ID, user.ID, "Called Off", when))
	standing := suite.createRailShow(venue.ID, user.ID, "Still On", when)

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Same Night Room")

	suite.Require().NotNil(row.NextShow)
	suite.Equal(*standing.Slug, row.NextShow.Slug,
		"a cancelled show must lose to a standing one on its own date, whatever the id order says")
	suite.Equal(1, row.UpcomingShowCount)
}

// TestGetVenuesWithShowCounts_RoomWhoseOnlyPastNightWasCancelledHasNoLastShow
// is the absent state on the quiet side: with nothing behind it that happened,
// the room names no last show rather than naming the night that did not.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_RoomWhoseOnlyPastNightWasCancelledHasNoLastShow() {
	venue := suite.createTestVenue("No Past Room", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.cancelShow(suite.createRailShow(venue.ID, user.ID, "Called Off", time.Now().UTC().AddDate(0, 0, -12)))

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "No Past Room")

	suite.Equal(0, row.UpcomingShowCount)
	suite.Nil(row.NextShow)
	suite.Nil(row.LastShow, "a cancelled night is not a night the room had a show")
}

// TestGetVenuesWithShowCounts_RailFollowsTheUncancelledPick pins the rail meta
// line against the same rule: the date and bill printed beside the row belong
// to the show the row picked, so they move with it when a night is called off.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_RailFollowsTheUncancelledPick() {
	venue := suite.createTestVenue("Cancelled Rail Room", "Phoenix", "AZ", true)
	loc := venueLocalZone(suite.T())
	suite.Require().NoError(suite.db.Model(venue).Update("timezone", loc.String()).Error)
	user := suite.createTestUser()
	suite.cancelShow(suite.createRailShow(venue.ID, user.ID, "Called Off",
		time.Now().In(loc).AddDate(0, 0, 2), "Cancelled Band"))
	standingAt := time.Now().In(loc).AddDate(0, 0, 9)
	standing := suite.createRailShow(venue.ID, user.ID, "Still On", standingAt, "Standing Band")

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{IncludeRailFields: true}, 10, 0)
	suite.Require().NoError(err)
	row := suite.findVenueResponse(resp, "Cancelled Rail Room")

	suite.Require().NotNil(row.NextShow)
	suite.Equal(*standing.Slug, row.NextShow.Slug)
	suite.Equal(standingAt.Format("2006-01-02"), row.NextShowDate)
	suite.Equal("Still On", row.NextShowTitle)
	suite.Equal([]string{"Standing Band"}, row.NextShowArtists,
		"the bill is the bill of the show the row picked")
	suite.Equal(0, row.ShowsThisWeek,
		"the only night inside the window is cancelled, so the chip must not keep this room")
}

// =============================================================================
// Integration: the list TOTALS
// =============================================================================

// TestGetVenuesWithShowCounts_UpcomingTotalSpansEveryPage is the ticket's
// headline acceptance: the upcoming total describes the whole filtered set, so
// a caption drawn from it says the same thing on every page.
//
// The rooms carry different counts and one is quiet, so a page's own rows sum
// to a different number on each page: that is what makes the assertion mean
// "not the page" rather than only "some number".
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_UpcomingTotalSpansEveryPage() {
	user := suite.createTestUser()
	busy := suite.createTestVenue("Totals Busy Room", "Phoenix", "AZ", true)
	middling := suite.createTestVenue("Totals Middling Room", "Phoenix", "AZ", true)
	suite.createTestVenue("Totals Quiet Room", "Phoenix", "AZ", true)
	for i := 0; i < 3; i++ {
		suite.createRailShow(busy.ID, user.ID, "Busy", time.Now().UTC().AddDate(0, 0, 3+i))
	}
	suite.createRailShow(middling.ID, user.ID, "Middling", time.Now().UTC().AddDate(0, 0, 5))

	var pageSums []int64
	for offset := 0; offset < 3; offset++ {
		page, totals, err := suite.venueService.GetVenuesWithShowCounts(
			contracts.VenueListFilters{}, 1, offset)
		suite.Require().NoErrorf(err, "offset=%d", offset)
		suite.Require().Lenf(page, 1, "offset=%d", offset)
		suite.Equalf(int64(3), totals.Venues, "offset=%d", offset)
		suite.Equalf(int64(4), totals.UpcomingShows,
			"offset=%d: the upcoming total spans every page, quiet rooms contributing zero", offset)
		pageSums = append(pageSums, int64(page[0].UpcomingShowCount))
	}
	suite.Equal([]int64{3, 1, 0}, pageSums,
		"each page sums to something different, so the total above cannot be the page's own")
	var acrossPages int64
	for _, n := range pageSums {
		acrossPages += n
	}
	suite.Equal(int64(4), acrossPages,
		"the rows of every page add up to the total each page reported")
}

// TestGetVenuesWithShowCounts_UpcomingTotalNarrowsWithTheFilters pins the
// total to the same set the rows are drawn from: the caption above a filtered
// list counts what that list holds, never the catalogue behind it.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_UpcomingTotalNarrowsWithTheFilters() {
	user := suite.createTestUser()
	slug := tagSlugFor("venues-upcoming-total")
	tagID := suite.createGenreTag(slug, slug)

	phoenix := suite.createTestVenue("Totals PHX Room", "Phoenix", "AZ", true)
	tucson := suite.createTestVenue("Totals TUC Room", "Tucson", "AZ", true)
	unverified := suite.createTestVenue("Totals Unverified Room", "Phoenix", "AZ", false)
	suite.tagVenue(phoenix.ID, tagID, user.ID)
	suite.tagVenue(unverified.ID, tagID, user.ID)

	for i := 0; i < 2; i++ {
		suite.createRailShow(phoenix.ID, user.ID, "PHX", time.Now().UTC().AddDate(0, 0, 3+i))
	}
	suite.createRailShow(tucson.ID, user.ID, "TUC", time.Now().UTC().AddDate(0, 0, 4))
	suite.createRailShow(unverified.ID, user.ID, "Hidden", time.Now().UTC().AddDate(0, 0, 5))

	_, unfiltered, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), unfiltered.Venues, "the browse gate holds: an unverified room is outside the set")
	suite.Equal(int64(3), unfiltered.UpcomingShows,
		"the unverified room's night is outside the total as well as the list")

	_, cityScoped, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Phoenix"}, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), cityScoped.Venues)
	suite.Equal(int64(2), cityScoped.UpcomingShows, "a city scope counts that city's nights only")

	_, tagScoped, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{TagSlugs: []string{slug}}, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), tagScoped.Venues)
	suite.Equal(int64(2), tagScoped.UpcomingShows, "a tag scope counts the tagged rooms' nights only")
}

// TestGetVenuesWithShowCounts_UpcomingTotalCountsWhatTheRowsCount is the
// agreement property: the total is the same number a reader would reach by
// adding up every row, on the same boundary and with the same exclusions. A
// cancelled night and a past one are both outside it.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_UpcomingTotalCountsWhatTheRowsCount() {
	user := suite.createTestUser()
	room := suite.createTestVenue("Totals Mixed Room", "Phoenix", "AZ", true)
	other := suite.createTestVenue("Totals Other Room", "Phoenix", "AZ", true)

	suite.createRailShow(room.ID, user.ID, "Standing", time.Now().UTC().AddDate(0, 0, 6))
	suite.cancelShow(suite.createRailShow(room.ID, user.ID, "Called Off", time.Now().UTC().AddDate(0, 0, 7)))
	suite.createRailShow(room.ID, user.ID, "Gone By", time.Now().UTC().AddDate(0, 0, -7))
	suite.createRailShow(other.ID, user.ID, "Standing Too", time.Now().UTC().AddDate(0, 0, 8))

	rows, totals, err := suite.venueService.GetVenuesWithShowCounts(contracts.VenueListFilters{}, 50, 0)
	suite.Require().NoError(err)

	var summed int64
	for _, r := range rows {
		summed += int64(r.UpcomingShowCount)
	}
	suite.Equal(int64(2), summed, "a cancelled night and a past one are outside the rows' own counts")
	suite.Equal(summed, totals.UpcomingShows,
		"the total is what the rows add up to when one page holds them all")
}
