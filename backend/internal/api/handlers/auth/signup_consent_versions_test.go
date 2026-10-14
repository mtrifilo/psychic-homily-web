package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// currentConsentVersionsQuery is the version pair the Google start accepts,
// as query parameters.
var currentConsentVersionsQuery = url.Values{
	"terms_version":   {legal.CurrentTermsVersion},
	"privacy_version": {legal.CurrentPrivacyVersion},
}.Encode()

// consentVersionRefusals are the version pairs every sign-up that takes
// consent refuses. A nil pointer leaves the field out of the request entirely.
func consentVersionRefusals() map[string][2]*string {
	terms, privacy := legal.CurrentTermsVersion, legal.CurrentPrivacyVersion
	empty, stale, unknown := "", "2025-01-01", "x"
	return map[string][2]*string{
		"terms stale":     {&stale, &privacy},
		"terms unknown":   {&unknown, &privacy},
		"terms empty":     {&empty, &privacy},
		"terms missing":   {nil, &privacy},
		"privacy stale":   {&terms, &stale},
		"privacy unknown": {&terms, &unknown},
		"privacy empty":   {&terms, &empty},
		"privacy missing": {&terms, nil},
		"both missing":    {nil, nil},
		"swapped":         {&privacy, &terms},
	}
}

// withVersions returns body with terms_version and privacy_version set from
// pair, leaving out a field whose pointer is nil.
func withVersions(body map[string]any, pair [2]*string) map[string]any {
	out := maps.Clone(body)
	if pair[0] != nil {
		out["terms_version"] = *pair[0]
	}
	if pair[1] != nil {
		out["privacy_version"] = *pair[1]
	}
	return out
}

func requireTermsAcceptanceRefusal(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d: %s", resp.Code, resp.Body.String())
	}
	body := decodeWireBody(t, resp.Body.Bytes())
	want := autherrors.ErrTermsAcceptanceRequired("")
	if success, _ := body["success"].(bool); success {
		t.Fatalf("expected a refusal: %s", resp.Body.String())
	}
	if code, _ := body["error_code"].(string); code != want.Code {
		t.Errorf("error_code = %q, want %q", code, want.Code)
	}
	if message, _ := body["message"].(string); message != want.UserMessage() {
		t.Errorf("message = %q, want %q", message, want.UserMessage())
	}
}

// --- POST /auth/register ---

func newRegisterWireAPI(t *testing.T, users *testhelpers.MockUserService) humatest.TestAPI {
	t.Helper()
	api, _ := newWireTestAPI(t)
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = users
		ah.jwtService = &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) { return "tok", nil }}
	})
	huma.Post(api, "/auth/register", h.RegisterHandler)
	return api
}

var registerConsentBody = map[string]any{
	"email":            "new@example.com",
	"password":         "a-valid-password-123",
	"terms_accepted":   true,
	"age_confirmed":    true,
	"min_age_attested": MinSignupAge,
}

func TestRegisterWire_RefusesConsentVersionsThatAreNotCurrent(t *testing.T) {
	for name, pair := range consentVersionRefusals() {
		t.Run(name, func(t *testing.T) {
			var created bool
			users := &testhelpers.MockUserService{
				CreateUserWithPasswordWithLegalFn: func(string, string, string, string, contracts.LegalAcceptance) (*authm.User, error) {
					created = true
					return &authm.User{ID: 1}, nil
				},
			}
			api := newRegisterWireAPI(t, users)

			requireTermsAcceptanceRefusal(t, api.Post("/auth/register", withVersions(registerConsentBody, pair)))
			if created {
				t.Error("expected no account for a refused consent")
			}
		})
	}
}

func TestRegisterWire_RecordsCurrentConsentVersions(t *testing.T) {
	var recorded contracts.LegalAcceptance
	users := &testhelpers.MockUserService{
		CreateUserWithPasswordWithLegalFn: func(email, _, _, _ string, acceptance contracts.LegalAcceptance) (*authm.User, error) {
			recorded = acceptance
			return &authm.User{ID: 1, Email: &email}, nil
		},
	}
	api := newRegisterWireAPI(t, users)

	current := legal.CurrentTermsVersion
	privacy := legal.CurrentPrivacyVersion
	requireWireSuccess(t, api.Post("/auth/register", withVersions(registerConsentBody, [2]*string{&current, &privacy})))
	if recorded.TermsVersion != legal.CurrentTermsVersion || recorded.PrivacyVersion != legal.CurrentPrivacyVersion {
		t.Errorf("recorded versions %q/%q, want the current ones", recorded.TermsVersion, recorded.PrivacyVersion)
	}
}

// --- POST /auth/passkey/signup/begin ---

var passkeySignupBeginBody = map[string]any{
	"email":            "new@example.com",
	"terms_accepted":   true,
	"age_confirmed":    true,
	"min_age_attested": MinSignupAge,
}

func TestPasskeySignupBeginWire_RefusesConsentVersionsThatAreNotCurrent(t *testing.T) {
	for name, pair := range consentVersionRefusals() {
		t.Run(name, func(t *testing.T) {
			var began bool
			mockWA := &testhelpers.MockWebAuthnService{
				BeginRegistrationForEmailFn: func(string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
					began = true
					return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
				},
			}
			users := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil }}
			api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, users))

			requireTermsAcceptanceRefusal(t, api.Post("/auth/passkey/signup/begin", withVersions(passkeySignupBeginBody, pair)))
			if began {
				t.Error("expected no ceremony for a refused consent")
			}
		})
	}
}

// signup/begin stores the consent it validated on the challenge, which is
// where signup/finish reads it from.
func TestPasskeySignupBeginWire_StoresValidatedConsentOnChallenge(t *testing.T) {
	var stored contracts.SignupChallenge
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationForEmailFn: func(string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return wireWebAuthn(t).BeginRegistration(&authm.User{ID: 5})
		},
		StoreSignupChallengeFn: func(signup contracts.SignupChallenge, _ *webauthn.SessionData, _ string) (string, error) {
			stored = signup
			return "c-1", nil
		},
	}
	users := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil }}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, users))

	terms, privacy := legal.CurrentTermsVersion, legal.CurrentPrivacyVersion
	requireWireSuccess(t, api.Post("/auth/passkey/signup/begin", withVersions(passkeySignupBeginBody, [2]*string{&terms, &privacy})))

	if stored.Email != "new@example.com" {
		t.Errorf("stored email %q", stored.Email)
	}
	got := stored.Acceptance
	if got.TermsVersion != terms || got.PrivacyVersion != privacy || got.MinAgeAttested != MinSignupAge {
		t.Errorf("stored acceptance %+v, want the current versions and age %d", got, MinSignupAge)
	}
	if got.TermsAcceptedAt.IsZero() || got.AgeConfirmedAt.IsZero() {
		t.Errorf("stored acceptance %+v carries no acceptance times", got)
	}
}

// --- POST /auth/passkey/signup/finish ---

// passkeySignupFinishAPI serves signup/finish over a challenge carrying
// signup, and writes the acceptance the account is created with to recorded.
// A zero recorded after a request means no account was created.
func passkeySignupFinishAPI(t *testing.T, signup contracts.SignupChallenge, recorded *contracts.LegalAcceptance) humatest.TestAPI {
	t.Helper()
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeSignupChallengeFn: func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
			return &webauthn.SessionData{}, signup, nil
		},
		FinishSignupRegistrationWithLegalFn: func(email string, _ *webauthn.SessionData, _ *protocol.ParsedCredentialCreationData, _ string, acceptance contracts.LegalAcceptance) (*authm.User, error) {
			*recorded = acceptance
			return &authm.User{ID: 9, Email: &email}, nil
		},
	}
	users := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil }}
	jwt := &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) { return "token", nil }}
	return newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, jwt, users))
}

// The finish body's consent fields are accepted and ignored: the account
// records what begin validated, whatever the body says.
func TestPasskeySignupFinishWire_RecordsTheConsentBeginValidated(t *testing.T) {
	registration, _ := softRegistration(t)
	signup := currentSignupChallenge("new@example.com")
	var recorded contracts.LegalAcceptance
	api := passkeySignupFinishAPI(t, signup, &recorded)

	requireWireSuccess(t, api.Post("/auth/passkey/signup/finish", map[string]any{
		"challenge_id":     "c-1",
		"response":         registration,
		"terms_accepted":   false,
		"terms_version":    "2025-01-01",
		"privacy_version":  "x",
		"age_confirmed":    false,
		"min_age_attested": 10,
	}))
	if recorded != signup.Acceptance {
		t.Errorf("recorded %+v, want the challenge's %+v", recorded, signup.Acceptance)
	}
}

func TestPasskeySignupFinishWire_RefusesAChallengeWhoseConsentIsNotCurrent(t *testing.T) {
	stale := currentSignupChallenge("new@example.com")
	stale.Acceptance.PrivacyVersion = "2025-01-01"
	noAge := currentSignupChallenge("new@example.com")
	noAge.Acceptance.AgeConfirmedAt = time.Time{}
	noAge.Acceptance.MinAgeAttested = 0

	for name, tc := range map[string]struct {
		signup   contracts.SignupChallenge
		wantCode string
	}{
		"no consent":       {contracts.SignupChallenge{Email: "new@example.com"}, autherrors.CodeTermsAcceptanceRequired},
		"stale versions":   {stale, autherrors.CodeTermsAcceptanceRequired},
		"no age confirmed": {noAge, autherrors.CodeAgeConfirmationRequired},
	} {
		t.Run(name, func(t *testing.T) {
			var recorded contracts.LegalAcceptance
			api := passkeySignupFinishAPI(t, tc.signup, &recorded)

			// The refusal comes before the credential is parsed, so the
			// response needs no ceremony behind it.
			resp := api.Post("/auth/passkey/signup/finish", map[string]any{
				"challenge_id":     "c-1",
				"response":         json.RawMessage(`{}`),
				"terms_accepted":   true,
				"terms_version":    legal.CurrentTermsVersion,
				"privacy_version":  legal.CurrentPrivacyVersion,
				"age_confirmed":    true,
				"min_age_attested": MinSignupAge,
			})
			body := decodeWireBody(t, resp.Body.Bytes())
			if success, _ := body["success"].(bool); success {
				t.Fatalf("expected a refusal: %s", resp.Body.String())
			}
			if code, _ := body["error_code"].(string); code != tc.wantCode {
				t.Errorf("error_code = %q, want %q", code, tc.wantCode)
			}
			if recorded != (contracts.LegalAcceptance{}) {
				t.Error("expected no account for a refused consent")
			}
		})
	}
}

// --- GET /auth/login/{provider} with signup_intent ---

func googleSignupStart(t *testing.T, query string) *httptest.ResponseRecorder {
	t.Helper()
	w, req := oauthLoginRequest("google")
	req.URL.RawQuery = "signup_intent=1&terms_accepted=true&age_confirmed=true&min_age_attested=16&" + query
	NewOAuthHTTPHandler(nil, nil, nil).OAuthLoginHTTPHandler(w, req)
	return w
}

func TestGoogleSignupStart_RefusesConsentVersionsThatAreNotCurrent(t *testing.T) {
	for name, pair := range consentVersionRefusals() {
		t.Run(name, func(t *testing.T) {
			query := url.Values{}
			if pair[0] != nil {
				query.Set("terms_version", *pair[0])
			}
			if pair[1] != nil {
				query.Set("privacy_version", *pair[1])
			}
			w := googleSignupStart(t, query.Encode())

			if w.Code != http.StatusTemporaryRedirect {
				t.Fatalf("status %d, want %d: %s", w.Code, http.StatusTemporaryRedirect, w.Body.String())
			}
			location, err := url.Parse(w.Header().Get("Location"))
			if err != nil {
				t.Fatalf("Location: %v", err)
			}
			want := autherrors.ErrTermsAcceptanceRequired("")
			if location.Path != "/auth" {
				t.Errorf("redirect path %q, want /auth", location.Path)
			}
			// The literal keys are the frontend auth page's contract.
			if got := location.Query().Get("error_code"); got != want.Code {
				t.Errorf("error_code = %q, want %q", got, want.Code)
			}
			if got := location.Query().Get("error"); got != want.UserMessage() {
				t.Errorf("error = %q, want %q", got, want.UserMessage())
			}
			for _, c := range w.Result().Cookies() {
				if c.Name == oauthSignupConsentCookieName {
					t.Error("expected no signup-consent cookie for a refused consent")
				}
			}
		})
	}
}

func TestGoogleSignupStart_RecordsCurrentConsentVersions(t *testing.T) {
	w := googleSignupStart(t, currentConsentVersionsQuery)

	for _, c := range w.Result().Cookies() {
		if c.Name != oauthSignupConsentCookieName {
			continue
		}
		consent, err := decodeOAuthSignupConsent(c.Value)
		if err != nil {
			t.Fatalf("decode consent cookie: %v", err)
		}
		if consent.TermsVersion != legal.CurrentTermsVersion || consent.PrivacyVersion != legal.CurrentPrivacyVersion {
			t.Errorf("consent versions %q/%q, want the current ones", consent.TermsVersion, consent.PrivacyVersion)
		}
		return
	}
	t.Fatalf("expected a signup-consent cookie, status %d", w.Code)
}

// --- POST /auth/passkey/login/begin with an email ---

// An address with no account and an account with no passkey answer the same
// bytes.
func TestPasskeyLoginBeginWire_UnknownAndPasskeylessAnswerIdentically(t *testing.T) {
	known := "has-account@example.com"
	mockWA := &testhelpers.MockWebAuthnService{
		BeginLoginFn: func(user *authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			if user == nil {
				t.Fatal("BeginLogin reached with a nil user")
			}
			return nil, nil, errNoPasskeysForTest
		},
	}
	users := &testhelpers.MockUserService{GetUserByEmailFn: func(email string) (*authm.User, error) {
		if email == known {
			return &authm.User{ID: 3, Email: &known, IsActive: true}, nil
		}
		return nil, nil
	}}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, users))

	unknown := api.Post("/auth/passkey/login/begin", map[string]any{"email": "no-account@example.com"})
	passkeyless := api.Post("/auth/passkey/login/begin", map[string]any{"email": known})

	requireIdenticalResponses(t, unknown, passkeyless)
	body := decodeWireBody(t, unknown.Body.Bytes())
	if code, _ := body["error_code"].(string); code != autherrors.CodeInvalidCredentials {
		t.Errorf("error_code = %q, want %q", code, autherrors.CodeInvalidCredentials)
	}
}

// errNoPasskeysForTest is BeginLogin's refusal, with a message of its own so
// the test shows the handler answers with its own copy instead.
var errNoPasskeysForTest = autherrors.NewAuthError(autherrors.CodeInvalidCredentials, "service copy", errors.New("user 3 has no registered passkeys"))

// requireIdenticalResponses fails unless a and b carry the same status, the
// same headers and the same body bytes.
func requireIdenticalResponses(t *testing.T, a, b *httptest.ResponseRecorder) {
	t.Helper()
	if a.Code != b.Code {
		t.Errorf("status %d vs %d", a.Code, b.Code)
	}
	if ah, bh := a.Result().Header, b.Result().Header; !reflect.DeepEqual(ah, bh) {
		t.Errorf("headers differ:\n%v\n%v", ah, bh)
	}
	if !bytes.Equal(a.Body.Bytes(), b.Body.Bytes()) {
		t.Errorf("bodies differ:\n%s\n%s", a.Body.String(), b.Body.String())
	}
}
