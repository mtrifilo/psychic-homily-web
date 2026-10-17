package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	usersvc "psychic-homily-backend/internal/services/user"
)

// sessionCookieRequest is a request carrying token the way a browser returning
// from a provider does: as the auth cookie, with no JWT middleware in front.
func sessionCookieRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/auth/callback/google", nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: token})
	}
	return req
}

func (s *JWTMiddlewareIntegrationSuite) sessionTokenFor(user *authm.User) string {
	s.T().Helper()
	token, err := s.jwtService.CreateToken(user)
	s.Require().NoError(err)
	return token
}

func (s *JWTMiddlewareIntegrationSuite) TestSessionUserFromRequest_ActiveAccountResolves() {
	user := s.createActiveUser("session-active@test.com")

	got, err := SessionUserFromRequest(s.jwtService, sessionCookieRequest(s.sessionTokenFor(user)))

	s.Require().NoError(err)
	s.Equal(user.ID, got.ID)
}

// A session minted while the account was open stays correctly signed after the
// account closes. Only the account's state can refuse it, and the refusal has
// to be distinguishable from a bad credential so the caller can say which.
func (s *JWTMiddlewareIntegrationSuite) TestSessionUserFromRequest_DeactivatedAfterSignInIsRefusedAsClosed() {
	user := s.createActiveUser("session-deactivated@test.com")
	token := s.sessionTokenFor(user)
	s.Require().NoError(s.db.Model(user).Update("is_active", false).Error)

	got, err := SessionUserFromRequest(s.jwtService, sessionCookieRequest(token))

	s.Nil(got)
	s.True(errors.Is(err, auth.ErrSessionAccountMayNotSignIn), "got %v", err)
}

func (s *JWTMiddlewareIntegrationSuite) TestSessionUserFromRequest_SoftDeletedAfterSignInIsRefusedAsClosed() {
	user := s.createActiveUser("session-soft-deleted@test.com")
	token := s.sessionTokenFor(user)
	s.Require().NoError(usersvc.NewUserService(s.db).SoftDeleteAccount(user.ID, nil))

	got, err := SessionUserFromRequest(s.jwtService, sessionCookieRequest(token))

	s.Nil(got)
	s.True(errors.Is(err, auth.ErrSessionAccountMayNotSignIn), "got %v", err)
}

// Every other failure is a missing or unusable credential, never the closed-
// account reason: that reason is reported to the reader, so it is only given
// for a credential that proved which account they hold.
func (s *JWTMiddlewareIntegrationSuite) TestSessionUserFromRequest_UnusableCredentialIsNotReportedAsClosed() {
	user := s.createActiveUser("session-hard-deleted@test.com")
	hardDeletedToken := s.sessionTokenFor(user)
	sqlDB, _ := s.db.DB()
	_, err := sqlDB.Exec("DELETE FROM users WHERE id = $1", user.ID)
	s.Require().NoError(err)

	otherKey := auth.NewJWTService(s.db, &config.Config{JWT: config.JWTConfig{
		SecretKey: "a-different-secret-key-of-32-chars!!",
		Expiry:    24,
	}}, nil)
	forged, err := otherKey.CreateToken(&authm.User{ID: user.ID})
	s.Require().NoError(err)

	// wantCode is checked only where the refusal's code is part of the
	// contract: a request with no credential at all is TOKEN_MISSING.
	cases := map[string]struct {
		service  *auth.JWTService
		req      *http.Request
		wantCode string
	}{
		"no credential":            {s.jwtService, sessionCookieRequest(""), autherrors.CodeTokenMissing},
		"not a token":              {s.jwtService, sessionCookieRequest("not-a-jwt"), ""},
		"signed by another key":    {s.jwtService, sessionCookieRequest(forged), ""},
		"account no longer exists": {s.jwtService, sessionCookieRequest(hardDeletedToken), ""},
		"no resolver":              {nil, sessionCookieRequest(hardDeletedToken), ""},
		"no request":               {s.jwtService, nil, ""},
	}
	for name, tc := range cases {
		got, err := SessionUserFromRequest(tc.service, tc.req)
		s.Nil(got, name)
		s.Require().Error(err, name)
		s.False(errors.Is(err, auth.ErrSessionAccountMayNotSignIn), "%s: got %v", name, err)
		if tc.wantCode != "" {
			var authErr *autherrors.AuthError
			s.Require().True(errors.As(err, &authErr), name)
			s.Equal(tc.wantCode, authErr.Code, name)
		}
	}
}
