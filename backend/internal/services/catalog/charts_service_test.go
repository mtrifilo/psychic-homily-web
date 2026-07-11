package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ChartsServiceIntegrationTestSuite struct {
	suite.Suite
	testDB        *testutil.TestDatabase
	db            *gorm.DB
	chartsService *ChartsService
}

func (suite *ChartsServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.chartsService = &ChartsService{db: suite.testDB.DB}
}

func (suite *ChartsServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *ChartsServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestChartsServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ChartsServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *ChartsServiceIntegrationTestSuite) createUser(email string) *authm.User {
	user := &authm.User{
		Email:         stringPtr(email),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *ChartsServiceIntegrationTestSuite) createVenue(name, city, state string) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:  name,
		City:  city,
		State: state,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *ChartsServiceIntegrationTestSuite) createArtist(name string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *ChartsServiceIntegrationTestSuite) createApprovedShow(title string, venueID, artistID, userID uint, eventDate time.Time) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   eventDate,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)

	err = suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error
	suite.Require().NoError(err)

	err = suite.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artistID, Position: 0}).Error
	suite.Require().NoError(err)

	return show
}

func (suite *ChartsServiceIntegrationTestSuite) createBookmark(userID uint, entityType engagementm.BookmarkEntityType, entityID uint, action engagementm.BookmarkAction) {
	bookmark := &engagementm.UserBookmark{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
	}
	err := suite.db.Create(bookmark).Error
	suite.Require().NoError(err)
}

func (suite *ChartsServiceIntegrationTestSuite) createRelease(title string) *catalogm.Release {
	release := &catalogm.Release{
		Title: title,
	}
	err := suite.db.Create(release).Error
	suite.Require().NoError(err)
	return release
}

func (suite *ChartsServiceIntegrationTestSuite) addArtistToRelease(artistID, releaseID uint) {
	ar := &catalogm.ArtistRelease{
		ArtistID:  artistID,
		ReleaseID: releaseID,
		Role:      catalogm.ArtistReleaseRoleMain,
		Position:  0,
	}
	err := suite.db.Create(ar).Error
	suite.Require().NoError(err)
}

// =============================================================================
// GetTrendingShows Tests
// =============================================================================

func (suite *ChartsServiceIntegrationTestSuite) TestGetTrendingShows_Empty() {
	shows, err := suite.chartsService.GetTrendingShows(20)
	suite.Require().NoError(err)
	suite.Empty(shows)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetTrendingShows_WithData() {
	user1 := suite.createUser("charts-user1@test.com")
	user2 := suite.createUser("charts-user2@test.com")
	user3 := suite.createUser("charts-user3@test.com")
	venue := suite.createVenue("Crescent Ballroom", "Phoenix", "AZ")
	artist := suite.createArtist("Band A")

	future := time.Now().UTC().AddDate(0, 0, 7)
	show1 := suite.createApprovedShow("Popular Show", venue.ID, artist.ID, user1.ID, future)
	show2 := suite.createApprovedShow("Less Popular Show", venue.ID, artist.ID, user1.ID, future.AddDate(0, 0, 1))

	// Show 1 has 3 saves
	suite.createBookmark(user1.ID, engagementm.BookmarkEntityShow, show1.ID, engagementm.BookmarkActionSave)
	suite.createBookmark(user2.ID, engagementm.BookmarkEntityShow, show1.ID, engagementm.BookmarkActionSave)
	suite.createBookmark(user3.ID, engagementm.BookmarkEntityShow, show1.ID, engagementm.BookmarkActionSave)

	// Show 2 has 1 save
	suite.createBookmark(user1.ID, engagementm.BookmarkEntityShow, show2.ID, engagementm.BookmarkActionSave)

	shows, err := suite.chartsService.GetTrendingShows(20)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 2)

	// Most popular show first
	suite.Equal(show1.ID, shows[0].ShowID)
	suite.Equal("Popular Show", shows[0].Title)
	suite.Equal(3, shows[0].SaveCount)
	suite.Equal("Crescent Ballroom", shows[0].VenueName)
	suite.Equal("Phoenix", shows[0].City)

	// Less popular show second
	suite.Equal(show2.ID, shows[1].ShowID)
	suite.Equal(1, shows[1].SaveCount)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetTrendingShows_ExcludesPastShows() {
	user := suite.createUser("charts-past@test.com")
	venue := suite.createVenue("Valley Bar", "Phoenix", "AZ")
	artist := suite.createArtist("Past Band")

	// Past show
	past := time.Now().UTC().AddDate(0, 0, -7)
	pastShow := suite.createApprovedShow("Past Show", venue.ID, artist.ID, user.ID, past)
	suite.createBookmark(user.ID, engagementm.BookmarkEntityShow, pastShow.ID, engagementm.BookmarkActionSave)

	// Future show
	future := time.Now().UTC().AddDate(0, 0, 7)
	futureShow := suite.createApprovedShow("Future Show", venue.ID, artist.ID, user.ID, future)
	suite.createBookmark(user.ID, engagementm.BookmarkEntityShow, futureShow.ID, engagementm.BookmarkActionSave)

	shows, err := suite.chartsService.GetTrendingShows(20)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 1)
	suite.Equal(futureShow.ID, shows[0].ShowID)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetTrendingShows_RespectsLimit() {
	user := suite.createUser("charts-limit@test.com")
	venue := suite.createVenue("Venue Limit", "Phoenix", "AZ")
	artist := suite.createArtist("Limit Band")

	future := time.Now().UTC().AddDate(0, 0, 7)
	for i := 0; i < 5; i++ {
		show := suite.createApprovedShow(
			fmt.Sprintf("Show %d", i),
			venue.ID, artist.ID, user.ID,
			future.AddDate(0, 0, i),
		)
		suite.createBookmark(user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionSave)
	}

	shows, err := suite.chartsService.GetTrendingShows(3)
	suite.Require().NoError(err)
	suite.Len(shows, 3)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetTrendingShows_WithoutBookmarks() {
	user := suite.createUser("charts-nobookmark@test.com")
	venue := suite.createVenue("No Bookmark Venue", "Phoenix", "AZ")
	artist := suite.createArtist("No Bookmark Band")

	future := time.Now().UTC().AddDate(0, 0, 7)
	show := suite.createApprovedShow("Unbookmarked Show", venue.ID, artist.ID, user.ID, future)

	shows, err := suite.chartsService.GetTrendingShows(20)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 1)
	suite.Equal(show.ID, shows[0].ShowID)
	suite.Equal("Unbookmarked Show", shows[0].Title)
	suite.Equal(0, shows[0].SaveCount)
	suite.Equal("No Bookmark Venue", shows[0].VenueName)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetTrendingShows_BookmarkedRankedFirst() {
	user := suite.createUser("charts-rank@test.com")
	venue := suite.createVenue("Rank Venue", "Phoenix", "AZ")
	artist := suite.createArtist("Rank Band")

	future := time.Now().UTC().AddDate(0, 0, 7)
	// Show with no bookmarks (further future date)
	unbookmarked := suite.createApprovedShow("Unbookmarked", venue.ID, artist.ID, user.ID, future.AddDate(0, 0, 10))
	// Show with bookmarks (same date)
	bookmarked := suite.createApprovedShow("Bookmarked", venue.ID, artist.ID, user.ID, future.AddDate(0, 0, 10))
	suite.createBookmark(user.ID, engagementm.BookmarkEntityShow, bookmarked.ID, engagementm.BookmarkActionSave)

	shows, err := suite.chartsService.GetTrendingShows(20)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 2)
	// Bookmarked show should be first
	suite.Equal(bookmarked.ID, shows[0].ShowID)
	suite.Equal(1, shows[0].SaveCount)
	// Unbookmarked show second
	suite.Equal(unbookmarked.ID, shows[1].ShowID)
	suite.Equal(0, shows[1].SaveCount)
}

// =============================================================================
// GetPopularArtists Tests
// =============================================================================

func (suite *ChartsServiceIntegrationTestSuite) TestGetPopularArtists_Empty() {
	artists, err := suite.chartsService.GetPopularArtists(20)
	suite.Require().NoError(err)
	suite.Empty(artists)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetPopularArtists_WithData() {
	user1 := suite.createUser("pop-artist1@test.com")
	user2 := suite.createUser("pop-artist2@test.com")
	venue := suite.createVenue("Pop Venue", "Phoenix", "AZ")

	artistA := suite.createArtist("Popular Artist")
	artistB := suite.createArtist("Less Popular Artist")

	// Artist A: 2 followers + 2 upcoming shows = 2*2 + 2 = 6
	suite.createBookmark(user1.ID, engagementm.BookmarkEntityArtist, artistA.ID, engagementm.BookmarkActionFollow)
	suite.createBookmark(user2.ID, engagementm.BookmarkEntityArtist, artistA.ID, engagementm.BookmarkActionFollow)
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("Show A1", venue.ID, artistA.ID, user1.ID, future)
	suite.createApprovedShow("Show A2", venue.ID, artistA.ID, user1.ID, future.AddDate(0, 0, 1))

	// Artist B: 1 follower + 0 upcoming shows = 1*2 + 0 = 2
	suite.createBookmark(user1.ID, engagementm.BookmarkEntityArtist, artistB.ID, engagementm.BookmarkActionFollow)

	artists, err := suite.chartsService.GetPopularArtists(20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 2)

	// Artist A should be first (higher score)
	suite.Equal(artistA.ID, artists[0].ArtistID)
	suite.Equal("Popular Artist", artists[0].Name)
	suite.Equal(2, artists[0].FollowCount)
	suite.Equal(2, artists[0].UpcomingShowCount)
	suite.Equal(6, artists[0].Score) // 2*2 + 2

	// Artist B second
	suite.Equal(artistB.ID, artists[1].ArtistID)
	suite.Equal(1, artists[1].FollowCount)
	suite.Equal(0, artists[1].UpcomingShowCount)
	suite.Equal(2, artists[1].Score) // 1*2 + 0
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetPopularArtists_OnlyUpcomingShows() {
	user := suite.createUser("pop-upcoming@test.com")
	venue := suite.createVenue("Upcoming Venue", "Phoenix", "AZ")
	artist := suite.createArtist("Upcoming Artist")

	// Artist with no followers but 3 upcoming shows = 0*2 + 3 = 3
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("U1", venue.ID, artist.ID, user.ID, future)
	suite.createApprovedShow("U2", venue.ID, artist.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("U3", venue.ID, artist.ID, user.ID, future.AddDate(0, 0, 2))

	artists, err := suite.chartsService.GetPopularArtists(20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1)
	suite.Equal(3, artists[0].UpcomingShowCount)
	suite.Equal(3, artists[0].Score)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetPopularArtists_RespectsLimit() {
	user := suite.createUser("pop-limit@test.com")

	for i := 0; i < 5; i++ {
		artist := suite.createArtist(fmt.Sprintf("Limit Artist %d", i))
		suite.createBookmark(user.ID, engagementm.BookmarkEntityArtist, artist.ID, engagementm.BookmarkActionFollow)
	}

	artists, err := suite.chartsService.GetPopularArtists(3)
	suite.Require().NoError(err)
	suite.Len(artists, 3)
}

// =============================================================================
// GetActiveVenues Tests
// =============================================================================

func (suite *ChartsServiceIntegrationTestSuite) TestGetActiveVenues_Empty() {
	venues, err := suite.chartsService.GetActiveVenues(20)
	suite.Require().NoError(err)
	suite.Empty(venues)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetActiveVenues_WithData() {
	user1 := suite.createUser("active-venue1@test.com")
	user2 := suite.createUser("active-venue2@test.com")

	venueA := suite.createVenue("Active Venue A", "Phoenix", "AZ")
	venueB := suite.createVenue("Active Venue B", "Phoenix", "AZ")
	artist := suite.createArtist("Active Artist")

	// Venue A: 3 upcoming shows + 1 follower = 3*2 + 1 = 7
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("VA1", venueA.ID, artist.ID, user1.ID, future)
	suite.createApprovedShow("VA2", venueA.ID, artist.ID, user1.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("VA3", venueA.ID, artist.ID, user1.ID, future.AddDate(0, 0, 2))
	suite.createBookmark(user1.ID, engagementm.BookmarkEntityVenue, venueA.ID, engagementm.BookmarkActionFollow)

	// Venue B: 1 upcoming show + 2 followers = 1*2 + 2 = 4
	suite.createApprovedShow("VB1", venueB.ID, artist.ID, user1.ID, future)
	suite.createBookmark(user1.ID, engagementm.BookmarkEntityVenue, venueB.ID, engagementm.BookmarkActionFollow)
	suite.createBookmark(user2.ID, engagementm.BookmarkEntityVenue, venueB.ID, engagementm.BookmarkActionFollow)

	venues, err := suite.chartsService.GetActiveVenues(20)
	suite.Require().NoError(err)
	suite.Require().Len(venues, 2)

	// Venue A should be first (higher score)
	suite.Equal(venueA.ID, venues[0].VenueID)
	suite.Equal("Active Venue A", venues[0].Name)
	suite.Equal(3, venues[0].UpcomingShowCount)
	suite.Equal(1, venues[0].FollowCount)
	suite.Equal(7, venues[0].Score)

	// Venue B second
	suite.Equal(venueB.ID, venues[1].VenueID)
	suite.Equal(1, venues[1].UpcomingShowCount)
	suite.Equal(2, venues[1].FollowCount)
	suite.Equal(4, venues[1].Score)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetActiveVenues_RespectsLimit() {
	user := suite.createUser("venue-limit@test.com")
	artist := suite.createArtist("Venue Limit Artist")

	future := time.Now().UTC().AddDate(0, 0, 7)
	for i := 0; i < 5; i++ {
		venue := suite.createVenue(fmt.Sprintf("Limit Venue %d", i), "Phoenix", "AZ")
		suite.createApprovedShow(
			fmt.Sprintf("VL Show %d", i),
			venue.ID, artist.ID, user.ID, future.AddDate(0, 0, i),
		)
	}

	venues, err := suite.chartsService.GetActiveVenues(3)
	suite.Require().NoError(err)
	suite.Len(venues, 3)
}

// =============================================================================
// GetHotReleases Tests
// =============================================================================

func (suite *ChartsServiceIntegrationTestSuite) TestGetHotReleases_Empty() {
	releases, err := suite.chartsService.GetHotReleases(20)
	suite.Require().NoError(err)
	suite.Empty(releases)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetHotReleases_WithData() {
	user1 := suite.createUser("hot-release1@test.com")
	user2 := suite.createUser("hot-release2@test.com")
	user3 := suite.createUser("hot-release3@test.com")

	artist := suite.createArtist("Release Artist")
	releaseA := suite.createRelease("Hot Album")
	releaseB := suite.createRelease("Warm Album")
	suite.addArtistToRelease(artist.ID, releaseA.ID)
	suite.addArtistToRelease(artist.ID, releaseB.ID)

	// Release A: 3 bookmarks
	suite.createBookmark(user1.ID, engagementm.BookmarkEntityRelease, releaseA.ID, engagementm.BookmarkActionBookmark)
	suite.createBookmark(user2.ID, engagementm.BookmarkEntityRelease, releaseA.ID, engagementm.BookmarkActionBookmark)
	suite.createBookmark(user3.ID, engagementm.BookmarkEntityRelease, releaseA.ID, engagementm.BookmarkActionBookmark)

	// Release B: 1 bookmark
	suite.createBookmark(user1.ID, engagementm.BookmarkEntityRelease, releaseB.ID, engagementm.BookmarkActionBookmark)

	releases, err := suite.chartsService.GetHotReleases(20)
	suite.Require().NoError(err)
	suite.Require().Len(releases, 2)

	// Hot Album first (3 bookmarks)
	suite.Equal(releaseA.ID, releases[0].ReleaseID)
	suite.Equal("Hot Album", releases[0].Title)
	suite.Equal(3, releases[0].BookmarkCount)
	suite.Require().Len(releases[0].ArtistNames, 1)
	suite.Equal("Release Artist", releases[0].ArtistNames[0])

	// Warm Album second (1 bookmark)
	suite.Equal(releaseB.ID, releases[1].ReleaseID)
	suite.Equal(1, releases[1].BookmarkCount)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetHotReleases_MultipleArtists() {
	user := suite.createUser("hot-multi@test.com")

	artistA := suite.createArtist("Alice")
	artistB := suite.createArtist("Bob")
	release := suite.createRelease("Collab Album")
	suite.addArtistToRelease(artistA.ID, release.ID)

	// Add second artist at position 1
	ar := &catalogm.ArtistRelease{
		ArtistID:  artistB.ID,
		ReleaseID: release.ID,
		Role:      catalogm.ArtistReleaseRoleFeatured,
		Position:  1,
	}
	suite.Require().NoError(suite.db.Create(ar).Error)

	suite.createBookmark(user.ID, engagementm.BookmarkEntityRelease, release.ID, engagementm.BookmarkActionBookmark)

	releases, err := suite.chartsService.GetHotReleases(20)
	suite.Require().NoError(err)
	suite.Require().Len(releases, 1)
	suite.Require().Len(releases[0].ArtistNames, 2)
	suite.Equal("Alice", releases[0].ArtistNames[0])
	suite.Equal("Bob", releases[0].ArtistNames[1])
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetHotReleases_RespectsLimit() {
	user := suite.createUser("hot-limit@test.com")
	artist := suite.createArtist("Limit Release Artist")

	for i := 0; i < 5; i++ {
		release := suite.createRelease(fmt.Sprintf("Release %d", i))
		suite.addArtistToRelease(artist.ID, release.ID)
		suite.createBookmark(user.ID, engagementm.BookmarkEntityRelease, release.ID, engagementm.BookmarkActionBookmark)
	}

	releases, err := suite.chartsService.GetHotReleases(3)
	suite.Require().NoError(err)
	suite.Len(releases, 3)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetHotReleases_WithoutBookmarks() {
	artist := suite.createArtist("Unbookmarked Artist")
	release := suite.createRelease("Unbookmarked Album")
	suite.addArtistToRelease(artist.ID, release.ID)

	releases, err := suite.chartsService.GetHotReleases(20)
	suite.Require().NoError(err)
	suite.Require().Len(releases, 1)
	suite.Equal(release.ID, releases[0].ReleaseID)
	suite.Equal("Unbookmarked Album", releases[0].Title)
	suite.Equal(0, releases[0].BookmarkCount)
	suite.Require().Len(releases[0].ArtistNames, 1)
	suite.Equal("Unbookmarked Artist", releases[0].ArtistNames[0])
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetHotReleases_BookmarkedRankedFirst() {
	user := suite.createUser("hot-rank@test.com")
	artist := suite.createArtist("Rank Release Artist")

	unbookmarked := suite.createRelease("Unbookmarked Release")
	suite.addArtistToRelease(artist.ID, unbookmarked.ID)

	bookmarked := suite.createRelease("Bookmarked Release")
	suite.addArtistToRelease(artist.ID, bookmarked.ID)
	suite.createBookmark(user.ID, engagementm.BookmarkEntityRelease, bookmarked.ID, engagementm.BookmarkActionBookmark)

	releases, err := suite.chartsService.GetHotReleases(20)
	suite.Require().NoError(err)
	suite.Require().Len(releases, 2)
	// Bookmarked release should be first
	suite.Equal(bookmarked.ID, releases[0].ReleaseID)
	suite.Equal(1, releases[0].BookmarkCount)
	// Unbookmarked release second
	suite.Equal(unbookmarked.ID, releases[1].ReleaseID)
	suite.Equal(0, releases[1].BookmarkCount)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetHotReleases_Only30Days() {
	user := suite.createUser("hot-30d@test.com")
	artist := suite.createArtist("Old Release Artist")

	release := suite.createRelease("Old Bookmarked Release")
	suite.addArtistToRelease(artist.ID, release.ID)

	// Create a bookmark, then manually backdate it to 31 days ago
	suite.createBookmark(user.ID, engagementm.BookmarkEntityRelease, release.ID, engagementm.BookmarkActionBookmark)
	suite.db.Exec("UPDATE user_bookmarks SET created_at = ? WHERE entity_id = ? AND entity_type = ? AND action = ?",
		time.Now().UTC().AddDate(0, 0, -31), release.ID, engagementm.BookmarkEntityRelease, engagementm.BookmarkActionBookmark)

	releases, err := suite.chartsService.GetHotReleases(20)
	suite.Require().NoError(err)
	// Release still appears but with 0 bookmark_count (old bookmark not counted)
	suite.Require().Len(releases, 1)
	suite.Equal("Old Bookmarked Release", releases[0].Title)
	suite.Equal(0, releases[0].BookmarkCount)
}

// =============================================================================
// GetChartsOverview Tests
// =============================================================================

func (suite *ChartsServiceIntegrationTestSuite) TestGetChartsOverview_Empty() {
	overview, err := suite.chartsService.GetChartsOverview()
	suite.Require().NoError(err)
	suite.Require().NotNil(overview)
	suite.Empty(overview.TrendingShows)
	suite.Empty(overview.PopularArtists)
	suite.Empty(overview.ActiveVenues)
	suite.Empty(overview.HotReleases)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetChartsOverview_LimitsToFive() {
	user := suite.createUser("overview@test.com")
	venue := suite.createVenue("Overview Venue", "Phoenix", "AZ")

	future := time.Now().UTC().AddDate(0, 0, 7)
	for i := 0; i < 8; i++ {
		artist := suite.createArtist(fmt.Sprintf("Overview Artist %d", i))
		show := suite.createApprovedShow(
			fmt.Sprintf("Overview Show %d", i),
			venue.ID, artist.ID, user.ID,
			future.AddDate(0, 0, i),
		)
		suite.createBookmark(user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionSave)
		suite.createBookmark(user.ID, engagementm.BookmarkEntityArtist, artist.ID, engagementm.BookmarkActionFollow)
	}

	overview, err := suite.chartsService.GetChartsOverview()
	suite.Require().NoError(err)
	suite.Require().NotNil(overview)
	suite.LessOrEqual(len(overview.TrendingShows), 5)
	suite.LessOrEqual(len(overview.PopularArtists), 5)
}

// =============================================================================
// GetMostActiveArtists Tests
// =============================================================================

// addArtistToShow appends an artist to an existing show's bill with an explicit
// position and set_type (createApprovedShow always seeds position 0 / default).
func (suite *ChartsServiceIntegrationTestSuite) addArtistToShow(showID, artistID uint, position int, setType string) {
	err := suite.db.Create(&catalogm.ShowArtist{
		ShowID:   showID,
		ArtistID: artistID,
		Position: position,
		SetType:  setType,
	}).Error
	suite.Require().NoError(err)
}

func TestChartWindowStart(t *testing.T) {
	// Deliberately mid-day: the start bound must truncate to midnight UTC so a
	// midnight-timestamped show exactly N days ago stays inside the window.
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)

	month := chartWindowStart(contracts.ChartWindowMonth, now)
	if month == nil || !month.Equal(time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("month window: got %v", month)
	}
	quarter := chartWindowStart(contracts.ChartWindowQuarter, now)
	if quarter == nil || !quarter.Equal(time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("quarter window: got %v", quarter)
	}
	if allTime := chartWindowStart(contracts.ChartWindowAllTime, now); allTime != nil {
		t.Fatalf("all_time window: expected nil lower bound, got %v", allTime)
	}
	// Empty/unknown values fall back to quarter via ChartWindow.OrDefault.
	if def := chartWindowStart(contracts.ChartWindow(""), now); def == nil || !def.Equal(*quarter) {
		t.Fatalf("default window: got %v", def)
	}
	if got := contracts.ChartWindow("bogus").OrDefault(); got != contracts.ChartWindowQuarter {
		t.Fatalf("OrDefault(bogus): got %v", got)
	}
	if got := contracts.ChartWindowMonth.OrDefault(); got != contracts.ChartWindowMonth {
		t.Fatalf("OrDefault(month): got %v", got)
	}
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostActiveArtists_Empty() {
	artists, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(artists)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostActiveArtists_WindowBoundaries() {
	user := suite.createUser("maa-window@test.com")
	venue := suite.createVenue("Window Venue", "Phoenix", "AZ")
	artist := suite.createArtist("Window Artist")

	now := time.Now().UTC()
	suite.createApprovedShow("Recent Show", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -10))
	suite.createApprovedShow("Mid Show", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -60))
	suite.createApprovedShow("Old Show", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -200))
	suite.createApprovedShow("Future Show", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 10))

	month, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowMonth, 20)
	suite.Require().NoError(err)
	suite.Require().Len(month, 1)
	suite.Equal(1, month[0].ShowCount)

	quarter, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(quarter, 1)
	suite.Equal(2, quarter[0].ShowCount)

	allTime, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowAllTime, 20)
	suite.Require().NoError(err)
	suite.Require().Len(allTime, 1)
	suite.Equal(3, allTime[0].ShowCount, "future shows never count, even all-time")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostActiveArtists_HeadlinePctAndLastShow() {
	user := suite.createUser("maa-headline@test.com")
	venue := suite.createVenue("Headline Venue", "Phoenix", "AZ")
	headliner := suite.createArtist("Bill Topper")
	support := suite.createArtist("Support Act")
	support.City = stringPtr("Tempe")
	support.State = stringPtr("AZ")
	suite.Require().NoError(suite.db.Save(support).Error)

	now := time.Now().UTC()
	// Show 1: support is position 0 (default set_type) -> headline slot.
	suite.createApprovedShow("Own Show", venue.ID, support.ID, user.ID, now.AddDate(0, 0, -40))
	// Show 2: support opens (position 1, opener) -> not a headline slot.
	s2 := suite.createApprovedShow("Opener Show", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -30))
	suite.addArtistToShow(s2.ID, support.ID, 1, "opener")
	// Show 3: set_type says headliner even at position 2 -> headline slot.
	s3 := suite.createApprovedShow("Co-headline Show", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -20))
	suite.addArtistToShow(s3.ID, support.ID, 2, "headliner")
	// Show 4: plain performer slot -> not a headline slot. Most recent show.
	s4 := suite.createApprovedShow("Latest Show", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -5))
	suite.addArtistToShow(s4.ID, support.ID, 1, "performer")

	artists, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 2)

	// support has 4 shows, headliner has 3 -> support ranks first.
	top := artists[0]
	suite.Equal(support.ID, top.ArtistID)
	suite.Equal(4, top.ShowCount)
	suite.Equal(50, top.HeadlinePct, "2 headline slots (position 0 + set_type headliner) of 4")
	suite.Equal("Tempe", top.City)
	suite.Equal("AZ", top.State)
	suite.Require().NotNil(top.LastShowDate)
	suite.WithinDuration(now.AddDate(0, 0, -5), *top.LastShowDate, time.Hour)
	suite.Equal("Headline Venue", top.LastShowVenue)

	suite.Equal(headliner.ID, artists[1].ArtistID)
	suite.Equal(3, artists[1].ShowCount)
	suite.Equal(100, artists[1].HeadlinePct)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostActiveArtists_OrderingAndTiebreak() {
	user := suite.createUser("maa-order@test.com")
	venue := suite.createVenue("Order Venue", "Phoenix", "AZ")
	alpha := suite.createArtist("Alpha")
	beta := suite.createArtist("Beta")
	gamma := suite.createArtist("Gamma")

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		suite.createApprovedShow(fmt.Sprintf("Alpha %d", i), venue.ID, alpha.ID, user.ID, now.AddDate(0, 0, -10-i))
		suite.createApprovedShow(fmt.Sprintf("Beta %d", i), venue.ID, beta.ID, user.ID, now.AddDate(0, 0, -10-i))
	}
	for i := 0; i < 5; i++ {
		suite.createApprovedShow(fmt.Sprintf("Gamma %d", i), venue.ID, gamma.ID, user.ID, now.AddDate(0, 0, -10-i))
	}

	artists, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 3)
	suite.Equal("Gamma", artists[0].Name)
	suite.Equal("Alpha", artists[1].Name, "equal counts tiebreak by name")
	suite.Equal("Beta", artists[2].Name)

	// Limit is respected.
	limited, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowQuarter, 1)
	suite.Require().NoError(err)
	suite.Require().Len(limited, 1)
	suite.Equal("Gamma", limited[0].Name)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostActiveArtists_ExcludesPendingAndFutureOnly() {
	user := suite.createUser("maa-status@test.com")
	venue := suite.createVenue("Status Venue", "Phoenix", "AZ")
	pendingArtist := suite.createArtist("Pending Only")
	futureArtist := suite.createArtist("Future Only")

	now := time.Now().UTC()
	// Pending show inside the window never counts.
	pending := &catalogm.Show{
		Title:       "Pending Show",
		EventDate:   now.AddDate(0, 0, -10),
		Status:      catalogm.ShowStatusPending,
		SubmittedBy: &user.ID,
	}
	suite.Require().NoError(suite.db.Create(pending).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: pending.ID, VenueID: venue.ID}).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{ShowID: pending.ID, ArtistID: pendingArtist.ID, Position: 0}).Error)

	// Artist with only future shows never appears.
	suite.createApprovedShow("Future Booked", venue.ID, futureArtist.ID, user.ID, now.AddDate(0, 0, 30))

	artists, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(artists)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostActiveArtists_LastShowTiebreakDeterministic() {
	user := suite.createUser("maa-tiebreak@test.com")
	venueA := suite.createVenue("Matinee Venue", "Phoenix", "AZ")
	venueB := suite.createVenue("Evening Venue", "Phoenix", "AZ")
	artist := suite.createArtist("Double Booked")

	// Two shows on the SAME event_date (midnight timestamps make this the
	// common case). The higher show id must win deterministically.
	sameDay := time.Now().UTC().AddDate(0, 0, -3).Truncate(24 * time.Hour)
	suite.createApprovedShow("Matinee", venueA.ID, artist.ID, user.ID, sameDay)
	suite.createApprovedShow("Evening", venueB.ID, artist.ID, user.ID, sameDay)

	for i := 0; i < 3; i++ {
		artists, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowQuarter, 20)
		suite.Require().NoError(err)
		suite.Require().Len(artists, 1)
		suite.Equal(2, artists[0].ShowCount)
		suite.Equal("Evening Venue", artists[0].LastShowVenue, "higher show id wins the same-date tie on every request")
	}
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostActiveArtists_ExcludesCancelledShows() {
	user := suite.createUser("maa-cancelled@test.com")
	venue := suite.createVenue("Cancelled Venue", "Phoenix", "AZ")
	artist := suite.createArtist("Cancels Sometimes")

	now := time.Now().UTC()
	suite.createApprovedShow("Played Show", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -20))
	cancelled := suite.createApprovedShow("Cancelled Show", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -5))
	suite.Require().NoError(suite.db.Model(cancelled).Update("is_cancelled", true).Error)

	artists, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1)
	suite.Equal(1, artists[0].ShowCount, "cancelled shows were never played")
	suite.Require().NotNil(artists[0].LastShowDate)
	suite.WithinDuration(now.AddDate(0, 0, -20), *artists[0].LastShowDate, time.Hour,
		"a cancelled show must not be the last show either")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostActiveArtists_WindowEdgeDayIncluded() {
	user := suite.createUser("maa-edge@test.com")
	venue := suite.createVenue("Edge Venue", "Phoenix", "AZ")
	artist := suite.createArtist("Edge Case")

	// Event dates are midnight timestamps; the window start truncates to
	// midnight, so the show exactly 90 days ago is IN, 91 days ago is OUT.
	now := time.Now().UTC()
	midnight := now.Truncate(24 * time.Hour)
	suite.createApprovedShow("Edge Show", venue.ID, artist.ID, user.ID, midnight.AddDate(0, 0, -90))
	suite.createApprovedShow("Outside Show", venue.ID, artist.ID, user.ID, midnight.AddDate(0, 0, -91))

	artists, err := suite.chartsService.GetMostActiveArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1)
	suite.Equal(1, artists[0].ShowCount, "exactly-90-days-ago is inside the quarter window; 91 is not")
}
