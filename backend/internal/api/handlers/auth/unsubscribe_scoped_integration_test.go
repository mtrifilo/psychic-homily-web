package auth

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
)

// The show-alert unsubscribe endpoint against the REAL user service and a real
// database. The sibling file drives the handler over a mock, which covers the
// transport rules but not whether the stream stops: that is decided by what the
// setter does to the rows the notifiers read.
//
// The two layers checked here are the ones the show-alert notifiers read: the
// account matrix's shows.email (the whole gate for scene emails) and an artist
// follow's explicit per-follow email override (which sits below it).

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

func (s *ScopedUnsubscribeIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

// optedInUser creates a user opted into show-alert email at both layers: the
// account matrix, and an explicit override on one artist follow.
func (s *ScopedUnsubscribeIntegrationSuite) optedInUser(email string) (userID, artistID uint) {
	user := &authm.User{Email: &email, IsActive: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)

	slug := "unsub-" + strings.SplitN(email, "@", 2)[0]
	artist := catalogm.Artist{Name: slug, Slug: &slug}
	s.Require().NoError(s.deps.DB.Create(&artist).Error)

	on := true
	s.Require().NoError(s.deps.UserService.SetAccountAlertDefaults(user.ID,
		authm.AccountAlertDefaultsUpdate{
			Shows: &authm.AlertChannelDefaultsUpdate{Email: &on},
		}))

	follows := engagement.NewFollowService(s.deps.DB)
	s.Require().NoError(follows.Follow(user.ID, "artist", artist.ID))
	_, err := follows.SetFollowAlertSettings(user.ID, "artist", artist.ID,
		contracts.FollowAlertUpdate{
			Shows: &contracts.FollowAlertPreferenceUpdate{Email: &on},
		})
	s.Require().NoError(err)

	return user.ID, artist.ID
}

// showAlertEmail reports the two layers' email state: the account channel, and
// the artist follow's resolved channel (override over account).
func (s *ScopedUnsubscribeIntegrationSuite) showAlertEmail(userID, artistID uint) (account, artistFollow bool) {
	prefs, err := s.deps.UserService.GetAlertPreferences(userID)
	s.Require().NoError(err)
	resolved, err := engagement.NewFollowService(s.deps.DB).
		GetFollowAlertSettings(userID, "artist", artistID)
	s.Require().NoError(err)
	return prefs.AlertDefaults.Shows.Email, resolved.Shows.Email
}

func (s *ScopedUnsubscribeIntegrationSuite) requireOptedIn(userID, artistID uint) {
	account, artistFollow := s.showAlertEmail(userID, artistID)
	s.Require().True(account && artistFollow, "fixture must start opted in at both layers")
}

// The RFC 8058 POST a mailbox provider's native button sends must silence both
// layers and leave in-app delivery alone.
func (s *ScopedUnsubscribeIntegrationSuite) TestOneClickPostSilencesBothLayers() {
	userID, artistID := s.optedInUser("oneclick@example.com")
	s.requireOptedIn(userID, artistID)

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	w := httptest.NewRecorder()
	h.UnsubscribeArtistShowAlertsPageHandler(w, s.oneClickPost(userID))

	s.Equal(http.StatusOK, w.Code, "body=%s", w.Body.String())
	s.Contains(w.Body.String(), `"unsubscribed":true`)

	account, artistFollow := s.showAlertEmail(userID, artistID)
	s.False(account, "the account layer must go quiet")
	s.False(artistFollow, "the per-follow override must go quiet too")

	prefs, err := s.deps.UserService.GetAlertPreferences(userID)
	s.Require().NoError(err)
	s.True(prefs.AlertDefaults.Shows.InApp, "an email opt-out leaves in-app alerts on")
}

// Mailbox providers retry, and a recipient can click twice. A repeat POST
// succeeds and changes nothing further.
func (s *ScopedUnsubscribeIntegrationSuite) TestOneClickPostIsIdempotent() {
	userID, artistID := s.optedInUser("twice@example.com")

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		h.UnsubscribeArtistShowAlertsPageHandler(w, s.oneClickPost(userID))
		s.Equal(http.StatusOK, w.Code, "attempt %d body=%s", i+1, w.Body.String())
	}

	account, artistFollow := s.showAlertEmail(userID, artistID)
	s.False(account)
	s.False(artistFollow)
}

// The human path: the emailed body link opens the confirm page, whose form POSTs
// back with the browser marker. That POST must unsubscribe and answer with the
// HTML page, not re-render the prompt.
func (s *ScopedUnsubscribeIntegrationSuite) TestBrowserConfirmPostSilencesBothLayers() {
	userID, artistID := s.optedInUser("browser@example.com")
	s.requireOptedIn(userID, artistID)

	req := httptest.NewRequest(http.MethodPost, s.signedTarget(userID),
		strings.NewReader(unsubscribeBrowserConfirmField+"=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	w := httptest.NewRecorder()
	h.UnsubscribeArtistShowAlertsPageHandler(w, req)

	s.Equal(http.StatusOK, w.Code, "body=%s", w.Body.String())
	s.Contains(w.Body.String(), "been unsubscribed from")
	s.NotContains(w.Body.String(), `method="POST"`, "the confirm prompt must not come back")

	account, artistFollow := s.showAlertEmail(userID, artistID)
	s.False(account)
	s.False(artistFollow)
}

// Behind the API-wide security headers, as served in production, the confirm
// page's CSP must let a browser submit its form back to this origin. The
// API-wide form-action 'none' would silently block the click.
func (s *ScopedUnsubscribeIntegrationSuite) TestConfirmPageCSPAllowsItsFormBehindSecurityHeaders() {
	userID, _ := s.optedInUser("csp@example.com")

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	served := middleware.SecurityHeaders(http.HandlerFunc(h.UnsubscribeArtistShowAlertsPageHandler))
	w := httptest.NewRecorder()
	served.ServeHTTP(w, httptest.NewRequest(http.MethodGet, s.signedTarget(userID), nil))

	s.Equal(http.StatusOK, w.Code)
	s.Require().Contains(w.Body.String(), `method="POST"`)
	csp := w.Header().Get("Content-Security-Policy")
	s.Contains(csp, "form-action 'self'")
	s.NotContains(csp, "form-action 'none'")
	s.Contains(csp, "frame-ancestors 'none'")
}

// A GET must not mutate: mail scanners and link-preview bots fetch links with no
// human involved.
func (s *ScopedUnsubscribeIntegrationSuite) TestGetConfirmsWithoutMutating() {
	userID, artistID := s.optedInUser("getonly@example.com")

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	w := httptest.NewRecorder()
	h.UnsubscribeArtistShowAlertsPageHandler(w, httptest.NewRequest(
		http.MethodGet, s.signedTarget(userID), nil))

	s.Equal(http.StatusOK, w.Code)
	s.Contains(w.Body.String(), `method="POST"`, "the page must post back to finish the job")
	s.requireOptedIn(userID, artistID)
}

// A signature minted for another category (the weekly scene digest) must not
// silence show alerts.
func (s *ScopedUnsubscribeIntegrationSuite) TestSignatureFromAnotherScopeIsRejected() {
	userID, artistID := s.optedInUser("wrongscope@example.com")

	wrongSig := engagement.ComputeScopedUnsubscribeSignature(
		userID, engagement.UnsubscribeScopeSceneDigest, s.secret)

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	w := httptest.NewRecorder()
	h.UnsubscribeArtistShowAlertsPageHandler(w, s.postWithSignature(userID, wrongSig))

	s.Equal(http.StatusForbidden, w.Code)
	s.requireOptedIn(userID, artistID)
}

// The signature binds the user, so one recipient's link cannot unsubscribe
// another account.
func (s *ScopedUnsubscribeIntegrationSuite) TestSignatureForAnotherUserIsRejected() {
	victim, victimArtist := s.optedInUser("victim@example.com")
	attacker, _ := s.optedInUser("attacker@example.com")

	attackerSig := engagement.ComputeScopedUnsubscribeSignature(
		attacker, engagement.UnsubscribeScopeArtistShowAlerts, s.secret)

	h := NewUserPreferencesHandler(s.deps.UserService, s.secret)
	w := httptest.NewRecorder()
	h.UnsubscribeArtistShowAlertsPageHandler(w, s.postWithSignature(victim, attackerSig))

	s.Equal(http.StatusForbidden, w.Code)
	s.requireOptedIn(victim, victimArtist)
}

// signedTarget is the path and query the email carries, built by the same
// function the senders use.
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
		"/unsubscribe/"+engagement.UnsubscribeScopeArtistShowAlerts+
			"?uid="+strconv.FormatUint(uint64(userID), 10)+"&sig="+sig,
		strings.NewReader("List-Unsubscribe=One-Click"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}
