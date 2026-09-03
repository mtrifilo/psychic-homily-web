package admin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// UNIT TESTS
// =============================================================================

func TestPlantedAffiliateTag(t *testing.T) {
	cases := []struct {
		name      string
		url       string
		wantParam string
		wantHost  string
		wantOK    bool
	}{
		{
			name:      "a tag on a configured vendor",
			url:       "https://www.ticketweb.com/event/2?irmp=9999999",
			wantParam: "irmp",
			wantHost:  "www.ticketweb.com",
			wantOK:    true,
		},
		{
			name:      "a tag on a host outside the vendor table",
			url:       "https://evil.example/e/1?irmp=9999999",
			wantParam: "irmp",
			wantHost:  "evil.example",
			wantOK:    true,
		},
		{
			name:      "a tag behind another parameter",
			url:       "https://evil.example/e/1?utm_source=x&irmp=9999999",
			wantParam: "irmp",
			wantHost:  "evil.example",
			wantOK:    true,
		},
		{
			name:      "a tag behind a semicolon separator",
			url:       "https://evil.example/e/1?a=1;irmp=9999999",
			wantParam: "irmp",
			wantHost:  "evil.example",
			wantOK:    true,
		},
		{
			name:      "a percent-encoded parameter name",
			url:       "https://evil.example/e/1?%69rmp=9999999",
			wantParam: "irmp",
			wantHost:  "evil.example",
			wantOK:    true,
		},
		{
			name:      "a scheme-less stored value still names its host",
			url:       "evil.example/e/1?irmp=9999999",
			wantParam: "irmp",
			wantHost:  "evil.example",
			wantOK:    true,
		},
		{
			name:      "a port is not part of the host",
			url:       "https://evil.example:8443/e/1?irmp=9999999",
			wantParam: "irmp",
			wantHost:  "evil.example",
			wantOK:    true,
		},
		// The spellings that credit nobody: a vendor reads a case-sensitive,
		// untrimmed key, so treating these as tags would name an operator to a
		// row nobody is being paid for.
		{name: "an upper-case spelling", url: "https://evil.example/e/1?IRMP=9999999"},
		{name: "a padded spelling", url: "https://evil.example/e/1?+irmp=9999999"},
		{name: "a valueless parameter", url: "https://evil.example/e/1?irmp="},
		{name: "a longer parameter that ends in the name", url: "https://evil.example/e/1?xirmp=9999999"},
		{name: "a tag in the fragment", url: "https://evil.example/e/1#?irmp=9999999"},
		{name: "no query at all", url: "https://www.ticketweb.com/event/2"},
		{name: "an empty value", url: ""},
		{name: "whitespace", url: "   "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			param, host, ok := plantedAffiliateTag(tc.url)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantParam, param)
			assert.Equal(t, tc.wantHost, host)
		})
	}
}

// The reason names the parameter and the host, and carries neither the partner
// ID (a third party's account identifier) nor the rest of the URL (contributor
// text).
func TestPlantedTagReason(t *testing.T) {
	reason := plantedTagReason("https://evil.example/e/1?next=/a/b&irmp=9999999")
	assert.Equal(t, "irmp on evil.example", reason)
	assert.NotContains(t, reason, "9999999")
	assert.NotContains(t, reason, "/a/b")

	// A value the SQL predicate matched but this parser cannot pin down still
	// gets a reason rather than an empty cell.
	assert.Equal(t, "Affiliate parameter in the stored ticket URL", plantedTagReason("?irmp=1"))
}

// The predicate the count and the item list share must name the parameter list
// the frontend vendor table generates.
func TestPlantedTagPatternNamesEveryKnownParam(t *testing.T) {
	assert.NotEmpty(t, knownAffiliateParams)
	for _, param := range knownAffiliateParams {
		assert.Contains(t, plantedTagPattern, param)
	}
}

// The planted-tag categories are moderation findings; the public /contribute
// surface must neither count them nor list them.
func TestPlantedTagCategoriesAreAdminOnly(t *testing.T) {
	for _, key := range []string{categoryShowsPlantedTicketTag, categoryFestivalsPlantedTicketTag} {
		assert.True(t, adminOnlyCategories[key], key)
		assert.Contains(t, categoryOrder, key)
		assert.NotContains(t, contributeCategoryOrder, key)
		_, defined := categoryDefinitions[key]
		assert.True(t, defined, key)
	}
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) createShowWithTicketURL(
	title string, status catalogm.ShowStatus, ticketURL string,
) *catalogm.Show {
	show := &catalogm.Show{
		Title:     title,
		EventDate: time.Now().Add(7 * 24 * time.Hour),
		Status:    status,
		Source:    catalogm.ShowSourceUser,
		TicketURL: &ticketURL,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	return show
}

func (suite *DataQualityServiceIntegrationTestSuite) createFestivalWithTicketURL(
	name, slug, ticketURL string,
) *catalogm.Festival {
	festival := &catalogm.Festival{
		Name:        name,
		Slug:        slug,
		SeriesSlug:  slug,
		EditionYear: 2026,
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-03",
		TicketURL:   &ticketURL,
		Status:      catalogm.FestivalStatusAnnounced,
	}
	suite.Require().NoError(suite.db.Create(festival).Error)
	return festival
}

func (suite *DataQualityServiceIntegrationTestSuite) TestShowsPlantedTicketTag() {
	planted := suite.createShowWithTicketURL(
		"Planted", catalogm.ShowStatusApproved,
		"https://www.ticketweb.com/event/2?irmp=9999999",
	)
	suite.createShowWithTicketURL(
		"Clean", catalogm.ShowStatusApproved,
		"https://www.ticketweb.com/event/3",
	)
	// A value nobody is credited through: the parameter carries no value.
	suite.createShowWithTicketURL(
		"Valueless", catalogm.ShowStatusApproved,
		"https://www.ticketweb.com/event/4?irmp=",
	)
	// A fragment never reaches the vendor's server.
	suite.createShowWithTicketURL(
		"Fragment", catalogm.ShowStatusApproved,
		"https://www.ticketweb.com/event/5#irmp=9999999",
	)
	// Not published, so nothing is being handed to a reader.
	suite.createShowWithTicketURL(
		"Pending", catalogm.ShowStatusPending,
		"https://www.ticketweb.com/event/6?irmp=9999999",
	)

	items, total, err := suite.service.GetCategoryItems(categoryShowsPlantedTicketTag, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(items, 1)
	suite.Equal("show", items[0].EntityType)
	suite.Equal(planted.ID, items[0].EntityID)
	suite.Equal("irmp on www.ticketweb.com", items[0].Reason)
	suite.NotContains(items[0].Reason, "9999999")

	summary, err := suite.service.GetSummary()
	suite.Require().NoError(err)
	suite.Equal(1, summaryCategoryCount(summary, categoryShowsPlantedTicketTag))
}

// summaryCategoryCount reads one category's count out of a summary.
func summaryCategoryCount(summary *contracts.DataQualitySummary, key string) int {
	for _, category := range summary.Categories {
		if category.Key == key {
			return category.Count
		}
	}
	return -1
}

func (suite *DataQualityServiceIntegrationTestSuite) TestFestivalsPlantedTicketTag() {
	planted := suite.createFestivalWithTicketURL(
		"Planted Fest", "planted-fest-2026",
		"https://dice.fm/festival/1?irmp=9999999",
	)
	suite.createFestivalWithTicketURL("Clean Fest", "clean-fest-2026", "https://dice.fm/festival/2")

	items, total, err := suite.service.GetCategoryItems(categoryFestivalsPlantedTicketTag, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(items, 1)
	suite.Equal("festival", items[0].EntityType)
	suite.Equal(planted.ID, items[0].EntityID)
	suite.Equal("planted-fest-2026", items[0].Slug)
	suite.Equal("irmp on dice.fm", items[0].Reason)
}

// The row leaves the category through the ordinary edit path, with no bespoke
// acknowledge state.
func (suite *DataQualityServiceIntegrationTestSuite) TestPlantedTicketTagClearsWhenTheTagLeaves() {
	show := suite.createShowWithTicketURL(
		"Planted", catalogm.ShowStatusApproved,
		"https://www.ticketweb.com/event/2?irmp=9999999",
	)

	_, total, err := suite.service.GetCategoryItems(categoryShowsPlantedTicketTag, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)

	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).
		Where("id = ?", show.ID).
		Update("ticket_url", "https://www.ticketweb.com/event/2").Error)

	_, total, err = suite.service.GetCategoryItems(categoryShowsPlantedTicketTag, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
}

// The public contribution surface neither counts nor lists a moderation
// finding about a contributor-writable column.
func (suite *DataQualityServiceIntegrationTestSuite) TestContributeSurfaceWithholdsPlantedTagCategories() {
	suite.createShowWithTicketURL(
		"Planted", catalogm.ShowStatusApproved,
		"https://www.ticketweb.com/event/2?irmp=9999999",
	)

	summary, err := suite.service.GetContributeSummary(nil)
	suite.Require().NoError(err)
	for _, category := range summary.Categories {
		suite.NotEqual(categoryShowsPlantedTicketTag, category.Key)
		suite.NotEqual(categoryFestivalsPlantedTicketTag, category.Key)
	}

	_, _, err = suite.service.GetContributeCategoryItems(categoryShowsPlantedTicketTag, nil, 50, 0)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "unknown category")
}
