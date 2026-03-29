package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewSceneService(t *testing.T) {
	svc := NewSceneService(nil)
	assert.NotNil(t, svc)
}

func TestBuildSceneSlug(t *testing.T) {
	tests := []struct {
		city, state, expected string
	}{
		{"Phoenix", "AZ", "phoenix-az"},
		{"New York", "NY", "new-york-ny"},
		{"San Francisco", "CA", "san-francisco-ca"},
		{"Mesa", "AZ", "mesa-az"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, buildSceneSlug(tc.city, tc.state))
		})
	}
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type SceneServiceIntegrationTestSuite struct {
	suite.Suite
	testDB       *testutil.TestDatabase
	db           *gorm.DB
	sceneService *SceneService
}

func (suite *SceneServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.sceneService = &SceneService{db: suite.testDB.DB}
}

func (suite *SceneServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *SceneServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestSceneServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(SceneServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) createVerifiedVenue(name, city, state string) *models.Venue {
	venue := &models.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: true,
	}
	// Create as verified=true, then update to true (GORM bool gotcha: false is zero-value)
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	// Explicitly set Verified = true
	suite.db.Model(venue).Update("verified", true)
	return venue
}

func (suite *SceneServiceIntegrationTestSuite) createUnverifiedVenue(name, city, state string) *models.Venue {
	venue := &models.Venue{
		Name:  name,
		City:  city,
		State: state,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	// Explicitly set Verified = false (GORM bool gotcha: default is true in DB)
	suite.db.Model(venue).Update("verified", false)
	return venue
}

func (suite *SceneServiceIntegrationTestSuite) createArtist(name string) *models.Artist {
	artist := &models.Artist{Name: name}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *SceneServiceIntegrationTestSuite) createUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("scene-user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *SceneServiceIntegrationTestSuite) createApprovedShow(title string, venueID, artistID, userID uint, eventDate time.Time) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   eventDate,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)

	err = suite.db.Create(&models.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error
	suite.Require().NoError(err)

	err = suite.db.Create(&models.ShowArtist{ShowID: show.ID, ArtistID: artistID, Position: 0}).Error
	suite.Require().NoError(err)

	return show
}

func (suite *SceneServiceIntegrationTestSuite) createFestival(name, city, state string) {
	festival := &models.Festival{
		Name:        name,
		Slug:        fmt.Sprintf("%s-%d", name, time.Now().UnixNano()),
		SeriesSlug:  name,
		EditionYear: 2026,
		City:        stringPtr(city),
		State:       stringPtr(state),
		StartDate:   "2026-03-01",
		EndDate:     "2026-03-03",
	}
	err := suite.db.Create(festival).Error
	suite.Require().NoError(err)
}

// seedSceneData creates the minimum data for Phoenix to qualify as a scene:
// 3 verified venues + 5 upcoming shows with artists.
func (suite *SceneServiceIntegrationTestSuite) seedSceneData() (venues []*models.Venue, artists []*models.Artist) {
	user := suite.createUser()

	v1 := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	venues = []*models.Venue{v1, v2, v3}

	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	a3 := suite.createArtist("Band C")
	artists = []*models.Artist{a1, a2, a3}

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("Show 1", v1.ID, a1.ID, user.ID, future)
	suite.createApprovedShow("Show 2", v1.ID, a2.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Show 3", v2.ID, a1.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("Show 4", v2.ID, a3.ID, user.ID, future.AddDate(0, 0, 3))
	suite.createApprovedShow("Show 5", v3.ID, a2.ID, user.ID, future.AddDate(0, 0, 4))

	return venues, artists
}

// =============================================================================
// ListScenes Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_Empty() {
	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Empty(scenes)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_BelowThreshold_TooFewVenues() {
	// Only 2 verified venues — below the 3-venue threshold
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Venue A", "Tucson", "AZ")
	v2 := suite.createVerifiedVenue("Venue B", "Tucson", "AZ")
	a := suite.createArtist("Tucson Band")

	future := time.Now().UTC().AddDate(0, 0, 7)
	for i := 0; i < 6; i++ {
		venueID := v1.ID
		if i%2 == 1 {
			venueID = v2.ID
		}
		suite.createApprovedShow(fmt.Sprintf("Tucson Show %d", i), venueID, a.ID, user.ID, future.AddDate(0, 0, i))
	}

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Empty(scenes)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_BelowThreshold_TooFewShows() {
	// 3 venues but only 4 upcoming shows (below 5 threshold)
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Venue X", "Flagstaff", "AZ")
	v2 := suite.createVerifiedVenue("Venue Y", "Flagstaff", "AZ")
	v3 := suite.createVerifiedVenue("Venue Z", "Flagstaff", "AZ")
	a := suite.createArtist("Flagstaff Band")

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("Flag Show 1", v1.ID, a.ID, user.ID, future)
	suite.createApprovedShow("Flag Show 2", v2.ID, a.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Flag Show 3", v3.ID, a.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("Flag Show 4", v1.ID, a.ID, user.ID, future.AddDate(0, 0, 3))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Empty(scenes)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_MeetsThreshold() {
	suite.seedSceneData()

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)

	scene := scenes[0]
	suite.Equal("Phoenix", scene.City)
	suite.Equal("AZ", scene.State)
	suite.Equal("phoenix-az", scene.Slug)
	suite.GreaterOrEqual(scene.VenueCount, 3)
	suite.GreaterOrEqual(scene.UpcomingShowCount, 5)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_MultipleScenes() {
	// Phoenix scene
	suite.seedSceneData()

	// Chicago scene
	user := suite.createUser()
	cv1 := suite.createVerifiedVenue("Metro", "Chicago", "IL")
	cv2 := suite.createVerifiedVenue("Empty Bottle", "Chicago", "IL")
	cv3 := suite.createVerifiedVenue("Thalia Hall", "Chicago", "IL")
	ca := suite.createArtist("Chicago Band")

	future := time.Now().UTC().AddDate(0, 0, 7)
	for i := 0; i < 7; i++ {
		venues := []*models.Venue{cv1, cv2, cv3}
		suite.createApprovedShow(
			fmt.Sprintf("Chi Show %d", i),
			venues[i%3].ID, ca.ID, user.ID,
			future.AddDate(0, 0, i),
		)
	}

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 2)

	// Should be sorted by upcoming show count descending
	// Chicago has 7, Phoenix has 5
	suite.Equal("Chicago", scenes[0].City)
	suite.Equal("Phoenix", scenes[1].City)
}

// =============================================================================
// GetSceneDetail Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_Success() {
	suite.seedSceneData()

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().NotNil(detail)

	suite.Equal("Phoenix", detail.City)
	suite.Equal("AZ", detail.State)
	suite.Equal("phoenix-az", detail.Slug)
	suite.Nil(detail.Description) // no scenes table yet

	// Stats
	suite.GreaterOrEqual(detail.Stats.VenueCount, 3)
	suite.GreaterOrEqual(detail.Stats.ArtistCount, 1)
	suite.GreaterOrEqual(detail.Stats.UpcomingShowCount, 5)

	// Pulse
	suite.NotNil(detail.Pulse.ShowsByMonth)
	suite.Len(detail.Pulse.ShowsByMonth, 6)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_NotFound() {
	detail, err := suite.sceneService.GetSceneDetail("Nonexistent", "XX")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
	suite.Nil(detail)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenueCountOnlyVerified() {
	suite.seedSceneData()
	// Add an unverified venue — should not be counted
	suite.createUnverifiedVenue("Sketchy Bar", "Phoenix", "AZ")

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(3, detail.Stats.VenueCount) // only the 3 verified ones
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_ArtistCount() {
	_, artists := suite.seedSceneData()
	// seedSceneData creates 3 artists across 5 shows
	_ = artists

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(3, detail.Stats.ArtistCount) // 3 distinct artists
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_FestivalCount() {
	suite.seedSceneData()
	suite.createFestival("M3F Fest", "Phoenix", "AZ")
	suite.createFestival("Arizona Roots", "Phoenix", "AZ")

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(2, detail.Stats.FestivalCount)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_PulseShowsByMonth() {
	// Create shows across different months
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("V3", "Phoenix", "AZ")
	a := suite.createArtist("Monthly Band")

	now := time.Now().UTC()
	thisMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Create shows in current month (count as upcoming too for threshold)
	for i := 0; i < 3; i++ {
		// Use dates in the future portion of this month
		showDate := thisMonthStart.AddDate(0, 1, -1) // last day of this month
		suite.createApprovedShow(
			fmt.Sprintf("This Month Show %d", i),
			[]*models.Venue{v1, v2, v3}[i%3].ID, a.ID, user.ID,
			showDate,
		)
	}

	// Create shows in previous month
	prevMonth := thisMonthStart.AddDate(0, -1, 5)
	suite.createApprovedShow("Prev Month Show 1", v1.ID, a.ID, user.ID, prevMonth)
	suite.createApprovedShow("Prev Month Show 2", v2.ID, a.ID, user.ID, prevMonth.AddDate(0, 0, 1))

	// Also create upcoming shows to meet threshold
	future := now.AddDate(0, 0, 7)
	suite.createApprovedShow("Future 1", v1.ID, a.ID, user.ID, future)
	suite.createApprovedShow("Future 2", v2.ID, a.ID, user.ID, future.AddDate(0, 0, 1))

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)

	// Shows by month should have 6 entries
	suite.Len(detail.Pulse.ShowsByMonth, 6)
	// Last entry (index 5) is current month — should have 3+ shows
	suite.GreaterOrEqual(detail.Pulse.ShowsByMonth[5], 3)
	// Second to last (index 4) is previous month — should have 2 shows
	suite.Equal(2, detail.Pulse.ShowsByMonth[4])
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_PulseShowsTrend() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Venue 1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Venue 2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("Venue 3", "Phoenix", "AZ")
	a := suite.createArtist("Trend Band")

	now := time.Now().UTC()
	thisMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// 5 shows this month
	for i := 0; i < 5; i++ {
		showDate := thisMonthStart.AddDate(0, 1, -1)
		suite.createApprovedShow(
			fmt.Sprintf("This Month %d", i),
			[]*models.Venue{v1, v2, v3}[i%3].ID, a.ID, user.ID,
			showDate,
		)
	}

	// 2 shows previous month
	prevMonth := thisMonthStart.AddDate(0, -1, 5)
	suite.createApprovedShow("Prev 1", v1.ID, a.ID, user.ID, prevMonth)
	suite.createApprovedShow("Prev 2", v2.ID, a.ID, user.ID, prevMonth.AddDate(0, 0, 1))

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)

	suite.Equal("+3", detail.Pulse.ShowsTrend) // 5 - 2 = +3
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_PulseNewArtists() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("PNV1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("PNV2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("PNV3", "Phoenix", "AZ")

	// Old artist — first show 60 days ago
	oldArtist := suite.createArtist("Old Band")
	past := time.Now().UTC().AddDate(0, 0, -60)
	suite.createApprovedShow("Old Show", v1.ID, oldArtist.ID, user.ID, past)

	// New artist — first show 10 days ago
	newArtist := suite.createArtist("New Band")
	recent := time.Now().UTC().AddDate(0, 0, -10)
	suite.createApprovedShow("New Show", v2.ID, newArtist.ID, user.ID, recent)

	// Another new artist — first show 5 days ago
	newerArtist := suite.createArtist("Newer Band")
	moreRecent := time.Now().UTC().AddDate(0, 0, -5)
	suite.createApprovedShow("Newer Show", v3.ID, newerArtist.ID, user.ID, moreRecent)

	// Need 5+ upcoming shows for threshold
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("F1", v1.ID, oldArtist.ID, user.ID, future)
	suite.createApprovedShow("F2", v2.ID, newArtist.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("F3", v3.ID, newerArtist.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("F4", v1.ID, newArtist.ID, user.ID, future.AddDate(0, 0, 3))
	suite.createApprovedShow("F5", v2.ID, oldArtist.ID, user.ID, future.AddDate(0, 0, 4))

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)

	// 2 new artists (first show in last 30 days)
	suite.Equal(2, detail.Pulse.NewArtists30d)
}

// =============================================================================
// GetActiveArtists Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_Success() {
	_, artists := suite.seedSceneData()
	// Band A has 2 shows (at v1 and v2), Band B has 2 shows (at v1 and v3), Band C has 1 show (at v2)
	_ = artists

	results, total, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(results, 3)

	// First should be highest show count (Band A or Band B, both have 2)
	suite.Equal(2, results[0].ShowCount)
	suite.Equal(2, results[1].ShowCount)
	suite.Equal(1, results[2].ShowCount)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_RespectsLimit() {
	suite.seedSceneData()

	results, total, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(results, 2)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_RespectsOffset() {
	suite.seedSceneData()

	results, total, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 2)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(results, 1) // 3 total, offset 2 = 1 remaining
}

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_Period() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Period V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Period V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("Period V3", "Phoenix", "AZ")

	recentArtist := suite.createArtist("Recent Artist")
	oldArtist := suite.createArtist("Old Artist")

	// Recent show (10 days ago)
	recent := time.Now().UTC().AddDate(0, 0, -10)
	suite.createApprovedShow("Recent Show", v1.ID, recentArtist.ID, user.ID, recent)

	// Old show (100 days ago — outside 90 day period)
	old := time.Now().UTC().AddDate(0, 0, -100)
	suite.createApprovedShow("Old Show", v2.ID, oldArtist.ID, user.ID, old)

	// Need upcoming shows for the scene threshold
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("F1", v1.ID, recentArtist.ID, user.ID, future)
	suite.createApprovedShow("F2", v2.ID, recentArtist.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("F3", v3.ID, recentArtist.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("F4", v1.ID, recentArtist.ID, user.ID, future.AddDate(0, 0, 3))
	suite.createApprovedShow("F5", v2.ID, recentArtist.ID, user.ID, future.AddDate(0, 0, 4))

	// With 90-day period: should only include recentArtist
	results, total, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 90, 20, 0)
	suite.Require().NoError(err)
	// recentArtist has shows within 90 days; oldArtist does not
	suite.Equal(int64(1), total)
	suite.Len(results, 1)
	suite.Equal("Recent Artist", results[0].Name)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_NotFound() {
	results, total, err := suite.sceneService.GetActiveArtists("Nowhere", "XX", 90, 20, 0)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
	suite.Nil(results)
	suite.Zero(total)
}

// =============================================================================
// ParseSceneSlug Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_Success() {
	suite.createVerifiedVenue("Test Venue", "Phoenix", "AZ")

	city, state, err := suite.sceneService.ParseSceneSlug("phoenix-az")
	suite.Require().NoError(err)
	suite.Equal("Phoenix", city)
	suite.Equal("AZ", state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_MultiWordCity() {
	suite.createVerifiedVenue("Test Venue", "New York", "NY")

	city, state, err := suite.sceneService.ParseSceneSlug("new-york-ny")
	suite.Require().NoError(err)
	suite.Equal("New York", city)
	suite.Equal("NY", state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_NotFound() {
	city, state, err := suite.sceneService.ParseSceneSlug("nonexistent-xx")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
	suite.Empty(city)
	suite.Empty(state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_IgnoresUnverifiedVenues() {
	suite.createUnverifiedVenue("Unverified Place", "Unverified City", "UC")

	city, state, err := suite.sceneService.ParseSceneSlug("unverified-city-uc")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
	suite.Empty(city)
	suite.Empty(state)
}

// =============================================================================
// NormalizedShannonEntropy Unit Tests
// =============================================================================

func TestNormalizedShannonEntropy_Empty(t *testing.T) {
	assert.Equal(t, 0.0, NormalizedShannonEntropy([]int{}))
}

func TestNormalizedShannonEntropy_SingleGenre(t *testing.T) {
	// Only 1 genre => max entropy = log2(1) = 0, so we return 0 (avoid div-by-zero)
	assert.Equal(t, 0.0, NormalizedShannonEntropy([]int{100}))
}

func TestNormalizedShannonEntropy_EqualDistribution(t *testing.T) {
	// Perfectly even distribution of 4 genres => normalized entropy = 1.0
	result := NormalizedShannonEntropy([]int{25, 25, 25, 25})
	assert.InDelta(t, 1.0, result, 0.001)
}

func TestNormalizedShannonEntropy_UnevenDistribution(t *testing.T) {
	// One dominant genre => low entropy
	result := NormalizedShannonEntropy([]int{90, 5, 3, 2})
	assert.Greater(t, result, 0.0)
	assert.Less(t, result, 0.6) // should be low
}

func TestNormalizedShannonEntropy_TwoGenres(t *testing.T) {
	// 50/50 split with 2 genres => normalized entropy = 1.0
	result := NormalizedShannonEntropy([]int{50, 50})
	assert.InDelta(t, 1.0, result, 0.001)
}

func TestNormalizedShannonEntropy_AllZeros(t *testing.T) {
	assert.Equal(t, 0.0, NormalizedShannonEntropy([]int{0, 0, 0}))
}

// =============================================================================
// DiversityLabel Unit Tests
// =============================================================================

func TestDiversityLabel(t *testing.T) {
	tests := []struct {
		index    float64
		expected string
	}{
		{-1, ""},
		{0.1, ""},
		{0.19, ""},
		{0.2, "Genre-focused"},
		{0.4, "Genre-focused"},
		{0.5, "Mixed"},
		{0.7, "Mixed"},
		{0.8, "Highly diverse"},
		{0.95, "Highly diverse"},
		{1.0, "Highly diverse"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%.2f", tc.index), func(t *testing.T) {
			assert.Equal(t, tc.expected, DiversityLabel(tc.index))
		})
	}
}

// =============================================================================
// Genre Distribution Integration Tests
// =============================================================================

// createGenreTag creates a genre tag for testing
func (suite *SceneServiceIntegrationTestSuite) createGenreTag(name, slug string) uint {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	var tagID uint
	err = sqlDB.QueryRow(`
		INSERT INTO tags (name, slug, category, is_official, usage_count, created_at, updated_at)
		VALUES ($1, $2, 'genre', true, 0, NOW(), NOW())
		RETURNING id
	`, name, slug).Scan(&tagID)
	suite.Require().NoError(err)
	return tagID
}

// tagArtist tags an artist with a genre tag
func (suite *SceneServiceIntegrationTestSuite) tagArtist(artistID, tagID, userID uint) {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, err = sqlDB.Exec(`
		INSERT INTO entity_tags (entity_type, entity_id, tag_id, added_by_user_id, created_at)
		VALUES ('artist', $1, $2, $3, NOW())
	`, artistID, tagID, userID)
	suite.Require().NoError(err)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGenreDistribution_InsufficientData() {
	// Seed scene with 3 venues and 5 shows (3 artists), no tags
	suite.seedSceneData()

	genres, err := suite.sceneService.GetSceneGenreDistribution("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Empty(genres) // No tagged artists at all
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGenreDistribution_BelowThreshold() {
	// Create scene data with a few tagged artists (below 30 threshold)
	venues, artists := suite.seedSceneData()
	_ = venues
	user := suite.createUser()

	punkTag := suite.createGenreTag("punk", "punk")
	suite.tagArtist(artists[0].ID, punkTag, user.ID) // 1 tagged artist, well below 30

	genres, err := suite.sceneService.GetSceneGenreDistribution("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Empty(genres) // Below threshold
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGenreDistribution_Success() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("G-V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("G-V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("G-V3", "Phoenix", "AZ")

	punkTag := suite.createGenreTag("punk", "punk")
	indieTag := suite.createGenreTag("indie rock", "indie-rock")
	metalTag := suite.createGenreTag("metal", "metal")

	future := time.Now().UTC().AddDate(0, 0, 7)

	// Create 35 artists with shows, tag them with genres
	// This ensures we meet the 30 tagged artist threshold
	venues := []*models.Venue{v1, v2, v3}
	tags := []uint{punkTag, punkTag, indieTag, indieTag, indieTag, metalTag}
	for i := 0; i < 35; i++ {
		a := suite.createArtist(fmt.Sprintf("Genre Artist %d", i))
		suite.createApprovedShow(
			fmt.Sprintf("Genre Show %d", i),
			venues[i%3].ID, a.ID, user.ID,
			future.AddDate(0, 0, i),
		)
		tagIdx := i % len(tags)
		suite.tagArtist(a.ID, tags[tagIdx], user.ID)
	}

	genres, err := suite.sceneService.GetSceneGenreDistribution("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.NotEmpty(genres)

	// Should be sorted by count DESC
	suite.GreaterOrEqual(genres[0].Count, genres[len(genres)-1].Count)

	// All genres should have tag_id, name, and slug
	for _, g := range genres {
		suite.NotZero(g.TagID)
		suite.NotEmpty(g.Name)
		suite.NotEmpty(g.Slug)
		suite.Greater(g.Count, 0)
	}
}

// =============================================================================
// Genre Diversity Index Integration Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestGetGenreDiversityIndex_InsufficientArtists() {
	suite.seedSceneData()
	// No tags => insufficient data
	index, err := suite.sceneService.GetGenreDiversityIndex("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(-1.0, index)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetGenreDiversityIndex_InsufficientGenres() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("DI-V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("DI-V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("DI-V3", "Phoenix", "AZ")

	punkTag := suite.createGenreTag("di-punk", "di-punk")

	future := time.Now().UTC().AddDate(0, 0, 7)
	venues := []*models.Venue{v1, v2, v3}

	// 55 artists all tagged with one genre => only 1 genre, below 5 minimum
	for i := 0; i < 55; i++ {
		a := suite.createArtist(fmt.Sprintf("DI Artist %d", i))
		suite.createApprovedShow(
			fmt.Sprintf("DI Show %d", i),
			venues[i%3].ID, a.ID, user.ID,
			future.AddDate(0, 0, i),
		)
		suite.tagArtist(a.ID, punkTag, user.ID)
	}

	index, err := suite.sceneService.GetGenreDiversityIndex("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(-1.0, index) // Insufficient genres (only 1)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetGenreDiversityIndex_Success() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("DIX-V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("DIX-V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("DIX-V3", "Phoenix", "AZ")

	// Create 6 genres to meet the 5-genre minimum
	genreTags := []uint{
		suite.createGenreTag("dix-punk", "dix-punk"),
		suite.createGenreTag("dix-indie", "dix-indie"),
		suite.createGenreTag("dix-metal", "dix-metal"),
		suite.createGenreTag("dix-jazz", "dix-jazz"),
		suite.createGenreTag("dix-electronic", "dix-electronic"),
		suite.createGenreTag("dix-folk", "dix-folk"),
	}

	future := time.Now().UTC().AddDate(0, 0, 7)
	venues := []*models.Venue{v1, v2, v3}

	// Create 55 artists evenly distributed across genres
	for i := 0; i < 55; i++ {
		a := suite.createArtist(fmt.Sprintf("DIX Artist %d", i))
		suite.createApprovedShow(
			fmt.Sprintf("DIX Show %d", i),
			venues[i%3].ID, a.ID, user.ID,
			future.AddDate(0, 0, i),
		)
		suite.tagArtist(a.ID, genreTags[i%len(genreTags)], user.ID)
	}

	index, err := suite.sceneService.GetGenreDiversityIndex("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Greater(index, 0.0)
	suite.LessOrEqual(index, 1.0)
	// With nearly even distribution across 6 genres, expect high diversity
	suite.Greater(index, 0.8)
}
