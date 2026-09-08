package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// JWT INTEGRATION TEST SUITE
// =============================================================================

// JWTIntegrationSuite tests the full JWT lifecycle with a real PostgreSQL database:
// generate token -> validate token -> look up user -> return authenticated user.
// This covers the security-critical path that unit tests cannot exercise due to nil DB.
type JWTIntegrationSuite struct {
	suite.Suite
	db      *gorm.DB
	testDB  *testutil.TestDatabase
	cfg     *config.Config
	svc     *JWTService
	authSvc *AuthService
}

// refreshGracePeriod is the window routes/auth.go gives LenientHumaJWTMiddleware
// on POST /auth/refresh.
const refreshGracePeriod = 7 * 24 * time.Hour

func TestJWTIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(JWTIntegrationSuite))
}

func (s *JWTIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	s.cfg = &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "jwt-integration-test-secret-key-32ch!",
			Expiry:    24,
		},
	}
	s.svc = NewJWTService(s.db, s.cfg, usersvc.NewUserService(s.db))
	s.authSvc = NewAuthService(s.db, s.cfg, usersvc.NewUserService(s.db))
}

func (s *JWTIntegrationSuite) TearDownTest() {
	sqlDB, _ := s.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM oauth_accounts")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func (s *JWTIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

// --- Helpers ---

func (s *JWTIntegrationSuite) createActiveUser(email string) *authm.User {
	s.T().Helper()
	user := &authm.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *JWTIntegrationSuite) createInactiveUser(email string) *authm.User {
	s.T().Helper()
	// Create as active first, then update to inactive (GORM bool zero-value gotcha)
	user := s.createActiveUser(email)
	s.Require().NoError(s.db.Model(user).Update("is_active", false).Error)
	user.IsActive = false
	return user
}

func (s *JWTIntegrationSuite) createAdminUser(email string) *authm.User {
	s.T().Helper()
	user := s.createActiveUser(email)
	s.Require().NoError(s.db.Model(user).Update("is_admin", true).Error)
	user.IsAdmin = true
	return user
}

// =============================================================================
// FULL LIFECYCLE: create token -> validate -> return user
// =============================================================================

func (s *JWTIntegrationSuite) TestValidateToken_ReturnsCorrectUser() {
	user := s.createActiveUser("alice@example.com")

	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	s.Require().NotEmpty(token)

	validated, err := s.svc.ValidateToken(token)
	s.Require().NoError(err)
	s.Require().NotNil(validated)

	s.Equal(user.ID, validated.ID)
	s.Equal("alice@example.com", *validated.Email)
	s.True(validated.IsActive)
}

func (s *JWTIntegrationSuite) TestValidateToken_ReturnsCurrentAdminStatus() {
	user := s.createAdminUser("admin@example.com")

	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	validated, err := s.svc.ValidateToken(token)
	s.Require().NoError(err)
	s.True(validated.IsAdmin, "validated user should reflect current admin status from DB")
}

func (s *JWTIntegrationSuite) TestValidateToken_ReflectsDBChanges() {
	// Create a regular user and generate a token
	user := s.createActiveUser("promoted@example.com")
	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	// Promote the user to admin after token creation
	s.Require().NoError(s.db.Model(user).Update("is_admin", true).Error)

	// ValidateToken should fetch the latest user state from DB
	validated, err := s.svc.ValidateToken(token)
	s.Require().NoError(err)
	s.True(validated.IsAdmin, "token validation should see the updated admin status")
}

func (s *JWTIntegrationSuite) TestValidateToken_MultipleUsers() {
	user1 := s.createActiveUser("user1@example.com")
	user2 := s.createActiveUser("user2@example.com")
	user3 := s.createAdminUser("user3@example.com")

	token1, err := s.svc.CreateToken(user1)
	s.Require().NoError(err)
	token2, err := s.svc.CreateToken(user2)
	s.Require().NoError(err)
	token3, err := s.svc.CreateToken(user3)
	s.Require().NoError(err)

	// All tokens are unique
	s.NotEqual(token1, token2)
	s.NotEqual(token2, token3)

	// Each token resolves to the correct user
	v1, err := s.svc.ValidateToken(token1)
	s.Require().NoError(err)
	s.Equal(user1.ID, v1.ID)

	v2, err := s.svc.ValidateToken(token2)
	s.Require().NoError(err)
	s.Equal(user2.ID, v2.ID)

	v3, err := s.svc.ValidateToken(token3)
	s.Require().NoError(err)
	s.Equal(user3.ID, v3.ID)
	s.True(v3.IsAdmin)
}

// =============================================================================
// INACTIVE USER
// =============================================================================

func (s *JWTIntegrationSuite) TestValidateToken_InactiveUser_Rejected() {
	user := s.createInactiveUser("inactive@example.com")

	// Generate token (uses the user struct directly — does not check active status)
	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	// Validation fetches user from DB and checks IsActive
	validated, err := s.svc.ValidateToken(token)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_INVALID")
}

func (s *JWTIntegrationSuite) TestValidateToken_UserDeactivatedAfterTokenCreation() {
	user := s.createActiveUser("will-deactivate@example.com")
	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	// Deactivate the user after token was issued
	s.Require().NoError(s.db.Model(user).Update("is_active", false).Error)

	validated, err := s.svc.ValidateToken(token)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_INVALID")
}

// =============================================================================
// EXPIRED TOKEN
// =============================================================================

func (s *JWTIntegrationSuite) TestValidateToken_ExpiredToken() {
	user := s.createActiveUser("expired@example.com")

	// Create a JWT service with 0-hour expiry (token expires immediately)
	shortCfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: s.cfg.JWT.SecretKey,
			Expiry:    0,
		},
	}
	shortSvc := NewJWTService(s.db, shortCfg, usersvc.NewUserService(s.db))

	token, err := shortSvc.CreateToken(user)
	s.Require().NoError(err)

	time.Sleep(100 * time.Millisecond)

	validated, err := s.svc.ValidateToken(token)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_EXPIRED")
}

// =============================================================================
// TAMPERED TOKEN
// =============================================================================

func (s *JWTIntegrationSuite) TestValidateToken_TamperedToken() {
	user := s.createActiveUser("tampered@example.com")

	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	// Flip a character in the middle of the signature
	mid := len(token) - len(token)/4
	replacement := byte('A')
	if token[mid] == 'A' {
		replacement = 'B'
	}
	tampered := token[:mid] + string(replacement) + token[mid+1:]

	validated, err := s.svc.ValidateToken(tampered)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_INVALID")
}

func (s *JWTIntegrationSuite) TestValidateToken_WrongSigningKey() {
	user := s.createActiveUser("wrong-key@example.com")

	otherCfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "completely-different-secret-key!!!!!",
			Expiry:    24,
		},
	}
	otherSvc := NewJWTService(s.db, otherCfg, usersvc.NewUserService(s.db))

	token, err := otherSvc.CreateToken(user)
	s.Require().NoError(err)

	validated, err := s.svc.ValidateToken(token)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_INVALID")
}

// =============================================================================
// NON-EXISTENT USER
// =============================================================================

func (s *JWTIntegrationSuite) TestValidateToken_NonExistentUserID() {
	// Craft a token referencing a user ID that doesn't exist in the DB
	fakeUser := &authm.User{
		ID:    999999,
		Email: stringPtr("ghost@example.com"),
	}

	token, err := s.svc.CreateToken(fakeUser)
	s.Require().NoError(err)

	validated, err := s.svc.ValidateToken(token)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to get user")
}

func (s *JWTIntegrationSuite) TestValidateToken_UserDeletedAfterTokenCreation() {
	user := s.createActiveUser("deleted@example.com")

	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	// Hard-delete the user
	s.Require().NoError(s.db.Unscoped().Delete(user).Error)

	validated, err := s.svc.ValidateToken(token)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to get user")
}

// =============================================================================
// REFRESH FLOW
// =============================================================================
//
// POST /auth/refresh runs LenientHumaJWTMiddleware -> RefreshTokenHandler ->
// AuthService.RefreshUserToken. These drive that composition rather than a
// service-level shorthand for it, so a renewal that behaved differently under
// the grace period than under strict validation would fail here.

// renewLikeTheRefreshEndpoint performs the two steps the refresh endpoint
// performs: read the presented credential the way the lenient middleware does,
// then renew with what that read established.
func (s *JWTIntegrationSuite) renewLikeTheRefreshEndpoint(token string) (string, error) {
	s.T().Helper()
	user, authAt, err := s.svc.ValidateSessionLenient(token, refreshGracePeriod)
	if err != nil {
		return "", err
	}
	return s.authSvc.RefreshUserToken(user, authAt)
}

func (s *JWTIntegrationSuite) TestRefreshFlow_IssuesAUsableTokenWithALaterExpiry() {
	user := s.createActiveUser("refresh@example.com")

	original, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	// Brief pause so timestamps differ
	time.Sleep(1100 * time.Millisecond)

	refreshed, err := s.renewLikeTheRefreshEndpoint(original)
	s.Require().NoError(err)
	s.NotEmpty(refreshed)
	s.NotEqual(original, refreshed, "refreshed token should differ (different iat/exp)")

	validated, err := s.svc.ValidateToken(refreshed)
	s.Require().NoError(err)
	s.Equal(user.ID, validated.ID)
	s.Equal("refresh@example.com", *validated.Email)

	s.Greater(
		int64(claimsOf(s.T(), s.cfg.JWT.SecretKey, refreshed)["exp"].(float64)),
		int64(claimsOf(s.T(), s.cfg.JWT.SecretKey, original)["exp"].(float64)),
		"refreshed token should have a later expiry",
	)
}

// Refresh renews a session without asking for anything, so the token it hands
// back reports the same authentication time the presented one did. Stamping a
// new one would let a holder of a stolen session satisfy every freshness gate
// with one extra request.
func (s *JWTIntegrationSuite) TestRefreshFlow_DoesNotMoveAuthTime() {
	user := s.createActiveUser("refresh-auth-at@example.com")

	original, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	_, originalAuthAt, err := s.svc.ValidateSession(original)
	s.Require().NoError(err)
	s.Require().False(originalAuthAt.IsZero())

	time.Sleep(1100 * time.Millisecond)

	refreshed, err := s.renewLikeTheRefreshEndpoint(original)
	s.Require().NoError(err)

	_, refreshedAuthAt, err := s.svc.ValidateSession(refreshed)
	s.Require().NoError(err)
	s.True(refreshedAuthAt.Equal(originalAuthAt))

	// The renewal is genuinely a new token: iat moved, auth_at did not.
	s.Greater(
		int64(claimsOf(s.T(), s.cfg.JWT.SecretKey, refreshed)["iat"].(float64)),
		int64(claimsOf(s.T(), s.cfg.JWT.SecretKey, original)["iat"].(float64)),
	)

	// Repeated renewals do not walk it forward either.
	again, err := s.renewLikeTheRefreshEndpoint(refreshed)
	s.Require().NoError(err)
	_, againAuthAt, err := s.svc.ValidateSession(again)
	s.Require().NoError(err)
	s.True(againAuthAt.Equal(originalAuthAt))
}

// The grace window is the case a service-level shorthand would miss: a token
// already expired when it arrives still renews, and still does not gain
// freshness by doing so.
func (s *JWTIntegrationSuite) TestRefreshFlow_ExpiredWithinGraceKeepsItsAuthTime() {
	user := s.createActiveUser("refresh-grace-auth-at@example.com")

	shortSvc := NewJWTService(s.db, &config.Config{
		JWT: config.JWTConfig{SecretKey: s.cfg.JWT.SecretKey, Expiry: 0},
	}, usersvc.NewUserService(s.db))

	token, err := shortSvc.CreateToken(user)
	s.Require().NoError(err)
	time.Sleep(1100 * time.Millisecond)

	_, _, err = s.svc.ValidateSession(token)
	s.Require().Error(err, "the token is expired for strict validation")

	_, presentedAuthAt, err := s.svc.ValidateSessionLenient(token, refreshGracePeriod)
	s.Require().NoError(err)
	s.Require().False(presentedAuthAt.IsZero())

	refreshed, err := s.renewLikeTheRefreshEndpoint(token)
	s.Require().NoError(err)

	_, refreshedAuthAt, err := s.svc.ValidateSession(refreshed)
	s.Require().NoError(err)
	s.True(refreshedAuthAt.Equal(presentedAuthAt),
		"renewing after expiry must neither stamp nor drop the authentication time")
}

// A session carrying no authentication time renews without acquiring one.
func (s *JWTIntegrationSuite) TestRefreshFlow_SessionWithoutAuthTimeStaysWithout() {
	user := s.createActiveUser("refresh-legacy@example.com")

	legacy, err := s.svc.RenewSessionToken(user, time.Time{})
	s.Require().NoError(err)
	_, legacyAuthAt, err := s.svc.ValidateSession(legacy)
	s.Require().NoError(err)
	s.Require().True(legacyAuthAt.IsZero())

	refreshed, err := s.renewLikeTheRefreshEndpoint(legacy)
	s.Require().NoError(err)

	// Still a working session, still carrying no authentication time.
	validated, refreshedAuthAt, err := s.svc.ValidateSession(refreshed)
	s.Require().NoError(err)
	s.Equal(user.ID, validated.ID)
	s.True(refreshedAuthAt.IsZero())
}

func (s *JWTIntegrationSuite) TestRefreshFlow_InvalidToken() {
	refreshed, err := s.renewLikeTheRefreshEndpoint("not.a.valid.token")
	s.Empty(refreshed)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_INVALID")
}

func (s *JWTIntegrationSuite) TestRefreshFlow_ExpiredBeyondGrace() {
	user := s.createActiveUser("refresh-expired@example.com")

	shortSvc := NewJWTService(s.db, &config.Config{
		JWT: config.JWTConfig{SecretKey: s.cfg.JWT.SecretKey, Expiry: 0},
	}, usersvc.NewUserService(s.db))

	token, err := shortSvc.CreateToken(user)
	s.Require().NoError(err)
	time.Sleep(1100 * time.Millisecond)

	user2, _, err := s.svc.ValidateSessionLenient(token, time.Nanosecond)
	s.Nil(user2)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_EXPIRED")
}

func (s *JWTIntegrationSuite) TestRefreshFlow_InactiveUser() {
	user := s.createActiveUser("refresh-inactive@example.com")

	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	// Deactivate user after token creation
	s.Require().NoError(s.db.Model(user).Update("is_active", false).Error)

	refreshed, err := s.renewLikeTheRefreshEndpoint(token)
	s.Empty(refreshed)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_INVALID")
}

// =============================================================================
// VALIDATE TOKEN LENIENT (grace period for recently-expired tokens)
// =============================================================================

func (s *JWTIntegrationSuite) TestValidateTokenLenient_ValidToken() {
	user := s.createActiveUser("lenient-valid@example.com")

	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	// A valid (non-expired) token should pass lenient validation
	validated, err := s.svc.ValidateTokenLenient(token, 5*time.Minute)
	s.Require().NoError(err)
	s.Equal(user.ID, validated.ID)
}

func (s *JWTIntegrationSuite) TestValidateTokenLenient_RecentlyExpiredWithinGrace() {
	user := s.createActiveUser("lenient-grace@example.com")

	// Create a token that expires immediately
	shortCfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: s.cfg.JWT.SecretKey,
			Expiry:    0,
		},
	}
	shortSvc := NewJWTService(s.db, shortCfg, usersvc.NewUserService(s.db))

	token, err := shortSvc.CreateToken(user)
	s.Require().NoError(err)

	time.Sleep(100 * time.Millisecond)

	// Strict validation should fail
	_, strictErr := s.svc.ValidateToken(token)
	s.Require().Error(strictErr)
	s.Contains(strictErr.Error(), "TOKEN_EXPIRED")

	// Lenient validation with generous grace period should succeed
	validated, err := s.svc.ValidateTokenLenient(token, 1*time.Minute)
	s.Require().NoError(err)
	s.Equal(user.ID, validated.ID)
	s.Equal("lenient-grace@example.com", *validated.Email)
}

func (s *JWTIntegrationSuite) TestValidateTokenLenient_ExpiredBeyondGrace() {
	user := s.createActiveUser("lenient-beyond@example.com")

	// Manually craft a token that expired 10 minutes ago
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   *user.Email,
		"exp":     time.Now().Add(-10 * time.Minute).Unix(),
		"iat":     time.Now().Add(-34 * time.Hour).Unix(),
		"iss":     "psychic-homily-backend",
		"aud":     "psychic-homily-users",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.cfg.JWT.SecretKey))
	s.Require().NoError(err)

	// Grace period of 5 minutes is not enough for a token that expired 10 min ago
	validated, err := s.svc.ValidateTokenLenient(tokenStr, 5*time.Minute)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_EXPIRED")
}

func (s *JWTIntegrationSuite) TestValidateTokenLenient_InactiveUser() {
	user := s.createInactiveUser("lenient-inactive@example.com")

	// Craft a recently-expired token
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   *user.Email,
		"exp":     time.Now().Add(-1 * time.Second).Unix(),
		"iat":     time.Now().Add(-24 * time.Hour).Unix(),
		"iss":     "psychic-homily-backend",
		"aud":     "psychic-homily-users",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.cfg.JWT.SecretKey))
	s.Require().NoError(err)

	// Even within grace period, inactive user should be rejected
	validated, err := s.svc.ValidateTokenLenient(tokenStr, 5*time.Minute)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_INVALID")
}

func (s *JWTIntegrationSuite) TestValidateTokenLenient_TamperedToken() {
	user := s.createActiveUser("lenient-tampered@example.com")

	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	// Tamper with the signature
	tampered := token[:len(token)-5] + "XXXXX"

	validated, err := s.svc.ValidateTokenLenient(tampered, 5*time.Minute)
	s.Nil(validated)
	s.Require().Error(err)
	s.Contains(err.Error(), "TOKEN_INVALID")
}
