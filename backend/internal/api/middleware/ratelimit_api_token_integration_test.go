package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
)

// SkipRateLimitForAdmin against real credentials: a real API token row, a real
// admin session JWT, and a real non-admin session JWT, all resolved by the
// services production wires in. The unit tests inject a stub validator, so this
// suite is what pins that the stub matches APITokenService.
type SkipRateLimitAPITokenSuite struct {
	suite.Suite
	db         *gorm.DB
	testDB     *testutil.TestDatabase
	jwtService *auth.JWTService
	tokenSvc   *adminsvc.APITokenService
}

func TestSkipRateLimitAPIToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(SkipRateLimitAPITokenSuite))
}

func (s *SkipRateLimitAPITokenSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	cfg := &config.Config{JWT: config.JWTConfig{
		SecretKey: "integration-test-secret-key-32chars!",
		Expiry:    24,
	}}
	s.jwtService = auth.NewJWTService(s.db, cfg, usersvc.NewUserService(s.db))
	s.tokenSvc = adminsvc.NewAPITokenService(s.db)
}

func (s *SkipRateLimitAPITokenSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *SkipRateLimitAPITokenSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, table := range []string{"api_tokens", "user_preferences", "oauth_accounts", "users"} {
		_, err := sqlDB.Exec("DELETE FROM " + table)
		s.Require().NoError(err, "cleanup %s", table)
	}
}

func (s *SkipRateLimitAPITokenSuite) createUser(email string, isAdmin bool) *authm.User {
	s.T().Helper()
	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true, IsAdmin: isAdmin}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *SkipRateLimitAPITokenSuite) sessionToken(user *authm.User) string {
	s.T().Helper()
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)
	return token
}

// newHandler is the unit tests' harness wired to the PRODUCTION validator, so a
// change to APITokenValidator's semantics shows up here rather than only in the
// stub the unit tests inject.
func (s *SkipRateLimitAPITokenSuite) newHandler() (http.Handler, *int) {
	s.T().Helper()
	return skipAdminMW(s.T(), s.jwtService, APITokenValidator(s.tokenSvc))
}

// send issues one request carrying the given Authorization header and cookie
// (either may be empty) and returns the status code.
func (s *SkipRateLimitAPITokenSuite) send(handler http.Handler, header, cookie, ip string) int {
	s.T().Helper()
	req := httptest.NewRequest(http.MethodPost, "/saved-shows/1", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: cookie})
	}
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Code
}

// A live API token bypasses the cap: the ph CLI's bulk imports keep working.
func (s *SkipRateLimitAPITokenSuite) TestLiveAPITokenIsExempt() {
	admin := s.createUser("token-owner@test.com", true)
	created, err := s.tokenSvc.CreateToken(admin.ID, nil, 30)
	s.Require().NoError(err)

	handler, hits := s.newHandler()
	for i := 0; i < 5; i++ {
		s.Equal(http.StatusOK, s.send(handler, "Bearer "+created.Token, "", "10.0.0.1:100"),
			"request %d: a live API token must bypass the limiter", i+1)
	}
	s.Equal(5, *hits)
}

// An admin session JWT keeps the PSY-345 bypass.
func (s *SkipRateLimitAPITokenSuite) TestAdminSessionJWTIsExempt() {
	admin := s.createUser("admin-session@test.com", true)
	token := s.sessionToken(admin)

	handler, hits := s.newHandler()
	for i := 0; i < 5; i++ {
		s.Equal(http.StatusOK, s.send(handler, "Bearer "+token, "", "10.0.0.2:100"),
			"request %d: an admin session JWT must bypass the limiter", i+1)
	}
	s.Equal(5, *hits)
}

// A revoked token is no token: its bearer is metered like anyone else.
func (s *SkipRateLimitAPITokenSuite) TestRevokedAPITokenIsLimited() {
	admin := s.createUser("revoked-owner@test.com", true)
	created, err := s.tokenSvc.CreateToken(admin.ID, nil, 30)
	s.Require().NoError(err)
	s.Require().NoError(s.tokenSvc.RevokeToken(admin.ID, created.ID))

	handler, _ := s.newHandler()
	s.Equal(http.StatusOK, s.send(handler, "Bearer "+created.Token, "", "10.0.0.3:100"))
	s.Equal(http.StatusTooManyRequests, s.send(handler, "Bearer "+created.Token, "", "10.0.0.3:101"),
		"a revoked API token must not bypass the limiter")
}

// The headline case. A non-admin cookie session that names an API token it does
// not hold is metered as the session it actually rides on.
func (s *SkipRateLimitAPITokenSuite) TestForgedAPITokenOverCookieSessionIsLimited() {
	admin := s.createUser("live-owner@test.com", true)
	created, err := s.tokenSvc.CreateToken(admin.ID, nil, 30)
	s.Require().NoError(err)

	member := s.createUser("member@test.com", false)
	session := s.sessionToken(member)

	// Both shapes: one the authenticator rejects outright, one it ignores
	// entirely so the request is authenticated from the cookie instead.
	headers := []string{
		"Bearer " + APITokenPrefix + "forged",
		"Bearer " + created.Token + " trailing",
	}
	for i, header := range headers {
		s.Run([]string{"unknown token", "malformed header"}[i], func() {
			// A fresh limiter and a distinct IP per case, so one case never
			// spends the other's one-request budget.
			handler, hits := s.newHandler()
			ip := fmt.Sprintf("10.0.1.%d:100", i+1)
			s.Equal(http.StatusOK, s.send(handler, header, session, ip))
			s.Equal(http.StatusTooManyRequests, s.send(handler, header, session, ip),
				"a forged API token must not exempt a cookie session")
			s.Equal(1, *hits)
		})
	}
}

// A non-admin session JWT was never exempt and still is not.
func (s *SkipRateLimitAPITokenSuite) TestNonAdminSessionJWTIsLimited() {
	member := s.createUser("plain-member@test.com", false)
	token := s.sessionToken(member)

	handler, _ := s.newHandler()
	s.Equal(http.StatusOK, s.send(handler, "Bearer "+token, "", "10.0.0.4:100"))
	s.Equal(http.StatusTooManyRequests, s.send(handler, "Bearer "+token, "", "10.0.0.4:101"))
}
