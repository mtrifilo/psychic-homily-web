package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Offset paging for the /artists BROWSE list (PSY-1774).
//
// Distinct from artist_shows_pagination_test.go next door, which pages ONE
// artist's shows. This is the catalogue list; GetArtistsWithShowCounts records
// why it had to be bounded. The properties under test are the ones a pager
// depends on and a happy-path assertion never notices — that the total counts
// the filtered set rather than the page, that consecutive pages are disjoint,
// and that a negative bound cannot fall back through to the unbounded read.

// seedActiveArtists creates `count` artists each with one upcoming approved
// show, so every one of them passes the browse activity gate. Names are
// zero-padded so alphabetical order is also numeric order, which is what makes
// the page-boundary assertions readable.
func (suite *ArtistServiceIntegrationTestSuite) seedActiveArtists(prefix string, count int) []*catalogm.Artist {
	venue := suite.createTestVenue(prefix+" Room", "Phoenix", "AZ")
	user := suite.createTestUser()
	artists := make([]*catalogm.Artist, 0, count)
	for i := 0; i < count; i++ {
		artist := suite.createTestArtist(fmt.Sprintf("%s %03d", prefix, i))
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
		artists = append(artists, artist)
	}
	return artists
}

// tagArtistForBrowse tags an artist through the raw junction table, matching
// what tag_filter_integration_test.go does and for the same reason: the inline
// create/permission flow is not what is under test here.
func (suite *ArtistServiceIntegrationTestSuite) tagArtistForBrowse(artistID uint, slug string) {
	var tag catalogm.Tag
	if err := suite.db.Where("slug = ?", slug).First(&tag).Error; err != nil {
		tag = catalogm.Tag{Name: slug, Slug: slug, Category: catalogm.TagCategoryGenre}
		suite.Require().NoError(suite.db.Create(&tag).Error)
	}
	suite.Require().NoError(suite.db.Create(&catalogm.EntityTag{
		TagID:         tag.ID,
		EntityType:    catalogm.TagEntityArtist,
		EntityID:      artistID,
		AddedByUserID: suite.createTestUser().ID,
	}).Error)
}

// The bound is the whole point of the change: a caller asking for a page must
// get a page, and must still learn how much it did not get.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistsWithShowCounts_BoundsThePageAndReportsTheTotal() {
	suite.seedActiveArtists("Bound", 7)

	page, total, err := suite.artistService.GetArtistsWithShowCounts(map[string]interface{}{}, 3, 0)

	suite.Require().NoError(err)
	suite.Len(page, 3, "the page must honour the limit")
	suite.Equal(int64(7), total, "the total must describe the whole matching set, not the page")
}

// Consecutive pages must partition the set. This is what the `artists.id`
// tiebreak in artistBrowseOrder buys: every one of these artists is tied on
// upcoming_show_count, so without a unique final sort key Postgres is free to
// order the ties differently per query and a boundary silently repeats one
// artist while dropping another.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistsWithShowCounts_PagesStayDisjoint() {
	artists := suite.seedActiveArtists("Disjoint", 6)

	seen := map[string]bool{}
	for offset := 0; offset < len(artists); offset += 2 {
		page, _, err := suite.artistService.GetArtistsWithShowCounts(map[string]interface{}{}, 2, offset)
		suite.Require().NoError(err)
		suite.Require().Lenf(page, 2, "offset %d should be a full page", offset)
		for _, a := range page {
			suite.Falsef(seen[a.Name], "%q appeared on two pages", a.Name)
			seen[a.Name] = true
		}
	}
	suite.Len(seen, len(artists), "every artist must appear on exactly one page")
}

// An offset past the end is a stale bookmark or a shrinking result set, not an
// error. It reports nothing on the page and the real total, so the pager can
// re-render itself against a page count it can actually reach.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistsWithShowCounts_OffsetPastTheEnd() {
	suite.seedActiveArtists("Overrun", 2)

	page, total, err := suite.artistService.GetArtistsWithShowCounts(map[string]interface{}{}, 50, 500)

	suite.Require().NoError(err)
	suite.Empty(page, "an offset past the end returns no rows")
	suite.Equal(int64(2), total, "...but still reports what the caller overran")
}

// Negative bounds are caller bugs, and GORM reads each as a DIFFERENT
// instruction: a negative offset becomes OFFSET -1, which Postgres rejects,
// while a negative limit CANCELS the limit and succeeds — handing back the
// entire catalogue, which is the exact failure this endpoint was paginated to
// remove. The second is the dangerous one because it looks like it worked.
//
// The huma `minimum` tags guard only the HTTP path; this is the service's own
// floor, for the callers that build the arguments directly.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistsWithShowCounts_NegativePageBoundsClamp() {
	artists := suite.seedActiveArtists("Negative", 3)

	page, _, err := suite.artistService.GetArtistsWithShowCounts(map[string]interface{}{}, 1, -5)
	suite.Require().NoError(err)
	suite.Require().Len(page, 1)
	suite.Equal(artists[0].Name, page[0].Name, "a negative offset must read as the first page")

	page, total, err := suite.artistService.GetArtistsWithShowCounts(map[string]interface{}{}, -1, 0)
	suite.Require().NoError(err)
	suite.Empty(page, "a negative limit must not fall through to an unbounded read")
	suite.Equal(int64(3), total, "...and still report the real total, like limit 0")
}

// The evergreen path (PSY-495) takes a different join than the gated one, and
// the total is counted over a query that deliberately omits the presentational
// last-show-date join. Both halves have to stay right, so both are asserted on
// a set where the gated and evergreen answers genuinely differ.
func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistsWithShowCounts_EvergreenPagesAndTotals() {
	active := suite.seedActiveArtists("Evergreen", 2)
	dormant := suite.createTestArtist("Evergreen Dormant")

	for _, a := range active {
		suite.tagArtistForBrowse(a.ID, "evergreen-tag")
	}
	suite.tagArtistForBrowse(dormant.ID, "evergreen-tag")

	filters := map[string]interface{}{
		"tag_filter":         TagFilter{TagSlugs: []string{"evergreen-tag"}},
		"skip_active_filter": true,
	}

	page, total, err := suite.artistService.GetArtistsWithShowCounts(filters, 2, 0)
	suite.Require().NoError(err)
	suite.Len(page, 2, "the evergreen page must honour the limit too")
	suite.Equal(int64(3), total, "the evergreen total must include the dormant artist")

	page, _, err = suite.artistService.GetArtistsWithShowCounts(filters, 2, 2)
	suite.Require().NoError(err)
	suite.Require().Len(page, 1, "the last evergreen page holds the remainder")
	suite.Equal("Evergreen Dormant", page[0].Name, "zero-count artists sort last")
}
