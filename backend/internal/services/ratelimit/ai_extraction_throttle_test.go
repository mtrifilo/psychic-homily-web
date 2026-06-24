package ratelimit

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
//
// The rolling-window reset/increment decision happens in a single Postgres
// upsert, so these run against a real Postgres testcontainer (not a mock) — the
// CASE + make_interval + RETURNING semantics are exactly what we're verifying.
// =============================================================================

type ThrottleIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *AIExtractionThrottleService
}

func (s *ThrottleIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = NewAIExtractionThrottleService(s.testDB.DB)
}

func (s *ThrottleIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *ThrottleIntegrationTestSuite) TearDownTest() {
	s.db.Exec("DELETE FROM ai_extraction_throttle")
	s.db.Exec("DELETE FROM users")
}

func TestThrottleIntegrationSuite(t *testing.T) {
	suite.Run(t, new(ThrottleIntegrationTestSuite))
}

func stringPtr(str string) *string { return &str }

func (s *ThrottleIntegrationTestSuite) createTestUser() *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("throttle-user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Throttle"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

// First attempt creates the row and is allowed.
func (s *ThrottleIntegrationTestSuite) TestFirstAttemptAllowed() {
	user := s.createTestUser()

	decision, err := s.svc.CheckAndIncrement(user.ID)
	s.NoError(err)
	s.True(decision.Allowed)
	s.Equal(0, decision.RetryAfterSeconds)
}

// Exactly AIExtractionLimit attempts are allowed; the next one is blocked.
func (s *ThrottleIntegrationTestSuite) TestBlocksAfterLimit() {
	user := s.createTestUser()

	for i := 0; i < AIExtractionLimit; i++ {
		decision, err := s.svc.CheckAndIncrement(user.ID)
		s.Require().NoError(err)
		s.Truef(decision.Allowed, "attempt %d should be allowed", i+1)
	}

	// The (limit+1)th attempt is blocked.
	decision, err := s.svc.CheckAndIncrement(user.ID)
	s.NoError(err)
	s.False(decision.Allowed)
	s.GreaterOrEqual(decision.RetryAfterSeconds, 1)
	// Window is one hour; retry-after should be within (0, 3600].
	s.LessOrEqual(decision.RetryAfterSeconds, int(AIExtractionWindow/time.Second))
}

// A user over the limit stays blocked on subsequent attempts within the window.
func (s *ThrottleIntegrationTestSuite) TestStaysBlockedWithinWindow() {
	user := s.createTestUser()
	for i := 0; i < AIExtractionLimit+1; i++ {
		_, err := s.svc.CheckAndIncrement(user.ID)
		s.Require().NoError(err)
	}

	decision, err := s.svc.CheckAndIncrement(user.ID)
	s.NoError(err)
	s.False(decision.Allowed)
}

// When the window has elapsed, the counter resets and the user is allowed again.
func (s *ThrottleIntegrationTestSuite) TestWindowResetsAfterExpiry() {
	user := s.createTestUser()

	// Exhaust the limit.
	for i := 0; i < AIExtractionLimit+1; i++ {
		_, err := s.svc.CheckAndIncrement(user.ID)
		s.Require().NoError(err)
	}
	blocked, err := s.svc.CheckAndIncrement(user.ID)
	s.Require().NoError(err)
	s.Require().False(blocked.Allowed)

	// Backdate the window so it has fully elapsed.
	s.Require().NoError(s.db.Exec(
		"UPDATE ai_extraction_throttle SET window_start = ? WHERE user_id = ?",
		time.Now().Add(-2*AIExtractionWindow), user.ID,
	).Error)

	// Next attempt resets the window and is allowed (count back to 1).
	decision, err := s.svc.CheckAndIncrement(user.ID)
	s.NoError(err)
	s.True(decision.Allowed)

	var count int
	s.Require().NoError(s.db.Raw(
		"SELECT request_count FROM ai_extraction_throttle WHERE user_id = ?", user.ID,
	).Scan(&count).Error)
	s.Equal(1, count)
}

// Two users have independent counters — one being throttled doesn't affect the other.
func (s *ThrottleIntegrationTestSuite) TestPerUserIsolation() {
	userA := s.createTestUser()
	userB := s.createTestUser()

	// Exhaust user A.
	for i := 0; i < AIExtractionLimit+1; i++ {
		_, err := s.svc.CheckAndIncrement(userA.ID)
		s.Require().NoError(err)
	}
	blockedA, err := s.svc.CheckAndIncrement(userA.ID)
	s.Require().NoError(err)
	s.False(blockedA.Allowed)

	// User B is untouched — first attempt allowed.
	decisionB, err := s.svc.CheckAndIncrement(userB.ID)
	s.NoError(err)
	s.True(decisionB.Allowed)
}

// A zero userID is rejected (defensive — the handler guarantees non-zero).
func (s *ThrottleIntegrationTestSuite) TestZeroUserIDRejected() {
	_, err := s.svc.CheckAndIncrement(0)
	s.Error(err)
}
