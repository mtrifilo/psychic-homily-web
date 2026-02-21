package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// JWTMiddlewareIntegrationSuite tests JWT middleware with a real PostgreSQL database.
type JWTMiddlewareIntegrationSuite struct {
	suite.Suite
	db         *gorm.DB
	container  testcontainers.Container
	ctx        context.Context
	cfg        *config.Config
	jwtService *services.JWTService
}

func TestJWTMiddlewareIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(JWTMiddlewareIntegrationSuite))
}

func (s *JWTMiddlewareIntegrationSuite) SetupSuite() {
	s.ctx = context.Background()

	container, err := testcontainers.GenericContainer(s.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	s.Require().NoError(err, "failed to start postgres container")
	s.container = container

	host, err := container.Host(s.ctx)
	s.Require().NoError(err, "failed to get host")
	port, err := container.MappedPort(s.ctx, "5432")
	s.Require().NoError(err, "failed to get port")

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	s.Require().NoError(err, "failed to connect to test database")
	s.db = db

	sqlDB, err := db.DB()
	s.Require().NoError(err, "failed to get sql.DB")

	// Run migrations — same list as handler integration tests
	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000002_add_artist_search_indexes.up.sql",
		"000003_add_venue_search_indexes.up.sql",
		"000004_update_venue_constraints.up.sql",
		"000005_add_show_status.up.sql",
		"000006_add_user_saved_shows.up.sql",
		"000007_add_private_show_status.up.sql",
		"000008_add_pending_venue_edits.up.sql",
		"000009_add_bandcamp_embed_url.up.sql",
		"000010_add_scraper_source_fields.up.sql",
		"000011_add_webauthn_tables.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000013_add_slugs.up.sql",
		"000014_add_account_lockout.up.sql",
		"000015_add_user_favorite_venues.up.sql",
		"000018_add_show_reports.up.sql",
		"000020_add_show_status_flags.up.sql",
		"000021_add_api_tokens.up.sql",
		"000022_add_audit_logs.up.sql",
		"000023_rename_scraper_to_discovery.up.sql",
		"000026_add_duplicate_of_show_id.up.sql",
		"000028_change_event_date_to_timestamptz.up.sql",
		"000029_fix_discovery_show_times.up.sql",
		"000030_add_artist_reports.up.sql",
		"000031_add_user_terms_acceptance.up.sql",
	}

	migrationDir := filepath.Join("..", "..", "..", "db", "migrations")

	for _, m := range migrations {
		migrationSQL, err := os.ReadFile(filepath.Join(migrationDir, m))
		s.Require().NoError(err, "failed to read migration %s", m)
		_, err = sqlDB.Exec(string(migrationSQL))
		s.Require().NoError(err, "failed to run migration %s", m)
	}

	// Migration 27 uses CONCURRENTLY — strip it for test transactions
	migration27, err := os.ReadFile(filepath.Join(migrationDir, "000027_add_index_duplicate_of_show_id.up.sql"))
	s.Require().NoError(err, "failed to read migration 000027")
	sql27 := strings.ReplaceAll(string(migration27), "CONCURRENTLY ", "")
	_, err = sqlDB.Exec(sql27)
	s.Require().NoError(err, "failed to run migration 000027")

	// Set up config and JWT service with real DB
	s.cfg = &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "integration-test-secret-key-32chars!",
			Expiry:    24,
		},
	}
	s.jwtService = services.NewJWTService(s.db, s.cfg)
}

func (s *JWTMiddlewareIntegrationSuite) TearDownTest() {
	sqlDB, _ := s.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM oauth_accounts")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func (s *JWTMiddlewareIntegrationSuite) TearDownSuite() {
	if s.container != nil {
		_ = s.container.Terminate(s.ctx)
	}
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
	expiredJWTService := services.NewJWTService(s.db, expiredCfg)
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
	expiredJWTService := services.NewJWTService(s.db, expiredCfg)
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
