package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	usersvc "psychic-homily-backend/internal/services/user"
)

func TestSessionsValidAfterFor_IsTheNextWholeSecond(t *testing.T) {
	cases := map[string]time.Time{
		"mid-second":      time.Date(2026, 10, 24, 12, 0, 5, 400_000_000, time.UTC),
		"on the second":   time.Date(2026, 10, 24, 12, 0, 5, 0, time.UTC),
		"just before one": time.Date(2026, 10, 24, 12, 0, 5, 999_999_999, time.UTC),
	}
	for name, now := range cases {
		got := sessionsValidAfterFor(now)
		want := time.Date(2026, 10, 24, 12, 0, 6, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
		// A token issued in now's second reads as issued at its start, and
		// must be refused.
		if !(&authm.User{SessionsValidAfter: &got}).SessionIssuedBeforeRevocation(now.Truncate(time.Second)) {
			t.Errorf("%s: a session issued in the revocation's own second is honored", name)
		}
	}
}

func TestSessionIssuedBeforeRevocation(t *testing.T) {
	validAfter := time.Date(2026, 10, 24, 12, 0, 6, 0, time.UTC)
	revoked := &authm.User{SessionsValidAfter: &validAfter}

	if !revoked.SessionIssuedBeforeRevocation(validAfter.Add(-time.Second)) {
		t.Error("a session issued before the revocation is honored")
	}
	if revoked.SessionIssuedBeforeRevocation(validAfter) {
		t.Error("a session issued at the revocation instant is refused")
	}
	if revoked.SessionIssuedBeforeRevocation(validAfter.Add(time.Hour)) {
		t.Error("a session issued after the revocation is refused")
	}
	if (&authm.User{}).SessionIssuedBeforeRevocation(time.Unix(0, 0)) {
		t.Error("an account with no revocation refuses a session")
	}
}

// --- Over a real database: the validator reads the column ---

func (s *JWTIntegrationSuite) requireRevoked(err error) {
	s.T().Helper()
	s.Require().Error(err)
	var authErr *apperrors.AuthError
	s.Require().True(errors.As(err, &authErr), "got %v", err)
	s.Equal(apperrors.CodeTokenInvalid, authErr.Code)
	s.ErrorIs(err, errSessionRevoked)
}

func (s *JWTIntegrationSuite) TestRevokeAllSessions_RefusesEarlierSessionsAndHonorsLaterOnes() {
	user := s.createActiveUser("revoke-all@test.com")
	before, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	s.Require().NoError(RevokeAllSessions(s.db, user, time.Now()))
	s.Require().NotNil(user.SessionsValidAfter)

	_, _, err = s.svc.ValidateSession(before)
	s.requireRevoked(err)
	_, _, err = s.svc.ValidateSessionLenient(before, refreshGracePeriod)
	s.requireRevoked(err)

	reloaded, err := usersvc.NewUserService(s.db).GetUserByID(user.ID)
	s.Require().NoError(err)
	after, err := s.svc.createTokenAfterRevocation(reloaded)
	s.Require().NoError(err)
	got, _, err := s.svc.ValidateSession(after)
	s.Require().NoError(err)
	s.Equal(user.ID, got.ID)

	// A session issued once the revocation instant has passed, by the
	// ordinary mint, is honored too.
	time.Sleep(time.Until(*user.SessionsValidAfter) + 10*time.Millisecond)
	later, err := s.svc.CreateToken(reloaded)
	s.Require().NoError(err)
	_, _, err = s.svc.ValidateSession(later)
	s.NoError(err)
}

func (s *JWTIntegrationSuite) TestRevokeAllSessions_AnAccountWithoutOneIsUntouched() {
	user := s.createActiveUser("never-revoked@test.com")
	other := s.createActiveUser("revoked-neighbour@test.com")
	token, err := s.svc.CreateToken(user)
	s.Require().NoError(err)

	s.Require().NoError(RevokeAllSessions(s.db, other, time.Now()))

	got, _, err := s.svc.ValidateSession(token)
	s.Require().NoError(err)
	s.Nil(got.SessionsValidAfter)
}

// A token carrying no issued-at cannot be shown to postdate a revocation.
func (s *JWTIntegrationSuite) TestRevokeAllSessions_RefusesASessionWithNoIssuedAt() {
	user := s.createActiveUser("no-iat@test.com")
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iss":     jwtIssuer,
		"aud":     jwtAudience,
		"sub":     jwtSessionSubject,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWT.SecretKey))
	s.Require().NoError(err)
	_, _, err = s.svc.ValidateSession(token)
	s.Require().NoError(err, "with no revocation the claim is not needed")

	s.Require().NoError(RevokeAllSessions(s.db, user, time.Now()))
	_, _, err = s.svc.ValidateSession(token)
	s.requireRevoked(err)
}

func (s *JWTIntegrationSuite) TestRevokeAllSessions_UnknownAccountFails() {
	missing := &authm.User{ID: 999_999}
	s.Error(RevokeAllSessions(s.db, missing, time.Now()))
	s.Nil(missing.SessionsValidAfter)
}
