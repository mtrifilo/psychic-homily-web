package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
)

// HumaAdminMiddlewareIntegrationSuite exercises HumaAdminMiddleware end-to-end:
// JWT middleware places the user in context, the admin middleware then
// asserts IsAdmin. Confirms the wiring used by routes.go (PSY-423) — JWT +
// Admin chained on the same group — short-circuits non-admin callers
// before the handler runs.
type HumaAdminMiddlewareIntegrationSuite struct {
	suite.Suite
	db         *gorm.DB
	testDB     *testutil.TestDatabase
	cfg        *config.Config
	jwtService *auth.JWTService
}

func TestHumaAdminMiddlewareIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(HumaAdminMiddlewareIntegrationSuite))
}

func (s *HumaAdminMiddlewareIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	s.cfg = &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "integration-test-secret-key-32chars!",
			Expiry:    24,
		},
	}
	s.jwtService = auth.NewJWTService(s.db, s.cfg, usersvc.NewUserService(s.db))
}

func (s *HumaAdminMiddlewareIntegrationSuite) TearDownTest() {
	sqlDB, _ := s.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM oauth_accounts")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func (s *HumaAdminMiddlewareIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *HumaAdminMiddlewareIntegrationSuite) createAdminUser(email string) *authm.User {
	s.T().Helper()
	user := &authm.User{
		Email:         &email,
		IsActive:      true,
		IsAdmin:       true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *HumaAdminMiddlewareIntegrationSuite) createNonAdminUser(email string) *authm.User {
	s.T().Helper()
	user := &authm.User{
		Email:         &email,
		IsActive:      true,
		IsAdmin:       false,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

// stubHandlerInput / stubHandlerOutput are zero-value request/response
// types for the canary admin handler used by the suite's tests.
type stubHandlerInput struct{}

type stubHandlerOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// buildAdminAPI mirrors the production wiring in routes/routes.go: a chi
// mux + huma API + admin group with JWT then HumaAdminMiddleware. Returns
// the chi router so tests can serve actual HTTP requests, plus a pointer
// the canary handler flips when invoked. If the gate works the canary
// must remain false for non-admin callers.
func (s *HumaAdminMiddlewareIntegrationSuite) buildAdminAPI() (*chi.Mux, *bool) {
	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("PSY-423 Test", "1.0.0"))
	api.UseMiddleware(HumaRequestIDMiddleware)

	adminGroup := huma.NewGroup(api, "")
	adminGroup.UseMiddleware(HumaJWTMiddleware(s.jwtService, s.cfg.Session))
	adminGroup.UseMiddleware(HumaAdminMiddleware)

	handlerCalled := false
	huma.Register(adminGroup, huma.Operation{
		OperationID: "canary-admin-endpoint",
		Method:      http.MethodGet,
		Path:        "/admin/canary",
	}, func(ctx context.Context, _ *stubHandlerInput) (*stubHandlerOutput, error) {
		handlerCalled = true
		out := &stubHandlerOutput{}
		out.Body.OK = true
		return out, nil
	})

	return router, &handlerCalled
}

// TestAdminUser_HandlerInvoked confirms the happy path: an admin token
// makes it through both middlewares and the handler runs and returns 200.
func (s *HumaAdminMiddlewareIntegrationSuite) TestAdminUser_HandlerInvoked() {
	user := s.createAdminUser("admin-canary@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	router, handlerCalled := s.buildAdminAPI()
	req := httptest.NewRequest(http.MethodGet, "/admin/canary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	s.True(*handlerCalled, "admin handler should be invoked for an admin user")
	s.Equal(http.StatusOK, rr.Code, "expected 200 OK for admin user")
}

// TestNonAdminUser_HandlerNotInvoked confirms the gate: a non-admin
// token short-circuits at the admin middleware and the handler is never
// invoked. This is the core PSY-423 contract.
func (s *HumaAdminMiddlewareIntegrationSuite) TestNonAdminUser_HandlerNotInvoked() {
	user := s.createNonAdminUser("nonadmin-canary@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	router, handlerCalled := s.buildAdminAPI()
	req := httptest.NewRequest(http.MethodGet, "/admin/canary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	s.False(*handlerCalled, "handler must NOT be invoked for a non-admin user")
	s.Equal(http.StatusForbidden, rr.Code, "expected 403 Forbidden")

	var body JWTErrorResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &body))
	s.False(body.Success)
	s.Equal("Admin access required", body.Message)
}

// TestNoToken_RejectedByJWTLayer confirms the JWT layer rejects before
// the admin middleware even runs. This is here to make the layering
// explicit: HumaJWTMiddleware owns the missing-token path, and the admin
// middleware is never reached for that case.
func (s *HumaAdminMiddlewareIntegrationSuite) TestNoToken_RejectedByJWTLayer() {
	router, handlerCalled := s.buildAdminAPI()
	req := httptest.NewRequest(http.MethodGet, "/admin/canary", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	s.False(*handlerCalled, "handler must NOT be invoked when no token is provided")
	s.Equal(http.StatusUnauthorized, rr.Code, "JWT middleware should return 401 for a missing token")
}

// TestExpiredToken_RejectedByJWTLayer confirms the same layering for
// expired tokens — JWT rejects, admin middleware never runs.
func (s *HumaAdminMiddlewareIntegrationSuite) TestExpiredToken_RejectedByJWTLayer() {
	user := s.createAdminUser("expired-admin@test.com")

	expiredCfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: s.cfg.JWT.SecretKey,
			Expiry:    0,
		},
	}
	expiredJWT := auth.NewJWTService(s.db, expiredCfg, usersvc.NewUserService(s.db))
	token, err := expiredJWT.CreateToken(user)
	s.Require().NoError(err)

	router, handlerCalled := s.buildAdminAPI()
	req := httptest.NewRequest(http.MethodGet, "/admin/canary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	s.False(*handlerCalled, "handler must NOT be invoked for an expired token")
	s.Equal(http.StatusUnauthorized, rr.Code, "JWT middleware should return 401 for an expired token")
	s.Contains(strings.ToLower(rr.Body.String()), "expired")
}

// silence unused import in environments where humatest pulls a transitive build tag.
var _ = humatest.NewContext
