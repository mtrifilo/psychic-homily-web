package notification

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestHasAnyCriteria(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.False(t, hasAnyCriteria(contracts.CreateFilterInput{}))
	})
	t.Run("artist_ids", func(t *testing.T) {
		assert.True(t, hasAnyCriteria(contracts.CreateFilterInput{ArtistIDs: []int64{1}}))
	})
	t.Run("venue_ids", func(t *testing.T) {
		assert.True(t, hasAnyCriteria(contracts.CreateFilterInput{VenueIDs: []int64{1}}))
	})
	t.Run("label_ids", func(t *testing.T) {
		assert.True(t, hasAnyCriteria(contracts.CreateFilterInput{LabelIDs: []int64{1}}))
	})
	t.Run("tag_ids", func(t *testing.T) {
		assert.True(t, hasAnyCriteria(contracts.CreateFilterInput{TagIDs: []int64{1}}))
	})
	t.Run("exclude_tag_ids", func(t *testing.T) {
		assert.True(t, hasAnyCriteria(contracts.CreateFilterInput{ExcludeTagIDs: []int64{1}}))
	})
	t.Run("cities", func(t *testing.T) {
		assert.True(t, hasAnyCriteria(contracts.CreateFilterInput{
			Cities: json.RawMessage(`[{"city":"Phoenix","state":"AZ"}]`),
		}))
	})
	t.Run("price_max_cents", func(t *testing.T) {
		cents := 2500
		assert.True(t, hasAnyCriteria(contracts.CreateFilterInput{PriceMaxCents: &cents}))
	})
}

func TestToInt64Array(t *testing.T) {
	assert.Nil(t, toInt64Array(nil))
	assert.Nil(t, toInt64Array([]int64{}))
	assert.Equal(t, pq.Int64Array{1, 2, 3}, toInt64Array([]int64{1, 2, 3}))
}

func TestFilterUnsubscribeSignature(t *testing.T) {
	secret := "test-secret"
	filterID := uint(42)

	sig := ComputeFilterUnsubscribeSignature(filterID, secret)
	assert.NotEmpty(t, sig)

	assert.True(t, VerifyFilterUnsubscribeSignature(filterID, sig, secret))
	assert.False(t, VerifyFilterUnsubscribeSignature(filterID, "wrong-sig", secret))
	assert.False(t, VerifyFilterUnsubscribeSignature(99, sig, secret))
}

func TestGenerateFilterUnsubscribeURL(t *testing.T) {
	url := GenerateFilterUnsubscribeURL("https://example.com", 42, "secret")
	assert.Contains(t, url, "https://example.com/unsubscribe/filter/42")
	assert.Contains(t, url, "sig=")
}

func TestBuildFilterEmailHTML(t *testing.T) {
	html := buildFilterEmailHTML("PHX punk shows", "Deafheaven at Rebel Lounge", "Monday, March 15, 2026", "The Rebel Lounge", "Deafheaven, Touche Amore", "$25", "https://example.com/shows/1", "https://example.com/unsubscribe/filter/1")
	assert.Contains(t, html, "PHX punk shows")
	assert.Contains(t, html, "Deafheaven at Rebel Lounge")
	assert.Contains(t, html, "Monday, March 15, 2026")
	assert.Contains(t, html, "The Rebel Lounge")
	assert.Contains(t, html, "Deafheaven, Touche Amore")
	assert.Contains(t, html, "$25")
	assert.Contains(t, html, "https://example.com/shows/1")
	assert.Contains(t, html, "https://example.com/unsubscribe/filter/1")
}

func TestMatchAndNotify_NilShow(t *testing.T) {
	svc := &NotificationFilterService{db: &gorm.DB{}}
	err := svc.MatchAndNotify(nil)
	assert.NoError(t, err)
}

// =============================================================================
// INTEGRATION TESTS (Testcontainer PostgreSQL)
// =============================================================================

type NotificationFilterSuite struct {
	suite.Suite
	db     *gorm.DB
	testDB *testutil.TestDatabase
	svc    *NotificationFilterService
}

func TestNotificationFilterSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(NotificationFilterSuite))
}

func (s *NotificationFilterSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	// Use a mock email service
	s.svc = NewNotificationFilterService(s.testDB.DB, &mockEmailService{}, "test-secret", "http://localhost:3000")
}

func (s *NotificationFilterSuite) TearDownTest() {
	// Clean up test data between tests
	s.db.Exec("DELETE FROM notification_log")
	s.db.Exec("DELETE FROM notification_filters")
	s.db.Exec("DELETE FROM show_artists")
	s.db.Exec("DELETE FROM show_venues")
	s.db.Exec("DELETE FROM shows")
	s.db.Exec("DELETE FROM artists")
	s.db.Exec("DELETE FROM venues")
	s.db.Exec("DELETE FROM users")
}

func (s *NotificationFilterSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

// createTestUser creates a user for testing.
func (s *NotificationFilterSuite) createTestUser() uint {
	email := fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
	firstName := "Test"
	lastName := "User"
	user := models.User{
		Email:     &email,
		FirstName: &firstName,
		LastName:  &lastName,
	}
	s.Require().NoError(s.db.Create(&user).Error)
	return user.ID
}

// createTestArtist creates an artist for testing.
func (s *NotificationFilterSuite) createTestArtist(name string) uint {
	slug := name
	artist := models.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(&artist).Error)
	return artist.ID
}

// createTestVenue creates a venue for testing.
func (s *NotificationFilterSuite) createTestVenue(name string) uint {
	slug := name
	venue := models.Venue{Name: name, Slug: &slug, City: "Phoenix", State: "AZ"}
	s.Require().NoError(s.db.Create(&venue).Error)
	return venue.ID
}

// createTestShow creates a show for testing.
func (s *NotificationFilterSuite) createTestShow(title string, artistIDs []uint, venueIDs []uint) uint {
	city := "Phoenix"
	state := "AZ"
	slug := title
	price := 25.0
	show := models.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: time.Now().Add(24 * time.Hour),
		City:      &city,
		State:     &state,
		Price:     &price,
		Status:    models.ShowStatusApproved,
	}
	s.Require().NoError(s.db.Create(&show).Error)

	for _, id := range artistIDs {
		s.Require().NoError(s.db.Exec("INSERT INTO show_artists (show_id, artist_id, position) VALUES (?, ?, 0)", show.ID, id).Error)
	}
	for _, id := range venueIDs {
		s.Require().NoError(s.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, id).Error)
	}

	return show.ID
}

// --- CRUD Tests ---

func (s *NotificationFilterSuite) TestCreateFilter() {
	userID := s.createTestUser()

	filter, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:        "PHX punk shows",
		ArtistIDs:   []int64{1, 2},
		VenueIDs:    []int64{3},
		NotifyEmail: true,
		NotifyInApp: true,
	})
	s.Require().NoError(err)
	s.Assert().NotZero(filter.ID)
	s.Assert().Equal("PHX punk shows", filter.Name)
	s.Assert().True(filter.IsActive)
	s.Assert().Equal(pq.Int64Array{1, 2}, filter.ArtistIDs)
	s.Assert().Equal(pq.Int64Array{3}, filter.VenueIDs)
	s.Assert().True(filter.NotifyEmail)
	s.Assert().True(filter.NotifyInApp)
	s.Assert().False(filter.NotifyPush)
}

func (s *NotificationFilterSuite) TestCreateFilter_NoCriteria() {
	userID := s.createTestUser()

	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name: "Empty filter",
	})
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "at least one filter criteria")
}

func (s *NotificationFilterSuite) TestCreateFilter_WithCities() {
	userID := s.createTestUser()

	cities := json.RawMessage(`[{"city":"Phoenix","state":"AZ"}]`)
	filter, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:   "PHX filter",
		Cities: cities,
	})
	s.Require().NoError(err)
	s.Assert().NotNil(filter.Cities)
}

func (s *NotificationFilterSuite) TestCreateFilter_WithPriceMax() {
	userID := s.createTestUser()

	cents := 2500
	filter, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:          "Cheap shows",
		PriceMaxCents: &cents,
	})
	s.Require().NoError(err)
	s.Assert().NotNil(filter.PriceMaxCents)
	s.Assert().Equal(2500, *filter.PriceMaxCents)
}

func (s *NotificationFilterSuite) TestCreateFilter_MaxLimit() {
	userID := s.createTestUser()

	// Create max filters
	for i := 0; i < maxFiltersPerUser; i++ {
		_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
			Name:      fmt.Sprintf("Filter %d", i),
			ArtistIDs: []int64{int64(i + 1)},
		})
		s.Require().NoError(err)
	}

	// One more should fail
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "One too many",
		ArtistIDs: []int64{99},
	})
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "maximum of")
}

func (s *NotificationFilterSuite) TestGetUserFilters() {
	userID := s.createTestUser()

	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "Filter 1",
		ArtistIDs: []int64{1},
	})
	s.Require().NoError(err)

	_, err = s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:     "Filter 2",
		VenueIDs: []int64{2},
	})
	s.Require().NoError(err)

	filters, err := s.svc.GetUserFilters(userID)
	s.Require().NoError(err)
	s.Assert().Len(filters, 2)
}

func (s *NotificationFilterSuite) TestGetFilter() {
	userID := s.createTestUser()

	created, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "My filter",
		ArtistIDs: []int64{1},
	})
	s.Require().NoError(err)

	filter, err := s.svc.GetFilter(userID, created.ID)
	s.Require().NoError(err)
	s.Assert().Equal(created.ID, filter.ID)
	s.Assert().Equal("My filter", filter.Name)
}

func (s *NotificationFilterSuite) TestGetFilter_WrongUser() {
	userID1 := s.createTestUser()
	userID2 := s.createTestUser()

	created, err := s.svc.CreateFilter(userID1, contracts.CreateFilterInput{
		Name:      "Private filter",
		ArtistIDs: []int64{1},
	})
	s.Require().NoError(err)

	_, err = s.svc.GetFilter(userID2, created.ID)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "filter not found")
}

func (s *NotificationFilterSuite) TestUpdateFilter() {
	userID := s.createTestUser()

	created, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "Original",
		ArtistIDs: []int64{1},
	})
	s.Require().NoError(err)

	newName := "Updated"
	isActive := false
	updated, err := s.svc.UpdateFilter(userID, created.ID, contracts.UpdateFilterInput{
		Name:     &newName,
		IsActive: &isActive,
	})
	s.Require().NoError(err)
	s.Assert().Equal("Updated", updated.Name)
	s.Assert().False(updated.IsActive)
}

func (s *NotificationFilterSuite) TestUpdateFilter_WrongUser() {
	userID1 := s.createTestUser()
	userID2 := s.createTestUser()

	created, err := s.svc.CreateFilter(userID1, contracts.CreateFilterInput{
		Name:      "Private filter",
		ArtistIDs: []int64{1},
	})
	s.Require().NoError(err)

	newName := "Stolen"
	_, err = s.svc.UpdateFilter(userID2, created.ID, contracts.UpdateFilterInput{
		Name: &newName,
	})
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "filter not found")
}

func (s *NotificationFilterSuite) TestDeleteFilter() {
	userID := s.createTestUser()

	created, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "To delete",
		ArtistIDs: []int64{1},
	})
	s.Require().NoError(err)

	err = s.svc.DeleteFilter(userID, created.ID)
	s.Require().NoError(err)

	_, err = s.svc.GetFilter(userID, created.ID)
	s.Assert().Error(err)
}

func (s *NotificationFilterSuite) TestDeleteFilter_WrongUser() {
	userID1 := s.createTestUser()
	userID2 := s.createTestUser()

	created, err := s.svc.CreateFilter(userID1, contracts.CreateFilterInput{
		Name:      "Not yours",
		ArtistIDs: []int64{1},
	})
	s.Require().NoError(err)

	err = s.svc.DeleteFilter(userID2, created.ID)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "filter not found")
}

func (s *NotificationFilterSuite) TestDeleteFilter_NotFound() {
	userID := s.createTestUser()
	err := s.svc.DeleteFilter(userID, 99999)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "filter not found")
}

// --- Quick Create Tests ---

func (s *NotificationFilterSuite) TestQuickCreateFilter_Artist() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Deafheaven")

	filter, err := s.svc.QuickCreateFilter(userID, "artist", artistID)
	s.Require().NoError(err)
	s.Assert().Equal("Deafheaven shows", filter.Name)
	s.Assert().Equal(pq.Int64Array{int64(artistID)}, filter.ArtistIDs)
}

func (s *NotificationFilterSuite) TestQuickCreateFilter_Venue() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("rebel-lounge")

	filter, err := s.svc.QuickCreateFilter(userID, "venue", venueID)
	s.Require().NoError(err)
	s.Assert().Contains(filter.Name, "Shows at")
	s.Assert().Equal(pq.Int64Array{int64(venueID)}, filter.VenueIDs)
}

func (s *NotificationFilterSuite) TestQuickCreateFilter_InvalidType() {
	userID := s.createTestUser()
	_, err := s.svc.QuickCreateFilter(userID, "show", 1)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "invalid entity type")
}

// --- Matching Tests ---

func (s *NotificationFilterSuite) TestMatchAndNotify_ArtistMatch() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Deafheaven")
	venueID := s.createTestVenue("rebel-lounge")

	// Create a filter matching this artist
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:        "Deafheaven shows",
		ArtistIDs:   []int64{int64(artistID)},
		NotifyEmail: true,
	})
	s.Require().NoError(err)

	// Create a show with the artist
	showID := s.createTestShow("Deafheaven live", []uint{artistID}, []uint{venueID})

	// Get the show model
	var show models.Show
	s.Require().NoError(s.db.First(&show, showID).Error)

	// Run matching
	err = s.svc.MatchAndNotify(&show)
	s.Require().NoError(err)

	// Verify notification log entry was created
	var count int64
	s.db.Model(&models.NotificationLog{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ?", userID, "show", showID).
		Count(&count)
	s.Assert().Equal(int64(1), count)
}

func (s *NotificationFilterSuite) TestMatchAndNotify_VenueMatch() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Some Band")
	venueID := s.createTestVenue("the-rebel-lounge")

	// Create a filter matching this venue
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:     "Rebel Lounge shows",
		VenueIDs: []int64{int64(venueID)},
	})
	s.Require().NoError(err)

	showID := s.createTestShow("Some Band live", []uint{artistID}, []uint{venueID})

	var show models.Show
	s.Require().NoError(s.db.First(&show, showID).Error)

	err = s.svc.MatchAndNotify(&show)
	s.Require().NoError(err)

	var count int64
	s.db.Model(&models.NotificationLog{}).
		Where("user_id = ? AND entity_id = ?", userID, showID).
		Count(&count)
	s.Assert().Equal(int64(1), count)
}

func (s *NotificationFilterSuite) TestMatchAndNotify_NoMatch() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Other Band")
	venueID := s.createTestVenue("other-venue")

	// Create a filter for a different artist
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "Wrong artist",
		ArtistIDs: []int64{99999},
	})
	s.Require().NoError(err)

	showID := s.createTestShow("Other show", []uint{artistID}, []uint{venueID})

	var show models.Show
	s.Require().NoError(s.db.First(&show, showID).Error)

	err = s.svc.MatchAndNotify(&show)
	s.Require().NoError(err)

	var count int64
	s.db.Model(&models.NotificationLog{}).
		Where("user_id = ?", userID).
		Count(&count)
	s.Assert().Zero(count)
}

func (s *NotificationFilterSuite) TestMatchAndNotify_InactiveFilterIgnored() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Band")
	venueID := s.createTestVenue("venue")

	// Create a filter then pause it
	filter, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "Paused filter",
		ArtistIDs: []int64{int64(artistID)},
	})
	s.Require().NoError(err)

	// Pause (set is_active = false) — use GORM directly since PauseFilter doesn't check user
	s.db.Model(&models.NotificationFilter{}).Where("id = ?", filter.ID).Update("is_active", false)

	showID := s.createTestShow("Show", []uint{artistID}, []uint{venueID})
	var show models.Show
	s.Require().NoError(s.db.First(&show, showID).Error)

	err = s.svc.MatchAndNotify(&show)
	s.Require().NoError(err)

	var count int64
	s.db.Model(&models.NotificationLog{}).Where("user_id = ?", userID).Count(&count)
	s.Assert().Zero(count)
}

func (s *NotificationFilterSuite) TestMatchAndNotify_Deduplication() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Band2")
	venueID := s.createTestVenue("venue2")

	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "Dedup test",
		ArtistIDs: []int64{int64(artistID)},
	})
	s.Require().NoError(err)

	showID := s.createTestShow("Show2", []uint{artistID}, []uint{venueID})
	var show models.Show
	s.Require().NoError(s.db.First(&show, showID).Error)

	// First match — should create notification
	err = s.svc.MatchAndNotify(&show)
	s.Require().NoError(err)

	// Second match — should NOT create duplicate notification
	err = s.svc.MatchAndNotify(&show)
	s.Require().NoError(err)

	var count int64
	s.db.Model(&models.NotificationLog{}).Where("user_id = ? AND entity_id = ?", userID, showID).Count(&count)
	s.Assert().Equal(int64(1), count, "should not create duplicate notification")
}

func (s *NotificationFilterSuite) TestMatchAndNotify_MultipleFiltersOneShow() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Band3")
	venueID := s.createTestVenue("venue3")

	// Two filters that both match the same show
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "Artist filter",
		ArtistIDs: []int64{int64(artistID)},
	})
	s.Require().NoError(err)

	_, err = s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:     "Venue filter",
		VenueIDs: []int64{int64(venueID)},
	})
	s.Require().NoError(err)

	showID := s.createTestShow("Show3", []uint{artistID}, []uint{venueID})
	var show models.Show
	s.Require().NoError(s.db.First(&show, showID).Error)

	err = s.svc.MatchAndNotify(&show)
	s.Require().NoError(err)

	// Both filters should have matched
	var count int64
	s.db.Model(&models.NotificationLog{}).Where("user_id = ? AND entity_id = ?", userID, showID).Count(&count)
	s.Assert().Equal(int64(2), count)
}

func (s *NotificationFilterSuite) TestMatchAndNotify_UpdatesMatchCount() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Band4")
	venueID := s.createTestVenue("venue4")

	filter, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "Count test",
		ArtistIDs: []int64{int64(artistID)},
	})
	s.Require().NoError(err)
	s.Assert().Equal(0, filter.MatchCount)

	showID := s.createTestShow("Show4", []uint{artistID}, []uint{venueID})
	var show models.Show
	s.Require().NoError(s.db.First(&show, showID).Error)

	err = s.svc.MatchAndNotify(&show)
	s.Require().NoError(err)

	// Reload filter and check match count
	updated, err := s.svc.GetFilter(userID, filter.ID)
	s.Require().NoError(err)
	s.Assert().Equal(1, updated.MatchCount)
	s.Assert().NotNil(updated.LastMatchedAt)
}

func (s *NotificationFilterSuite) TestMatchAndNotify_ANDLogicAcrossCriteria() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Band5")
	venueID1 := s.createTestVenue("venue5a")
	venueID2 := s.createTestVenue("venue5b")

	// Filter requires BOTH this artist AND this venue
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "Specific combo",
		ArtistIDs: []int64{int64(artistID)},
		VenueIDs:  []int64{int64(venueID1)},
	})
	s.Require().NoError(err)

	// Show at wrong venue — should NOT match
	showID := s.createTestShow("Show5", []uint{artistID}, []uint{venueID2})
	var show models.Show
	s.Require().NoError(s.db.First(&show, showID).Error)

	err = s.svc.MatchAndNotify(&show)
	s.Require().NoError(err)

	var count int64
	s.db.Model(&models.NotificationLog{}).Where("user_id = ?", userID).Count(&count)
	s.Assert().Zero(count, "should not match when venue doesn't overlap")
}

// --- Batch Matching ---

func (s *NotificationFilterSuite) TestMatchAndNotifyBatch() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("BatchBand")
	venueID := s.createTestVenue("batch-venue")

	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "Batch test",
		ArtistIDs: []int64{int64(artistID)},
	})
	s.Require().NoError(err)

	showID1 := s.createTestShow("Batch1", []uint{artistID}, []uint{venueID})
	showID2 := s.createTestShow("Batch2", []uint{artistID}, []uint{venueID})

	var show1, show2 models.Show
	s.Require().NoError(s.db.First(&show1, showID1).Error)
	s.Require().NoError(s.db.First(&show2, showID2).Error)

	err = s.svc.MatchAndNotifyBatch([]models.Show{show1, show2})
	s.Require().NoError(err)

	var count int64
	s.db.Model(&models.NotificationLog{}).Where("user_id = ?", userID).Count(&count)
	s.Assert().Equal(int64(2), count)
}

// --- Notification Log ---

func (s *NotificationFilterSuite) TestGetUserNotifications() {
	userID := s.createTestUser()

	// Insert some notification log entries directly
	for i := 0; i < 3; i++ {
		s.db.Create(&models.NotificationLog{
			UserID:     userID,
			EntityType: "show",
			EntityID:   uint(i + 1),
			Channel:    "email",
			SentAt:     time.Now().UTC(),
		})
	}

	entries, err := s.svc.GetUserNotifications(userID, 10, 0)
	s.Require().NoError(err)
	s.Assert().Len(entries, 3)
}

func (s *NotificationFilterSuite) TestGetUnreadCount() {
	userID := s.createTestUser()

	// 2 unread, 1 read
	s.db.Create(&models.NotificationLog{UserID: userID, EntityType: "show", EntityID: 1, Channel: "email", SentAt: time.Now().UTC()})
	s.db.Create(&models.NotificationLog{UserID: userID, EntityType: "show", EntityID: 2, Channel: "email", SentAt: time.Now().UTC()})
	now := time.Now().UTC()
	s.db.Create(&models.NotificationLog{UserID: userID, EntityType: "show", EntityID: 3, Channel: "email", SentAt: time.Now().UTC(), ReadAt: &now})

	count, err := s.svc.GetUnreadCount(userID)
	s.Require().NoError(err)
	s.Assert().Equal(int64(2), count)
}

// --- PauseFilter ---

func (s *NotificationFilterSuite) TestPauseFilter() {
	userID := s.createTestUser()

	filter, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:      "To pause",
		ArtistIDs: []int64{1},
	})
	s.Require().NoError(err)
	s.Assert().True(filter.IsActive)

	err = s.svc.PauseFilter(filter.ID)
	s.Require().NoError(err)

	paused, err := s.svc.GetFilter(userID, filter.ID)
	s.Require().NoError(err)
	s.Assert().False(paused.IsActive)
}

func (s *NotificationFilterSuite) TestPauseFilter_NotFound() {
	err := s.svc.PauseFilter(99999)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "filter not found")
}

// =============================================================================
// Mock Email Service
// =============================================================================

type mockEmailService struct {
	sendCalls int
}

func (m *mockEmailService) IsConfigured() bool                      { return true }
func (m *mockEmailService) SendVerificationEmail(_, _ string) error { return nil }
func (m *mockEmailService) SendMagicLinkEmail(_, _ string) error    { return nil }
func (m *mockEmailService) SendAccountRecoveryEmail(_ string, _ string, _ int) error {
	return nil
}
func (m *mockEmailService) SendShowReminderEmail(_ string, _ string, _ string, _ string, _ time.Time, _ []string) error {
	return nil
}
func (m *mockEmailService) SendFilterNotificationEmail(_, _, _, _ string) error {
	m.sendCalls++
	return nil
}
