package middleware

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// The middleware plants how the credential arrived and how old the session
// is, which the refresh handler and the profile read.
func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_CookieSession_PlantsSourceAndIssuedAt() {
	user := s.createActiveUser("facts-cookie@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	ctx, _ := newHumaContext(s.T(), req)

	var asCookie bool
	var issuedAt time.Time
	HumaJWTMiddleware(s.jwtService)(ctx, func(next huma.Context) {
		asCookie = SessionPresentedAsCookieFromContext(next.Context())
		issuedAt = GetSessionIssuedAtFromContext(next.Context())
	})

	s.True(asCookie, "a cookie credential is reported as one")
	s.WithinDuration(time.Now(), issuedAt, 5*time.Second, "a fresh token's age is about zero")
}

func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_BearerSession_IsNotACookie() {
	user := s.createActiveUser("facts-bearer@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	asCookie := true
	var issuedAt time.Time
	HumaJWTMiddleware(s.jwtService)(ctx, func(next huma.Context) {
		asCookie = SessionPresentedAsCookieFromContext(next.Context())
		issuedAt = GetSessionIssuedAtFromContext(next.Context())
	})

	s.False(asCookie, "a Bearer credential is not reported as a cookie")
	s.False(issuedAt.IsZero(), "a session token still carries its age")
}

// The lenient middleware on the refresh route plants the same facts.
func (s *JWTMiddlewareIntegrationSuite) TestLenientJWT_CookieSession_PlantsSourceAndIssuedAt() {
	user := s.createActiveUser("facts-lenient@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	ctx, _ := newHumaContext(s.T(), req)

	var asCookie bool
	var issuedAt time.Time
	LenientHumaJWTMiddleware(s.jwtService, 7*24*time.Hour)(ctx, func(next huma.Context) {
		asCookie = SessionPresentedAsCookieFromContext(next.Context())
		issuedAt = GetSessionIssuedAtFromContext(next.Context())
	})

	s.True(asCookie)
	s.WithinDuration(time.Now(), issuedAt, 5*time.Second)
}
