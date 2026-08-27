package auth

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
)

// The scoped unsubscribe endpoint against the REAL user service and a real
// database (PSY-1926).
//
// Its sibling file covers the handler over a mock, which is the right shape for
// the transport rules (parameter parsing, signature verification, the response
// bodies). It cannot cover the thing an unsubscribe is judged on, which is
// whether the stream actually stops: the mock records that the setter was
// called and asserts nothing about what the setter did to the rows the notifier
// reads.
//
// That gap is what made this ticket necessary. The scene email advertised
// one-click unsubscribe over a frontend page, so nothing in the chain was ever
// exercised end to end and no test noticed that the button could not work.

type ScopedUnsubscribeIntegrationSuite struct {
	suite.Suite
	deps   *testhelpers.IntegrationDeps
	secret string
}

func TestScopedUnsubscribeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ScopedUnsubscribeIntegrationSuite))
}

func (s *ScopedUnsubscribeIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.secret = "test-secret-key-at-least-32-characters-long"
}

// optedInUser creates a user who has opted into show-alert emails at BOTH
// layers, on a scene follow. Both, because either one left standing keeps the
// mail flowing and this suite exists to prove neither is.
func (s *ScopedUnsubscribeIntegrationSuite) optedInUser(email string) (uint, uint) {
	user := &authm.User{Email: &email, IsActive: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	// A distinct CITY per test, not just a distinct slug: scenes carry a unique
	// index on (city, state), so reusing Phoenix across cases collides on the
	// second insert.
	city := strings.SplitN(email, "@", 2)[0]
	var sceneID uint
	s.Require().NoError(s.deps.DB.Raw(`
		INSERT INTO scenes (metro, city, state, slug)
		VALUES (NULL, ?, 'AZ', ?)
		RETURNING id`, city, "unsub-"+city).Scan(&sceneID).Error)

	on := true
	s.Require().NoError(s.deps.UserService.SetAccountAlertDefaults(user.ID,
		authm.AccountAlertDefaultsUpdate{
			Shows: &authm.AlertChannelDefaultsUpdate{Email: &on},
		}))

	follows := engagement.NewFollowService(s.deps.DB)
	s.Require().NoError(follows.Follow(user.ID, "scene", sceneID))
	_, err := follows.SetFollowAlertSettings(user.ID, "scene", sceneID,
		contracts.FollowAlertUpdate{
			Shows: &contracts.FollowAlertPreferenceUpdate{Email: &on},
		})
	s.Require().NoError(err)

	return user.ID, sceneID
}

// sceneEmailIsOn reports what the notifier would resolve for this follow, which
// is the only reading of "is this user still being emailed" that counts.
func (s *ScopedUnsubscribeIntegrationSuite) sceneEmailIsOn(userID, sceneID uint) bool {
	resolved, err := engagement.NewFollowService(s.deps.DB).
		GetFollowAlertSettings(userID, "scene", sceneID)
	s.Require().NoError(err)
	return resolved.Shows.Email
}

// The RFC 8058 POST is the path a mailbox provider's native Unsubscribe button
// takes, and it has to silence BOTH layers. The per-follow override is the one
// that would otherwise keep sending: it sits below the account default in the
// inherit chain, so an unsubscribe that only wrote the account row would report
// success and keep mailing this user about this scene.
func (s *ScopedUnsubscribeIntegrationSuite) TestOneClickPostSilencesBothLayers() {
	userID, sceneID := s.optedInUser("oneclick@example.com")
	s.Require().True(s.sceneEmailIsOn(userID, sceneID), "fixture must start opted in")

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	w := httptest.NewRecorder()
	h.UnsubscribeArtistShowAlertsPageHandler(w, s.oneClickPost(userID))

	s.Equal(http.StatusOK, w.Code, "body=%s", w.Body.String())
	s.Contains(w.Body.String(), `"unsubscribed":true`)

	prefs, err := s.deps.UserService.GetAlertPreferences(userID)
	s.Require().NoError(err)
	s.False(prefs.AlertDefaults.Shows.Email, "the account layer must go quiet")
	s.False(s.sceneEmailIsOn(userID, sceneID), "the per-follow override must go quiet too")

	// An email opt-out is not a request to stop being notified in the product.
	s.True(prefs.AlertDefaults.Shows.InApp)
}

// A GET must not mutate. The setter behind this scope rewrites every one of the
// user's artist, venue and scene follows, and corporate mail scanners and
// link-preview bots fetch links with no human involved: a GET that unsubscribed
// would let a scanner silently destroy opt-ins the user chose.
func (s *ScopedUnsubscribeIntegrationSuite) TestGetConfirmsWithoutMutating() {
	userID, sceneID := s.optedInUser("getonly@example.com")

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	w := httptest.NewRecorder()
	h.UnsubscribeArtistShowAlertsPageHandler(w, httptest.NewRequest(
		http.MethodGet, s.signedTarget(userID), nil))

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), `method="POST"`, "the page must post back to finish the job")
	s.True(s.sceneEmailIsOn(userID, sceneID), "a GET must leave the subscription alone")
}

// The scope binds the signature to one notification category. A link minted for
// the weekly scene digest must not silence the per-show alerts: the two are
// different subscriptions, and a token that worked across them would let any
// recipient of one email cancel a stream they never received.
func (s *ScopedUnsubscribeIntegrationSuite) TestSignatureFromAnotherScopeIsRejected() {
	userID, sceneID := s.optedInUser("wrongscope@example.com")

	wrongSig := engagement.ComputeScopedUnsubscribeSignature(
		userID, engagement.UnsubscribeScopeSceneDigest, s.secret)

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	w := httptest.NewRecorder()
	h.UnsubscribeArtistShowAlertsPageHandler(w, s.postWithSignature(userID, wrongSig))

	s.Equal(http.StatusForbidden, w.Code)
	s.True(s.sceneEmailIsOn(userID, sceneID), "a rejected link must change nothing")
}

// The signature is over the USER as well as the scope, so one recipient's link
// cannot unsubscribe another account.
func (s *ScopedUnsubscribeIntegrationSuite) TestSignatureForAnotherUserIsRejected() {
	victim, victimScene := s.optedInUser("victim@example.com")
	attacker, _ := s.optedInUser("attacker@example.com")

	attackerSig := engagement.ComputeScopedUnsubscribeSignature(
		attacker, engagement.UnsubscribeScopeArtistShowAlerts, s.secret)

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	w := httptest.NewRecorder()
	h.UnsubscribeArtistShowAlertsPageHandler(w, s.postWithSignature(victim, attackerSig))

	s.Equal(http.StatusForbidden, w.Code)
	s.True(s.sceneEmailIsOn(victim, victimScene))
}

// signedTarget is the URL the email body carries, built by the same function
// that builds it in the email, so a change to the query-string shape breaks
// here rather than in production.
func (s *ScopedUnsubscribeIntegrationSuite) signedTarget(userID uint) string {
	full := engagement.GenerateScopedUnsubscribeURL(
		"http://api.example.com", userID, engagement.UnsubscribeScopeArtistShowAlerts, s.secret)
	return strings.TrimPrefix(full, "http://api.example.com")
}

func (s *ScopedUnsubscribeIntegrationSuite) oneClickPost(userID uint) *http.Request {
	req := httptest.NewRequest(http.MethodPost, s.signedTarget(userID),
		strings.NewReader("List-Unsubscribe=One-Click"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func (s *ScopedUnsubscribeIntegrationSuite) postWithSignature(userID uint, sig string) *http.Request {
	req := httptest.NewRequest(http.MethodPost,
		"/unsubscribe/artist-show-alerts?uid="+strconv.FormatUint(uint64(userID), 10)+"&sig="+sig,
		strings.NewReader("List-Unsubscribe=One-Click"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}
