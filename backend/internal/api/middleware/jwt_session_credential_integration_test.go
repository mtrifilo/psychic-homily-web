package middleware

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/db"
	adminsvc "psychic-homily-backend/internal/services/admin"
)

// A handler that may act only for a session (the sign-out-everywhere-else
// renewal) reads IsSessionCredentialFromContext. These hold that each JWT
// middleware reports a session principal as one and an API-token principal as
// not one.

func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_ReportsASessionPrincipalAsASession() {
	user := s.createActiveUser("huma-session-credential@test.com")
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(s.T(), req)

	var reached, isSession bool
	HumaJWTMiddleware(s.jwtService)(ctx, func(next huma.Context) {
		reached, isSession = true, IsSessionCredentialFromContext(next.Context())
	})
	s.Require().True(reached)
	s.True(isSession)
}

func (s *JWTMiddlewareIntegrationSuite) TestHumaJWT_ReportsAnAPITokenPrincipalAsNotASession() {
	// The middleware validates API tokens on the process-wide handle it reads
	// when it is built.
	prev := db.DB
	db.DB = s.db
	s.T().Cleanup(func() { db.DB = prev })

	admin := s.createActiveUser("huma-api-token-credential@test.com")
	s.Require().NoError(s.db.Model(admin).Update("is_admin", true).Error)
	created, err := adminsvc.NewAPITokenService(s.db).CreateToken(admin.ID, nil, 30)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		sqlDB, _ := s.db.DB()
		_, _ = sqlDB.Exec("DELETE FROM api_tokens")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+created.Token)
	ctx, _ := newHumaContext(s.T(), req)

	reached, isSession := false, true
	HumaJWTMiddleware(s.jwtService)(ctx, func(next huma.Context) {
		reached, isSession = true, IsSessionCredentialFromContext(next.Context())
	})
	s.Require().True(reached, "the API token must authenticate for this to test anything")
	s.False(isSession)
}

func (s *JWTMiddlewareIntegrationSuite) TestOtherJWTMiddlewares_ReportASessionPrincipalAsASession() {
	user := s.createActiveUser("other-session-credential@test.com")
	token, err := s.jwtService.RenewSessionToken(user, time.Now())
	s.Require().NoError(err)

	s.Run("chi JWTMiddleware", func() {
		var isSession bool
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isSession = IsSessionCredentialFromContext(r.Context())
		})
		req := httptest.NewRequest(http.MethodGet, "/auth/link/google", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		JWTMiddleware(s.jwtService)(inner).ServeHTTP(httptest.NewRecorder(), req)
		s.True(isSession)
	})

	for name, mw := range map[string]func(huma.Context, func(huma.Context)){
		"OptionalHumaJWTMiddleware": OptionalHumaJWTMiddleware(s.jwtService),
		"LenientHumaJWTMiddleware":  LenientHumaJWTMiddleware(s.jwtService, time.Minute),
	} {
		s.Run(name, func() {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			ctx, _ := newHumaContext(s.T(), req)
			var isSession bool
			mw(ctx, func(next huma.Context) {
				isSession = IsSessionCredentialFromContext(next.Context())
			})
			s.True(isSession)
		})
	}
}
