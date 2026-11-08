package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/markbates/goth"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/auth"
	engagementsvc "psychic-homily-backend/internal/services/engagement"
)

// A request that passed its session check before the mailbox proof committed
// cannot attach a credential after it: the write compares the session's
// revocation stamp with the account's under the users-row lock, so the
// squatter's in-flight Settings link and calendar mint are refused with the
// answers a revoked session gets.

// squattedAccountWithPendingProof is an account an attacker registered on
// the victim's address, signed in to by the attacker, and the victim's
// emailed link that proves the mailbox.
func (s *OAuthHandlerIntegrationSuite) squattedAccountWithPendingProof(address string) (squatted *authm.User, attackerSession string, victim *linkStepEnv, proof string) {
	s.T().Helper()
	squatted = s.passwordAccount(address, false)
	attackerSession = s.sessionFrom(s.login(s.linkEnv(), address, collisionPassword))
	victim = s.linkEnv()
	proof = s.requestLink(victim, s.startLink(collisionIdentity(address, "google-victim-"+address)))
	return squatted, attackerSession, victim, proof
}

func (s *OAuthHandlerIntegrationSuite) providersOf(userID uint) []string {
	s.T().Helper()
	var providers []string
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).Where("user_id = ?", userID).Order("provider").Pluck("provider", &providers).Error)
	return providers
}

func (s *OAuthHandlerIntegrationSuite) calendarTokenRows(userID uint) int64 {
	s.T().Helper()
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&engagementm.CalendarToken{}).Where("user_id = ?", userID).Count(&rows).Error)
	return rows
}

// The attacker's Settings link to GitHub passes the session gate, and the
// victim's mailbox proof commits while the attacker is at the provider. The
// link attaches nothing and sends the attacker to sign in again.
func (s *OAuthHandlerIntegrationSuite) TestPreHijack_LinkInFlightAcrossTheMailboxProofIsRefused() {
	squatted, attackerSession, victim, proof := s.squattedAccountWithPendingProof("inflight-link@test.com")

	completer := &mockOAuthCompleter{
		user: goth.User{Provider: "github", UserID: "github-attacker-inflight", Email: "attacker@test.com"},
		onExchange: func() {
			s.Equal(true, s.decode(s.verifyLink(victim, proof))["success"], "the mailbox proof commits during the exchange")
		},
	}
	w, req := oauthCallbackRequest("github")
	cookie, _ := s.armLinkIntentOn(req, squatted.ID, "github")
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: attackerSession})

	s.newHandler(completer).OAuthCallbackHTTPHandler(w, req)

	s.Equal(1, completer.calls, "the attacker's session passes the gate and reaches the provider")
	s.assertLinkRefusal(w, oauthLinkErrorReauthRequired)
	s.assertLinkIntentSpent(w, cookie)
	s.Equal([]string{"google"}, s.providersOf(squatted.ID), "only the victim's Google identity is attached")
	_, _, err := s.jwtService().ValidateSession(attackerSession)
	s.Error(err, "the attacker's session is refused on its next request")
}

// A link from a session minted after the account's latest revocation carries
// that revocation's stamp, and links.
func (s *OAuthHandlerIntegrationSuite) TestLinkCallback_SessionMintedAfterARevocationLinks() {
	user := &authm.User{Email: strPtr("link-after-revocation@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)
	s.Require().NoError(auth.RevokeAllSessions(s.deps.DB, user, time.Now()))

	w, req := oauthCallbackRequest("github")
	s.armLinkIntentOn(req, user.ID, "github")
	s.addSessionCookie(req, user)
	s.newHandler(&mockOAuthCompleter{user: goth.User{Provider: "github", UserID: "github-after-revocation"}}).OAuthCallbackHTTPHandler(w, req)

	s.Equal("connected", s.parseLinkRedirect(w.Header().Get("Location")).Get(oauthLinkResultParam))
	s.Equal([]string{"github"}, s.providersOf(user.ID))
}

// calendarMint posts the feed-token mint for the principal the JWT middleware
// loaded from session, recently authenticated, through the real handler and
// service.
func (s *OAuthHandlerIntegrationSuite) calendarMint(principal *authm.User) (*engagementh.CreateCalendarTokenResponse, error) {
	h := engagementh.NewCalendarHandler(engagementsvc.NewCalendarService(s.deps.DB, nil), s.cfg)
	return h.CreateCalendarTokenHandler(testhelpers.CtxWithSession(principal, time.Now()), &engagementh.CreateCalendarTokenRequest{})
}

// The attacker's calendar mint passed the session check (the middleware
// loaded the account) before the victim's mailbox proof committed. The mint
// writes no token and answers the 401 the middleware gives the session next.
func (s *OAuthHandlerIntegrationSuite) TestPreHijack_CalendarMintInFlightAcrossTheMailboxProofIsRefused() {
	squatted, attackerSession, victim, proof := s.squattedAccountWithPendingProof("inflight-feed@test.com")
	principal, _, err := s.jwtService().ValidateSession(attackerSession)
	s.Require().NoError(err, "the attacker's request passes the session check")

	s.Equal(true, s.decode(s.verifyLink(victim, proof))["success"])
	resp, err := s.calendarMint(principal)

	s.Nil(resp)
	var refusal *middleware.SessionRefusedError
	s.Require().True(errors.As(err, &refusal), "expected the session refusal, got %v", err)
	s.Equal(http.StatusUnauthorized, refusal.GetStatus())
	s.Equal(autherrors.CodeTokenInvalid, refusal.ErrorCode)
	s.Zero(s.calendarTokenRows(squatted.ID), "no feed token outlives the proof")
}

// A mint from a session minted after the proof mints.
func (s *OAuthHandlerIntegrationSuite) TestCalendarMint_SessionMintedAfterTheProofMints() {
	squatted, _, victim, proof := s.squattedAccountWithPendingProof("after-proof-feed@test.com")
	verified := s.verifyLink(victim, proof)
	s.Equal(true, s.decode(verified)["success"])
	principal, _, err := s.jwtService().ValidateSession(s.sessionFrom(verified))
	s.Require().NoError(err)
	s.Require().NotNil(principal.SessionsValidAfter)

	resp, err := s.calendarMint(principal)

	s.Require().NoError(err)
	s.NotEmpty(resp.Body.Token)
	s.Equal(int64(1), s.calendarTokenRows(squatted.ID))
}
