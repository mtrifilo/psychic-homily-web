package admin

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// mockEmailService is a simple mock for testing tier change email notifications.
type mockEmailService struct {
	configured             bool
	promotionCalls         []promotionCall
	demotionCalls          []demotionCall
	demotionWarningCalls   []demotionWarningCall
	promotionError         error
	demotionError          error
	demotionWarningError   error
}

type promotionCall struct {
	toEmail        string
	username       string
	oldTier        string
	newTier        string
	reason         string
	newPermissions []string
}

type demotionCall struct {
	toEmail  string
	username string
	oldTier  string
	newTier  string
	reason   string
}

type demotionWarningCall struct {
	toEmail     string
	username    string
	currentTier string
	currentRate float64
	threshold   float64
}

func (m *mockEmailService) IsConfigured() bool { return m.configured }
func (m *mockEmailService) SendVerificationEmail(_, _ string) error { return nil }
func (m *mockEmailService) SendMagicLinkEmail(_, _ string) error { return nil }
func (m *mockEmailService) SendAccountRecoveryEmail(_, _ string, _ int) error { return nil }
func (m *mockEmailService) SendShowReminderEmail(_, _, _, _ string, _ time.Time, _ []string) error {
	return nil
}
func (m *mockEmailService) SendFilterNotificationEmail(_, _, _, _ string) error { return nil }

func (m *mockEmailService) SendTierPromotionEmail(toEmail, username, oldTier, newTier, reason string, newPermissions []string) error {
	m.promotionCalls = append(m.promotionCalls, promotionCall{toEmail, username, oldTier, newTier, reason, newPermissions})
	return m.promotionError
}

func (m *mockEmailService) SendTierDemotionEmail(toEmail, username, oldTier, newTier, reason string) error {
	m.demotionCalls = append(m.demotionCalls, demotionCall{toEmail, username, oldTier, newTier, reason})
	return m.demotionError
}

func (m *mockEmailService) SendTierDemotionWarningEmail(toEmail, username, currentTier string, currentRate float64, threshold float64) error {
	m.demotionWarningCalls = append(m.demotionWarningCalls, demotionWarningCall{toEmail, username, currentTier, currentRate, threshold})
	return m.demotionWarningError
}

// =============================================================================
// INTEGRATION TESTS — EMAIL NOTIFICATIONS
// =============================================================================

type AutoPromotionEmailTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *AutoPromotionEmailTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *AutoPromotionEmailTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *AutoPromotionEmailTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM pending_entity_edits")
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestAutoPromotionEmailSuite(t *testing.T) {
	suite.Run(t, new(AutoPromotionEmailTestSuite))
}

func (s *AutoPromotionEmailTestSuite) createUserWithEmail(tier string, emailVerified bool, createdAt time.Time, email string) *models.User {
	username := fmt.Sprintf("user-%d", time.Now().UnixNano())
	user := &models.User{
		Email:         &email,
		Username:      &username,
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		IsAdmin:       false,
		EmailVerified: emailVerified,
		UserTier:      tier,
	}
	err := s.db.Create(user).Error
	s.Require().NoError(err)

	err = s.db.Model(user).Update("created_at", createdAt).Error
	s.Require().NoError(err)

	return user
}

func (s *AutoPromotionEmailTestSuite) createApprovedEdit(userID uint, entityType string, entityID uint) {
	raw := testRawJSON()
	edit := &models.PendingEntityEdit{
		EntityType:   entityType,
		EntityID:     entityID,
		SubmittedBy:  userID,
		FieldChanges: raw,
		Summary:      "test edit",
		Status:       models.PendingEditStatusApproved,
	}
	err := s.db.Create(edit).Error
	s.Require().NoError(err)
}

func (s *AutoPromotionEmailTestSuite) createRejectedEditWithTime(userID uint, entityType string, entityID uint, createdAt time.Time) {
	raw := testRawJSON()
	edit := &models.PendingEntityEdit{
		EntityType:   entityType,
		EntityID:     entityID,
		SubmittedBy:  userID,
		FieldChanges: raw,
		Summary:      "test edit",
		Status:       models.PendingEditStatusRejected,
	}
	err := s.db.Create(edit).Error
	s.Require().NoError(err)

	err = s.db.Model(edit).Update("created_at", createdAt).Error
	s.Require().NoError(err)
}

func (s *AutoPromotionEmailTestSuite) createTestArtist(name string) *models.Artist {
	slug := fmt.Sprintf("test-artist-%d", time.Now().UnixNano())
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}
	err := s.db.Create(artist).Error
	s.Require().NoError(err)
	return artist
}

// TestPromotionSendsEmail verifies that a promotion triggers a promotion email.
func (s *AutoPromotionEmailTestSuite) TestPromotionSendsEmail() {
	emailSvc := &mockEmailService{configured: true}
	svc := NewAutoPromotionService(s.db, emailSvc)

	user := s.createUserWithEmail(TierNewUser, true, time.Now().Add(-15*24*time.Hour), "promo@test.com")
	artist := s.createTestArtist("Promo Artist")

	for i := 0; i < 5; i++ {
		s.createApprovedEdit(user.ID, "artist", artist.ID)
	}

	result, err := svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Require().Len(result.Promoted, 1)

	// Verify promotion email was sent
	s.Require().Len(emailSvc.promotionCalls, 1)
	call := emailSvc.promotionCalls[0]
	s.Equal("promo@test.com", call.toEmail)
	s.Equal(TierNewUser, call.oldTier)
	s.Equal(TierContributor, call.newTier)
	s.NotEmpty(call.newPermissions)
}

// TestDemotionSendsEmail verifies that a demotion triggers a demotion email.
func (s *AutoPromotionEmailTestSuite) TestDemotionSendsEmail() {
	emailSvc := &mockEmailService{configured: true}
	svc := NewAutoPromotionService(s.db, emailSvc)

	user := s.createUserWithEmail(TierContributor, true, time.Now().Add(-60*24*time.Hour), "demote@test.com")
	artist := s.createTestArtist("Demote Artist")

	// 1 approved, 3 rejected in last 30 days (25% approval — below 80%)
	s.createApprovedEdit(user.ID, "artist", artist.ID)
	for i := 0; i < 3; i++ {
		s.createRejectedEditWithTime(user.ID, "artist", artist.ID, time.Now().Add(-time.Duration(i+1)*24*time.Hour))
	}

	result, err := svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Require().Len(result.Demoted, 1)

	// Verify demotion email was sent
	s.Require().Len(emailSvc.demotionCalls, 1)
	call := emailSvc.demotionCalls[0]
	s.Equal("demote@test.com", call.toEmail)
	s.Equal(TierContributor, call.oldTier)
	s.Equal(TierNewUser, call.newTier)
}

// TestEmailErrorDoesNotFailPromotion verifies fire-and-forget behavior.
func (s *AutoPromotionEmailTestSuite) TestEmailErrorDoesNotFailPromotion() {
	emailSvc := &mockEmailService{
		configured:     true,
		promotionError: fmt.Errorf("email send failed"),
	}
	svc := NewAutoPromotionService(s.db, emailSvc)

	user := s.createUserWithEmail(TierNewUser, true, time.Now().Add(-15*24*time.Hour), "fail@test.com")
	artist := s.createTestArtist("Error Artist")

	for i := 0; i < 5; i++ {
		s.createApprovedEdit(user.ID, "artist", artist.ID)
	}

	// Email error should not prevent the tier change
	result, err := svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Require().Len(result.Promoted, 1)

	// Verify the tier was still updated in the DB
	var updatedUser models.User
	s.Require().NoError(s.db.First(&updatedUser, user.ID).Error)
	s.Equal(TierContributor, updatedUser.UserTier)

	// Email was attempted
	s.Require().Len(emailSvc.promotionCalls, 1)
}

// TestNilEmailServiceDoesNotPanic verifies that nil email service is handled gracefully.
func (s *AutoPromotionEmailTestSuite) TestNilEmailServiceDoesNotPanic() {
	svc := NewAutoPromotionService(s.db, nil)

	user := s.createUserWithEmail(TierNewUser, true, time.Now().Add(-15*24*time.Hour), "nil@test.com")
	artist := s.createTestArtist("Nil Artist")

	for i := 0; i < 5; i++ {
		s.createApprovedEdit(user.ID, "artist", artist.ID)
	}

	// Should not panic with nil email service
	result, err := svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Require().Len(result.Promoted, 1)
}

// TestUnconfiguredEmailServiceSkipsEmail verifies that unconfigured email service is handled.
func (s *AutoPromotionEmailTestSuite) TestUnconfiguredEmailServiceSkipsEmail() {
	emailSvc := &mockEmailService{configured: false}
	svc := NewAutoPromotionService(s.db, emailSvc)

	user := s.createUserWithEmail(TierNewUser, true, time.Now().Add(-15*24*time.Hour), "unconfig@test.com")
	artist := s.createTestArtist("Unconfig Artist")

	for i := 0; i < 5; i++ {
		s.createApprovedEdit(user.ID, "artist", artist.ID)
	}

	result, err := svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Require().Len(result.Promoted, 1)

	// No email should have been attempted
	s.Empty(emailSvc.promotionCalls)
}

// TestAuditLogWrittenOnPromotion verifies that an audit log entry is created for promotions.
func (s *AutoPromotionEmailTestSuite) TestAuditLogWrittenOnPromotion() {
	svc := NewAutoPromotionService(s.db, nil)

	user := s.createUserWithEmail(TierNewUser, true, time.Now().Add(-15*24*time.Hour), "audit@test.com")
	artist := s.createTestArtist("Audit Artist")

	for i := 0; i < 5; i++ {
		s.createApprovedEdit(user.ID, "artist", artist.ID)
	}

	result, err := svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Require().Len(result.Promoted, 1)

	// Verify audit log was written
	var auditLog models.AuditLog
	err = s.db.Where("entity_type = ? AND entity_id = ? AND action = ?", "user", user.ID, "tier_promotion").
		First(&auditLog).Error
	s.Require().NoError(err)
	s.Equal("tier_promotion", auditLog.Action)
	s.Equal("user", auditLog.EntityType)
	s.Equal(user.ID, auditLog.EntityID)
	s.Nil(auditLog.ActorID) // system action
	s.NotNil(auditLog.Metadata)
}

// TestAuditLogWrittenOnDemotion verifies that an audit log entry is created for demotions.
func (s *AutoPromotionEmailTestSuite) TestAuditLogWrittenOnDemotion() {
	svc := NewAutoPromotionService(s.db, nil)

	user := s.createUserWithEmail(TierContributor, true, time.Now().Add(-60*24*time.Hour), "audit-demote@test.com")
	artist := s.createTestArtist("Audit Demote Artist")

	s.createApprovedEdit(user.ID, "artist", artist.ID)
	for i := 0; i < 3; i++ {
		s.createRejectedEditWithTime(user.ID, "artist", artist.ID, time.Now().Add(-time.Duration(i+1)*24*time.Hour))
	}

	result, err := svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Require().Len(result.Demoted, 1)

	// Verify audit log was written
	var auditLog models.AuditLog
	err = s.db.Where("entity_type = ? AND entity_id = ? AND action = ?", "user", user.ID, "tier_demotion").
		First(&auditLog).Error
	s.Require().NoError(err)
	s.Equal("tier_demotion", auditLog.Action)
	s.Nil(auditLog.ActorID)
}
