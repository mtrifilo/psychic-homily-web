package auth

import (
	"errors"
	"sync"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	engagementsvc "psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// principal validates session as the JWT middleware does and returns what it
// puts in the request context: the account and the authentication time.
func (s *JWTIntegrationSuite) principal(session string) (*authm.User, time.Time) {
	s.T().Helper()
	user, authAt, err := s.svc.ValidateSession(session)
	s.Require().NoError(err)
	return user, authAt
}

// The feed URL is a bearer credential a session can carry away, so signing
// the other sessions out deletes it too.
func (s *JWTIntegrationSuite) TestSignOutOtherSessions_RevokesTheCalendarFeedToken() {
	user := s.createActiveUser("sign-out-others-feed@test.com")
	caller, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	feed := s.feedToken(user.ID)
	s.requireFeedTokenHonored(user.ID, feed)

	principal, authAt := s.principal(caller)
	renewed, err := s.authSvc.SignOutOtherSessions(principal, authAt)
	s.Require().NoError(err)

	s.requireFeedTokenRevoked(user.ID, feed)
	// The renewal names the stamp the revocation wrote, so it can mint the
	// owner a new feed URL.
	renewedPrincipal, _ := s.principal(renewed)
	created, err := s.calendar().CreateToken(user.ID, renewedPrincipal.SessionsValidAfter, "http://localhost:8080")
	s.Require().NoError(err)
	s.requireFeedTokenHonored(user.ID, created.Token)
}

// A feed-token mint that reaches the account row after the sign-out has
// committed is refused: the session that asked for it is one the sign-out
// revoked.
func (s *JWTIntegrationSuite) TestSignOutOtherSessions_RefusesAMintThatReadsTheAccountAfterIt() {
	user := s.createActiveUser("sign-out-others-feed-after@test.com")
	caller, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	principal, authAt := s.principal(caller)

	hooked, ran := testutil.WithWriteBeforeItsFirst(s.T(), s.db, func(*gorm.DB) {
		_, err := s.authSvc.SignOutOtherSessions(principal, authAt)
		s.Require().NoError(err)
	}, testutil.BeforeQuery)
	minted, err := engagementsvc.NewCalendarService(hooked, nil).CreateToken(user.ID, principal.SessionsValidAfter, "http://localhost:8080")

	s.Require().True(*ran, "the sign-out must commit inside the mint's window")
	s.Nil(minted)
	s.ErrorIs(err, shared.ErrSessionRevoked)
	s.Zero(s.feedTokenRows(user.ID), "a refused mint writes no token")
}

// A feed-token mint that holds the account row when the sign-out starts
// commits first; the sign-out waits for it and then deletes the token it
// wrote, so no feed URL a revoked session minted outlives the call.
func (s *JWTIntegrationSuite) TestSignOutOtherSessions_DeletesTheTokenOfAMintItWaitedFor() {
	user := s.createActiveUser("sign-out-others-feed-race@test.com")
	caller, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	principal, authAt := s.principal(caller)

	s.requireRevocationDeletesTheTokenOfAMintItWaitedFor(principal, func() error {
		_, err := s.authSvc.SignOutOtherSessions(principal, authAt)
		return err
	})
}

func (s *JWTIntegrationSuite) TestSignOutOtherSessions_RefusesEveryEarlierSessionAndRenewsTheCaller() {
	user := s.createActiveUser("sign-out-others@test.com")
	authAt := time.Now().Add(-2 * time.Hour).Truncate(time.Second)

	caller, err := s.svc.RenewSessionToken(user, authAt)
	s.Require().NoError(err)
	otherBrowser, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	// A CLI token is a session JWT minted the way GenerateCLITokenHandler
	// mints it.
	cliToken, err := s.svc.RenewSessionToken(user, time.Now().Add(-time.Minute))
	s.Require().NoError(err)

	principal, principalAuthAt := s.principal(caller)
	renewed, err := s.authSvc.SignOutOtherSessions(principal, principalAuthAt)
	s.Require().NoError(err)

	for _, earlier := range []string{caller, otherBrowser, cliToken} {
		_, _, err := s.svc.ValidateSession(earlier)
		s.requireRevoked(err)
	}

	got, gotAuthAt, err := s.svc.ValidateSession(renewed)
	s.Require().NoError(err)
	s.Equal(user.ID, got.ID)
	s.True(gotAuthAt.Equal(authAt), "auth_at %v, want the caller's %v carried through", gotAuthAt, authAt)
	s.Require().NotNil(got.SessionsValidAfter, "the account carries the revocation stamp")
}

func (s *JWTIntegrationSuite) TestSignOutOtherSessions_ZeroAuthTimeRenewsWithNone() {
	user := s.createActiveUser("sign-out-others-no-auth-at@test.com")
	caller, err := s.svc.RenewSessionToken(user, time.Time{})
	s.Require().NoError(err)

	principal, principalAuthAt := s.principal(caller)
	s.Require().True(principalAuthAt.IsZero())
	renewed, err := s.authSvc.SignOutOtherSessions(principal, principalAuthAt)
	s.Require().NoError(err)

	_, gotAuthAt, err := s.svc.ValidateSession(renewed)
	s.Require().NoError(err)
	s.True(gotAuthAt.IsZero(), "a renewal must not invent an authentication time, got %v", gotAuthAt)
}

// A revocation that commits after the caller's session was validated leaves
// that session unable to mint itself a current one, and this call writes
// nothing.
func (s *JWTIntegrationSuite) TestSignOutOtherSessions_RefusesASessionRevokedSinceValidation() {
	user := s.createActiveUser("sign-out-others-revoked@test.com")
	caller, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	principal, authAt := s.principal(caller)

	s.Require().NoError(RevokeAllSessions(s.db, s.reloadUser(user.ID), time.Now()))
	stampAfterOtherRevocation := *s.reloadUser(user.ID).SessionsValidAfter

	token, err := s.authSvc.SignOutOtherSessions(principal, authAt)
	s.Empty(token)
	s.Require().ErrorIs(err, shared.ErrSessionRevoked)
	s.True(shared.IsSessionRefusal(err))
	var authErr *apperrors.AuthError
	s.Require().True(errors.As(err, &authErr))
	s.Equal(apperrors.CodeTokenInvalid, authErr.Code)

	s.True(s.reloadUser(user.ID).SessionsValidAfter.Equal(stampAfterOtherRevocation),
		"a refused call must not write the stamp")
}

func (s *JWTIntegrationSuite) TestSignOutOtherSessions_RefusesAnAccountClosedSinceValidation() {
	user := s.createActiveUser("sign-out-others-closed@test.com")
	caller, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	principal, authAt := s.principal(caller)

	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", user.ID).Update("is_active", false).Error)

	token, err := s.authSvc.SignOutOtherSessions(principal, authAt)
	s.Empty(token)
	s.Require().ErrorIs(err, shared.ErrSessionAccountClosed)
	s.True(shared.IsSessionRefusal(err))
	s.Nil(s.reloadUser(user.ID).SessionsValidAfter, "a refused call must not write the stamp")
}

// The stamp commits only together with the renewal: a mint that fails rolls
// the revocation back, so the caller keeps the session it came with.
func (s *JWTIntegrationSuite) TestSignOutOtherSessions_MintFailureRevokesNothing() {
	user := s.createActiveUser("sign-out-others-mint-fails@test.com")
	caller, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	otherBrowser, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	feed := s.feedToken(user.ID)
	principal, authAt := s.principal(caller)

	mintFailed := errors.New("mint failed")
	token, err := signOutOtherSessions(s.db, principal, authAt, time.Now(),
		func(*authm.User, time.Time) (string, error) { return "", mintFailed })
	s.Empty(token)
	s.Require().ErrorIs(err, mintFailed)
	s.False(shared.IsSessionRefusal(err))

	s.Nil(s.reloadUser(user.ID).SessionsValidAfter, "the revocation must roll back with the failed mint")
	for _, session := range []string{caller, otherBrowser} {
		_, _, err := s.svc.ValidateSession(session)
		s.NoError(err)
	}
	s.requireFeedTokenHonored(user.ID, feed)
}

// Two sessions pressing the control at once, both validated against the same
// stamp: the row lock orders them, the first signs the second out, and the
// second is refused rather than signing the first out in turn.
func (s *JWTIntegrationSuite) TestSignOutOtherSessions_ConcurrentCallsLeaveExactlyOneSession() {
	user := s.createActiveUser("sign-out-others-race@test.com")
	first, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	second, err := s.svc.CreateToken(user)
	s.Require().NoError(err)
	firstPrincipal, firstAuthAt := s.principal(first)
	secondPrincipal, secondAuthAt := s.principal(second)

	// The first call is held inside its transaction, after its stamp write
	// and before its commit, until the second is seen waiting on a lock.
	// With an unlocked read the second would read the old stamp, pass the
	// check, and wait only at its own write, which then succeeds. This pins
	// that the account is read under a lock; it does not tell FOR UPDATE
	// from FOR SHARE, which differ only when both calls read before either
	// writes.
	holding, release := make(chan struct{}), make(chan struct{})
	heldMint := func(account *authm.User, authAt time.Time) (string, error) {
		close(holding)
		<-release
		return s.svc.RenewSessionToken(account, authAt)
	}

	var firstToken, secondToken string
	var firstErr, secondErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		firstToken, firstErr = signOutOtherSessions(s.db, firstPrincipal, firstAuthAt, time.Now(), heldMint)
	}()
	<-holding
	go func() {
		defer wg.Done()
		secondToken, secondErr = s.authSvc.SignOutOtherSessions(secondPrincipal, secondAuthAt)
	}()
	s.waitForALockWaiter()
	close(release)
	wg.Wait()

	s.Require().NoError(firstErr)
	s.Empty(secondToken)
	s.Require().ErrorIs(secondErr, shared.ErrSessionRevoked, "the second call is refused as revoked")

	_, _, err = s.svc.ValidateSession(firstToken)
	s.NoError(err, "the first call's renewal is honored")
	for _, session := range []string{first, second} {
		_, _, err := s.svc.ValidateSession(session)
		s.requireRevoked(err)
	}
}

// waitForALockWaiter returns once some session on the test database is
// waiting on a lock, failing the test if none is within ten seconds.
func (s *JWTIntegrationSuite) waitForALockWaiter() {
	s.T().Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int64
		s.Require().NoError(s.db.Raw(
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`,
		).Scan(&waiting).Error)
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.FailNow("no session waited on a lock: the second call did not queue behind the first")
}
