package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	engagementsvc "psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
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
	s.ErrorIs(err, shared.ErrSessionRevoked)
}

func (s *JWTIntegrationSuite) reloadUser(id uint) *authm.User {
	s.T().Helper()
	user, err := usersvc.NewUserService(s.db).GetUserByID(id)
	s.Require().NoError(err)
	return user
}

// calendar is the feed-token service over the suite's database.
func (s *JWTIntegrationSuite) calendar() *engagementsvc.CalendarService {
	return engagementsvc.NewCalendarService(s.db, nil)
}

// feedToken mints a calendar-feed token for userID's account as it stands,
// as Settings does, and returns the plaintext the feed URL carries.
func (s *JWTIntegrationSuite) feedToken(userID uint) string {
	s.T().Helper()
	created, err := s.calendar().CreateToken(userID, s.reloadUser(userID).SessionsValidAfter, "http://localhost:8080")
	s.Require().NoError(err)
	return created.Token
}

// feedTokenRows counts the calendar-feed tokens userID holds.
func (s *JWTIntegrationSuite) feedTokenRows(userID uint) int64 {
	s.T().Helper()
	var rows int64
	s.Require().NoError(s.db.Model(&engagementm.CalendarToken{}).Where("user_id = ?", userID).Count(&rows).Error)
	return rows
}

// requireFeedTokenRevoked checks that userID holds no feed token and that the
// feed validator refuses token.
func (s *JWTIntegrationSuite) requireFeedTokenRevoked(userID uint, token string) {
	s.T().Helper()
	s.Zero(s.feedTokenRows(userID), "the account holds no feed token")
	_, err := s.calendar().ValidateCalendarToken(token)
	s.Error(err, "the old feed URL is refused")
}

// requireRevocationDeletesTheTokenOfAMintItWaitedFor starts revoke while a
// feed-token mint for principal's session holds the account row, and checks
// that revoke waits for the mint and then deletes the token it wrote.
func (s *JWTIntegrationSuite) requireRevocationDeletesTheTokenOfAMintItWaitedFor(principal *authm.User, revoke func() error) {
	s.T().Helper()
	var revocation <-chan error
	waitedForMint := false
	hooked, ran := testutil.WithWriteBeforeItsFirst(s.T(), s.db, func(*gorm.DB) {
		revocation = testutil.StartOnAnotherConnection(s.T(), s.db, revoke)
		waitedForMint = len(revocation) == 0
	}, testutil.BeforeCreate)
	// The mint's read-back of its row runs after its commit, racing the
	// revocation's delete, so it may report the row gone; no other failure is
	// allowed, so the token the revocation deletes is one the mint committed.
	minted, err := engagementsvc.NewCalendarService(hooked, nil).CreateToken(principal.ID, principal.SessionsValidAfter, "http://localhost:8080")
	if err != nil {
		s.Require().ErrorIs(err, gorm.ErrRecordNotFound, "only the mint's post-commit read-back may fail")
	}

	s.Require().True(*ran, "the revocation must start before the mint's insert")
	s.Require().NoError(testutil.AwaitConcurrentWrite(s.T(), revocation))
	s.True(waitedForMint, "the revocation must wait for the mint holding the account row")
	s.Zero(s.feedTokenRows(principal.ID), "the revocation deletes the token the mint wrote")
	if minted != nil {
		_, err := s.calendar().ValidateCalendarToken(minted.Token)
		s.Error(err, "the minted feed URL is refused")
	}
}

// linkReset is userID's calendar link-reset marker as the status endpoint
// reports it.
func (s *JWTIntegrationSuite) linkReset(userID uint) (*time.Time, *engagementm.CalendarLinkResetReason) {
	s.T().Helper()
	status, err := s.calendar().GetTokenStatus(userID, "http://localhost:8080")
	s.Require().NoError(err)
	return status.LinkResetAt, status.LinkResetReason
}

// requireLinkReset checks that userID holds a link-reset marker for reason,
// stamped no earlier than notBefore.
func (s *JWTIntegrationSuite) requireLinkReset(userID uint, reason engagementm.CalendarLinkResetReason, notBefore time.Time) {
	s.T().Helper()
	at, got := s.linkReset(userID)
	s.Require().NotNil(at, "the revocation records the link reset")
	s.Require().NotNil(got)
	s.Equal(reason, *got)
	s.False(at.Before(notBefore.Truncate(time.Microsecond)), "reset at %v, revocation started %v", at, notBefore)
}

// requireNoLinkReset checks that userID holds no link-reset marker.
func (s *JWTIntegrationSuite) requireNoLinkReset(userID uint) {
	s.T().Helper()
	at, reason := s.linkReset(userID)
	s.Nil(at, "no link reset is recorded")
	s.Nil(reason)
}

// requireFeedTokenHonored checks that the feed validator still serves token
// to userID.
func (s *JWTIntegrationSuite) requireFeedTokenHonored(userID uint, token string) {
	s.T().Helper()
	got, err := s.calendar().ValidateCalendarToken(token)
	s.Require().NoError(err, "the feed URL still works")
	s.Equal(userID, got.ID)
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

// Closing an account revokes its sessions, so a session minted before
// SoftDeleteAccount stays refused after RestoreAccount, including one minted
// late from a row read before the closure, while a session minted from the
// restored row is honored.
func (s *JWTIntegrationSuite) TestSoftDeleteAccount_SessionsBeforeItStayRefusedAfterRestore() {
	user := s.createActiveUser("closed-then-restored@test.com")
	before, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	readBeforeClosure := s.reloadUser(user.ID)

	s.softDelete(user)
	_, _, err = s.svc.ValidateSession(before)
	s.assertClosedAccountRefusal(err, user.ID)

	s.Require().NoError(usersvc.NewUserService(s.db).RestoreAccount(user.ID))
	_, _, err = s.svc.ValidateSession(before)
	s.requireRevoked(err)
	_, _, err = s.svc.ValidateSessionLenient(before, refreshGracePeriod)
	s.requireRevoked(err)

	late, err := s.svc.CreateToken(readBeforeClosure)
	s.Require().NoError(err)
	_, _, err = s.svc.ValidateSession(late)
	s.requireRevoked(err)

	after, err := s.svc.CreateToken(s.reloadUser(user.ID))
	s.Require().NoError(err)
	got, _, err := s.svc.ValidateSession(after)
	s.Require().NoError(err)
	s.Equal(user.ID, got.ID)
}
