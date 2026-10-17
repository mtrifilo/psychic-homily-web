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

// assertClosedAccountRefusal checks that err tells a closed account apart from
// a bad credential, and names the account.
func (s *JWTMiddlewareIntegrationSuite) assertClosedAccountRefusal(err error, userID uint) {
	s.T().Helper()
	var closed *auth.SessionAccountClosedError
	s.Require().True(errors.As(err, &closed), "got %v", err)
	s.Equal(userID, closed.UserID)
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
	s.assertClosedAccountRefusal(err, user.ID)
}

func (s *JWTMiddlewareIntegrationSuite) TestSessionUserFromRequest_SoftDeletedAfterSignInIsRefusedAsClosed() {
	user := s.createActiveUser("session-soft-deleted@test.com")
	token := s.sessionTokenFor(user)
	s.Require().NoError(usersvc.NewUserService(s.db).SoftDeleteAccount(user.ID, nil))

	got, err := SessionUserFromRequest(s.jwtService, sessionCookieRequest(token))

	s.Nil(got)
	s.assertClosedAccountRefusal(err, user.ID)
}

// Every other failure is never the closed-account reason: that reason is
// reported to the reader, so it is only given for a credential that proved
// which account they hold. Refusals of the credential are AuthErrors with the
// code that says why; a failure to resolve the session at all is not an
// AuthError, so a caller cannot mistake a fault for a verdict.
func (s *JWTMiddlewareIntegrationSuite) TestSessionUserFromRequest_FailuresAreNotReportedAsClosed() {
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

	pastExpiry := auth.NewJWTService(s.db, &config.Config{JWT: config.JWTConfig{
		SecretKey: s.cfg.JWT.SecretKey,
		Expiry:    -1,
	}}, nil)
	live := s.createActiveUser("session-expired@test.com")
	expired, err := pastExpiry.CreateToken(live)
	s.Require().NoError(err)

	storeUnavailable := auth.NewJWTService(nil, s.cfg, usersvc.NewUserService(nil))
	liveToken := s.sessionTokenFor(live)

	// wantCode is the credential refusal's code; empty means a fault, which
	// must not be an AuthError at all.
	cases := map[string]struct {
		service  *auth.JWTService
		req      *http.Request
		wantCode string
	}{
		"no credential":             {s.jwtService, sessionCookieRequest(""), autherrors.CodeTokenMissing},
		"not a token":               {s.jwtService, sessionCookieRequest("not-a-jwt"), autherrors.CodeTokenInvalid},
		"signed by another key":     {s.jwtService, sessionCookieRequest(forged), autherrors.CodeTokenInvalid},
		"expired":                   {s.jwtService, sessionCookieRequest(expired), autherrors.CodeTokenExpired},
		"account no longer exists":  {s.jwtService, sessionCookieRequest(hardDeletedToken), autherrors.CodeTokenInvalid},
		"account store unavailable": {storeUnavailable, sessionCookieRequest(liveToken), ""},
		"no resolver":               {nil, sessionCookieRequest(liveToken), ""},
		"no request":                {s.jwtService, nil, ""},
	}
	for name, tc := range cases {
		got, err := SessionUserFromRequest(tc.service, tc.req)
		s.Nil(got, name)
		s.Require().Error(err, name)
		var closed *auth.SessionAccountClosedError
		s.False(errors.As(err, &closed), "%s: got %v", name, err)
		var refusal *autherrors.AuthError
		isRefusal := errors.As(err, &refusal)
		if tc.wantCode == "" {
			s.False(isRefusal, "%s: a fault must not read as a credential refusal: got %v", name, err)
			continue
		}
		s.Require().True(isRefusal, "%s: got %v", name, err)
		s.Equal(tc.wantCode, refusal.Code, name)
	}
}
