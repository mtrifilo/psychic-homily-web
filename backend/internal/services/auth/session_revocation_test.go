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

func TestSessionMintedBeforeRevocation(t *testing.T) {
	revokedAt := time.Date(2026, 10, 24, 12, 0, 6, 0, time.UTC)
	revoked := &authm.User{SessionsValidAfter: &revokedAt}

	cases := map[string]struct {
		claims jwt.MapClaims
		user   *authm.User
		want   bool
	}{
		"never revoked, no claim":        {jwt.MapClaims{}, &authm.User{}, false},
		"revoked, no claim":              {jwt.MapClaims{}, revoked, true},
		"revoked, earlier instant":       {jwt.MapClaims{jwtSessionsValidAfterClaim: float64(revokedAt.Unix() - 1)}, revoked, true},
		"revoked, current instant":       {jwt.MapClaims{jwtSessionsValidAfterClaim: float64(revokedAt.Unix())}, revoked, false},
		"revoked, claim of another type": {jwt.MapClaims{jwtSessionsValidAfterClaim: "1792843206"}, revoked, true},
	}
	for name, c := range cases {
		if got := sessionMintedBeforeRevocation(c.claims, c.user); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
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

func (s *JWTIntegrationSuite) reloadUser(id uint) *authm.User {
	s.T().Helper()
	user, err := usersvc.NewUserService(s.db).GetUserByID(id)
	s.Require().NoError(err)
	return user
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

	// Minted in the revocation's own second, for the account as it stands
	// after it.
	after, err := s.svc.CreateToken(s.reloadUser(user.ID))
	s.Require().NoError(err)
	got, _, err := s.svc.ValidateSession(after)
	s.Require().NoError(err)
	s.Equal(user.ID, got.ID)

	// A renewal of the honored session is honored too.
	renewed, err := s.svc.RenewSessionToken(got, time.Time{})
	s.Require().NoError(err)
	_, _, err = s.svc.ValidateSession(renewed)
	s.NoError(err)
}

// A sign-in that read the account before the revocation committed and mints
// after it, the race a password login in flight during the mailbox proof
// runs, is refused however late the mint is.
func (s *JWTIntegrationSuite) TestRevokeAllSessions_RefusesAMintFromARowReadBeforeIt() {
	user := s.createActiveUser("stale-read@test.com")
	readBeforeRevocation := s.reloadUser(user.ID)

	s.Require().NoError(RevokeAllSessions(s.db, user, time.Now()))
	time.Sleep(1100 * time.Millisecond)

	late, err := s.svc.CreateToken(readBeforeRevocation)
	s.Require().NoError(err)
	_, _, err = s.svc.ValidateSession(late)
	s.requireRevoked(err)
}

// Two revocations in one second still refuse a session minted between them.
func (s *JWTIntegrationSuite) TestRevokeAllSessions_EachRevocationMovesTheInstant() {
	user := s.createActiveUser("twice@test.com")
	now := time.Now()
	s.Require().NoError(RevokeAllSessions(s.db, user, now))
	first := *user.SessionsValidAfter
	between, err := s.svc.CreateToken(s.reloadUser(user.ID))
	s.Require().NoError(err)

	s.Require().NoError(RevokeAllSessions(s.db, user, now))
	s.True(user.SessionsValidAfter.After(first), "the second revocation writes a later instant")
	s.Equal(time.Duration(0), user.SessionsValidAfter.Sub(user.SessionsValidAfter.Truncate(time.Second)))

	_, _, err = s.svc.ValidateSession(between)
	s.requireRevoked(err)
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

func (s *JWTIntegrationSuite) TestRevokeAllSessions_UnknownAccountFails() {
	missing := &authm.User{ID: 999_999}
	s.Error(RevokeAllSessions(s.db, missing, time.Now()))
	s.Nil(missing.SessionsValidAfter)
}
