package catalog

import (
	"fmt"
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// artistCitiesScope subtracts the place keys and carries everything else, so the
// closed set it enumerates is the one artistBrowseScope owns. A narrowing added
// to the list travels by construction; a place key that stopped being subtracted
// would narrow the breakdown to the place already picked.
func TestArtistCitiesScope_SubtractsPlaceKeysAndCarriesTheRest(t *testing.T) {
	scope := artistCitiesScope(map[string]interface{}{
		"cities":                []map[string]string{{"city": "Phoenix", "state": "AZ"}},
		"city":                  "Phoenix",
		"state":                 "AZ",
		"tag_filter":            TagFilter{TagSlugs: []string{"shoegaze"}},
		"skip_active_filter":    true,
		FilterMissingListenLink: true,
		"a_filter_added_later":  true,
	})

	for _, key := range browsePlaceKeys {
		if _, ok := scope[key]; ok {
			t.Errorf("%q names a place and must not reach the breakdown", key)
		}
	}
	for _, key := range []string{
		"tag_filter", "skip_active_filter", FilterMissingListenLink, "a_filter_added_later",
	} {
		if _, ok := scope[key]; !ok {
			t.Errorf("%q narrows the list and must reach the breakdown", key)
		}
	}
}

// The city facet's ordinal premise, one test per endpoint: under a given filter
// set, each city's count is the total its list reports for that city, so the SUM
// over every city is the total for no city at all.
//
// It is the premise the bottom sheet's apply button is derived from ("Show N
// venues" is the sum of the ticked rows' counts), so a facet counted on a wider
// set than the list it filters puts a number on that button the page then
// contradicts.
//
// Every fixture below is tagged with a slug unique to its test, and every
// assertion is made under that tag. That is what makes the sum exact rather than
// approximate: the suites share one database across their tests, so an
// unfiltered sum would also count rows seeded by neighbours.
//
// The invariant has ONE documented exemption, exercised by the third case here:
// a row carrying no city or state belongs to the list and to no facet row,
// because there is no place to file it under.

// tagSlugFor mints a slug no other test in the package uses, so a fixture is
// only ever counted by the test that seeded it.
func tagSlugFor(name string) string {
	return fmt.Sprintf("facet-%s-%d", name, time.Now().UnixNano())
}

func sumVenueCityCounts(cities []*contracts.VenueCityResponse) int64 {
	var sum int64
	for _, c := range cities {
		sum += int64(c.VenueCount)
	}
	return sum
}

func sumShowCityCounts(cities []contracts.ShowCityResponse) int64 {
	var sum int64
	for _, c := range cities {
		sum += int64(c.ShowCount)
	}
	return sum
}

func sumArtistCityCounts(cities []*contracts.ArtistCityResponse) int64 {
	var sum int64
	for _, c := range cities {
		sum += int64(c.ArtistCount)
	}
	return sum
}

// createArtistInCity places an artist, which createTestArtist deliberately does
// not: a facet keyed on (city, state) needs both columns set, and the empty pair
// is the exemption this file documents.
func (suite *ArtistServiceIntegrationTestSuite) createArtistInCity(name, city, state string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name}
	if city != "" {
		artist.City = &city
	}
	if state != "" {
		artist.State = &state
	}
	suite.Require().NoError(suite.db.Create(artist).Error)
	return artist
}

// createTaggedArtist is the shows suite's tag fixture. A show is not tagged
// directly: the filter reaches a bill transitively through its artists, so the
// tag has to hang on an artist for the show to match. An empty slug leaves the
// artist untagged, which is what the control rows need.
func (suite *ShowServiceIntegrationTestSuite) createTaggedArtist(name, slug string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: fmt.Sprintf("%s %d", name, time.Now().UnixNano())}
	suite.Require().NoError(suite.db.Create(artist).Error)
	if slug == "" {
		return artist
	}

	var tag catalogm.Tag
	if err := suite.db.Where("slug = ?", slug).First(&tag).Error; err != nil {
		tag = catalogm.Tag{Name: slug, Slug: slug, Category: catalogm.TagCategoryGenre}
		suite.Require().NoError(suite.db.Create(&tag).Error)
	}
	suite.Require().NoError(suite.db.Create(&catalogm.EntityTag{
		TagID:         tag.ID,
		EntityType:    catalogm.TagEntityArtist,
		EntityID:      artist.ID,
		AddedByUserID: suite.createTestUser().ID,
	}).Error)
	return artist
}

func (suite *ShowServiceIntegrationTestSuite) billArtist(showID, artistID uint) {
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: showID, ArtistID: artistID, Position: 0,
	}).Error)
}

// =============================================================================
// GET /venues/cities
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueCities_ScopedSumEqualsTheListTotal() {
	user := suite.createTestUser()
	slug := tagSlugFor("venues")
	tagID := suite.createGenreTag(slug, slug)

	tagged := []*catalogm.Venue{
		suite.createTestVenue("Facet PHX One", "Phoenix", "AZ", true),
		suite.createTestVenue("Facet PHX Two", "Phoenix", "AZ", true),
		suite.createTestVenue("Facet TUC One", "Tucson", "AZ", true),
	}
	for _, v := range tagged {
		suite.tagVenue(v.ID, tagID, user.ID)
	}
	// Untagged and unverified neighbours: the first must not be counted by the
	// filter, the second by anything.
	suite.createTestVenue("Facet PHX Untagged", "Phoenix", "AZ", true)
	unverified := suite.createTestVenue("Facet MES Unverified", "Mesa", "AZ", false)
	suite.tagVenue(unverified.ID, tagID, user.ID)

	filters := contracts.VenueListFilters{Sort: "upcoming", TagSlugs: []string{slug}}

	cities, err := suite.venueService.GetVenueCities(filters)
	suite.Require().NoError(err)

	_, total, err := suite.venueService.GetVenuesWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(3), total, "the tag should select exactly the three verified fixtures")
	suite.Equal(total, sumVenueCityCounts(cities),
		"the sum of the scoped city counts is the list total under the same filters")

	// The per-city halves, because a sum can agree while the split is wrong.
	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.VenueCount
	}
	suite.Equal(2, byCity["Phoenix"])
	suite.Equal(1, byCity["Tucson"])
	suite.NotContains(byCity, "Mesa", "an unverified venue is outside the browse gate")

	// Each row is also the total the list reports for that city alone.
	phoenixOnly := filters
	phoenixOnly.Cities = []contracts.CityStateFilter{{City: "Phoenix", State: "AZ"}}
	_, phoenixTotal, err := suite.venueService.GetVenuesWithShowCounts(phoenixOnly, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(byCity["Phoenix"]), phoenixTotal)
}

// The venue facet's exemption-free sum, exercised on the row that would create
// an exemption if one existed. `venues.city` is NOT NULL, so an unplaced room
// carries an empty string; it groups under an empty city here and counts in the
// list's total, and the sum still holds. A `city != ''` guard, which the show
// and artist facets do carry, would break it.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenueCities_CountsAPlacelessRoomOnBothSides() {
	user := suite.createTestUser()
	slug := tagSlugFor("venues-placeless")
	tagID := suite.createGenreTag(slug, slug)

	placed := suite.createTestVenue("Facet Placeless Placed", "Yuma", "AZ", true)
	placeless := suite.createTestVenue("Facet Placeless Room", "", "", true)
	suite.tagVenue(placed.ID, tagID, user.ID)
	suite.tagVenue(placeless.ID, tagID, user.ID)

	filters := contracts.VenueListFilters{Sort: "upcoming", TagSlugs: []string{slug}}

	cities, err := suite.venueService.GetVenueCities(filters)
	suite.Require().NoError(err)

	_, total, err := suite.venueService.GetVenuesWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(2), total)
	suite.Equal(total, sumVenueCityCounts(cities),
		"an unplaced room is counted by the facet and by the list alike")

	var empties int
	for _, c := range cities {
		if c.City == "" {
			empties++
		}
	}
	suite.Equal(1, empties, "the unplaced room groups under an empty city rather than vanishing")
}

// The unfiltered read is the one every other surface already depends on, so it
// has to answer exactly as it did before the endpoint took filters at all.
func (suite *VenueServiceIntegrationTestSuite) TestGetVenueCities_UnfilteredIgnoresTheTagFixtures() {
	user := suite.createTestUser()
	slug := tagSlugFor("venues-unfiltered")
	tagID := suite.createGenreTag(slug, slug)

	tagged := suite.createTestVenue("Facet Unfiltered Tagged", "Flagstaff", "AZ", true)
	suite.tagVenue(tagged.ID, tagID, user.ID)
	suite.createTestVenue("Facet Unfiltered Plain", "Flagstaff", "AZ", true)

	cities, err := suite.venueService.GetVenueCities(contracts.VenueListFilters{})
	suite.Require().NoError(err)

	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.VenueCount
	}
	suite.Equal(2, byCity["Flagstaff"], "an unfiltered facet counts tagged and untagged rooms alike")
}

// =============================================================================
// GET /shows/cities
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_ScopedSumEqualsTheListTotal() {
	user := suite.createTestUser()
	slug := tagSlugFor("shows")

	phoenix := suite.createTestVenue("Facet Show PHX Room", "Phoenix", "AZ", true)
	tucson := suite.createTestVenue("Facet Show TUC Room", "Tucson", "AZ", true)

	// One artist per night: show_artists carries a one-bill-per-artist-per-night
	// guard, so a fixture reusing one act across two shows on one date is
	// rejected by the schema rather than by anything under test.
	first := suite.createTaggedArtist("Facet Show Band A", slug)
	second := suite.createTaggedArtist("Facet Show Band B", slug)
	third := suite.createTaggedArtist("Facet Show Band C", slug)
	fourth := suite.createTaggedArtist("Facet Show Band D", slug)
	untagged := suite.createTaggedArtist("Facet Show Other Band", "")

	at := time.Now().UTC().AddDate(0, 0, 7)
	// The first Phoenix bill carries the tag TWICE, on two different acts. It is
	// the row that makes this a real count rather than a row check: the tag
	// filter constrains with IN (subquery) rather than joining the matches in, so
	// a show matching twice is still one show. Rewritten as a join it would be
	// two, and both the facet and the list total would double together — which
	// the sum assertion alone cannot see.
	doubleTagged := suite.createApprovedShowAt(phoenix.ID, user.ID, "Phoenix", "AZ", at)
	suite.billArtist(doubleTagged.ID, first.ID)
	suite.billArtist(doubleTagged.ID, second.ID)
	suite.billArtist(suite.createApprovedShowAt(phoenix.ID, user.ID, "Phoenix", "AZ", at).ID, third.ID)
	suite.billArtist(suite.createApprovedShowAt(tucson.ID, user.ID, "Tucson", "AZ", at).ID, fourth.ID)
	// A show on the same night whose bill does not carry the tag.
	suite.billArtist(suite.createApprovedShowAt(phoenix.ID, user.ID, "Phoenix", "AZ", at).ID, untagged.ID)

	filters := &contracts.UpcomingShowsFilter{TagSlugs: []string{slug}}

	cities, err := suite.showService.GetShowCities("UTC", filters, contracts.ShowCalendarWindow{})
	suite.Require().NoError(err)

	_, total, err := suite.showService.GetUpcomingShowsPage(
		contracts.ShowCalendarQuery{Limit: 50}, false, filters,
	)
	suite.Require().NoError(err)

	suite.Equal(int64(3), total,
		"the tag selects exactly the three tagged bills, and the one matching twice counts once")
	suite.Equal(total, sumShowCityCounts(cities),
		"the sum of the scoped city counts is the list total under the same filters")

	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.ShowCount
	}
	// 2, not 3: the double-tagged bill is one show in the facet as well.
	suite.Equal(2, byCity["Phoenix"])
	suite.Equal(1, byCity["Tucson"])
}

// The window narrows the facet exactly as it narrows the list, which is the half
// the tag case cannot show: a window is applied to the venue-local calendar day
// rather than to a row's own columns.
func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_ScopedSumEqualsTheListTotalInAWindow() {
	user := suite.createTestUser()
	slug := tagSlugFor("shows-window")

	const zone = "America/Phoenix"
	venue := newVenueInZone(suite.T(), suite.db, "Facet Window Room", "AZ", zone, true)
	first := suite.createTaggedArtist("Facet Window Band A", slug)
	second := suite.createTaggedArtist("Facet Window Band B", slug)
	later := suite.createTaggedArtist("Facet Window Band C", slug)

	inWindow := venueLocalInstant(suite.T(), zone, 7, 20)
	outOfWindow := venueLocalInstant(suite.T(), zone, 30, 20)
	loc, err := time.LoadLocation(zone)
	suite.Require().NoError(err)
	local := inWindow.In(loc)

	suite.billArtist(suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", inWindow).ID, first.ID)
	suite.billArtist(suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", inWindow).ID, second.ID)
	suite.billArtist(suite.createApprovedShowAt(venue.ID, user.ID, "Phoenix", "AZ", outOfWindow).ID, later.ID)

	filters := &contracts.UpcomingShowsFilter{TagSlugs: []string{slug}}
	window := contracts.ShowCalendarWindow{
		Year:  local.Year(),
		Month: int(local.Month()),
		Day:   local.Day(),
	}

	cities, err := suite.showService.GetShowCities("UTC", filters, window)
	suite.Require().NoError(err)

	_, total, err := suite.showService.GetUpcomingShowsPage(
		contracts.ShowCalendarQuery{ShowCalendarWindow: window, Limit: 50}, false, filters,
	)
	suite.Require().NoError(err)

	suite.Equal(int64(2), total, "only the two shows on the addressed venue-local day")
	suite.Equal(total, sumShowCityCounts(cities),
		"the sum of the scoped city counts is the list total under the same window")
}

// A half-stated window narrows to nothing in SQL, so the facet refuses it rather
// than answering for the whole upcoming catalog.
//
// Asserted on the message, not merely on non-nil: every other failure this
// method can return (a nil db, a query error) is also an error, and a guard that
// stopped running would otherwise leave this green.
func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_RefusesAHalfStatedWindow() {
	_, err := suite.showService.GetShowCities("UTC", nil, contracts.ShowCalendarWindow{Month: 6})
	suite.Require().Error(err)
	suite.Contains(err.Error(), "invalid show calendar window")
}

// =============================================================================
// GET /artists/cities
// =============================================================================

// The artists case carries the extra half: a tag filter drops the activity gate
// (PSY-495), so the facet has to drop it too or it counts the gated set under a
// filter that lists the evergreen one.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistCities_ScopedSumEqualsTheListTotal() {
	slug := tagSlugFor("artists")
	venue := suite.createTestVenue("Facet Artist Room", "Phoenix", "AZ")
	user := suite.createTestUser()

	active := suite.createArtistInCity("Facet Artist Active", "Phoenix", "AZ")
	suite.createApprovedShowWithArtist(active.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.tagArtistForBrowse(active.ID, slug)

	// No upcoming show: outside the gated set, inside the evergreen one.
	quiet := suite.createArtistInCity("Facet Artist Quiet", "Tucson", "AZ")
	suite.tagArtistForBrowse(quiet.ID, slug)

	// Tagged but placeless: the documented exemption. It is in the list total and
	// in no facet row.
	placeless := suite.createArtistInCity("Facet Artist Placeless", "", "")
	suite.tagArtistForBrowse(placeless.ID, slug)

	// Untagged neighbour with a show, to prove the tag is doing the selecting.
	other := suite.createArtistInCity("Facet Artist Untagged", "Phoenix", "AZ")
	suite.createApprovedShowWithArtist(other.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	filters := map[string]interface{}{
		"tag_filter":         TagFilter{TagSlugs: []string{slug}},
		"skip_active_filter": true,
	}

	cities, err := suite.artistService.GetArtistCities(filters)
	suite.Require().NoError(err)

	_, total, err := suite.artistService.GetArtistsWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(3), total, "the evergreen list carries the quiet and the placeless artist")
	suite.Equal(total-1, sumArtistCityCounts(cities),
		"the sum is the list total less the one artist that names no place")

	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.ArtistCount
	}
	suite.Equal(1, byCity["Phoenix"], "the untagged Phoenix artist is outside the filter")
	suite.Equal(1, byCity["Tucson"], "a quiet artist is counted because the tag drops the activity gate")
}

// The unfiltered facet keeps the activity gate, which is what the /artists
// landing page's counts have always meant.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistCities_UnfilteredKeepsTheActivityGate() {
	venue := suite.createTestVenue("Facet Gate Room", "Phoenix", "AZ")
	user := suite.createTestUser()

	active := suite.createArtistInCity("Facet Gate Active", "Sedona", "AZ")
	suite.createApprovedShowWithArtist(active.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.createArtistInCity("Facet Gate Quiet", "Sedona", "AZ")

	cities, err := suite.artistService.GetArtistCities(nil)
	suite.Require().NoError(err)

	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.ArtistCount
	}
	suite.Equal(1, byCity["Sedona"], "an artist with no upcoming show is outside the gated facet")
}

// createArtistWithListenLink places an artist carrying one of the four
// streaming columns, which is what puts it OUTSIDE the gap population.
func (suite *ArtistServiceIntegrationTestSuite) createArtistWithListenLink(
	name, city, state string,
) *catalogm.Artist {
	artist := suite.createArtistInCity(name, city, state)
	suite.Require().NoError(
		setArtistLink(suite.db, artist, "spotify", "https://open.spotify.com/artist/"+name))
	return artist
}

// The gap filter's half of the same premise, and the second filter that drops
// the activity gate. Under it the list is evergreen and narrowed to the bands
// with no listen link, so a facet reading neither half counts the gated whole
// catalogue under a filter that lists a fraction of it.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistCities_MissingListenSumEqualsTheListTotal() {
	venue := suite.createTestVenue("Facet Gap Room", "Phoenix", "AZ")
	user := suite.createTestUser()

	// The gap, with nothing booked: in the list, and the row a gated facet drops.
	suite.createArtistInCity("Facet Gap Quiet", "Flagstaff", "AZ")

	// The gap, with a show: in both readings of the filter.
	active := suite.createArtistInCity("Facet Gap Active", "Flagstaff", "AZ")
	suite.createApprovedShowWithArtist(active.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	// A listen link and a show: outside the filter, inside an unfiltered facet.
	linked := suite.createArtistWithListenLink("Facet Gap Linked", "Bisbee", "AZ")
	suite.createApprovedShowWithArtist(linked.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	// The gap, placeless: the documented exemption.
	suite.createArtistInCity("Facet Gap Placeless", "", "")

	filters := map[string]interface{}{FilterMissingListenLink: true}

	cities, err := suite.artistService.GetArtistCities(filters)
	suite.Require().NoError(err)

	_, total, err := suite.artistService.GetArtistsWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(3), total,
		"the gap list carries the quiet, the active and the placeless band")
	suite.Equal(total-1, sumArtistCityCounts(cities),
		"the sum is the list total less the one artist that names no place")

	byCity := map[string]int{}
	for _, c := range cities {
		byCity[c.City] = c.ArtistCount
	}
	suite.Equal(2, byCity["Flagstaff"],
		"a quiet band is counted because the gap filter drops the activity gate")
	suite.Equal(0, byCity["Bisbee"],
		"a band with a listen link is outside the gap population")
}
