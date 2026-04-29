package engagement

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS — HMAC Functions (No Database Required)
// =============================================================================

const testSecret = "test-jwt-secret-key-for-hmac"

func TestComputeUnsubscribeSignature_Deterministic(t *testing.T) {
	sig1 := ComputeUnsubscribeSignature(42, testSecret)
	sig2 := ComputeUnsubscribeSignature(42, testSecret)

	assert.NotEmpty(t, sig1)
	assert.Equal(t, sig1, sig2, "same inputs should produce the same signature")
}

func TestComputeUnsubscribeSignature_DifferentUsers(t *testing.T) {
	sig1 := ComputeUnsubscribeSignature(1, testSecret)
	sig2 := ComputeUnsubscribeSignature(2, testSecret)

	assert.NotEqual(t, sig1, sig2, "different user IDs should produce different signatures")
}

func TestComputeUnsubscribeSignature_DifferentSecrets(t *testing.T) {
	sig1 := ComputeUnsubscribeSignature(1, "secret-a")
	sig2 := ComputeUnsubscribeSignature(1, "secret-b")

	assert.NotEqual(t, sig1, sig2, "different secrets should produce different signatures")
}

func TestComputeUnsubscribeSignature_HexEncoded(t *testing.T) {
	sig := ComputeUnsubscribeSignature(1, testSecret)

	// HMAC-SHA256 produces 32 bytes = 64 hex chars
	assert.Len(t, sig, 64, "HMAC-SHA256 hex output should be 64 characters")

	// Verify it's valid hex
	for _, c := range sig {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"signature should be lowercase hex, got char: %c", c)
	}
}

func TestGenerateUnsubscribeURL_Format(t *testing.T) {
	result := GenerateUnsubscribeURL("https://example.com", 42, testSecret)

	parsed, err := url.Parse(result)
	assert.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "example.com", parsed.Host)
	assert.Equal(t, "/unsubscribe/show-reminders", parsed.Path)
	assert.Equal(t, "42", parsed.Query().Get("uid"))
	assert.NotEmpty(t, parsed.Query().Get("sig"))
}

func TestGenerateUnsubscribeURL_EmbeddedSignature(t *testing.T) {
	result := GenerateUnsubscribeURL("https://example.com", 99, testSecret)

	parsed, err := url.Parse(result)
	assert.NoError(t, err)

	expectedSig := ComputeUnsubscribeSignature(99, testSecret)
	assert.Equal(t, expectedSig, parsed.Query().Get("sig"))
}

func TestVerifyUnsubscribeSignature_Valid(t *testing.T) {
	sig := ComputeUnsubscribeSignature(42, testSecret)
	assert.True(t, VerifyUnsubscribeSignature(42, sig, testSecret))
}

func TestVerifyUnsubscribeSignature_InvalidSignature(t *testing.T) {
	assert.False(t, VerifyUnsubscribeSignature(42, "totally-wrong-signature", testSecret))
}

func TestVerifyUnsubscribeSignature_WrongUserID(t *testing.T) {
	sig := ComputeUnsubscribeSignature(42, testSecret)
	// Signature for user 42 should not verify for user 43
	assert.False(t, VerifyUnsubscribeSignature(43, sig, testSecret))
}

func TestVerifyUnsubscribeSignature_WrongSecret(t *testing.T) {
	sig := ComputeUnsubscribeSignature(42, "secret-a")
	assert.False(t, VerifyUnsubscribeSignature(42, sig, "secret-b"))
}

func TestVerifyUnsubscribeSignature_EmptySignature(t *testing.T) {
	assert.False(t, VerifyUnsubscribeSignature(42, "", testSecret))
}

func TestVerifyUnsubscribeSignature_TamperedSignature(t *testing.T) {
	sig := ComputeUnsubscribeSignature(42, testSecret)
	// Flip the last character
	tampered := sig[:len(sig)-1] + "0"
	if sig[len(sig)-1] == '0' {
		tampered = sig[:len(sig)-1] + "1"
	}
	assert.False(t, VerifyUnsubscribeSignature(42, tampered, testSecret))
}

func TestHMACRoundTrip(t *testing.T) {
	// Generate URL → extract signature → verify
	userID := uint(777)
	secret := "round-trip-secret"
	baseURL := "https://psychichomily.com"

	generatedURL := GenerateUnsubscribeURL(baseURL, userID, secret)

	parsed, err := url.Parse(generatedURL)
	assert.NoError(t, err)

	extractedSig := parsed.Query().Get("sig")
	assert.NotEmpty(t, extractedSig)

	assert.True(t, VerifyUnsubscribeSignature(userID, extractedSig, secret),
		"round-trip: signature generated in URL should verify successfully")

	// Also verify it fails for a different user
	assert.False(t, VerifyUnsubscribeSignature(userID+1, extractedSig, secret),
		"round-trip: signature should not verify for a different user ID")
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

// mockReminderEmailService tracks calls to SendShowReminderEmail
type mockReminderEmailService struct {
	calls       []reminderEmailCall
	shouldError bool
}

type reminderEmailCall struct {
	ToEmail        string
	ShowTitle      string
	ShowURL        string
	UnsubscribeURL string
	EventDate      time.Time
	Venues         []string
}

func (m *mockReminderEmailService) IsConfigured() bool { return true }
func (m *mockReminderEmailService) SendVerificationEmail(_, _ string) error {
	return nil
}
func (m *mockReminderEmailService) SendMagicLinkEmail(_, _ string) error {
	return nil
}
func (m *mockReminderEmailService) SendAccountRecoveryEmail(_ string, _ string, _ int) error {
	return nil
}
func (m *mockReminderEmailService) SendShowReminderEmail(toEmail, showTitle, showURL, unsubscribeURL string, eventDate time.Time, venues []string) error {
	m.calls = append(m.calls, reminderEmailCall{
		ToEmail:        toEmail,
		ShowTitle:      showTitle,
		ShowURL:        showURL,
		UnsubscribeURL: unsubscribeURL,
		EventDate:      eventDate,
		Venues:         venues,
	})
	if m.shouldError {
		return fmt.Errorf("mock email send error")
	}
	return nil
}
func (m *mockReminderEmailService) SendFilterNotificationEmail(_, _, _, _ string) error {
	return nil
}
func (m *mockReminderEmailService) SendTierPromotionEmail(_, _, _, _, _ string, _ []string) error {
	return nil
}
func (m *mockReminderEmailService) SendTierDemotionEmail(_, _, _, _, _ string) error { return nil }
func (m *mockReminderEmailService) SendTierDemotionWarningEmail(_, _, _ string, _, _ float64) error {
	return nil
}
func (m *mockReminderEmailService) SendEditApprovedEmail(_, _, _, _, _ string) error { return nil }
func (m *mockReminderEmailService) SendEditRejectedEmail(_, _, _, _, _ string) error { return nil }
func (m *mockReminderEmailService) SendCommentNotification(_, _, _, _, _, _, _ string) error {
	return nil
}
func (m *mockReminderEmailService) SendMentionNotification(_, _, _, _, _, _, _ string) error {
	return nil
}
func (m *mockReminderEmailService) SendCollectionDigestEmail(_ string, _ []contracts.CollectionDigestGroup, _ string) error {
	return nil
}

// ReminderServiceIntegrationTestSuite tests the reminder service with a real database
type ReminderServiceIntegrationTestSuite struct {
	suite.Suite
	testDB          *testutil.TestDatabase
	db              *gorm.DB
	emailMock       *mockReminderEmailService
	reminderService *ReminderService
	cfg             *config.Config
}

func (s *ReminderServiceIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	s.cfg = &config.Config{
		Email: config.EmailConfig{
			FrontendURL: "https://test.psychichomily.com",
		},
		JWT: config.JWTConfig{
			SecretKey: testSecret,
		},
	}
}

func (s *ReminderServiceIntegrationTestSuite) SetupTest() {
	s.emailMock = &mockReminderEmailService{}
	s.reminderService = &ReminderService{
		db:           s.db,
		emailService: s.emailMock,
		interval:     1 * time.Second,
		stopCh:       make(chan struct{}),
		logger:       testLogger(),
		frontendURL:  s.cfg.Email.FrontendURL,
		jwtSecret:    s.cfg.JWT.SecretKey,
	}
}

func (s *ReminderServiceIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *ReminderServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestReminderServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ReminderServiceIntegrationTestSuite))
}

// =============================================================================
// Helpers
// =============================================================================

func testLogger() *slog.Logger {
	return slog.Default()
}

func (s *ReminderServiceIntegrationTestSuite) createTestUserWithPrefs(showReminders bool) *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("reminder-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := s.db.Create(user).Error
	s.Require().NoError(err)

	// Create user preferences with show_reminders setting.
	// GORM bool gotcha: create with ShowReminders true, then update to false if needed.
	prefs := &models.UserPreferences{
		UserID:        user.ID,
		ShowReminders: true,
	}
	err = s.db.Create(prefs).Error
	s.Require().NoError(err)

	if !showReminders {
		err = s.db.Model(prefs).Update("show_reminders", false).Error
		s.Require().NoError(err)
	}

	return user
}

func (s *ReminderServiceIntegrationTestSuite) createShowAt(title string, eventDate time.Time, userID uint) *models.Show {
	slug := fmt.Sprintf("show-%d", time.Now().UnixNano())
	show := &models.Show{
		Title:       title,
		Slug:        &slug,
		EventDate:   eventDate,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := s.db.Create(show).Error
	s.Require().NoError(err)
	return show
}

func (s *ReminderServiceIntegrationTestSuite) createVenueForShow(showID uint, venueName string) *models.Venue {
	venue := &models.Venue{
		Name:  venueName,
		City:  "Phoenix",
		State: "AZ",
	}
	s.db.Create(venue)
	s.db.Create(&models.ShowVenue{ShowID: showID, VenueID: venue.ID})
	return venue
}

func (s *ReminderServiceIntegrationTestSuite) saveShow(userID, showID uint) {
	bookmark := &models.UserBookmark{
		UserID:     userID,
		EntityType: models.BookmarkEntityShow,
		EntityID:   showID,
		Action:     models.BookmarkActionSave,
	}
	err := s.db.Create(bookmark).Error
	s.Require().NoError(err)
}

// =============================================================================
// Group 1: runReminderCycle — Happy Path
// =============================================================================

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_SendsReminderForTomorrowShow() {
	user := s.createTestUserWithPrefs(true)
	// Show happening ~24h from now (within the 23-25h window)
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("Tomorrow Concert", eventDate, user.ID)
	venue := s.createVenueForShow(show.ID, "The Rebel Lounge")
	s.saveShow(user.ID, show.ID)

	s.reminderService.RunReminderCycleNow()

	s.Require().Len(s.emailMock.calls, 1)
	call := s.emailMock.calls[0]
	s.Equal(*user.Email, call.ToEmail)
	s.Equal("Tomorrow Concert", call.ShowTitle)
	s.Contains(call.ShowURL, *show.Slug)
	s.Contains(call.UnsubscribeURL, "/unsubscribe/show-reminders")
	s.Require().Len(call.Venues, 1)
	s.Equal(venue.Name, call.Venues[0])

	// Verify reminder_sent_at was set (deduplication)
	var bookmark models.UserBookmark
	err := s.db.Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
		user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionSave).
		First(&bookmark).Error
	s.Require().NoError(err)
	s.NotNil(bookmark.ReminderSentAt, "reminder_sent_at should be set after sending")
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_MultipleVenues() {
	user := s.createTestUserWithPrefs(true)
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("Multi-Venue Show", eventDate, user.ID)
	s.createVenueForShow(show.ID, "Valley Bar")
	s.createVenueForShow(show.ID, "Crescent Ballroom")
	s.saveShow(user.ID, show.ID)

	s.reminderService.RunReminderCycleNow()

	s.Require().Len(s.emailMock.calls, 1)
	s.Len(s.emailMock.calls[0].Venues, 2)
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_MultipleUsers() {
	user1 := s.createTestUserWithPrefs(true)
	user2 := s.createTestUserWithPrefs(true)
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("Shared Show", eventDate, user1.ID)
	s.createVenueForShow(show.ID, "The Van Buren")
	s.saveShow(user1.ID, show.ID)
	s.saveShow(user2.ID, show.ID)

	s.reminderService.RunReminderCycleNow()

	s.Len(s.emailMock.calls, 2, "both users should receive a reminder")
}

// =============================================================================
// Group 2: runReminderCycle — Filtering / Edge Cases
// =============================================================================

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_NoRemindersToSend() {
	// No saved shows at all
	s.reminderService.RunReminderCycleNow()
	s.Empty(s.emailMock.calls)
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_ShowOutsideWindow_TooSoon() {
	user := s.createTestUserWithPrefs(true)
	// Show in 20 hours — before the 23h window start
	eventDate := time.Now().Add(20 * time.Hour)
	show := s.createShowAt("Too Soon Show", eventDate, user.ID)
	s.saveShow(user.ID, show.ID)

	s.reminderService.RunReminderCycleNow()

	s.Empty(s.emailMock.calls, "show before 23h window should not trigger reminder")
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_ShowOutsideWindow_TooFar() {
	user := s.createTestUserWithPrefs(true)
	// Show in 48 hours — past the 25h window end
	eventDate := time.Now().Add(48 * time.Hour)
	show := s.createShowAt("Far Future Show", eventDate, user.ID)
	s.saveShow(user.ID, show.ID)

	s.reminderService.RunReminderCycleNow()

	s.Empty(s.emailMock.calls, "show past 25h window should not trigger reminder")
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_ShowRemindersDisabled() {
	user := s.createTestUserWithPrefs(false) // reminders OFF
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("No Reminder Show", eventDate, user.ID)
	s.saveShow(user.ID, show.ID)

	s.reminderService.RunReminderCycleNow()

	s.Empty(s.emailMock.calls, "user with show_reminders=false should not receive reminder")
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_CancelledShow() {
	user := s.createTestUserWithPrefs(true)
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("Cancelled Show", eventDate, user.ID)
	s.saveShow(user.ID, show.ID)

	// Mark show as cancelled
	s.db.Model(show).Update("is_cancelled", true)

	s.reminderService.RunReminderCycleNow()

	s.Empty(s.emailMock.calls, "cancelled shows should not trigger reminders")
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_PendingShow() {
	user := s.createTestUserWithPrefs(true)
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("Pending Show", eventDate, user.ID)
	s.saveShow(user.ID, show.ID)

	// Change status to pending
	s.db.Model(show).Update("status", models.ShowStatusPending)

	s.reminderService.RunReminderCycleNow()

	s.Empty(s.emailMock.calls, "non-approved shows should not trigger reminders")
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_InactiveUser() {
	user := s.createTestUserWithPrefs(true)
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("Inactive User Show", eventDate, user.ID)
	s.saveShow(user.ID, show.ID)

	// Deactivate the user
	s.db.Model(user).Update("is_active", false)

	s.reminderService.RunReminderCycleNow()

	s.Empty(s.emailMock.calls, "inactive users should not receive reminders")
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_SoftDeletedUser() {
	user := s.createTestUserWithPrefs(true)
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("Deleted User Show", eventDate, user.ID)
	s.saveShow(user.ID, show.ID)

	// Soft-delete the user
	now := time.Now()
	s.db.Model(&models.User{}).Where("id = ?", user.ID).Update("deleted_at", now)

	s.reminderService.RunReminderCycleNow()

	s.Empty(s.emailMock.calls, "soft-deleted users should not receive reminders")
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_Deduplication() {
	user := s.createTestUserWithPrefs(true)
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("Dedup Show", eventDate, user.ID)
	s.saveShow(user.ID, show.ID)

	// First cycle sends the reminder
	s.reminderService.RunReminderCycleNow()
	s.Len(s.emailMock.calls, 1)

	// Second cycle should NOT send again (reminder_sent_at is set)
	s.reminderService.RunReminderCycleNow()
	s.Len(s.emailMock.calls, 1, "reminder should not be sent twice for the same bookmark")
}

func (s *ReminderServiceIntegrationTestSuite) TestRunReminderCycle_EmailError_ContinuesOthers() {
	user1 := s.createTestUserWithPrefs(true)
	user2 := s.createTestUserWithPrefs(true)
	eventDate := time.Now().Add(24 * time.Hour)
	show := s.createShowAt("Error Test Show", eventDate, user1.ID)
	s.createVenueForShow(show.ID, "Test Venue")
	s.saveShow(user1.ID, show.ID)
	s.saveShow(user2.ID, show.ID)

	// Make email service fail
	s.emailMock.shouldError = true

	s.reminderService.RunReminderCycleNow()

	// Both users should have been attempted (both calls recorded before error check)
	s.Len(s.emailMock.calls, 2)

	// Verify reminder_sent_at was NOT set (since email failed)
	var bookmark models.UserBookmark
	err := s.db.Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
		user1.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionSave).
		First(&bookmark).Error
	s.Require().NoError(err)
	s.Nil(bookmark.ReminderSentAt, "reminder_sent_at should not be set when email fails")
}

// =============================================================================
// Group 3: Start/Stop
// =============================================================================

func (s *ReminderServiceIntegrationTestSuite) TestStartStop_NoError() {
	svc := &ReminderService{
		db:           s.db,
		emailService: s.emailMock,
		interval:     100 * time.Millisecond,
		stopCh:       make(chan struct{}),
		logger:       testLogger(),
		frontendURL:  s.cfg.Email.FrontendURL,
		jwtSecret:    s.cfg.JWT.SecretKey,
	}

	svc.Start(context.Background())

	// Let it run for a brief moment
	time.Sleep(50 * time.Millisecond)

	// Stop should return without hanging
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success — stopped cleanly
	case <-time.After(5 * time.Second):
		s.Fail("Stop() did not return within timeout")
	}
}

func (s *ReminderServiceIntegrationTestSuite) TestStartStop_ContextCancellation() {
	ctx, cancel := context.WithCancel(context.Background())

	svc := &ReminderService{
		db:           s.db,
		emailService: s.emailMock,
		interval:     100 * time.Millisecond,
		stopCh:       make(chan struct{}),
		logger:       testLogger(),
		frontendURL:  s.cfg.Email.FrontendURL,
		jwtSecret:    s.cfg.JWT.SecretKey,
	}

	svc.Start(ctx)

	// Cancel the context to stop the service
	cancel()

	// Wait for the goroutine to exit via wg.Wait()
	done := make(chan struct{})
	go func() {
		svc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success — exited on context cancellation
	case <-time.After(5 * time.Second):
		s.Fail("service did not stop on context cancellation within timeout")
	}
}

func (s *ReminderServiceIntegrationTestSuite) TestNewReminderService_DefaultInterval() {
	svc := NewReminderService(s.db, s.emailMock, s.cfg)
	s.Equal(DefaultReminderInterval, svc.interval)
}

func (s *ReminderServiceIntegrationTestSuite) TestNewReminderService_StoresConfig() {
	svc := NewReminderService(s.db, s.emailMock, s.cfg)
	s.Equal(s.cfg.Email.FrontendURL, svc.frontendURL)
	s.Equal(s.cfg.JWT.SecretKey, svc.jwtSecret)
	s.NotNil(svc.stopCh)
	s.NotNil(svc.logger)
}
