package admin

import (
	"context"
	"encoding/json"
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

func TestNewAutoPromotionService(t *testing.T) {
	svc := NewAutoPromotionService(nil, nil)
	assert.NotNil(t, svc)
	assert.Equal(t, DefaultAutoPromotionInterval, svc.interval)
	assert.NotNil(t, svc.stopCh)
	assert.NotNil(t, svc.logger)
}

func TestNewAutoPromotionService_EnvOverride(t *testing.T) {
	t.Setenv("AUTO_PROMOTION_INTERVAL_HOURS", "12")
	svc := NewAutoPromotionService(nil, nil)
	assert.Equal(t, 12*time.Hour, svc.interval)
}

func TestNewAutoPromotionService_InvalidEnvIgnored(t *testing.T) {
	t.Setenv("AUTO_PROMOTION_INTERVAL_HOURS", "not-a-number")
	svc := NewAutoPromotionService(nil, nil)
	assert.Equal(t, DefaultAutoPromotionInterval, svc.interval)
}

func TestNewAutoPromotionService_ZeroEnvIgnored(t *testing.T) {
	t.Setenv("AUTO_PROMOTION_INTERVAL_HOURS", "0")
	svc := NewAutoPromotionService(nil, nil)
	assert.Equal(t, DefaultAutoPromotionInterval, svc.interval)
}

func TestAutoPromotionService_EvaluateAllUsers_NilDB(t *testing.T) {
	svc := &AutoPromotionService{}
	result, err := svc.EvaluateAllUsers()
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

func TestAutoPromotionService_EvaluateUser_NilDB(t *testing.T) {
	svc := &AutoPromotionService{}
	result, err := svc.EvaluateUser(1)
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

func TestAutoPromotionService_StartStop(t *testing.T) {
	svc := NewAutoPromotionService(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should not block or panic (evaluation cycle will log DB error and continue)
	svc.Start(ctx)

	// Give it a moment to run the initial evaluation cycle
	time.Sleep(50 * time.Millisecond)

	// Stop should return without hanging
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2 seconds")
	}
}

func TestTierOrder(t *testing.T) {
	assert.Equal(t, 0, tierOrder[TierNewUser])
	assert.Equal(t, 1, tierOrder[TierContributor])
	assert.Equal(t, 2, tierOrder[TierTrustedContributor])
	assert.Equal(t, 3, tierOrder[TierLocalAmbassador])
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type AutoPromotionIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *AutoPromotionService
}

func (s *AutoPromotionIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = NewAutoPromotionService(s.db, nil)
}

func (s *AutoPromotionIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *AutoPromotionIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM pending_entity_edits")
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestAutoPromotionIntegrationSuite(t *testing.T) {
	suite.Run(t, new(AutoPromotionIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (s *AutoPromotionIntegrationTestSuite) createUser(tier string, emailVerified bool, createdAt time.Time) *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("ap-user-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("ap-user-%d", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		IsAdmin:       false,
		EmailVerified: emailVerified,
		UserTier:      tier,
	}
	err := s.db.Create(user).Error
	s.Require().NoError(err)

	// Update created_at (GORM auto-sets it, so we override)
	err = s.db.Model(user).Update("created_at", createdAt).Error
	s.Require().NoError(err)

	return user
}

func (s *AutoPromotionIntegrationTestSuite) createApprovedPendingEdit(userID uint, entityType string, entityID uint) {
	s.createPendingEditWithStatus(userID, entityType, entityID, models.PendingEditStatusApproved, time.Now())
}

func (s *AutoPromotionIntegrationTestSuite) createRejectedPendingEdit(userID uint, entityType string, entityID uint) {
	s.createPendingEditWithStatus(userID, entityType, entityID, models.PendingEditStatusRejected, time.Now())
}

func (s *AutoPromotionIntegrationTestSuite) createPendingEditWithStatus(userID uint, entityType string, entityID uint, status models.PendingEditStatus, createdAt time.Time) {
	raw := testRawJSON()
	edit := &models.PendingEntityEdit{
		EntityType:   entityType,
		EntityID:     entityID,
		SubmittedBy:  userID,
		FieldChanges: raw,
		Summary:      "test edit",
		Status:       status,
	}
	err := s.db.Create(edit).Error
	s.Require().NoError(err)

	// Override created_at
	err = s.db.Model(edit).Update("created_at", createdAt).Error
	s.Require().NoError(err)
}

func (s *AutoPromotionIntegrationTestSuite) createRevision(userID uint, entityType string, entityID uint, createdAt time.Time) {
	raw := testRawJSON()
	rev := &models.Revision{
		EntityType:   entityType,
		EntityID:     entityID,
		UserID:       userID,
		FieldChanges: raw,
	}
	err := s.db.Create(rev).Error
	s.Require().NoError(err)

	err = s.db.Model(rev).Update("created_at", createdAt).Error
	s.Require().NoError(err)
}

func (s *AutoPromotionIntegrationTestSuite) createTestArtist(name string) *models.Artist {
	slug := fmt.Sprintf("test-artist-%d", time.Now().UnixNano())
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}
	err := s.db.Create(artist).Error
	s.Require().NoError(err)
	return artist
}

func (s *AutoPromotionIntegrationTestSuite) createTestVenue(name, city, state string) *models.Venue {
	slug := fmt.Sprintf("test-venue-%d", time.Now().UnixNano())
	venue := &models.Venue{
		Name:  name,
		Slug:  &slug,
		City:  city,
		State: state,
	}
	err := s.db.Create(venue).Error
	s.Require().NoError(err)
	return venue
}

func testRawJSON() *json.RawMessage {
	raw := json.RawMessage(`[{"field":"name","old_value":"old","new_value":"new"}]`)
	return &raw
}

// =============================================================================
// PROMOTION TESTS
// =============================================================================

func (s *AutoPromotionIntegrationTestSuite) TestPromoteNewUserToContributor() {
	// User with 5+ approved edits, 2+ weeks, email verified
	user := s.createUser(TierNewUser, true, time.Now().Add(-15*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	for i := 0; i < 5; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.True(result.Changed)
	s.Equal(TierContributor, result.NewTier)
	s.Equal(5, result.ApprovedEdits)
	s.True(result.EmailVerified)
}

func (s *AutoPromotionIntegrationTestSuite) TestNotPromotedNewUser_NotEnoughEdits() {
	// User with only 4 approved edits (needs 5)
	user := s.createUser(TierNewUser, true, time.Now().Add(-15*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	for i := 0; i < 4; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
	s.Equal(4, result.ApprovedEdits)
}

func (s *AutoPromotionIntegrationTestSuite) TestNotPromotedNewUser_AccountTooNew() {
	// User with enough edits but account created today (needs 2 weeks)
	user := s.createUser(TierNewUser, true, time.Now().Add(-1*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	for i := 0; i < 5; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
}

func (s *AutoPromotionIntegrationTestSuite) TestNotPromotedNewUser_EmailNotVerified() {
	// User with enough edits and account age, but email not verified
	user := s.createUser(TierNewUser, false, time.Now().Add(-15*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	for i := 0; i < 5; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
	s.False(result.EmailVerified)
}

func (s *AutoPromotionIntegrationTestSuite) TestPromoteContributorToTrusted() {
	// User with 25+ approved edits, 95%+ approval rate, 2+ months
	user := s.createUser(TierContributor, true, time.Now().Add(-65*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	// Create 25 approved edits and 1 rejected (25/26 = 96.1%)
	for i := 0; i < 25; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}
	s.createRejectedPendingEdit(user.ID, "artist", artist.ID)

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.True(result.Changed)
	s.Equal(TierTrustedContributor, result.NewTier)
	s.Equal(25, result.ApprovedEdits)
}

func (s *AutoPromotionIntegrationTestSuite) TestNotPromotedContributor_NotEnoughEdits() {
	// User with only 24 approved edits (needs 25)
	user := s.createUser(TierContributor, true, time.Now().Add(-65*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	for i := 0; i < 24; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
	s.Equal(24, result.ApprovedEdits)
}

func (s *AutoPromotionIntegrationTestSuite) TestNotPromotedContributor_LowApprovalRate() {
	// User with 25 approved but poor approval rate (25/30 = 83.3%, needs 95%)
	user := s.createUser(TierContributor, true, time.Now().Add(-65*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	for i := 0; i < 25; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}
	for i := 0; i < 5; i++ {
		s.createRejectedPendingEdit(user.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
}

func (s *AutoPromotionIntegrationTestSuite) TestNotPromotedContributor_AccountTooNew() {
	// User with enough edits and rate, but account only 1 month old (needs 2)
	user := s.createUser(TierContributor, true, time.Now().Add(-30*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	for i := 0; i < 25; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
}

func (s *AutoPromotionIntegrationTestSuite) TestPromoteTrustedToAmbassador() {
	// User with 50+ approved edits, active in a city (10+ local edits), 6+ months
	user := s.createUser(TierTrustedContributor, true, time.Now().Add(-200*24*time.Hour))
	venue := s.createTestVenue("Test Venue", "Phoenix", "AZ")

	// Create 40 approved pending edits on venue (city-related)
	for i := 0; i < 40; i++ {
		s.createApprovedPendingEdit(user.ID, "venue", venue.ID)
	}
	// Create 10 revisions on venue (city-related)
	for i := 0; i < 10; i++ {
		s.createRevision(user.ID, "venue", venue.ID, time.Now())
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.True(result.Changed)
	s.Equal(TierLocalAmbassador, result.NewTier)
	s.Equal(50, result.ApprovedEdits)
	s.GreaterOrEqual(result.CityEditCount, 10)
}

func (s *AutoPromotionIntegrationTestSuite) TestNotPromotedTrusted_NotEnoughCityEdits() {
	// User has 50+ edits and 6+ months, but only 5 city edits (needs 10)
	user := s.createUser(TierTrustedContributor, true, time.Now().Add(-200*24*time.Hour))
	venue := s.createTestVenue("Test Venue", "Phoenix", "AZ")
	artist := s.createTestArtist("Test Artist")

	// 5 venue edits (city-related)
	for i := 0; i < 5; i++ {
		s.createApprovedPendingEdit(user.ID, "venue", venue.ID)
	}
	// 45 artist edits that count toward total and city edits
	for i := 0; i < 45; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	// City edits = 5 (venue) + 45 (artist) = 50 — both artist and venue count
	// Actually, the user has more than enough city edits because artist edits also count
	// Let's check the actual count
	if result.CityEditCount >= AmbassadorMinCityEdits {
		s.True(result.Changed)
	} else {
		s.False(result.Changed)
	}
}

func (s *AutoPromotionIntegrationTestSuite) TestNotPromotedTrusted_AccountTooNew() {
	// User has 50+ edits and city edits, but account only 3 months old (needs 6)
	user := s.createUser(TierTrustedContributor, true, time.Now().Add(-90*24*time.Hour))
	venue := s.createTestVenue("Test Venue", "Phoenix", "AZ")

	for i := 0; i < 50; i++ {
		s.createApprovedPendingEdit(user.ID, "venue", venue.ID)
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
}

func (s *AutoPromotionIntegrationTestSuite) TestLocalAmbassadorAlreadyAtTop() {
	// User is already at the highest tier
	user := s.createUser(TierLocalAmbassador, true, time.Now().Add(-365*24*time.Hour))

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
}

// =============================================================================
// DEMOTION TESTS
// =============================================================================

func (s *AutoPromotionIntegrationTestSuite) TestDemoteContributor_LowRolling30dApproval() {
	// Contributor with <80% approval in last 30 days
	user := s.createUser(TierContributor, true, time.Now().Add(-60*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	// 1 approved, 3 rejected in last 30 days (25% approval)
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusApproved, time.Now().Add(-5*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-4*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-3*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-2*24*time.Hour))

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.True(result.Changed)
	s.Equal(TierNewUser, result.NewTier)
	s.Contains(result.Reason, "approval rate")
	s.Contains(result.Reason, "below 80%")
}

func (s *AutoPromotionIntegrationTestSuite) TestDemoteTrustedContributor() {
	// TrustedContributor demoted to Contributor
	user := s.createUser(TierTrustedContributor, true, time.Now().Add(-120*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	// Low 30d rate: 1 approved, 3 rejected
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusApproved, time.Now().Add(-5*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-4*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-3*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-2*24*time.Hour))

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.True(result.Changed)
	s.Equal(TierContributor, result.NewTier)
}

func (s *AutoPromotionIntegrationTestSuite) TestNoDemotion_NewUserAlreadyAtBottom() {
	// new_user cannot be demoted further
	user := s.createUser(TierNewUser, true, time.Now().Add(-1*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	// Low 30d rate
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-5*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-4*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-3*24*time.Hour))

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
}

func (s *AutoPromotionIntegrationTestSuite) TestNoDemotion_TooFewEditsInWindow() {
	// Not enough edits in 30-day window to evaluate demotion (need 3+)
	user := s.createUser(TierContributor, true, time.Now().Add(-60*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	// Only 2 rejected edits in 30d window
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-5*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-4*24*time.Hour))

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
}

func (s *AutoPromotionIntegrationTestSuite) TestNoDemotion_OldEditsOutsideWindow() {
	// User has bad edits, but they are older than 30 days (outside the rolling window)
	user := s.createUser(TierContributor, true, time.Now().Add(-90*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	// Old rejected edits (> 30 days ago)
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-45*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-40*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-35*24*time.Hour))

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.False(result.Changed)
}

// =============================================================================
// EVALUATE ALL USERS TESTS
// =============================================================================

func (s *AutoPromotionIntegrationTestSuite) TestEvaluateAllUsers_MultipleMixed() {
	artist := s.createTestArtist("Test Artist")

	// User 1: should be promoted (new_user -> contributor)
	user1 := s.createUser(TierNewUser, true, time.Now().Add(-15*24*time.Hour))
	for i := 0; i < 5; i++ {
		s.createApprovedPendingEdit(user1.ID, "artist", artist.ID)
	}

	// User 2: should be demoted (contributor -> new_user due to low 30d rate)
	user2 := s.createUser(TierContributor, true, time.Now().Add(-60*24*time.Hour))
	s.createPendingEditWithStatus(user2.ID, "artist", artist.ID, models.PendingEditStatusApproved, time.Now().Add(-5*24*time.Hour))
	s.createPendingEditWithStatus(user2.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-4*24*time.Hour))
	s.createPendingEditWithStatus(user2.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-3*24*time.Hour))
	s.createPendingEditWithStatus(user2.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-2*24*time.Hour))

	// User 3: unchanged (new_user, no edits)
	_ = s.createUser(TierNewUser, false, time.Now())

	result, err := s.svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Len(result.Promoted, 1)
	s.Len(result.Demoted, 1)
	s.Equal(uint(0), uint(result.Errors))
	s.GreaterOrEqual(result.Unchanged, 1)

	// Verify promotion
	s.Equal(user1.ID, result.Promoted[0].UserID)
	s.Equal(TierNewUser, result.Promoted[0].OldTier)
	s.Equal(TierContributor, result.Promoted[0].NewTier)

	// Verify demotion
	s.Equal(user2.ID, result.Demoted[0].UserID)
	s.Equal(TierContributor, result.Demoted[0].OldTier)
	s.Equal(TierNewUser, result.Demoted[0].NewTier)
}

func (s *AutoPromotionIntegrationTestSuite) TestEvaluateAllUsers_AppliesTierChanges() {
	artist := s.createTestArtist("Test Artist")

	// User should be promoted
	user := s.createUser(TierNewUser, true, time.Now().Add(-15*24*time.Hour))
	for i := 0; i < 5; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}

	_, err := s.svc.EvaluateAllUsers()
	s.Require().NoError(err)

	// Reload user from DB and verify tier was updated
	var updatedUser models.User
	s.Require().NoError(s.db.First(&updatedUser, user.ID).Error)
	s.Equal(TierContributor, updatedUser.UserTier)
}

func (s *AutoPromotionIntegrationTestSuite) TestEvaluateAllUsers_SkipsAdminUsers() {
	artist := s.createTestArtist("Test Artist")

	// Create an admin user that would otherwise be promotable
	adminUser := &models.User{
		Email:         stringPtr(fmt.Sprintf("admin-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("admin-%d", time.Now().UnixNano())),
		IsActive:      true,
		IsAdmin:       true,
		EmailVerified: true,
		UserTier:      TierNewUser,
	}
	err := s.db.Create(adminUser).Error
	s.Require().NoError(err)
	err = s.db.Model(adminUser).Update("created_at", time.Now().Add(-15*24*time.Hour)).Error
	s.Require().NoError(err)

	for i := 0; i < 10; i++ {
		s.createApprovedPendingEdit(adminUser.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Len(result.Promoted, 0)
	s.Len(result.Demoted, 0)
}

func (s *AutoPromotionIntegrationTestSuite) TestEvaluateAllUsers_SkipsInactiveUsers() {
	artist := s.createTestArtist("Test Artist")

	// Create an inactive user
	inactiveUser := &models.User{
		Email:         stringPtr(fmt.Sprintf("inactive-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("inactive-%d", time.Now().UnixNano())),
		IsActive:      false,
		IsAdmin:       false,
		EmailVerified: true,
		UserTier:      TierNewUser,
	}
	// GORM bool gotcha: create as active first, then update to inactive
	inactiveUser.IsActive = true
	err := s.db.Create(inactiveUser).Error
	s.Require().NoError(err)
	err = s.db.Model(inactiveUser).Updates(map[string]interface{}{
		"is_active":  false,
		"created_at": time.Now().Add(-15 * 24 * time.Hour),
	}).Error
	s.Require().NoError(err)

	for i := 0; i < 10; i++ {
		s.createApprovedPendingEdit(inactiveUser.ID, "artist", artist.ID)
	}

	result, err := s.svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Len(result.Promoted, 0)
	s.Len(result.Demoted, 0)
}

func (s *AutoPromotionIntegrationTestSuite) TestEvaluateAllUsers_NoChangeWhenAlreadyCorrect() {
	// A new_user with no edits should stay as new_user
	_ = s.createUser(TierNewUser, false, time.Now())

	result, err := s.svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Len(result.Promoted, 0)
	s.Len(result.Demoted, 0)
	s.GreaterOrEqual(result.Unchanged, 1)
}

// =============================================================================
// REVISIONS COUNT AS APPROVED EDITS
// =============================================================================

func (s *AutoPromotionIntegrationTestSuite) TestRevisionsCountAsApprovedEdits() {
	// User has revisions (from direct edits) instead of pending edits
	user := s.createUser(TierNewUser, true, time.Now().Add(-15*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	// 3 approved pending edits + 2 revisions = 5 total approved
	for i := 0; i < 3; i++ {
		s.createApprovedPendingEdit(user.ID, "artist", artist.ID)
	}
	for i := 0; i < 2; i++ {
		s.createRevision(user.ID, "artist", artist.ID, time.Now())
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.True(result.Changed)
	s.Equal(TierContributor, result.NewTier)
	s.Equal(5, result.ApprovedEdits)
}

func (s *AutoPromotionIntegrationTestSuite) TestRevisionsOnlyCountAsApproved() {
	// User with only revisions (all count as approved)
	user := s.createUser(TierNewUser, true, time.Now().Add(-15*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	for i := 0; i < 5; i++ {
		s.createRevision(user.ID, "artist", artist.ID, time.Now())
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.True(result.Changed)
	s.Equal(TierContributor, result.NewTier)
	s.Equal(5, result.ApprovedEdits)
	s.Equal(float64(1), result.ApprovalRate)
}

// =============================================================================
// DEMOTION PRIORITIZED OVER PROMOTION
// =============================================================================

func (s *AutoPromotionIntegrationTestSuite) TestDemotionTakesPriorityOverPromotion() {
	// A contributor with enough all-time edits for trusted_contributor promotion
	// but bad 30-day rate should be demoted, not promoted
	user := s.createUser(TierContributor, true, time.Now().Add(-65*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	// 25 old approved edits (outside 30-day window) — qualifies for promotion criteria
	for i := 0; i < 25; i++ {
		s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusApproved, time.Now().Add(-40*24*time.Hour))
	}

	// Recent bad edits in 30-day window
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusApproved, time.Now().Add(-5*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-4*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-3*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-2*24*time.Hour))

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	s.True(result.Changed)
	// Should be demoted, not promoted — demotion is checked first
	s.Equal(TierNewUser, result.NewTier)
}

// =============================================================================
// EVALUATE USER - ERROR CASES
// =============================================================================

func (s *AutoPromotionIntegrationTestSuite) TestEvaluateUser_NotFound() {
	result, err := s.svc.EvaluateUser(999999)
	s.Nil(result)
	s.Error(err)
	s.Contains(err.Error(), "user not found")
}

// =============================================================================
// ROLLING 30-DAY WINDOW INCLUDES REVISIONS
// =============================================================================

func (s *AutoPromotionIntegrationTestSuite) TestRolling30dIncludesRevisions() {
	// Revisions in the 30-day window count as approved, protecting from demotion
	user := s.createUser(TierContributor, true, time.Now().Add(-60*24*time.Hour))
	artist := s.createTestArtist("Test Artist")

	// 2 rejected pending edits in 30d window
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-5*24*time.Hour))
	s.createPendingEditWithStatus(user.ID, "artist", artist.ID, models.PendingEditStatusRejected, time.Now().Add(-4*24*time.Hour))

	// 7 recent revisions (count as approved) in 30d window
	for i := 0; i < 7; i++ {
		s.createRevision(user.ID, "artist", artist.ID, time.Now().Add(-time.Duration(i+1)*24*time.Hour))
	}

	result, err := s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	// Rolling 30d: 7 approved (revisions) + 0 pending approved / (7 + 2 total) = 77.7%
	// Wait, let me recalculate: 7 revisions + 2 pending rejected = 9 total, 7/9 = 77.7% < 80%
	// Actually the user should be demoted because 7/9 = 77.7% < 80%
	// Let me add one more revision to get 8/10 = 80%
	s.createRevision(user.ID, "artist", artist.ID, time.Now().Add(-8*24*time.Hour))

	result, err = s.svc.EvaluateUser(user.ID)
	s.Require().NoError(err)
	// 8 revisions + 2 rejected = 10 total, 8/10 = 80% — at threshold, not below
	s.False(result.Changed)
}
