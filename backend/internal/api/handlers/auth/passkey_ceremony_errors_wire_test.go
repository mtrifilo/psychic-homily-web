package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// wireOutcome is the status, code and message a refused or failed passkey
// step answers with.
type wireOutcome struct {
	status  int
	code    string
	message string
}

// unavailable is the outcome every failure answers with.
var unavailable = wireOutcome{http.StatusServiceUnavailable, autherrors.CodeServiceUnavailable, "Service temporarily unavailable"}

// requireWireOutcome asserts resp is a refused or failed step answering want,
// with no account, no session, and none of the cause's text: errWireDriver's
// and the leaks fragments.
func requireWireOutcome(t *testing.T, resp *httptest.ResponseRecorder, want wireOutcome, leaks ...string) {
	t.Helper()
	if resp.Code != want.status {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, want.status, resp.Body.String())
	}
	body := decodeWireBody(t, resp.Body.Bytes())
	if success, _ := body["success"].(bool); success {
		t.Errorf("expected success=false: %s", resp.Body.String())
	}
	if code, _ := body["error_code"].(string); code != want.code {
		t.Errorf("error_code = %q, want %q", code, want.code)
	}
	if message, _ := body["message"].(string); message != want.message {
		t.Errorf("message = %q, want %q", message, want.message)
	}
	if _, hasUser := body["user"]; hasUser {
		t.Errorf("a refused or failed step returns no account: %s", resp.Body.String())
	}
	if cookie := resp.Header().Get("Set-Cookie"); cookie != "" {
		t.Errorf("a refused or failed step sets no cookie, got %q", cookie)
	}
	assertNoInternalText(t, resp.Body.String(), append([]string{"duplicate email", "failed to"}, leaks...)...)
}

var (
	errRegistrationRefused = autherrors.NewAuthError(autherrors.CodeValidationFailed, "Invalid credential response", fmt.Errorf("create credential: origin mismatch"))
	// errUnroutedRefusal is a refusal no passkey step in these tests routes,
	// so each answers it as a failure.
	errUnroutedRefusal = autherrors.ErrAccountLocked("locked")
	// errStepFailed is a service failure.
	errStepFailed = fmt.Errorf("failed to query: %w", errWireDriver)
)

// register/finish answers a registration the service refuses with the
// refusal's code and message, and a failure with a 503.
func TestPasskeyWire_RegisterFinishRegistrationErrors(t *testing.T) {
	body, _ := softRegistration(t)
	cases := []struct {
		name string
		err  error
		want wireOutcome
	}{
		{"registration refused", errRegistrationRefused, wireOutcome{http.StatusOK, autherrors.CodeValidationFailed, "Invalid credential response"}},
		{"credential cannot be stored", errStepFailed, unavailable},
		{"unrouted refusal code", errUnroutedRefusal, unavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockWA := &testhelpers.MockWebAuthnService{
				ConsumeRegistrationChallengeFn: func(string, uint) (*webauthn.SessionData, error) {
					return &webauthn.SessionData{}, nil
				},
				FinishRegistrationFn: func(*authm.User, *webauthn.SessionData, *protocol.ParsedCredentialCreationData, string) (*authm.WebAuthnCredential, error) {
					return nil, tc.err
				},
			}
			api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{}))

			resp := api.Post("/auth/passkey/register/finish", map[string]any{"challenge_id": "c-1", "response": body})
			requireWireOutcome(t, resp, tc.want)
		})
	}
}

// login/begin answers a failure at any step with a 503 and no challenge. Each
// case names the step that errs; every other step succeeds.
func TestPasskeyWire_LoginBeginErrors(t *testing.T) {
	cases := []struct {
		name     string
		beginErr error
		storeErr error
	}{
		{name: "ceremony cannot start", beginErr: errStepFailed},
		{name: "challenge cannot be stored", storeErr: errStepFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockWA := &testhelpers.MockWebAuthnService{
				BeginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
					if tc.beginErr != nil {
						return nil, nil, tc.beginErr
					}
					return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
				},
				StoreChallengeFn: func(uint, *webauthn.SessionData, string) (string, error) {
					return "c-1", tc.storeErr
				},
			}
			api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{}))

			resp := api.Post("/auth/passkey/login/begin", map[string]any{})
			requireWireOutcome(t, resp, unavailable)
			if _, hasChallenge := decodeWireBody(t, resp.Body.Bytes())["challenge_id"]; hasChallenge {
				t.Errorf("a failed begin issues no challenge: %s", resp.Body.String())
			}
		})
	}
}

// login/begin's body declares no fields: the schema refuses an email, or any
// other field, with a 422 before the handler runs, so no ceremony starts and
// no challenge is stored. An empty body begins a discoverable login.
func TestPasskeyWire_LoginBeginRefusesBodyFields(t *testing.T) {
	var begun, stored int
	mockWA := &testhelpers.MockWebAuthnService{
		BeginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			begun++
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		StoreChallengeFn: func(userID uint, _ *webauthn.SessionData, _ string) (string, error) {
			stored++
			if userID != 0 {
				t.Errorf("challenge stored under user %d, want no account", userID)
			}
			return "c-1", nil
		},
	}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{}))

	for name, body := range map[string]map[string]any{
		"email":         {"email": "holder@example.com"},
		"empty email":   {"email": ""},
		"unknown field": {"username": "holder"},
	} {
		t.Run(name, func(t *testing.T) {
			resp := api.Post("/auth/passkey/login/begin", body)
			if resp.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusUnprocessableEntity, resp.Body.String())
			}
			if _, hasChallenge := decodeWireBody(t, resp.Body.Bytes())["challenge_id"]; hasChallenge {
				t.Errorf("a refused body issues no challenge: %s", resp.Body.String())
			}
		})
	}
	if begun != 0 || stored != 0 {
		t.Fatalf("a refused body reached the handler: %d ceremonies begun, %d challenges stored", begun, stored)
	}

	resp := api.Post("/auth/passkey/login/begin", map[string]any{})
	requireWireSuccess(t, resp)
	if got, _ := decodeWireBody(t, resp.Body.Bytes())["challenge_id"].(string); got != "c-1" {
		t.Errorf("challenge_id = %q, want %q", got, "c-1")
	}
	if begun != 1 || stored != 1 {
		t.Errorf("an empty body began %d ceremonies and stored %d challenges, want 1 and 1", begun, stored)
	}
}

// login/finish answers with the account the service decided on, and reads no
// account of its own.
func TestPasskeyWire_LoginFinishAnswersTheDecidedAccount(t *testing.T) {
	_, device := softRegistration(t)
	email := "decided@example.com"
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 0, nil
		},
		FinishDiscoverableLoginFn: func(*webauthn.SessionData, *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error) {
			return &authm.User{ID: 7, Email: &email, IsActive: true}, &authm.WebAuthnCredential{ID: 1}, nil
		},
	}
	jwt := &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) { return "token", nil }}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, jwt, noAccountLoads(t)))

	resp := api.Post("/auth/passkey/login/finish", map[string]any{
		"challenge_id": "c-1",
		"response":     softAssertion(t, device),
	})
	requireWireSuccess(t, resp)
	user, _ := decodeWireBody(t, resp.Body.Bytes())["user"].(map[string]any)
	if id, _ := user["id"].(float64); id != 7 {
		t.Errorf("user.id = %v, want 7", user["id"])
	}
	if got, _ := user["email"].(string); got != email {
		t.Errorf("user.email = %q, want %q", got, email)
	}
}

// signup/finish answers each refusal with its code and message, and a
// failure at any step with a 503 and no session. Each case names the step
// that errs; every other step succeeds.
func TestPasskeyWire_SignupFinishErrors(t *testing.T) {
	body, _ := softRegistration(t)
	address := "new@example.com"
	cases := []struct {
		name       string
		consumeErr error
		lookupErr  error
		finishErr  error
		mintErr    error
		want       wireOutcome
	}{
		{
			name:       "challenge refused",
			consumeErr: autherrors.NewAuthError(autherrors.CodeValidationFailed, "Invalid or expired challenge", fmt.Errorf("challenge not found")),
			want:       wireOutcome{http.StatusOK, autherrors.CodeValidationFailed, "Invalid or expired challenge"},
		},
		{name: "challenge cannot be read", consumeErr: errStepFailed, want: unavailable},
		{name: "challenge consume answers an unrouted refusal code", consumeErr: errUnroutedRefusal, want: unavailable},
		{name: "address check fails", lookupErr: errStepFailed, want: unavailable},
		{
			name:      "registration refused",
			finishErr: errRegistrationRefused,
			want:      wireOutcome{http.StatusOK, autherrors.CodeValidationFailed, "Invalid credential response"},
		},
		{
			name:      "address taken at insert",
			finishErr: autherrors.ErrUserExists(address),
			want:      wireOutcome{http.StatusOK, autherrors.CodeUserExists, "An account with this email already exists"},
		},
		{name: "account cannot be created", finishErr: errStepFailed, want: unavailable},
		{name: "unrouted refusal code", finishErr: errUnroutedRefusal, want: unavailable},
		{name: "token cannot be minted", mintErr: errStepFailed, want: unavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockWA := &testhelpers.MockWebAuthnService{
				ConsumeSignupChallengeFn: func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
					if tc.consumeErr != nil {
						return nil, contracts.SignupChallenge{}, tc.consumeErr
					}
					return &webauthn.SessionData{}, currentSignupChallenge(address), nil
				},
				FinishSignupRegistrationWithLegalFn: func(string, *webauthn.SessionData, *protocol.ParsedCredentialCreationData, string, contracts.LegalAcceptance) (*authm.User, error) {
					if tc.finishErr != nil {
						return nil, tc.finishErr
					}
					return &authm.User{ID: 9, Email: &address}, nil
				},
			}
			users := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, tc.lookupErr }}
			jwt := &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) {
				if tc.mintErr != nil {
					return "", tc.mintErr
				}
				return "token", nil
			}}
			api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, jwt, users))

			resp := api.Post("/auth/passkey/signup/finish", signupFinishBody("c-1", body))
			requireWireOutcome(t, resp, tc.want)
		})
	}
}
