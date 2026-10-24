package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/markbates/goth"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// These run the "Create your account" step over the real auth and user
// services and the suite's database: the callback holds a new identity, and
// the step's two routes, mounted on a huma API so the cookie parameters bind
// as they do in production, read and finish it.

func (s *OAuthHandlerIntegrationSuite) stepIdentity(tag string) goth.User {
	return goth.User{
		Provider:  "google",
		UserID:    "google-step-" + tag,
		Email:     "step-" + tag + "@test.com",
		Name:      "Alice Moreno",
		FirstName: "Alice",
		LastName:  "Moreno",
		AvatarURL: "https://example.com/avatar-" + tag + ".png",
		RawData:   map[string]any{"verified_email": true},
	}
}

// startStep runs the sign-in callback for a new identity and returns the
// pending sign-up token the browser was handed.
func (s *OAuthHandlerIntegrationSuite) startStep(identity goth.User, cookies ...*http.Cookie) string {
	s.T().Helper()
	w, req := oauthCallbackRequest("google")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	s.newHandler(&mockOAuthCompleter{user: identity}).OAuthCallbackHTTPHandler(w, req)
	s.Require().Equal("http://localhost:3000"+oauthSignupStepPath, w.Header().Get("Location"))
	pending := findCookie(w.Result(), oauthPendingSignupCookie.name)
	s.Require().NotNil(pending, "expected the pending sign-up cookie")
	return pending.Value
}

func (s *OAuthHandlerIntegrationSuite) stepAPI() humatest.TestAPI {
	_, api := humatest.New(s.T(), huma.DefaultConfig("signup-step", "1.0.0"))
	h := NewOAuthSignupHandler(auth.NewAuthService(s.deps.DB, s.cfg, s.deps.UserService), s.cfg)
	huma.Get(api, OAuthPendingSignupRoute, h.PendingSignupHandler)
	huma.Post(api, OAuthFinishSignupRoute, h.FinishSignupHandler)
	return api
}

func pendingCookieHeader(token string) string {
	return "Cookie: " + oauthPendingSignupCookie.name + "=" + token
}

func validStepBody(dateOfBirth string) map[string]any {
	return map[string]any{
		"terms_accepted":  true,
		"terms_version":   legal.CurrentTermsVersion,
		"privacy_version": legal.CurrentPrivacyVersion,
		"date_of_birth":   dateOfBirth,
	}
}

// dateOfBirthAged is the date of birth of someone who turns years old today,
// in UTC.
func dateOfBirthAged(years int) string {
	now := time.Now().UTC()
	return time.Date(now.Year()-years, now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(legal.DateOfBirthLayout)
}

func (s *OAuthHandlerIntegrationSuite) decode(resp *httptest.ResponseRecorder) map[string]any {
	s.T().Helper()
	s.Require().Equal(http.StatusOK, resp.Code, resp.Body.String())
	var body map[string]any
	s.Require().NoError(json.Unmarshal(resp.Body.Bytes(), &body))
	return body
}

func (s *OAuthHandlerIntegrationSuite) requireRefusal(resp *httptest.ResponseRecorder, code string) map[string]any {
	s.T().Helper()
	body := s.decode(resp)
	s.Equal(false, body["success"], resp.Body.String())
	s.Equal(code, body["error_code"], resp.Body.String())
	return body
}

func (s *OAuthHandlerIntegrationSuite) pendingRows(subject string) int64 {
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthPendingSignup{}).Where("provider_user_id = ?", subject).Count(&rows).Error)
	return rows
}

func (s *OAuthHandlerIntegrationSuite) accountsFor(email string) int64 {
	var users int64
	s.Require().NoError(s.deps.DB.Model(&authm.User{}).Where("email = ?", email).Count(&users).Error)
	return users
}

func setCookies(resp *httptest.ResponseRecorder) map[string]*http.Cookie {
	out := map[string]*http.Cookie{}
	for _, c := range (&http.Response{Header: resp.Header()}).Cookies() {
		out[c.Name] = c
	}
	return out
}

// --- GET /auth/oauth/pending-signup ---

func (s *OAuthHandlerIntegrationSuite) TestPendingSignup_ShowsTheHeldIdentity() {
	identity := s.stepIdentity("read")
	token := s.startStep(identity)

	resp := s.stepAPI().Get(OAuthPendingSignupRoute, pendingCookieHeader(token))

	body := s.decode(resp)
	s.Equal("pending", body["status"])
	s.Equal(identity.Email, body["email"])
	s.Equal(identity.Name, body["name"])
	s.NotEmpty(body["expires_at"])
	s.Equal("/", body["return_to"])
	s.Equal("no-store", resp.Header().Get("Cache-Control"))
	s.Equal(int64(1), s.pendingRows(identity.UserID), "a read consumes nothing")

	var row authm.OAuthPendingSignup
	s.Require().NoError(s.deps.DB.Where("provider_user_id = ?", identity.UserID).First(&row).Error)
	id := strings.Split(token, ".")[0]
	s.NotEqual(id, row.TokenHash, "the table holds a hash of the id, never the id")
	s.Len(row.TokenHash, 64)
}

// Starting a sign-up deletes every expired one, so held identities do not
// outlive their step.
func (s *OAuthHandlerIntegrationSuite) TestPendingSignup_StartSweepsExpiredRows() {
	stale := s.stepIdentity("sweep-stale")
	s.startStep(stale)
	s.Require().NoError(s.deps.DB.Exec(
		"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE provider_user_id = ?", stale.UserID).Error)

	s.startStep(s.stepIdentity("sweep-fresh"))

	s.Zero(s.pendingRows(stale.UserID))
}

func (s *OAuthHandlerIntegrationSuite) TestPendingSignup_ExpiredForAnythingButALiveToken() {
	identity := s.stepIdentity("read-expired")
	token := s.startStep(identity)
	tampered := token[:len(token)-2] + "xx"
	otherBrowsers := strings.Repeat("ab", 32) + "." + strings.Split(token, ".")[1]

	api := s.stepAPI()
	for name, header := range map[string][]any{
		"no cookie":                 nil,
		"tampered signature":        {pendingCookieHeader(tampered)},
		"id with another signature": {pendingCookieHeader(otherBrowsers)},
		"garbage":                   {pendingCookieHeader("not-a-token")},
	} {
		s.Run(name, func() {
			body := s.decode(api.Get(OAuthPendingSignupRoute, header...))
			s.Equal("expired", body["status"])
			s.Nil(body["email"], "an expired read names no address")
		})
	}

	s.Require().NoError(s.deps.DB.Exec(
		"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE provider_user_id = ?", identity.UserID).Error)
	s.Equal("expired", s.decode(api.Get(OAuthPendingSignupRoute, pendingCookieHeader(token)))["status"])
}

func (s *OAuthHandlerIntegrationSuite) TestPendingSignup_AgeMarkerWins() {
	token := s.startStep(s.stepIdentity("read-marker"))

	body := s.decode(s.stepAPI().Get(OAuthPendingSignupRoute,
		"Cookie: "+oauthPendingSignupCookie.name+"="+token+"; "+signupAgeRefusalCookieName+"=1"))
	s.Equal("age_refused", body["status"])
}

// --- POST /auth/oauth/finish-signup ---

func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_CreatesTheAccountAndSignsIn() {
	identity := s.stepIdentity("finish")
	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows?city=phoenix")})
	s.newHandler(&mockOAuthCompleter{user: identity}).OAuthCallbackHTTPHandler(w, req)
	token := findCookie(w.Result(), oauthPendingSignupCookie.name).Value

	dateOfBirth := "1998-04-27"
	body := validStepBody(dateOfBirth)
	body["name"] = "  Alice M.  "
	before := time.Now().UTC()
	resp := s.stepAPI().Post(OAuthFinishSignupRoute, pendingCookieHeader(token), body)

	decoded := s.decode(resp)
	s.Equal(true, decoded["success"], resp.Body.String())
	s.Equal("/shows?city=phoenix", decoded["return_to"])
	s.NotEmpty(decoded["token"])
	cookies := setCookies(resp)
	s.Require().Contains(cookies, "auth_token")
	s.NotEmpty(cookies["auth_token"].Value)
	s.WithinDuration(time.Now().Add(oauthSessionLifetime), cookies["auth_token"].Expires, time.Minute,
		"the session lives as long as a callback sign-in's")
	s.Require().Contains(cookies, oauthPendingSignupCookie.name)
	s.Less(cookies[oauthPendingSignupCookie.name].MaxAge, 0, "the pending cookie is cleared")

	var user authm.User
	s.Require().NoError(s.deps.DB.Where("email = ?", identity.Email).First(&user).Error)
	s.Require().NotNil(user.TermsVersion)
	s.Equal(legal.CurrentTermsVersion, *user.TermsVersion)
	s.Equal(legal.CurrentPrivacyVersion, *user.PrivacyVersion)
	s.NotNil(user.TermsAcceptedAt)
	s.Require().NotNil(user.AgeConfirmedAt)
	s.False(user.AgeConfirmedAt.Before(before.Add(-time.Second)))
	s.Require().NotNil(user.MinAgeAttested)
	s.Equal(legal.AgeOn(time.Date(1998, 4, 27, 0, 0, 0, 0, time.UTC), time.Now()), *user.MinAgeAttested)
	s.Require().NotNil(user.DateOfBirth)
	s.Equal(dateOfBirth, user.DateOfBirth.UTC().Format(legal.DateOfBirthLayout))
	s.Require().NotNil(user.AgeAttestedAt)
	s.True(user.AgeAttestedAt.Equal(*user.AgeConfirmedAt))
	s.Require().NotNil(user.DisplayName)
	s.Equal("Alice M.", *user.DisplayName)
	s.Equal("Alice", *user.FirstName, "the provider's names are kept beside a chosen display name")
	s.True(user.EmailVerified, "the provider vouched for the address")

	var link authm.OAuthAccount
	s.Require().NoError(s.deps.DB.Where("provider = ? AND provider_user_id = ?", "google", identity.UserID).First(&link).Error)
	s.Equal(user.ID, link.UserID)
	s.Nil(link.AccessToken, "no provider credential is stored")
	s.Zero(s.pendingRows(identity.UserID), "the pending sign-up is used up")

	// The date of birth is never serialized.
	s.NotContains(resp.Body.String(), dateOfBirth)
}

// Submitting again with the same cookie is a replay of a used step.
func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_IsSingleUse() {
	identity := s.stepIdentity("replay")
	token := s.startStep(identity)
	api := s.stepAPI()

	s.Equal(true, s.decode(api.Post(OAuthFinishSignupRoute, pendingCookieHeader(token), validStepBody("1990-01-01")))["success"])
	s.requireRefusal(api.Post(OAuthFinishSignupRoute, pendingCookieHeader(token), validStepBody("1990-01-01")),
		autherrors.CodeOAuthSignupExpired)
	s.Equal(int64(1), s.accountsFor(identity.Email))
}

func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_RefusesWhatIsNotALiveStep() {
	identity := s.stepIdentity("finish-expired")
	token := s.startStep(identity)
	api := s.stepAPI()

	for name, header := range map[string][]any{
		"no cookie":          nil,
		"tampered signature": {pendingCookieHeader(token[:len(token)-2] + "xx")},
		"another browser's":  {pendingCookieHeader(strings.Repeat("cd", 32) + "." + strings.Split(token, ".")[1])},
	} {
		s.Run(name, func() {
			s.requireRefusal(api.Post(OAuthFinishSignupRoute, append(header, validStepBody("1990-01-01"))...),
				autherrors.CodeOAuthSignupExpired)
		})
	}
	s.Equal(int64(1), s.pendingRows(identity.UserID), "a refused request leaves the real step alone")

	s.Require().NoError(s.deps.DB.Exec(
		"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE provider_user_id = ?", identity.UserID).Error)
	s.requireRefusal(api.Post(OAuthFinishSignupRoute, pendingCookieHeader(token), validStepBody("1990-01-01")),
		autherrors.CodeOAuthSignupExpired)
	s.Zero(s.accountsFor(identity.Email))
}

// The terms box and the versions are judged by the rule every sign-up path
// shares; a refusal leaves the step usable.
func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_ConsentIsExactMatch() {
	identity := s.stepIdentity("consent")
	token := s.startStep(identity)
	api := s.stepAPI()

	unticked := validStepBody("1990-01-01")
	unticked["terms_accepted"] = false
	s.requireRefusal(api.Post(OAuthFinishSignupRoute, pendingCookieHeader(token), unticked), autherrors.CodeTermsAcceptanceRequired)

	for name, pair := range consentVersionRefusals() {
		s.Run(name, func() {
			body := validStepBody("1990-01-01")
			delete(body, "terms_version")
			delete(body, "privacy_version")
			s.requireRefusal(api.Post(OAuthFinishSignupRoute, pendingCookieHeader(token), withVersions(body, pair)),
				autherrors.CodeTermsAcceptanceRequired)
		})
	}
	s.Equal(int64(1), s.pendingRows(identity.UserID))
	s.Zero(s.accountsFor(identity.Email))
}

// Every pair legal.AcceptedVersions takes now, the grace set included when a
// window is open, finishes the step and is recorded as sent.
func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_AcceptedVersionsAreRecordedAsSent() {
	api := s.stepAPI()
	i := 0
	for name, pair := range acceptedVersionPairs() {
		i++
		s.Run(name, func() {
			identity := s.stepIdentity("versions-" + strings.ReplaceAll(name, " ", "-"))
			token := s.startStep(identity)
			body := validStepBody("1990-01-01")
			s.Equal(true, s.decode(api.Post(OAuthFinishSignupRoute, pendingCookieHeader(token), withVersions(body, pair)))["success"])

			var user authm.User
			s.Require().NoError(s.deps.DB.Where("email = ?", identity.Email).First(&user).Error)
			s.Equal(*pair[0], *user.TermsVersion)
			s.Equal(*pair[1], *user.PrivacyVersion)
		})
	}
	s.Positive(i)
}

func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_UnderSixteenIsRefusedAndUsesTheStepUp() {
	identity := s.stepIdentity("under-16")
	token := s.startStep(identity)
	api := s.stepAPI()

	resp := api.Post(OAuthFinishSignupRoute, pendingCookieHeader(token), validStepBody(dateOfBirthAged(legal.MinSignupAge-1)))

	s.requireRefusal(resp, autherrors.CodeAgeConfirmationRequired)
	cookies := setCookies(resp)
	s.Require().Contains(cookies, signupAgeRefusalCookieName)
	s.Equal(int(SignupAgeRefusalMarkerLifetime/time.Second), cookies[signupAgeRefusalCookieName].MaxAge)
	s.True(cookies[signupAgeRefusalCookieName].HttpOnly)
	s.Zero(s.pendingRows(identity.UserID), "the refusal is the end of this sign-up")
	s.Zero(s.accountsFor(identity.Email))

	// A different date from the same browser is refused without a form.
	s.requireRefusal(api.Post(OAuthFinishSignupRoute,
		pendingCookieHeader(token)+"; "+signupAgeRefusalCookieName+"=1", validStepBody("1990-01-01")),
		autherrors.CodeAgeConfirmationRequired)
	s.Zero(s.accountsFor(identity.Email))
}

func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_SixteenOnTheBirthdayIsAccepted() {
	identity := s.stepIdentity("sixteen")
	token := s.startStep(identity)

	body := s.decode(s.stepAPI().Post(OAuthFinishSignupRoute, pendingCookieHeader(token), validStepBody(dateOfBirthAged(legal.MinSignupAge))))

	s.Equal(true, body["success"])
	var user authm.User
	s.Require().NoError(s.deps.DB.Where("email = ?", identity.Email).First(&user).Error)
	s.Equal(legal.MinSignupAge, *user.MinAgeAttested)
}

// A field the person can fix is refused with the step left in place.
func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_FieldRefusalsKeepTheStep() {
	identity := s.stepIdentity("fields")
	token := s.startStep(identity)
	api := s.stepAPI()

	for name, tc := range map[string]struct {
		mutate  func(map[string]any)
		message string
	}{
		"date not a date":    {func(b map[string]any) { b["date_of_birth"] = "1990-02-30" }, dateOfBirthInvalidMessage},
		"date in the future": {func(b map[string]any) { b["date_of_birth"] = "2999-01-01" }, dateOfBirthInvalidMessage},
		"date too old":       {func(b map[string]any) { b["date_of_birth"] = "1800-01-01" }, dateOfBirthInvalidMessage},
		"date missing":       {func(b map[string]any) { delete(b, "date_of_birth") }, dateOfBirthMissingMessage},
		"name with control":  {func(b map[string]any) { b["name"] = "Alice\x07" }, ""},
		"name too long":      {func(b map[string]any) { b["name"] = strings.Repeat("a", maxProfileFieldRunes+1) }, ""},
	} {
		s.Run(name, func() {
			body := validStepBody("1990-01-01")
			tc.mutate(body)
			refusal := s.requireRefusal(api.Post(OAuthFinishSignupRoute, pendingCookieHeader(token), body), autherrors.CodeValidationFailed)
			if tc.message != "" {
				s.Equal(tc.message, refusal["message"])
			}
		})
	}
	s.Equal(int64(1), s.pendingRows(identity.UserID))
	s.Zero(s.accountsFor(identity.Email))
}

// A cleared name stores no name at all, not the provider's.
func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_ClearedNameStoresNoName() {
	identity := s.stepIdentity("no-name")
	token := s.startStep(identity)
	body := validStepBody("1990-01-01")
	body["name"] = "   "

	s.Equal(true, s.decode(s.stepAPI().Post(OAuthFinishSignupRoute, pendingCookieHeader(token), body))["success"])

	var user authm.User
	s.Require().NoError(s.deps.DB.Where("email = ?", identity.Email).First(&user).Error)
	s.Nil(user.DisplayName)
	s.Empty(*user.FirstName)
	s.Empty(*user.LastName)
}

// An account that took the address between the callback and the finish is
// refused the same way a callback would refuse it.
func (s *OAuthHandlerIntegrationSuite) TestFinishSignup_AddressTakenMeanwhileIsRefused() {
	identity := s.stepIdentity("taken")
	token := s.startStep(identity)
	s.Require().NoError(s.deps.DB.Create(&authm.User{Email: strPtr(identity.Email), IsActive: true, EmailVerified: true}).Error)

	s.requireRefusal(s.stepAPI().Post(OAuthFinishSignupRoute, pendingCookieHeader(token), validStepBody("1990-01-01")),
		autherrors.CodeOAuthLinkRefused)
	var links int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).Where("provider_user_id = ?", identity.UserID).Count(&links).Error)
	s.Zero(links)
}

// The returning identity never sees the step, and a second callback for a
// held identity mints a second pending sign-up rather than an account.
func (s *OAuthHandlerIntegrationSuite) TestCallback_ReturningIdentityAfterTheStepSignsIn() {
	identity := s.stepIdentity("returning")
	token := s.startStep(identity)
	s.Equal(true, s.decode(s.stepAPI().Post(OAuthFinishSignupRoute, pendingCookieHeader(token), validStepBody("1990-01-01")))["success"])

	w, req := oauthCallbackRequest("google")
	s.newHandler(&mockOAuthCompleter{user: identity}).OAuthCallbackHTTPHandler(w, req)

	s.assertSignedIn(w)
	s.Nil(findCookie(w.Result(), oauthPendingSignupCookie.name))
}

// The read hands the step the destination the sign-in named, so "Use a
// different account" can carry it into the next start.
func (s *OAuthHandlerIntegrationSuite) TestPendingSignup_CarriesTheDestination() {
	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: oauthReturnToCookieName, Value: encodedReturnTo("/shows?city=phoenix")})
	s.newHandler(&mockOAuthCompleter{user: s.stepIdentity("read-return-to")}).OAuthCallbackHTTPHandler(w, req)
	token := findCookie(w.Result(), oauthPendingSignupCookie.name).Value

	body := s.decode(s.stepAPI().Get(OAuthPendingSignupRoute, pendingCookieHeader(token)))
	s.Equal("/shows?city=phoenix", body["return_to"])
}

// The pending cookie lives as long as the pending sign-up it names, inside the
// handshake cookies' bound.
func TestOAuthPendingSignupCookie_LivesAsLongAsThePendingSignup(t *testing.T) {
	if oauthPendingSignupCookie.lifetime != contracts.OAuthPendingSignupTTL {
		t.Errorf("cookie lives %v, pending sign-up %v", oauthPendingSignupCookie.lifetime, contracts.OAuthPendingSignupTTL)
	}
	if oauthPendingSignupCookie.lifetime <= 0 || oauthPendingSignupCookie.lifetime > oauthHandshakeCookieMaxLifetime {
		t.Errorf("cookie lives %v, want (0, %v]", oauthPendingSignupCookie.lifetime, oauthHandshakeCookieMaxLifetime)
	}
}

// The sign-up forms' date-of-birth field shows the same sentences the
// sign-up endpoints refuse with, so a date the field lets through and an
// endpoint refuses reads the same in the banner as under the fields.
func TestDateOfBirthMessagesMatchTheField(t *testing.T) {
	const stepPath = "../../../../../frontend/app/auth/_components/date-of-birth-field.tsx"
	source, err := os.ReadFile(stepPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v; repoint this test if the field moved", stepPath, err)
	}
	for name, backend := range map[string]string{
		"DATE_OF_BIRTH_MISSING_MESSAGE": dateOfBirthMissingMessage,
		"DATE_OF_BIRTH_INVALID_MESSAGE": dateOfBirthInvalidMessage,
	} {
		match := regexp.MustCompile(`(?m)^export const ` + name + `\s*=\s*'([^']*)'`).FindSubmatch(source)
		if match == nil {
			t.Errorf("no %s string constant in %s", name, stepPath)
			continue
		}
		if frontend := string(match[1]); frontend != backend {
			t.Errorf("%s differs: frontend %q, backend %q", name, frontend, backend)
		}
	}
}

// The expired step's copy names the pending sign-up's lifetime, so changing
// the lifetime fails here until the copy (this message and ExpiredCard in
// frontend/app/auth/complete/create-account-step.tsx) changes with it.
func TestOAuthSignupExpiredCopyNamesTheLifetime(t *testing.T) {
	minutes := fmt.Sprintf("%d minutes", int(contracts.OAuthPendingSignupTTL/time.Minute))
	if got := autherrors.ErrOAuthSignupExpired(nil).UserMessage(); !strings.Contains(got, minutes) {
		t.Errorf("message %q does not say %q", got, minutes)
	}
}

// huma binds no parameter declared on an embedded struct, so the step's two
// request structs each declare the cookie fields. This holds the copies
// together.
func TestSignupStepRequestsDeclareTheirCookies(t *testing.T) {
	cookieTags := func(v any) []string {
		var tags []string
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumField(); i++ {
			if tag := typ.Field(i).Tag.Get("cookie"); tag != "" {
				tags = append(tags, tag)
			}
		}
		return tags
	}
	want := []string{
		oauthPendingSignupCookie.setName(false),
		oauthPendingSignupCookie.setName(true),
		signupAgeRefusalCookieNameFor(false),
		signupAgeRefusalCookieNameFor(true),
	}
	if got := cookieTags(PendingOAuthSignupRequest{}); !reflect.DeepEqual(got, want) {
		t.Errorf("PendingOAuthSignupRequest cookies = %v, want %v", got, want)
	}
	if got := cookieTags(FinishOAuthSignupRequest{}); !reflect.DeepEqual(got, want) {
		t.Errorf("FinishOAuthSignupRequest cookies = %v, want %v", got, want)
	}
}

// With Secure cookies, only the "__Host-" names count: a bare one could have
// been planted by a sibling subdomain.
func TestSignupStepCookies_SecureReadsOnlyThePrefixedNames(t *testing.T) {
	c := signupStepCookies{pending: "bare", pendingHost: "host", ageRefused: signupAgeRefusalCookies{plain: "1"}}
	if got := c.pendingToken(true); got != "host" {
		t.Errorf("secure pending token = %q, want the prefixed one", got)
	}
	if c.refusedForAge(true) {
		t.Error("a bare marker counted on a Secure deployment")
	}
	if got := c.pendingToken(false); got != "bare" {
		t.Errorf("insecure pending token = %q, want the bare one", got)
	}
	if !c.refusedForAge(false) {
		t.Error("the bare marker did not count without Secure cookies")
	}
}
