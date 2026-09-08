package middleware

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	usersvc "psychic-homily-backend/internal/services/user"
)

// The re-authentication gates read the session's auth_at out of the request
// context. These hold that each middleware below puts it there, and that the
// value agrees with the minting service about when the session last stood
// behind a factor.

func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_CarriesSessionAuthTime() {
	user := s.createActiveUser("huma-auth-time@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)
	_, minted, err := s.jwtService.ValidateSession(token)
	s.Require().NoError(err)
	s.Require().False(minted.IsZero())

	mw := HumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	var got time.Time
	mw(ctx, func(next huma.Context) {
		got = GetSessionAuthTimeFromContext(next.Context())
	})

	s.True(got.Equal(minted))
}

// A session renewed by the refresh path reports the authentication time of the
// factor that started it, however long ago that was, so a gate that refused the
// old token refuses the renewed one too.
func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_RenewedSessionReportsTheOriginalAuthTime() {
	user := s.createActiveUser("huma-renewed-auth-time@test.com")

	authAt := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	renewed, err := s.jwtService.RenewSessionToken(user, authAt)
	s.Require().NoError(err)

	mw := HumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+renewed)
	ctx, _ := newHumaContext(s.T(), req)

	var got time.Time
	mw(ctx, func(next huma.Context) {
		got = GetSessionAuthTimeFromContext(next.Context())
	})

	s.True(got.Equal(authAt.UTC()))
	s.Greater(time.Since(got), time.Hour, "renewal does not make a session recently authenticated")
}

// A session carrying no auth_at reaches the handler with the zero time rather
// than with its issue time substituted.
func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_SessionWithoutAuthTimeCarriesZero() {
	user := s.createActiveUser("huma-legacy-auth-time@test.com")
	legacy, err := s.jwtService.RenewSessionToken(user, time.Time{})
	s.Require().NoError(err)

	mw := HumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+legacy)
	ctx, _ := newHumaContext(s.T(), req)

	var ctxUser *authm.User
	var got time.Time
	mw(ctx, func(next huma.Context) {
		ctxUser, _ = next.Context().Value(UserContextKey).(*authm.User)
		got = GetSessionAuthTimeFromContext(next.Context())
	})

	s.Require().NotNil(ctxUser, "a session without the claim is still a valid session")
	s.Equal(user.ID, ctxUser.ID)
	s.True(got.IsZero())
}

func (s *JWTMiddlewareIntegrationSuite) TestChiJWT_CarriesSessionAuthTime() {
	user := s.createActiveUser("chi-auth-time@test.com")

	authAt := time.Now().Add(-30 * time.Minute).Truncate(time.Second)
	token, err := s.jwtService.RenewSessionToken(user, authAt)
	s.Require().NoError(err)

	var got time.Time
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetSessionAuthTimeFromContext(r.Context())
	})
	handler := JWTMiddleware(s.jwtService)(inner)

	req := httptest.NewRequest(http.MethodGet, "/auth/link/google", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	s.True(got.Equal(authAt.UTC()))
}

func (s *JWTMiddlewareIntegrationSuite) TestOptionalJWT_CarriesSessionAuthTime() {
	user := s.createActiveUser("optional-auth-time@test.com")

	authAt := time.Now().Add(-45 * time.Minute).Truncate(time.Second)
	token, err := s.jwtService.RenewSessionToken(user, authAt)
	s.Require().NoError(err)

	mw := OptionalHumaJWTMiddleware(s.jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	var got time.Time
	mw(ctx, func(next huma.Context) {
		got = GetSessionAuthTimeFromContext(next.Context())
	})

	s.True(got.Equal(authAt.UTC()))
}

// The refresh endpoint runs behind the lenient middleware, so the claim has to
// survive a token that expired inside the grace window. Losing it there would
// quietly strip the standing from every session that refreshes late.
func (s *JWTMiddlewareIntegrationSuite) TestLenientJWT_CarriesSessionAuthTimeAcrossExpiry() {
	user := s.createActiveUser("lenient-auth-time@test.com")

	expiredCfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: s.cfg.JWT.SecretKey,
			Expiry:    0,
		},
	}
	expiredJWTService := auth.NewJWTService(s.db, expiredCfg, usersvc.NewUserService(s.db))
	token, err := expiredJWTService.CreateToken(user)
	s.Require().NoError(err)
	// exp has whole-second resolution, so wait past it: without this the token
	// can still be valid and the middleware takes the strict path, leaving the
	// grace window this test is named for unexercised.
	time.Sleep(1100 * time.Millisecond)
	_, _, strictErr := s.jwtService.ValidateSession(token)
	s.Require().Error(strictErr, "the token must be expired for this to test the grace window")
	_, minted, err := expiredJWTService.ValidateSessionLenient(token, 10*time.Minute)
	s.Require().NoError(err)
	s.Require().False(minted.IsZero())

	mw := LenientHumaJWTMiddleware(s.jwtService, 10*time.Minute)
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	var got time.Time
	mw(ctx, func(next huma.Context) {
		got = GetSessionAuthTimeFromContext(next.Context())
	})

	s.True(got.Equal(minted))
}
