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
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM radio_networks")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM release_labels")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM labels")
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

// =============================================================================
// GetBusiestVenues Tests
// =============================================================================

func (suite *ChartsServiceIntegrationTestSuite) TestGetBusiestVenues_Empty() {
	venues, err := suite.chartsService.GetBusiestVenues(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(venues)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetBusiestVenues_WindowsAndCancelled() {
	user := suite.createUser("bv-window@test.com")
	venue := suite.createVenue("Windowed Hall", "Phoenix", "AZ")
	artist := suite.createArtist("House Band")

	now := time.Now().UTC()
	suite.createApprovedShow("Recent", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -10))
	suite.createApprovedShow("Mid", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -60))
	suite.createApprovedShow("Old", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -200))
	suite.createApprovedShow("Future", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 10))
	cancelled := suite.createApprovedShow("Cancelled", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -5))
	suite.Require().NoError(suite.db.Model(cancelled).Update("is_cancelled", true).Error)

	month, err := suite.chartsService.GetBusiestVenues(contracts.ChartWindowMonth, 20)
	suite.Require().NoError(err)
	suite.Require().Len(month, 1)
	suite.Equal(1, month[0].ShowCount)

	quarter, err := suite.chartsService.GetBusiestVenues(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(quarter, 1)
	suite.Equal(2, quarter[0].ShowCount)
	suite.Equal("Windowed Hall", quarter[0].Name)
	suite.Equal("Phoenix", quarter[0].City)
	suite.Equal("AZ", quarter[0].State)

	allTime, err := suite.chartsService.GetBusiestVenues(contracts.ChartWindowAllTime, 20)
	suite.Require().NoError(err)
	suite.Require().Len(allTime, 1)
	suite.Equal(3, allTime[0].ShowCount, "future + cancelled shows never count")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetBusiestVenues_OrderingTiebreakAndMultiVenue() {
	user := suite.createUser("bv-order@test.com")
	alpha := suite.createVenue("Alpha Hall", "Phoenix", "AZ")
	beta := suite.createVenue("Beta Room", "Tempe", "AZ")
	gamma := suite.createVenue("Gamma Stage", "Tucson", "AZ")
	artist := suite.createArtist("Tie Band")

	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		suite.createApprovedShow(fmt.Sprintf("Alpha %d", i), alpha.ID, artist.ID, user.ID, now.AddDate(0, 0, -10-i))
		suite.createApprovedShow(fmt.Sprintf("Beta %d", i), beta.ID, artist.ID, user.ID, now.AddDate(0, 0, -10-i))
	}
	for i := 0; i < 2; i++ {
		suite.createApprovedShow(fmt.Sprintf("Gamma %d", i), gamma.ID, artist.ID, user.ID, now.AddDate(0, 0, -10-i))
	}
	// A dual-venue show counts for BOTH gamma and alpha (multi-venue bills exist).
	dual := suite.createApprovedShow("Dual", gamma.ID, artist.ID, user.ID, now.AddDate(0, 0, -3))
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: dual.ID, VenueID: alpha.ID}).Error)

	venues, err := suite.chartsService.GetBusiestVenues(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(venues, 3)
	suite.Equal("Alpha Hall", venues[0].Name, "3 shows incl. the dual-venue one")
	suite.Equal(3, venues[0].ShowCount)
	suite.Equal("Gamma Stage", venues[1].Name)
	suite.Equal(3, venues[1].ShowCount)
	suite.Equal("Beta Room", venues[2].Name)
	suite.Equal(2, venues[2].ShowCount)
	// Alpha before Gamma at equal counts: name tiebreak.
}

// =============================================================================
// GetOpenersToWatch Tests
// =============================================================================

func (suite *ChartsServiceIntegrationTestSuite) TestGetOpenersToWatch_Empty() {
	artists, err := suite.chartsService.GetOpenersToWatch(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(artists)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOpenersToWatch_CountsSupportSlotsOnly() {
	user := suite.createUser("otw-count@test.com")
	venue := suite.createVenue("Support Venue", "Phoenix", "AZ")
	headliner := suite.createArtist("Always Headlines")
	opener := suite.createArtist("Always Opens")
	opener.City = stringPtr("Mesa")
	opener.State = stringPtr("AZ")
	suite.Require().NoError(suite.db.Save(opener).Error)

	now := time.Now().UTC()
	s1 := suite.createApprovedShow("Bill 1", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -10))
	suite.addArtistToShow(s1.ID, opener.ID, 1, "opener")
	s2 := suite.createApprovedShow("Bill 2", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -20))
	suite.addArtistToShow(s2.ID, opener.ID, 1, "performer")
	s3 := suite.createApprovedShow("Bill 3", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -30))
	suite.addArtistToShow(s3.ID, opener.ID, 2, "special_guest")

	artists, err := suite.chartsService.GetOpenersToWatch(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1, "the headliner (position 0) must not appear")
	suite.Equal(opener.ID, artists[0].ArtistID)
	suite.Equal(3, artists[0].SupportSlotCount)
	suite.Equal("Mesa", artists[0].City)
	suite.Equal("AZ", artists[0].State)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOpenersToWatch_AnyHeadlineSlotExcludes() {
	user := suite.createUser("otw-exclude@test.com")
	venue := suite.createVenue("Exclude Venue", "Phoenix", "AZ")
	headliner := suite.createArtist("Bill Top")
	sometimes := suite.createArtist("Sometimes Headlines")

	now := time.Now().UTC()
	// Two support slots in-window...
	s1 := suite.createApprovedShow("Support A", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -10))
	suite.addArtistToShow(s1.ID, sometimes.ID, 1, "opener")
	s2 := suite.createApprovedShow("Support B", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -15))
	suite.addArtistToShow(s2.ID, sometimes.ID, 1, "opener")
	// ...but one co-headline slot (set_type headliner despite position 2) in-window.
	s3 := suite.createApprovedShow("Co-headline", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -20))
	suite.addArtistToShow(s3.ID, sometimes.ID, 2, "headliner")

	artists, err := suite.chartsService.GetOpenersToWatch(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(artists, "any headline slot in-window excludes the artist entirely")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOpenersToWatch_HeadlineExclusionIsWindowScoped() {
	user := suite.createUser("otw-window@test.com")
	venue := suite.createVenue("Window Venue", "Phoenix", "AZ")
	headliner := suite.createArtist("Perma Headliner")
	riser := suite.createArtist("Former Headliner")

	now := time.Now().UTC()
	// Headlined long ago (outside the quarter window)...
	suite.createApprovedShow("Old Glory", venue.ID, riser.ID, user.ID, now.AddDate(0, 0, -200))
	// ...but only opens within the quarter.
	s1 := suite.createApprovedShow("Now Opens A", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -10))
	suite.addArtistToShow(s1.ID, riser.ID, 1, "opener")
	s2 := suite.createApprovedShow("Now Opens B", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -20))
	suite.addArtistToShow(s2.ID, riser.ID, 1, "opener")

	quarter, err := suite.chartsService.GetOpenersToWatch(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(quarter, 1, "old headline slot is outside the window — riser qualifies")
	suite.Equal(riser.ID, quarter[0].ArtistID)
	suite.Equal(2, quarter[0].SupportSlotCount)

	allTime, err := suite.chartsService.GetOpenersToWatch(contracts.ChartWindowAllTime, 20)
	suite.Require().NoError(err)
	suite.Empty(allTime, "all-time window sees the old headline slot and excludes")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOpenersToWatch_CancelledSupportSlotsDoNotCount() {
	user := suite.createUser("otw-cancelled@test.com")
	venue := suite.createVenue("Cancel Venue", "Phoenix", "AZ")
	headliner := suite.createArtist("Cancel Top")
	opener := suite.createArtist("Cancel Opener")

	now := time.Now().UTC()
	s1 := suite.createApprovedShow("Kept", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -10))
	suite.addArtistToShow(s1.ID, opener.ID, 1, "opener")
	s2 := suite.createApprovedShow("Called Off", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -5))
	suite.addArtistToShow(s2.ID, opener.ID, 1, "opener")
	suite.Require().NoError(suite.db.Model(s2).Update("is_cancelled", true).Error)

	artists, err := suite.chartsService.GetOpenersToWatch(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1)
	suite.Equal(1, artists[0].SupportSlotCount)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOpenersToWatch_NullSetTypeCountsAsSupport() {
	user := suite.createUser("otw-null@test.com")
	venue := suite.createVenue("Null Venue", "Phoenix", "AZ")
	headliner := suite.createArtist("Null Top")
	opener := suite.createArtist("Null Opener")

	now := time.Now().UTC()
	s1 := suite.createApprovedShow("Null Bill", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -10))
	// Raw insert with NULL set_type (backfills/ingest can bypass the GORM
	// default) — three-valued logic must not drop the slot.
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 1, NULL)",
		s1.ID, opener.ID).Error)

	artists, err := suite.chartsService.GetOpenersToWatch(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1)
	suite.Equal(opener.ID, artists[0].ArtistID)
	suite.Equal(1, artists[0].SupportSlotCount, "NULL set_type at position>0 is a support slot")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetBusiestVenues_ExcludesPendingShows() {
	user := suite.createUser("bv-pending@test.com")
	venue := suite.createVenue("Pending Venue", "Phoenix", "AZ")
	artist := suite.createArtist("Pending Band")

	now := time.Now().UTC()
	pending := suite.createApprovedShow("Pending Show", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -10))
	suite.Require().NoError(suite.db.Model(pending).Update("status", catalogm.ShowStatusPending).Error)

	venues, err := suite.chartsService.GetBusiestVenues(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(venues, "pending shows never count toward hosted-show totals")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOpenersToWatch_ExcludesPendingShows() {
	user := suite.createUser("otw-pending@test.com")
	venue := suite.createVenue("Pending OTW Venue", "Phoenix", "AZ")
	headliner := suite.createArtist("Pending Top")
	opener := suite.createArtist("Pending Opener")

	now := time.Now().UTC()
	pending := suite.createApprovedShow("Pending Bill", venue.ID, headliner.ID, user.ID, now.AddDate(0, 0, -10))
	suite.addArtistToShow(pending.ID, opener.ID, 1, "opener")
	suite.Require().NoError(suite.db.Model(pending).Update("status", catalogm.ShowStatusPending).Error)

	artists, err := suite.chartsService.GetOpenersToWatch(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(artists, "pending shows never contribute support slots")
}

// =============================================================================
// GetOnTheRadioArtists Tests
// =============================================================================

// --- radio fixtures ---

func (suite *ChartsServiceIntegrationTestSuite) createRadioNetwork(name, slug string) *catalogm.RadioNetwork {
	network := &catalogm.RadioNetwork{Name: name, Slug: slug}
	suite.Require().NoError(suite.db.Create(network).Error)
	return network
}

// createWindowedEpisode creates an episode daysAgo days back with a frozen
// air window at noon of that day, so it passes the WINDOWED branch of the
// shared aired gate (airedEpisodeVisibleSQL) for any daysAgo >= 1. Named
// distinctly from radio_test.go's createAiredEpisode, which seeds the
// windowless shape (no StartsAt) — the two exercise different gate branches.
func (suite *ChartsServiceIntegrationTestSuite) createWindowedEpisode(showID uint, daysAgo int) *catalogm.RadioEpisode {
	day := time.Now().UTC().AddDate(0, 0, -daysAgo).Truncate(24 * time.Hour)
	starts := day.Add(12 * time.Hour)
	ends := starts.Add(time.Hour)
	ep := &catalogm.RadioEpisode{
		ShowID:   showID,
		AirDate:  day.Format("2006-01-02"),
		StartsAt: &starts,
		EndsAt:   &ends,
	}
	suite.Require().NoError(suite.db.Create(ep).Error)
	return ep
}

// createRadioPlay attaches a play to an episode. artistID nil = unmatched
// play. Position doubles as the content-hash discriminator for the
// (episode_id, dedup_key) unique index, so give plays on the same episode
// distinct positions.
func (suite *ChartsServiceIntegrationTestSuite) createRadioPlay(episodeID uint, artistID *uint, position int, isNew bool) {
	play := &catalogm.RadioPlay{
		EpisodeID:  episodeID,
		Position:   position,
		ArtistName: fmt.Sprintf("Raw Name %d", position),
		ArtistID:   artistID,
		IsNew:      isNew,
	}
	suite.Require().NoError(suite.db.Create(play).Error)
}

// createRadioStack creates one station plus a show to hang episodes off;
// networkID nil = standalone station, non-nil = member of that network family.
// timezone (optional, first value wins) sets the station's IANA zone for
// station-local aired-bound tests.
func (suite *ChartsServiceIntegrationTestSuite) createRadioStack(name, slug string, networkID *uint, timezone ...string) *catalogm.RadioShow {
	station := &catalogm.RadioStation{
		Name:          name,
		Slug:          slug,
		BroadcastType: catalogm.BroadcastTypeBoth,
		NetworkID:     networkID,
	}
	if len(timezone) > 0 {
		station.Timezone = &timezone[0]
	}
	suite.Require().NoError(suite.db.Create(station).Error)
	show := &catalogm.RadioShow{StationID: station.ID, Name: name + " Show", Slug: slug + "-show", IsActive: true}
	suite.Require().NoError(suite.db.Create(show).Error)
	return show
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOnTheRadioArtists_Empty() {
	artists, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(artists)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOnTheRadioArtists_WindowBoundaries() {
	show := suite.createRadioStack("KTEST", "ktest", nil)
	artist := suite.createArtist("Windowed Band")

	// 30 sits exactly ON the month window's inclusive lower edge
	// (chartWindowStart truncates to midnight of the day 30 days back); 31 is
	// the first excluded day. Pinning both guards the >= vs > off-by-one.
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 10).ID, &artist.ID, 1, false)
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 30).ID, &artist.ID, 1, false)
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 31).ID, &artist.ID, 1, false)
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 60).ID, &artist.ID, 1, false)
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 200).ID, &artist.ID, 1, false)

	month, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowMonth, 20)
	suite.Require().NoError(err)
	suite.Require().Len(month, 1)
	suite.Equal(2, month[0].PlayCount, "month window counts the 10-day play plus the inclusive 30-day edge, never the 31-day one")

	quarter, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(quarter, 1)
	suite.Equal(4, quarter[0].PlayCount, "quarter window adds the 31- and 60-day plays")

	allTime, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowAllTime, 20)
	suite.Require().NoError(err)
	suite.Require().Len(allTime, 1)
	suite.Equal(5, allTime[0].PlayCount, "all_time counts every aired play")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOnTheRadioArtists_StationCountCollapsesNetworks() {
	network := suite.createRadioNetwork("WTEST", "wtest")
	flagship := suite.createRadioStack("WTEST 91.1", "wtest-fm", &network.ID)
	substream := suite.createRadioStack("WTEST Substream", "wtest-substream", &network.ID)
	standalone := suite.createRadioStack("KSOLO", "ksolo", nil)

	familyOnly := suite.createArtist("Family Only")
	suite.createRadioPlay(suite.createWindowedEpisode(flagship.ID, 5).ID, &familyOnly.ID, 1, false)
	suite.createRadioPlay(suite.createWindowedEpisode(substream.ID, 6).ID, &familyOnly.ID, 1, false)

	broad := suite.createArtist("Broad Reach")
	suite.createRadioPlay(suite.createWindowedEpisode(flagship.ID, 7).ID, &broad.ID, 1, false)
	suite.createRadioPlay(suite.createWindowedEpisode(substream.ID, 8).ID, &broad.ID, 1, false)
	suite.createRadioPlay(suite.createWindowedEpisode(standalone.ID, 9).ID, &broad.ID, 1, false)

	artists, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 2)

	byName := map[string]contracts.OnTheRadioArtist{}
	for _, a := range artists {
		byName[a.Name] = a
	}
	suite.Equal(1, byName["Family Only"].StationCount, "two same-network stations collapse to one broadcaster")
	suite.Equal(2, byName["Broad Reach"].StationCount, "network family plus a standalone station is two broadcasters")
	suite.Equal(3, byName["Broad Reach"].PlayCount, "collapse affects station_count only, never play_count")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOnTheRadioArtists_IsNewWindowScoped() {
	show := suite.createRadioStack("KNEW", "knew", nil)

	freshInWindow := suite.createArtist("Fresh In Window")
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 5).ID, &freshInWindow.ID, 1, true)
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 6).ID, &freshInWindow.ID, 1, false)

	staleFlag := suite.createArtist("Stale Flag")
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 7).ID, &staleFlag.ID, 1, false)
	// is_new only on a play OUTSIDE the month window — must not leak in.
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 60).ID, &staleFlag.ID, 1, true)

	artists, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowMonth, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 2)

	byName := map[string]contracts.OnTheRadioArtist{}
	for _, a := range artists {
		byName[a.Name] = a
	}
	suite.True(byName["Fresh In Window"].IsNew, "any in-window new-rotation play sets the flag")
	suite.False(byName["Stale Flag"].IsNew, "an out-of-window new-rotation play must not set the flag")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOnTheRadioArtists_ExcludesUnmatchedAndUnaired() {
	show := suite.createRadioStack("KGATE", "kgate", nil)
	now := time.Now().UTC()

	// Unmatched play (NULL artist_id): never counts.
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 3).ID, nil, 1, false)

	// Future-dated windowless episode already carrying a play (the WFMU
	// pre-publish shape): excluded by the air_date bound even though the
	// windowless visibility branch (play_count > 0) passes.
	futureArtist := suite.createArtist("Future Only")
	futureDay := now.AddDate(0, 0, 2)
	futureEp := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: futureDay.Format("2006-01-02"), PlayCount: 1}
	suite.Require().NoError(suite.db.Create(futureEp).Error)
	suite.createRadioPlay(futureEp.ID, &futureArtist.ID, 1, false)

	// Today-dated windowed episode whose air window hasn't started yet:
	// excluded by the starts_at gate.
	pendingArtist := suite.createArtist("Not Yet Aired")
	starts := now.Add(2 * time.Hour)
	ends := starts.Add(time.Hour)
	pendingEp := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: now.Format("2006-01-02"), StartsAt: &starts, EndsAt: &ends}
	suite.Require().NoError(suite.db.Create(pendingEp).Error)
	suite.createRadioPlay(pendingEp.ID, &pendingArtist.ID, 1, false)

	// Past windowless episode with a play but play_count still 0 (denormalized
	// count not yet reconciled): the shared windowless branch requires
	// play_count > 0, so it stays out — same convention as the feeds.
	unreconciledArtist := suite.createArtist("Unreconciled")
	staleEp := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: now.AddDate(0, 0, -4).Format("2006-01-02")}
	suite.Require().NoError(suite.db.Create(staleEp).Error)
	suite.createRadioPlay(staleEp.ID, &unreconciledArtist.ID, 1, false)

	// Past windowless episode with plays AND a reconciled play_count: counts.
	windowlessArtist := suite.createArtist("Windowless Aired")
	airedEp := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: now.AddDate(0, 0, -5).Format("2006-01-02"), PlayCount: 1}
	suite.Require().NoError(suite.db.Create(airedEp).Error)
	suite.createRadioPlay(airedEp.ID, &windowlessArtist.ID, 1, false)

	artists, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1, "only the aired windowless play survives the gates")
	suite.Equal("Windowless Aired", artists[0].Name)
	suite.Equal(1, artists[0].PlayCount)
	suite.Equal(1, artists[0].StationCount)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOnTheRadioArtists_OrderingAndLimit() {
	show := suite.createRadioStack("KRANK", "krank", nil)

	twoPlays := suite.createArtist("Zed Twoplays")
	ep := suite.createWindowedEpisode(show.ID, 5)
	suite.createRadioPlay(ep.ID, &twoPlays.ID, 1, false)
	suite.createRadioPlay(ep.ID, &twoPlays.ID, 2, false)

	alphaOne := suite.createArtist("Alpha Oneplay")
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 6).ID, &alphaOne.ID, 1, false)
	betaOne := suite.createArtist("Beta Oneplay")
	suite.createRadioPlay(suite.createWindowedEpisode(show.ID, 7).ID, &betaOne.ID, 1, false)

	artists, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 3)
	suite.Equal("Zed Twoplays", artists[0].Name, "play count ranks first")
	suite.Equal("Alpha Oneplay", artists[1].Name, "ties break by name")
	suite.Equal("Beta Oneplay", artists[2].Name)

	limited, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowQuarter, 2)
	suite.Require().NoError(err)
	suite.Len(limited, 2)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetOnTheRadioArtists_ExcludesPseudoArtistPlays() {
	show := suite.createRadioStack("KPSEUDO", "kpseudo", nil)
	artist := suite.createArtist("Bulk Linked Band")

	// A "Music behind DJ:" background-segment play that an admin bulk-link
	// resolved to a real artist_id: resolution does not make it airplay, and
	// every other radio aggregation excludes it via pseudoArtistExclusionSQL.
	ep := suite.createWindowedEpisode(show.ID, 5)
	pseudo := &catalogm.RadioPlay{
		EpisodeID:  ep.ID,
		Position:   1,
		ArtistName: "Music behind DJ: Bulk Linked Band",
		ArtistID:   &artist.ID,
	}
	suite.Require().NoError(suite.db.Create(pseudo).Error)

	artists, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(artists, "pseudo-artist background-music plays never chart, even when resolved")
}

// TestGetOnTheRadioArtists_AiredBoundIsStationLocal pins the aired upper
// bound to the STATION-LOCAL calendar day, not the UTC one. Two stations on
// the extreme fixed zones bracket UTC: at any instant at least one of them
// is on a different calendar day than UTC, so at least one side exercises
// the skew no matter when the test runs. Episodes are windowless with a
// reconciled play_count (the WFMU pre-publish shape) so only the air_date
// bound separates aired from upcoming.
func (suite *ChartsServiceIntegrationTestSuite) TestGetOnTheRadioArtists_AiredBoundIsStationLocal() {
	now := time.Now().UTC()
	// POSIX sign inversion: Etc/GMT+12 is UTC-12, Etc/GMT-14 is UTC+14.
	for _, tc := range []struct {
		zone string
		slug string
	}{
		{"Etc/GMT+12", "kbehind"},
		{"Etc/GMT-14", "kahead"},
	} {
		show := suite.createRadioStack("Station "+tc.slug, tc.slug, nil, tc.zone)
		loc, err := time.LoadLocation(tc.zone)
		suite.Require().NoError(err)
		localToday := now.In(loc).Format("2006-01-02")
		localTomorrow := now.In(loc).AddDate(0, 0, 1).Format("2006-01-02")

		airedArtist := suite.createArtist("Aired " + tc.slug)
		airedEp := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: localToday, PlayCount: 1}
		suite.Require().NoError(suite.db.Create(airedEp).Error)
		suite.createRadioPlay(airedEp.ID, &airedArtist.ID, 1, false)

		upcomingArtist := suite.createArtist("Upcoming " + tc.slug)
		upcomingEp := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: localTomorrow, PlayCount: 1}
		suite.Require().NoError(suite.db.Create(upcomingEp).Error)
		suite.createRadioPlay(upcomingEp.ID, &upcomingArtist.ID, 1, false)
	}

	artists, err := suite.chartsService.GetOnTheRadioArtists(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)

	names := make([]string, len(artists))
	for i, a := range artists {
		names[i] = a.Name
	}
	suite.ElementsMatch(
		[]string{"Aired kbehind", "Aired kahead"}, names,
		"station-local today counts on both sides of UTC; station-local tomorrow never does",
	)
}

// =============================================================================
// GetMostAnticipatedShows Tests
// =============================================================================
// Dual-shape discipline: ranked mode must keep the floor + counts, AND
// fallback mode must omit counts on every row. Both directions live here
// together so a future edit can't fix one and silently break the other.

// createSaves bookmarks a show with `count` distinct users.
func (suite *ChartsServiceIntegrationTestSuite) createSaves(showID uint, users []*authm.User, count int) {
	for i := 0; i < count; i++ {
		suite.createBookmark(users[i].ID, engagementm.BookmarkEntityShow, showID, engagementm.BookmarkActionSave)
	}
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostAnticipatedShows_RankedMode() {
	user := suite.createUser("ma-owner@test.com")
	venue := suite.createVenue("MA Venue", "Phoenix", "AZ")
	artist := suite.createArtist("MA Artist")
	savers := []*authm.User{
		suite.createUser("ma-saver-1@test.com"),
		suite.createUser("ma-saver-2@test.com"),
		suite.createUser("ma-saver-3@test.com"),
		suite.createUser("ma-saver-4@test.com"),
	}

	now := time.Now().UTC()
	// Five qualifying shows: one at 4 saves, four at exactly the floor (3).
	// Dates ascend so the count tie among the 3-save shows breaks by date.
	big := suite.createApprovedShow("Big Draw", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 10))
	suite.createSaves(big.ID, savers, 4)
	floorShows := make([]uint, 4)
	for i := range floorShows {
		show := suite.createApprovedShow(fmt.Sprintf("Floor Show %d", i), venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 20+i))
		suite.createSaves(show.ID, savers, 3)
		floorShows[i] = show.ID
	}
	// Sub-floor (2 saves) and zero-save shows must NOT appear in ranked mode.
	subFloor := suite.createApprovedShow("Two Saves Only", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 5))
	suite.createSaves(subFloor.ID, savers, 2)
	suite.createApprovedShow("No Saves", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 6))

	result, err := suite.chartsService.GetMostAnticipatedShows(20)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeRanked, result.Mode)
	suite.Require().Len(result.Shows, 5, "only floor-clearing shows rank; 2-save and 0-save shows stay out")

	suite.Equal("Big Draw", result.Shows[0].Title, "highest save count ranks first")
	suite.Require().NotNil(result.Shows[0].SaveCount)
	suite.Equal(4, *result.Shows[0].SaveCount)
	for i, id := range floorShows {
		row := result.Shows[i+1]
		suite.Equal(id, row.ShowID, "count ties break by soonest date")
		suite.Require().NotNil(row.SaveCount, "ranked mode always carries counts")
		suite.Equal(3, *row.SaveCount)
	}
	suite.Equal([]string{"MA Artist"}, result.Shows[0].ArtistNames)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostAnticipatedShows_FallbackBelowMinQualifying() {
	user := suite.createUser("ma-fb-owner@test.com")
	venue := suite.createVenue("MA FB Venue", "Phoenix", "AZ")
	artist := suite.createArtist("MA FB Artist")
	savers := []*authm.User{
		suite.createUser("ma-fb-saver-1@test.com"),
		suite.createUser("ma-fb-saver-2@test.com"),
		suite.createUser("ma-fb-saver-3@test.com"),
	}

	now := time.Now().UTC()
	// Only four shows clear the floor — one short of ranked mode's minimum.
	qualifying := make([]uint, 4)
	for i := range qualifying {
		show := suite.createApprovedShow(fmt.Sprintf("Qualifying %d", i), venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 30+i))
		suite.createSaves(show.ID, savers, 3)
		qualifying[i] = show.ID
	}
	soonest := suite.createApprovedShow("Soonest Zero Saves", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 2))

	result, err := suite.chartsService.GetMostAnticipatedShows(20)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeSoonestUpcoming, result.Mode)
	suite.Require().Len(result.Shows, 5, "fallback lists ALL upcoming shows, floor ignored")
	suite.Equal(soonest.ID, result.Shows[0].ShowID, "fallback orders by soonest date, not saves")
	for _, row := range result.Shows {
		suite.Nil(row.SaveCount, "fallback omits counts on every row — even rows that do have saves")
	}
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostAnticipatedShows_ExcludesPastAndPending() {
	user := suite.createUser("ma-ex-owner@test.com")
	venue := suite.createVenue("MA EX Venue", "Phoenix", "AZ")
	artist := suite.createArtist("MA EX Artist")
	savers := []*authm.User{
		suite.createUser("ma-ex-saver-1@test.com"),
		suite.createUser("ma-ex-saver-2@test.com"),
		suite.createUser("ma-ex-saver-3@test.com"),
	}

	now := time.Now().UTC()
	past := suite.createApprovedShow("Past Heavy Saves", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, -3))
	suite.createSaves(past.ID, savers, 3)
	pending := suite.createApprovedShow("Pending Show", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 3))
	suite.createSaves(pending.ID, savers, 3)
	suite.Require().NoError(suite.db.Model(pending).Update("status", catalogm.ShowStatusPending).Error)

	result, err := suite.chartsService.GetMostAnticipatedShows(20)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeSoonestUpcoming, result.Mode)
	suite.Empty(result.Shows, "past and pending shows appear in neither mode")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostAnticipatedShows_RespectsLimit() {
	user := suite.createUser("ma-lim-owner@test.com")
	venue := suite.createVenue("MA LIM Venue", "Phoenix", "AZ")
	artist := suite.createArtist("MA LIM Artist")
	savers := []*authm.User{
		suite.createUser("ma-lim-saver-1@test.com"),
		suite.createUser("ma-lim-saver-2@test.com"),
		suite.createUser("ma-lim-saver-3@test.com"),
	}

	now := time.Now().UTC()
	for i := 0; i < 6; i++ {
		show := suite.createApprovedShow(fmt.Sprintf("Limit Show %d", i), venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 10+i))
		suite.createSaves(show.ID, savers, 3)
	}

	result, err := suite.chartsService.GetMostAnticipatedShows(5)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeRanked, result.Mode)
	suite.Len(result.Shows, 5)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostAnticipatedShows_SmallLimitStaysRanked() {
	user := suite.createUser("ma-sl-owner@test.com")
	venue := suite.createVenue("MA SL Venue", "Phoenix", "AZ")
	artist := suite.createArtist("MA SL Artist")
	savers := []*authm.User{
		suite.createUser("ma-sl-saver-1@test.com"),
		suite.createUser("ma-sl-saver-2@test.com"),
		suite.createUser("ma-sl-saver-3@test.com"),
	}

	now := time.Now().UTC()
	for i := 0; i < 6; i++ {
		show := suite.createApprovedShow(fmt.Sprintf("SL Show %d", i), venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 10+i))
		suite.createSaves(show.ID, savers, 3)
	}

	// The mode is about how many shows QUALIFY, not how many were requested:
	// a compact 2-row widget must still get ranked mode when 6 shows qualify.
	result, err := suite.chartsService.GetMostAnticipatedShows(2)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeRanked, result.Mode, "a limit below the qualifying minimum must not force fallback")
	suite.Require().Len(result.Shows, 2)
	suite.Require().NotNil(result.Shows[0].SaveCount)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostAnticipatedShows_MultiVenueShowAppearsOnce() {
	user := suite.createUser("ma-mv-owner@test.com")
	// Created first, so it has the LOWER venue id despite sorting last by
	// name — the pick must follow venue_id (the show-page primary-venue
	// convention), and this ordering makes the test fail if it ever reverts
	// to a name-ordered pick.
	venueZ := suite.createVenue("Zeta Hall", "Phoenix", "AZ")
	venueA := suite.createVenue("Alpha Hall", "Tempe", "AZ")
	artist := suite.createArtist("MA MV Artist")
	savers := []*authm.User{
		suite.createUser("ma-mv-saver-1@test.com"),
		suite.createUser("ma-mv-saver-2@test.com"),
		suite.createUser("ma-mv-saver-3@test.com"),
	}

	now := time.Now().UTC()
	multi := suite.createApprovedShow("Two Venue Fest", venueZ.ID, artist.ID, user.ID, now.AddDate(0, 0, 7))
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: multi.ID, VenueID: venueA.ID}).Error)
	suite.createSaves(multi.ID, savers, 3)

	// Fallback mode (1 qualifying < 5): the two-venue show must still be ONE
	// row, carrying the lowest-venue_id pick.
	result, err := suite.chartsService.GetMostAnticipatedShows(20)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeSoonestUpcoming, result.Mode)
	suite.Require().Len(result.Shows, 1, "a multi-venue show is one row, not one per venue")
	suite.Equal("Zeta Hall", result.Shows[0].VenueName, "venue pick follows lowest venue_id, not name order")

	// Ranked mode: pad with four more qualifying shows; the multi-venue show
	// must occupy exactly one slot and count once toward min-qualifying.
	for i := 0; i < 4; i++ {
		show := suite.createApprovedShow(fmt.Sprintf("MV Pad %d", i), venueA.ID, artist.ID, user.ID, now.AddDate(0, 0, 20+i))
		suite.createSaves(show.ID, savers, 3)
	}
	result, err = suite.chartsService.GetMostAnticipatedShows(20)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeRanked, result.Mode)
	suite.Len(result.Shows, 5, "5 distinct qualifying shows — the two-venue show counted once")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostAnticipatedShows_ExcludesCancelled() {
	user := suite.createUser("ma-cx-owner@test.com")
	venue := suite.createVenue("MA CX Venue", "Phoenix", "AZ")
	artist := suite.createArtist("MA CX Artist")
	savers := []*authm.User{
		suite.createUser("ma-cx-saver-1@test.com"),
		suite.createUser("ma-cx-saver-2@test.com"),
		suite.createUser("ma-cx-saver-3@test.com"),
	}

	now := time.Now().UTC()
	cancelled := suite.createApprovedShow("Cancelled Hype", venue.ID, artist.ID, user.ID, now.AddDate(0, 0, 4))
	suite.createSaves(cancelled.ID, savers, 3)
	suite.Require().NoError(suite.db.Model(cancelled).Update("is_cancelled", true).Error)

	result, err := suite.chartsService.GetMostAnticipatedShows(20)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeSoonestUpcoming, result.Mode)
	suite.Empty(result.Shows, "a cancelled show never appears in either mode, saves or not")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetMostAnticipatedShows_IncludesTodayMidnightShow() {
	user := suite.createUser("ma-td-owner@test.com")
	venue := suite.createVenue("MA TD Venue", "Phoenix", "AZ")
	artist := suite.createArtist("MA TD Artist")
	savers := []*authm.User{
		suite.createUser("ma-td-saver-1@test.com"),
		suite.createUser("ma-td-saver-2@test.com"),
		suite.createUser("ma-td-saver-3@test.com"),
	}

	// Date-only shows are stored at midnight UTC, so tonight's show sorts
	// BEFORE the current instant all day long — the eligibility bound must be
	// start-of-today or the chart drops its most actionable rows.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	tonight := suite.createApprovedShow("Tonight Show", venue.ID, artist.ID, user.ID, today)
	suite.createSaves(tonight.ID, savers, 3)

	result, err := suite.chartsService.GetMostAnticipatedShows(20)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeSoonestUpcoming, result.Mode)
	suite.Require().Len(result.Shows, 1, "a show happening today must count as upcoming in fallback mode")
	suite.Equal(tonight.ID, result.Shows[0].ShowID)

	for i := 0; i < 4; i++ {
		show := suite.createApprovedShow(fmt.Sprintf("TD Pad %d", i), venue.ID, artist.ID, user.ID, today.AddDate(0, 0, 15+i))
		suite.createSaves(show.ID, savers, 3)
	}
	result, err = suite.chartsService.GetMostAnticipatedShows(20)
	suite.Require().NoError(err)
	suite.Equal(contracts.MostAnticipatedModeRanked, result.Mode, "today's show is the 5th qualifier — it must count toward ranked mode")
	suite.Require().Len(result.Shows, 5)
}

// =============================================================================
// GetNewReleases Tests
// =============================================================================

// createDatedRelease creates a release with an explicit graph-added time and
// an optional world release date (nil = unknown, the coalesce falls back to
// the added date).
func (suite *ChartsServiceIntegrationTestSuite) createDatedRelease(title string, releaseDate *string, addedAt time.Time) *catalogm.Release {
	release := &catalogm.Release{
		Title:       title,
		ReleaseDate: releaseDate,
		CreatedAt:   addedAt,
	}
	suite.Require().NoError(suite.db.Create(release).Error)
	return release
}

func (suite *ChartsServiceIntegrationTestSuite) addLabelToRelease(releaseID uint, name string) {
	slug := fmt.Sprintf("%s-%d", name, releaseID)
	label := &catalogm.Label{Name: name, Slug: &slug}
	suite.Require().NoError(suite.db.Create(label).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ReleaseLabel{ReleaseID: releaseID, LabelID: label.ID}).Error)
}

func dateStr(t time.Time) *string {
	s := t.Format("2006-01-02")
	return &s
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetNewReleases_Empty() {
	releases, err := suite.chartsService.GetNewReleases(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Empty(releases)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetNewReleases_CoalescedDateOrdering() {
	now := time.Now().UTC()
	// World-dated 5 days ago but added long ago: orders by release_date.
	worldDated := suite.createDatedRelease("World Dated", dateStr(now.AddDate(0, 0, -5)), now.AddDate(0, 0, -80))
	// No release_date, added 2 days ago: coalesce falls back to added date,
	// which is NEWER than the world-dated release above.
	suite.createDatedRelease("Graph New", nil, now.AddDate(0, 0, -2))

	releases, err := suite.chartsService.GetNewReleases(contracts.ChartWindowMonth, 20)
	suite.Require().NoError(err)
	suite.Require().Len(releases, 2)
	suite.Equal("Graph New", releases[0].Title, "coalesced date orders the graph-added fallback above an older world date")
	suite.Equal("World Dated", releases[1].Title)
	suite.Nil(releases[0].ReleaseDate, "unknown world date stays null so the frontend can badge graph-new")
	suite.Require().NotNil(releases[1].ReleaseDate)
	suite.Equal(worldDated.ID, releases[1].ReleaseID)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetNewReleases_WindowBoundaries() {
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	// Exactly on the month window's inclusive lower edge (30 days), first
	// excluded day (31), quarter-only (60), all_time-only (200).
	suite.createDatedRelease("Edge Thirty", dateStr(today.AddDate(0, 0, -30)), now.AddDate(0, 0, -200))
	suite.createDatedRelease("Thirty One", dateStr(today.AddDate(0, 0, -31)), now.AddDate(0, 0, -200))
	suite.createDatedRelease("Sixty", dateStr(today.AddDate(0, 0, -60)), now.AddDate(0, 0, -200))
	suite.createDatedRelease("Ancient", dateStr(today.AddDate(0, 0, -200)), now.AddDate(0, 0, -200))

	month, err := suite.chartsService.GetNewReleases(contracts.ChartWindowMonth, 20)
	suite.Require().NoError(err)
	suite.Require().Len(month, 1, "month window keeps the inclusive 30-day edge, drops day 31")
	suite.Equal("Edge Thirty", month[0].Title)

	quarter, err := suite.chartsService.GetNewReleases(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Len(quarter, 3, "quarter adds days 31 and 60")

	allTime, err := suite.chartsService.GetNewReleases(contracts.ChartWindowAllTime, 20)
	suite.Require().NoError(err)
	suite.Len(allTime, 4)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetNewReleases_ExcludesFutureDated() {
	now := time.Now().UTC()
	suite.createDatedRelease("Announced Preorder", dateStr(now.AddDate(0, 0, 14)), now.AddDate(0, 0, -1))
	included := suite.createDatedRelease("Out Today", dateStr(now), now.AddDate(0, 0, -10))

	releases, err := suite.chartsService.GetNewReleases(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(releases, 1, "a future release_date stays out until its release day; today counts")
	suite.Equal(included.ID, releases[0].ReleaseID)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetNewReleases_NoEngagementDependence() {
	now := time.Now().UTC()
	user := suite.createUser("nr-user@test.com")
	older := suite.createDatedRelease("Older But Bookmarked", dateStr(now.AddDate(0, 0, -20)), now.AddDate(0, 0, -20))
	suite.createBookmark(user.ID, engagementm.BookmarkEntityRelease, older.ID, engagementm.BookmarkActionBookmark)
	newer := suite.createDatedRelease("Newer No Bookmarks", dateStr(now.AddDate(0, 0, -2)), now.AddDate(0, 0, -2))

	releases, err := suite.chartsService.GetNewReleases(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(releases, 2)
	suite.Equal(newer.ID, releases[0].ReleaseID, "date orders the list; bookmarks must not")
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetNewReleases_ArtistAndLabelJoins() {
	now := time.Now().UTC()
	release := suite.createDatedRelease("Joined Release", dateStr(now.AddDate(0, 0, -3)), now.AddDate(0, 0, -3))
	first := suite.createArtist("First Credit")
	second := suite.createArtist("Second Credit")
	suite.addArtistToRelease(first.ID, release.ID)
	ar := &catalogm.ArtistRelease{ArtistID: second.ID, ReleaseID: release.ID, Role: catalogm.ArtistReleaseRoleMain, Position: 1}
	suite.Require().NoError(suite.db.Create(ar).Error)
	suite.addLabelToRelease(release.ID, "Sub Rosa")

	bare := suite.createDatedRelease("Bare Release", dateStr(now.AddDate(0, 0, -4)), now.AddDate(0, 0, -4))

	releases, err := suite.chartsService.GetNewReleases(contracts.ChartWindowQuarter, 20)
	suite.Require().NoError(err)
	suite.Require().Len(releases, 2)
	suite.Equal([]string{"First Credit", "Second Credit"}, releases[0].ArtistNames, "artist credits in position order")
	suite.Equal([]string{"Sub Rosa"}, releases[0].LabelNames)
	suite.Equal(bare.ID, releases[1].ReleaseID)
	suite.Empty(releases[1].ArtistNames, "no credits -> empty slice, not nil panic")
	suite.Empty(releases[1].LabelNames)
}

func (suite *ChartsServiceIntegrationTestSuite) TestGetNewReleases_RespectsLimit() {
	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		suite.createDatedRelease(fmt.Sprintf("Limit Release %d", i), dateStr(now.AddDate(0, 0, -1-i)), now.AddDate(0, 0, -1-i))
	}
	releases, err := suite.chartsService.GetNewReleases(contracts.ChartWindowQuarter, 2)
	suite.Require().NoError(err)
	suite.Require().Len(releases, 2)
	suite.Equal("Limit Release 0", releases[0].Title, "newest first")
}
