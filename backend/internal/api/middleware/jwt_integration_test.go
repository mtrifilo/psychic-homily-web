package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
)

// JWTMiddlewareIntegrationSuite tests JWT middleware with a real PostgreSQL database.
type JWTMiddlewareIntegrationSuite struct {
	suite.Suite
	db         *gorm.DB
	testDB     *testutil.TestDatabase
	cfg        *config.Config
	jwtService *auth.JWTService
}

func TestJWTMiddlewareIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(JWTMiddlewareIntegrationSuite))
}

func (s *JWTMiddlewareIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	// Set up config and JWT service with real DB
	s.cfg = &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "integration-test-secret-key-32chars!",
			Expiry:    24,
		},
	}
	s.jwtService = auth.NewJWTService(s.db, s.cfg, usersvc.NewUserService(s.db))
}

func (s *JWTMiddlewareIntegrationSuite) TearDownTest() {
	sqlDB, _ := s.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM oauth_accounts")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func (s *JWTMiddlewareIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

// --- Helpers ---

func (s *JWTMiddlewareIntegrationSuite) createActiveUser(email string) *models.User {
	s.T().Helper()
	user := &models.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *JWTMiddlewareIntegrationSuite) createInactiveUser(email string) *models.User {
	s.T().Helper()
	// Create as active first, then update to inactive (GORM bool zero-value gotcha)
	user := s.createActiveUser(email)
	s.Require().NoError(s.db.Model(user).Update("is_active", false).Error)
	user.IsActive = false
	return user
}

// --- HumaJWTMiddleware (strict) ---

func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_ValidBearerToken_UserInContext() {
	user := s.createActiveUser("bearer@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	mw := HumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	nextCalled := false
	var ctxUser *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			ctxUser = u
		}
	})

	s.True(nextCalled, "next() should be called for valid JWT")
	s.Require().NotNil(ctxUser, "user should be in context")
	s.Equal(user.ID, ctxUser.ID)
	s.Equal("bearer@test.com", *ctxUser.Email)
	s.True(ctxUser.IsActive)
}

func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_ValidCookieToken_UserInContext() {
	user := s.createActiveUser("cookie@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	mw := HumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	ctx, _ := newHumaContext(s.T(), req)

	nextCalled := false
	var ctxUser *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			ctxUser = u
		}
	})

	s.True(nextCalled, "next() should be called for valid JWT via cookie")
	s.Require().NotNil(ctxUser, "user should be in context")
	s.Equal(user.ID, ctxUser.ID)
	s.Equal("cookie@test.com", *ctxUser.Email)
}

func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_ValidToken_InactiveUser_Rejected() {
	user := s.createInactiveUser("inactive@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	mw := HumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, rr := newHumaContext(s.T(), req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	s.False(nextCalled, "next() should NOT be called for inactive user")
	var body JWTErrorResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &body))
	s.Equal("TOKEN_INVALID", body.ErrorCode)
}

func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_ValidToken_DeletedUser_Rejected() {
	user := s.createActiveUser("deleted@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	// Hard-delete the user via raw SQL (User.DeletedAt is *time.Time, not gorm.DeletedAt)
	sqlDB, _ := s.db.DB()
	_, err = sqlDB.Exec("DELETE FROM users WHERE id = $1", user.ID)
	s.Require().NoError(err)

	mw := HumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, rr := newHumaContext(s.T(), req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	s.False(nextCalled, "next() should NOT be called for deleted user")
	var body JWTErrorResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &body))
	s.Equal("TOKEN_INVALID", body.ErrorCode)
}

// --- LenientHumaJWTMiddleware ---

func (s *JWTMiddlewareIntegrationSuite) TestLenientJWT_ValidNonExpiredToken_UserInContext() {
	user := s.createActiveUser("lenient-valid@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	mw := LenientHumaJWTMiddleware(s.jwtService, 5*time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	nextCalled := false
	var ctxUser *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			ctxUser = u
		}
	})

	s.True(nextCalled, "next() should be called for valid non-expired JWT")
	s.Require().NotNil(ctxUser, "user should be in context")
	s.Equal(user.ID, ctxUser.ID)
	s.Equal("lenient-valid@test.com", *ctxUser.Email)
}

func (s *JWTMiddlewareIntegrationSuite) TestLenientJWT_ExpiredWithinGrace_UserInContext() {
	user := s.createActiveUser("lenient-grace@test.com")

	// Create token with 0 expiry (immediately expired) using a temporary JWTService
	expiredCfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: s.cfg.JWT.SecretKey,
			Expiry:    0,
		},
	}
	expiredJWTService := auth.NewJWTService(s.db, expiredCfg, usersvc.NewUserService(s.db))
	token, err := expiredJWTService.CreateToken(user)
	s.Require().NoError(err)

	// Use a JWTService with real DB for validation, with 10min grace period
	mw := LenientHumaJWTMiddleware(s.jwtService, 10*time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	nextCalled := false
	var ctxUser *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			ctxUser = u
		}
	})

	s.True(nextCalled, "next() should be called for expired JWT within grace period")
	s.Require().NotNil(ctxUser, "user should be in context")
	s.Equal(user.ID, ctxUser.ID)
	s.Equal("lenient-grace@test.com", *ctxUser.Email)
}

func (s *JWTMiddlewareIntegrationSuite) TestLenientJWT_ExpiredWithinGrace_InactiveUser_Rejected() {
	user := s.createInactiveUser("lenient-inactive@test.com")

	expiredCfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: s.cfg.JWT.SecretKey,
			Expiry:    0,
		},
	}
	expiredJWTService := auth.NewJWTService(s.db, expiredCfg, usersvc.NewUserService(s.db))
	token, err := expiredJWTService.CreateToken(user)
	s.Require().NoError(err)

	mw := LenientHumaJWTMiddleware(s.jwtService, 10*time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, rr := newHumaContext(s.T(), req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	s.False(nextCalled, "next() should NOT be called for inactive user")
	var body JWTErrorResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &body))
	s.Equal("TOKEN_INVALID", body.ErrorCode)
}

// --- OptionalHumaJWTMiddleware ---

func (s *JWTMiddlewareIntegrationSuite) TestOptionalJWT_ValidToken_UserInContext() {
	user := s.createActiveUser("optional-valid@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	mw := OptionalHumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	nextCalled := false
	var ctxUser *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			ctxUser = u
		}
	})

	s.True(nextCalled, "next() should be called")
	s.Require().NotNil(ctxUser, "user should be in context for valid JWT")
	s.Equal(user.ID, ctxUser.ID)
	s.Equal("optional-valid@test.com", *ctxUser.Email)
}

func (s *JWTMiddlewareIntegrationSuite) TestOptionalJWT_ValidToken_InactiveUser_ProceedsWithoutUser() {
	user := s.createInactiveUser("optional-inactive@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	mw := OptionalHumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	nextCalled := false
	var ctxUser *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			ctxUser = u
		}
	})

	s.True(nextCalled, "next() should be called (optional never blocks)")
	s.Nil(ctxUser, "user should NOT be in context for inactive user")
}

// --- JWTMiddleware (Chi/http.Handler) ---

func (s *JWTMiddlewareIntegrationSuite) TestChiJWT_ValidBearerToken_UserInContext() {
	user := s.createActiveUser("chi@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	var ctxUser *models.User
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxUser = GetUserFromContext(r.Context())
	})

	handler := JWTMiddleware(s.jwtService)(innerHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	s.Equal(http.StatusOK, rr.Code)
	s.Require().NotNil(ctxUser, "user should be set in context via GetUserFromContext")
	s.Equal(user.ID, ctxUser.ID)
	s.Equal("chi@test.com", *ctxUser.Email)
	s.True(ctxUser.IsActive)
}
