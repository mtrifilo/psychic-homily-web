package catalog

import (
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// GET /artists' `missing=listen` filter (PSY-2050) — run inside the
// SceneServiceIntegrationTestSuite because the property under test is an
// agreement between two services: the number GetSceneGaps publishes and the
// rows the browse list answers with have to be one population.

// phoenixCityFilters is the filters map GET /artists builds from
// `?cities=Phoenix,AZ`, which is the link the scene gap line writes.
func phoenixCityFilters() map[string]interface{} {
	return map[string]interface{}{
		"cities": []map[string]string{{"city": "Phoenix", "state": "AZ"}},
	}
}

// artistNames reduces a browse page to the names it listed.
func artistNames(rows []*contracts.ArtistWithShowCountResponse) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

// The acceptance property: on a metro scene holding both mismatch shapes the
// ticket names — a band whose stored city is a case variant of the scene's
// display city, and a band in a MEMBER city of the metro — the gap count and
// the filtered list's total are the same number.
//
// Both shapes are invisible to the unfiltered `?cities=Phoenix,AZ` list, which
// matches `city = 'Phoenix'` exactly and case-sensitively, so this test fails
// on any implementation that spells the scope a second time instead of reusing
// the scene's.
func (suite *SceneServiceIntegrationTestSuite) TestArtistsMissingListen_TotalEqualsGapCount() {
	suite.sceneWithTwoVenues()

	suite.createArtist("Bare Phoenix Band")
	suite.createArtistInNullMetro("Lowercase City Band", "phoenix", "AZ")
	suite.createArtistIn("Mesa Member Band", "Mesa", "AZ")

	// Out of the population, one per rule: a band with a listen link, and a
	// linkless band based in another metro.
	linked := suite.createArtist("Linked Phoenix Band")
	suite.setLink(linked, "bandcamp", "https://example.com/bandcamp")
	suite.createArtistIn("Portland Linkless", "Portland", "OR")

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Equal(3, gaps.ArtistsMissingListenLink)

	artistService := NewArtistService(suite.db)
	filters := phoenixCityFilters()
	filters[FilterMissingListenLink] = true

	rows, total, err := artistService.GetArtistsWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(gaps.ArtistsMissingListenLink), total,
		"the gap line states this count and links to this list; they are one population or the sentence is false")
	suite.ElementsMatch(
		[]string{"Bare Phoenix Band", "Lowercase City Band", "Mesa Member Band"},
		artistNames(rows),
	)
}

// The filter drops the browse list's default "has an upcoming approved show"
// gate. Without that, the two numbers diverge on the most ordinary scene there
// is: bands with a gap and no show booked are exactly the ones the gap line is
// asking a reader to fix.
func (suite *SceneServiceIntegrationTestSuite) TestArtistsMissingListen_CountsBandsWithNoUpcomingShow() {
	suite.sceneWithTwoVenues()
	suite.createArtist("No Shows Booked")

	artistService := NewArtistService(suite.db)

	gated, _, err := artistService.GetArtistsWithShowCounts(phoenixCityFilters(), 50, 0)
	suite.Require().NoError(err)
	suite.Empty(gated, "the unfiltered browse list gates on upcoming shows")

	filters := phoenixCityFilters()
	filters[FilterMissingListenLink] = true
	_, total, err := artistService.GetArtistsWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
}

// Without the filter the city stays literal. The scope widening is the filter's
// own, not a change to what `?cities=` means for every other caller.
func (suite *SceneServiceIntegrationTestSuite) TestArtistsMissingListen_LeavesTheUnfilteredCityRuleAlone() {
	venue := suite.sceneWithTwoVenues()
	user := suite.createUser()

	local := suite.createArtist("Phoenix Booked Band")
	mesa := suite.createArtistIn("Mesa Booked Band", "Mesa", "AZ")
	upcoming := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("local bill", venue.ID, local.ID, user.ID, upcoming)
	suite.createApprovedShow("mesa bill", venue.ID, mesa.ID, user.ID, upcoming)

	artistService := NewArtistService(suite.db)
	rows, total, err := artistService.GetArtistsWithShowCounts(phoenixCityFilters(), 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(1), total)
	suite.Equal([]string{"Phoenix Booked Band"}, artistNames(rows))
}

// An empty string in a link column is not a link, so the browse filter reads it
// the way the gap count does. A bare IS NULL test on either side would report a
// smaller gap than the other one states.
func (suite *SceneServiceIntegrationTestSuite) TestArtistsMissingListen_TreatsBlankLinksAsMissing() {
	suite.sceneWithTwoVenues()

	blank := suite.createArtist("Blank String Band")
	suite.setLink(blank, "spotify", "")

	// A non-music social is not a listen link either, on both sides.
	social := suite.createArtist("Instagram Only Band")
	suite.setLink(social, "instagram", "https://example.com/instagram")

	gaps, err := suite.sceneService.GetSceneGaps("Phoenix", "AZ")
	suite.Require().NoError(err)

	artistService := NewArtistService(suite.db)
	filters := phoenixCityFilters()
	filters[FilterMissingListenLink] = true
	_, total, err := artistService.GetArtistsWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(2, gaps.ArtistsMissingListenLink)
	suite.Equal(int64(gaps.ArtistsMissingListenLink), total)
}

// Naming no place still filters. The gap population is scene-scoped, but the
// parameter is independent of the city one, and a caller that omits the city
// gets every band with the gap rather than every band.
func (suite *SceneServiceIntegrationTestSuite) TestArtistsMissingListen_WithoutACityFiltersEveryBand() {
	suite.sceneWithTwoVenues()

	suite.createArtist("Phoenix Linkless")
	suite.createArtistIn("Portland Linkless", "Portland", "OR")
	linked := suite.createArtistIn("Portland Linked", "Portland", "OR")
	suite.setLink(linked, "youtube", "https://example.com/youtube")

	artistService := NewArtistService(suite.db)
	_, total, err := artistService.GetArtistsWithShowCounts(
		map[string]interface{}{FilterMissingListenLink: true}, 50, 0,
	)
	suite.Require().NoError(err)

	suite.Equal(int64(2), total)
}

// The single `city`/`state` pair reads the same rule as `cities`, so the two
// spellings of one place cannot answer differently.
func (suite *SceneServiceIntegrationTestSuite) TestArtistsMissingListen_SingleCityParamUsesTheSameScope() {
	suite.sceneWithTwoVenues()

	suite.createArtist("Bare Phoenix Band")
	suite.createArtistIn("Mesa Member Band", "Mesa", "AZ")

	artistService := NewArtistService(suite.db)
	_, total, err := artistService.GetArtistsWithShowCounts(map[string]interface{}{
		"city":                  "Phoenix",
		"state":                 "AZ",
		FilterMissingListenLink: true,
	}, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(2), total)
}

// A tag filter and the gap filter compose: both drop the activity gate, and
// both constrain, so engaging one must not silently discard the other.
func (suite *SceneServiceIntegrationTestSuite) TestArtistsMissingListen_ComposesWithATagFilter() {
	suite.sceneWithTwoVenues()

	user := suite.createUser()
	tagID := suite.createTagInCategory("Post-Punk", "post-punk", "genre")
	tagged := suite.createArtist("Tagged Linkless")
	suite.createArtist("Untagged Linkless")
	suite.tagArtist(tagged.ID, tagID, user.ID)

	artistService := NewArtistService(suite.db)
	filters := phoenixCityFilters()
	filters[FilterMissingListenLink] = true
	filters["tag_filter"] = TagFilter{TagSlugs: []string{"post-punk"}}
	filters["skip_active_filter"] = true

	rows, total, err := artistService.GetArtistsWithShowCounts(filters, 50, 0)
	suite.Require().NoError(err)

	suite.Equal(int64(1), total)
	suite.Equal([]string{"Tagged Linkless"}, artistNames(rows))
}
