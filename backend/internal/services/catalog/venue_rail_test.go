package catalog

import (
	"fmt"
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// Unit: venueLocalDate
// =============================================================================

// TestVenueLocalDate pins the rail's date rule: the calendar date a person
// standing AT THE VENUE would name, not the UTC one. A 9pm Friday show in
// Austin is a Saturday-morning UTC timestamp; rendering it in UTC would put
// "NEXT Sat" on a Friday show.
func TestVenueLocalDate(t *testing.T) {
	// 2026-08-01 02:00 UTC == 2026-07-31 21:00 in Chicago (Austin's zone).
	fridayNight := time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC)
	austin := "America/Chicago"

	if got := venueLocalDate(fridayNight, &austin); got != "2026-07-31" {
		t.Errorf("venue-local date = %q, want 2026-07-31 (the Friday the show is on)", got)
	}
	if got := venueLocalDate(fridayNight, nil); got != "2026-08-01" {
		t.Errorf("nil-timezone date = %q, want the UTC fallback 2026-08-01", got)
	}
	empty := ""
	if got := venueLocalDate(fridayNight, &empty); got != "2026-08-01" {
		t.Errorf("empty-timezone date = %q, want the UTC fallback 2026-08-01", got)
	}
	junk := "Not/AZone"
	if got := venueLocalDate(fridayNight, &junk); got != "2026-08-01" {
		t.Errorf("unloadable-timezone date = %q, want the UTC fallback 2026-08-01", got)
	}
}

// =============================================================================
// Integration: GET /venues rail payload
// =============================================================================

// createRailShow makes one approved show at a venue, on a given date, with the
// given title and bill (in listed order).
func (suite *VenueServiceIntegrationTestSuite) createRailShow(
	venueID, userID uint, title string, eventDate time.Time, artistNames ...string,
) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   eventDate,
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)
	for i, name := range artistNames {
		a := suite.createArtist(name)
		suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
			ShowID: show.ID, ArtistID: a.ID, Position: i,
		}).Error)
	}
	return show
}

func (suite *VenueServiceIntegrationTestSuite) findVenueResponse(
	venues []*contracts.VenueWithShowCountResponse, name string,
) *contracts.VenueWithShowCountResponse {
	for _, v := range venues {
		if v.Name == name {
			return v
		}
	}
	suite.Require().Failf("venue missing from list", "no venue named %q in the response", name)
	return nil
}

// TestGetVenuesWithShowCounts_NextShowFields covers the rail's meta line: the
// SOONEST upcoming show wins (not the first created), its bill comes back in
// position order, and a titled show keeps its title.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_NextShowFields() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Rail Next Show", "Austin", "TX", true)
	now := time.Now().UTC()

	// Created far-first, near-second: proves the pick is by date, not by id.
	suite.createRailShow(venue.ID, user.ID, "Levitation pre-party", now.AddDate(0, 0, 20), "Far Band")
	suite.createRailShow(venue.ID, user.ID, "", now.AddDate(0, 0, 3), "Gouge Away", "Militarie Gun")
	// A PAST show must never win the "next" slot.
	suite.createRailShow(venue.ID, user.ID, "Last month", now.AddDate(0, 0, -10), "Old Band")

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	got := suite.findVenueResponse(venues, "Rail Next Show")
	suite.Equal(now.AddDate(0, 0, 3).Format("2006-01-02"), got.NextShowDate,
		"next show must be the soonest UPCOMING one")
	suite.Equal("", got.NextShowTitle, "titleless show must not invent a title")
	suite.Equal([]string{"Gouge Away", "Militarie Gun"}, got.NextShowArtists,
		"bill must come back in position order so the client can compose a display name")
	suite.Equal(2, got.UpcomingShowCount, "past show must not count as upcoming")
}

// TestGetVenuesWithShowCounts_ThisWeekSlice pins the "Next 7 days" chip's data:
// only upcoming shows inside the 7-day window count.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_ThisWeekSlice() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Rail This Week", "Austin", "TX", true)
	now := time.Now().UTC()

	suite.createRailShow(venue.ID, user.ID, "In two days", now.AddDate(0, 0, 2))
	suite.createRailShow(venue.ID, user.ID, "In six days", now.AddDate(0, 0, 6))
	suite.createRailShow(venue.ID, user.ID, "In three weeks", now.AddDate(0, 0, 21))

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	got := suite.findVenueResponse(venues, "Rail This Week")
	suite.Equal(3, got.UpcomingShowCount)
	suite.Equal(2, got.ShowsThisWeek, "only the <=7-day rolling slice counts")
}

// TestGetVenuesWithShowCounts_NoUpcomingShows: a venue with nothing booked
// reports zeros and an EMPTY next-show date, never a stale or zero-time one.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_NoUpcomingShows() {
	suite.createTestVenue("Rail Quiet Venue", "Austin", "TX", true)

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	got := suite.findVenueResponse(venues, "Rail Quiet Venue")
	suite.Equal(0, got.UpcomingShowCount)
	suite.Equal(0, got.ShowsThisWeek)
	suite.Equal("", got.NextShowDate)
	suite.Empty(got.NextShowArtists)
	suite.Equal("", got.DominantGenre)
}

// TestGetVenuesWithShowCounts_RailFieldsAreOptIn pins the cost gate: the venue
// browse page calls this same method and renders none of the rail fields, so
// without the opt-in it must not pay for the three aggregations that fill them.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_RailFieldsAreOptIn() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Rail Opt In", "Austin", "TX", true)
	suite.createRailShow(venue.ID, user.ID, "Soon", time.Now().UTC().AddDate(0, 0, 2), "A Band")

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX"}, 50, 0)
	suite.Require().NoError(err)

	got := suite.findVenueResponse(venues, "Rail Opt In")
	suite.Equal(1, got.UpcomingShowCount, "the count itself is not part of the opt-in")
	suite.Equal(0, got.ShowsThisWeek)
	suite.Equal("", got.NextShowDate)
	suite.Empty(got.NextShowArtists)
	suite.Equal("", got.DominantGenre)
}

// TestGetVenuesWithShowCounts_DominantGenreIgnoresAncientHistory pins the genre
// window: a venue whose only tagged bookings predate it reads as untinted, so
// the rail describes what a venue books NOW.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_DominantGenreIgnoresAncientHistory() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Rail Ancient Venue", "Austin", "TX", true)
	punkTag := suite.createGenreTag("rail-ancient-punk", "punk")
	longAgo := time.Now().UTC().AddDate(0, -(venueGenreWindowMonths + 6), 0)

	for i := 0; i < 6; i++ {
		show := suite.createRailShow(venue.ID, user.ID, "", longAgo.AddDate(0, 0, i),
			fmt.Sprintf("Rail Ancient Band %d", i))
		var artistID uint
		suite.Require().NoError(suite.db.Raw(
			`SELECT artist_id FROM show_artists WHERE show_id = ?`, show.ID).Scan(&artistID).Error)
		suite.tagArtist(artistID, punkTag, user.ID)
	}

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	got := suite.findVenueResponse(venues, "Rail Ancient Venue")
	suite.Equal("", got.DominantGenre, "bookings outside the window must not tint the rail")
}

// TestGetVenuesWithShowCounts_DominantGenre proves the rail's genre column uses
// the SAME family keys and confident-dominance rule scenes use.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_DominantGenre() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Rail Punk Venue", "Austin", "TX", true)
	punkTag := suite.createGenreTag("rail-punk", "punk")
	now := time.Now().UTC()

	// Six distinct punk-tagged artists clears dominantGenreFamilyMinTagged.
	for i := 0; i < 6; i++ {
		show := suite.createRailShow(venue.ID, user.ID, "", now.AddDate(0, 0, i+1),
			fmt.Sprintf("Rail Punk Band %d", i))
		var artistID uint
		suite.Require().NoError(suite.db.Raw(
			`SELECT artist_id FROM show_artists WHERE show_id = ?`, show.ID).Scan(&artistID).Error)
		suite.tagArtist(artistID, punkTag, user.ID)
	}

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	got := suite.findVenueResponse(venues, "Rail Punk Venue")
	suite.Equal("punk_hardcore", got.DominantGenre)
}

// TestGetVenuesWithShowCounts_DominantGenreNeutralWhenThin mirrors the scene
// rule's other half: too little tagged mass stays neutral rather than reading
// a 1-artist venue as "100% punk".
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_DominantGenreNeutralWhenThin() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Rail Thin Venue", "Austin", "TX", true)
	punkTag := suite.createGenreTag("rail-thin-punk", "hardcore")
	now := time.Now().UTC()

	show := suite.createRailShow(venue.ID, user.ID, "", now.AddDate(0, 0, 1), "Rail Thin Band")
	var artistID uint
	suite.Require().NoError(suite.db.Raw(
		`SELECT artist_id FROM show_artists WHERE show_id = ?`, show.ID).Scan(&artistID).Error)
	suite.tagArtist(artistID, punkTag, user.ID)

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	got := suite.findVenueResponse(venues, "Rail Thin Venue")
	suite.Equal("", got.DominantGenre, "one tagged artist is not confident dominance")
}

// TestGetVenuesWithShowCounts_UnverifiedVenueNeverStreetMapped is the PRIVACY
// GATE (PSY-1536), asserted at the layer the Atlas rail actually reads: an
// unverified venue with street coordinates on the row must not appear in the
// list at all, and a verified one must not leak street coordinates it has no
// fresh geocode for — either way the pin falls back to the city centroid.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_UnverifiedVenueNeverStreetMapped() {
	lat, lng := 30.2672, -97.7431

	unverified := suite.createTestVenue("Rail House Venue", "Austin", "TX", false)
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", unverified.ID).
		Updates(map[string]any{"street_latitude": lat, "street_longitude": lng}).Error)

	// Verified, street coords present, but NO geocoded_address key -> not fresh.
	stale := suite.createTestVenue("Rail Stale Geocode", "Austin", "TX", true)
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", stale.ID).
		Updates(map[string]any{"street_latitude": lat, "street_longitude": lng}).Error)

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	for _, v := range venues {
		suite.NotEqual("Rail House Venue", v.Name,
			"unverified venues must never reach the public venue list")
	}
	got := suite.findVenueResponse(venues, "Rail Stale Geocode")
	suite.Nil(got.StreetLatitude, "a stale geocode must not be served as a street pin")
	suite.Nil(got.StreetLongitude)
}

// createOtherTag inserts a category='other' tag and returns its ID. The
// all-ages tag is not a genre, so it cannot reuse createGenreTag.
func (suite *VenueServiceIntegrationTestSuite) createOtherTag(name, slug string) uint {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	var tagID uint
	err = sqlDB.QueryRow(`
		INSERT INTO tags (name, slug, category, is_official, usage_count, created_at, updated_at)
		VALUES ($1, $2, 'other', true, 0, NOW(), NOW())
		RETURNING id
	`, name, slug).Scan(&tagID)
	suite.Require().NoError(err)
	return tagID
}

func (suite *VenueServiceIntegrationTestSuite) tagVenue(venueID, tagID, userID uint) {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, err = sqlDB.Exec(`
		INSERT INTO entity_tags (entity_type, entity_id, tag_id, added_by_user_id, created_at)
		VALUES ('venue', $1, $2, $3, NOW())
	`, venueID, tagID, userID)
	suite.Require().NoError(err)
}

// TestGetVenuesWithShowCounts_HostsAllAges pins the "All-ages shows" chip's
// data (PSY-1573): the flag follows the canonical tag and nothing else.
//
// The untagged venue is the half that matters. Its false is "nobody has said",
// not "this room is 21+" — the tag has near-zero coverage — which is why the
// rail's empty state has to name the tag rather than report a fact about the
// city.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_HostsAllAges() {
	user := suite.createTestUser()
	tagged := suite.createTestVenue("Rail All Ages Room", "Austin", "TX", true)
	suite.createTestVenue("Rail Untagged Room", "Austin", "TX", true)

	suite.tagVenue(tagged.ID, suite.createOtherTag("All Ages", catalogm.TagSlugAllAges), user.ID)

	// A second, unrelated `other` tag on the SAME venue proves the lookup keys
	// on the slug rather than on the category.
	suite.tagVenue(tagged.ID, suite.createOtherTag("Full Bar", "rail-full-bar"), user.ID)

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	suite.True(suite.findVenueResponse(venues, "Rail All Ages Room").HostsAllAges,
		"a venue carrying the canonical all-ages tag must report it")
	suite.False(suite.findVenueResponse(venues, "Rail Untagged Room").HostsAllAges,
		"an untagged venue reports false, which the client must read as unknown")
}

// TestGetVenuesWithShowCounts_HostsAllAgesIgnoresAgePolicy keeps the tag and
// the venues.age_policy column apart (PSY-1682 vs PSY-1573). The column is the
// HOUSE DEFAULT; the tag is "hosts all-ages shows sometimes". Deriving the flag
// from the column would both invent tags for rooms nobody vouched for and hide
// the 21+ room that books an all-ages matinee — the case the chip exists for.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_HostsAllAgesIgnoresAgePolicy() {
	user := suite.createTestUser()
	byPolicy := suite.createTestVenue("Rail Policy Only Room", "Austin", "TX", true)
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", byPolicy.ID).
		Update("age_policy", "all ages").Error)

	byTag := suite.createTestVenue("Rail Tagged 21plus Room", "Austin", "TX", true)
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("id = ?", byTag.ID).
		Update("age_policy", "21+").Error)
	suite.tagVenue(byTag.ID, suite.createOtherTag("All Ages", catalogm.TagSlugAllAges), user.ID)

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX", IncludeRailFields: true}, 50, 0)
	suite.Require().NoError(err)

	suite.False(suite.findVenueResponse(venues, "Rail Policy Only Room").HostsAllAges,
		`age_policy "all ages" is a house default, not the community tag`)
	suite.True(suite.findVenueResponse(venues, "Rail Tagged 21plus Room").HostsAllAges,
		"a 21+ house that books all-ages shows is exactly what the tag is for")
}

// TestGetVenuesWithShowCounts_HostsAllAgesIsOptIn: the flag rides
// IncludeRailFields with the rest of the rail payload, so the venue browse
// page does not pay for a scan it renders nothing from.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_HostsAllAgesIsOptIn() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Rail All Ages Opt In", "Austin", "TX", true)
	suite.tagVenue(venue.ID, suite.createOtherTag("All Ages", catalogm.TagSlugAllAges), user.ID)

	venues, _, err := suite.venueService.GetVenuesWithShowCounts(
		contracts.VenueListFilters{City: "Austin", State: "TX"}, 50, 0)
	suite.Require().NoError(err)

	suite.False(suite.findVenueResponse(venues, "Rail All Ages Opt In").HostsAllAges,
		"the all-ages flag must not be filled without the rail opt-in")
}
