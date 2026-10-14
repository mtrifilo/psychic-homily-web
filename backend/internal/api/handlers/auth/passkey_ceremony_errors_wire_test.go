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

// wireOutcome is what a passkey step answers for err from the service.
type wireOutcome struct {
	name        string
	err         error
	wantStatus  int
	wantCode    string
	wantMessage string
}

// requireWireOutcome asserts resp is a failed step answering want, with no
// session and none of the cause's text.
func requireWireOutcome(t *testing.T, resp *httptest.ResponseRecorder, want wireOutcome) {
	t.Helper()
	if resp.Code != want.wantStatus {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, want.wantStatus, resp.Body.String())
	}
	body := decodeWireBody(t, resp.Body.Bytes())
	if success, _ := body["success"].(bool); success {
		t.Errorf("expected success=false: %s", resp.Body.String())
	}
	if code, _ := body["error_code"].(string); code != want.wantCode {
		t.Errorf("error_code = %q, want %q", code, want.wantCode)
	}
	if message, _ := body["message"].(string); message != want.wantMessage {
		t.Errorf("message = %q, want %q", message, want.wantMessage)
	}
	if _, hasUser := body["user"]; hasUser {
		t.Errorf("a refused or failed step returns no account: %s", resp.Body.String())
	}
	if cookie := resp.Header().Get("Set-Cookie"); cookie != "" {
		t.Errorf("a refused or failed step sets no cookie, got %q", cookie)
	}
	assertNoInternalText(t, resp.Body.String(), "duplicate email", "failed to save", "connection reset")
}

// serviceUnavailable is the outcome every failure answers with.
func serviceUnavailable(name string, err error) wireOutcome {
	return wireOutcome{name, err, http.StatusServiceUnavailable, autherrors.CodeServiceUnavailable, "Service temporarily unavailable"}
}

var (
	errRegistrationRefused = autherrors.NewAuthError(autherrors.CodeValidationFailed, "Invalid credential response", fmt.Errorf("create credential: origin mismatch"))
	errStoreFailed         = fmt.Errorf("failed to save credential: %w", errWireDriver)
	// errUnroutedRefusal is a refusal no passkey step in these tests routes,
	// so each answers it as a failure.
	errUnroutedRefusal = autherrors.ErrAccountLocked("locked")
)

// register/finish answers a registration the service refuses with the
// refusal's code and message, and a failure with a 503.
func TestPasskeyWire_RegisterFinishRegistrationErrors(t *testing.T) {
	body, _ := softRegistration(t)
	outcomes := []wireOutcome{
		{"registration refused", errRegistrationRefused, http.StatusOK, autherrors.CodeValidationFailed, "Invalid credential response"},
		serviceUnavailable("credential cannot be stored", errStoreFailed),
		serviceUnavailable("unrouted refusal code", errUnroutedRefusal),
	}
	for _, outcome := range outcomes {
		t.Run(outcome.name, func(t *testing.T) {
			mockWA := &testhelpers.MockWebAuthnService{
				ConsumeRegistrationChallengeFn: func(string, uint) (*webauthn.SessionData, error) {
					return &webauthn.SessionData{}, nil
				},
				FinishRegistrationFn: func(*authm.User, *webauthn.SessionData, *protocol.ParsedCredentialCreationData, string) (*authm.WebAuthnCredential, error) {
					return nil, outcome.err
				},
			}
			api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{}))

			resp := api.Post("/auth/passkey/register/finish", map[string]any{"challenge_id": "c-1", "response": body})
			requireWireOutcome(t, resp, outcome)
		})
	}
}

// login/begin with an email answers a refusal from BeginLogin exactly as it
// answers an address with no account, and a failure at any step with a 503
// and no challenge.
func TestPasskeyWire_LoginBeginErrors(t *testing.T) {
	email := "holder@example.com"
	holder := func(string) (*authm.User, error) { return &authm.User{ID: 3, Email: &email, IsActive: true}, nil }
	began := func(*authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
		return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
	}
	noPasskeys := wireOutcome{"", nil, http.StatusOK, autherrors.CodeInvalidCredentials, "No passkeys registered for this account"}

	steps := []struct {
		name       string
		body       map[string]any
		lookup     func(string) (*authm.User, error)
		beginLogin func(*authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error)
		beginAny   func() (*protocol.CredentialAssertion, *webauthn.SessionData, error)
		store      func(uint, *webauthn.SessionData, string) (string, error)
		want       wireOutcome
	}{
		{
			name:   "address with no account",
			body:   map[string]any{"email": email},
			lookup: func(string) (*authm.User, error) { return nil, nil },
			want:   noPasskeys,
		},
		{
			name:   "account BeginLogin refuses",
			body:   map[string]any{"email": email},
			lookup: holder,
			beginLogin: func(*authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
				return nil, nil, errNoPasskeysForTest
			},
			want: noPasskeys,
		},
		{
			name:   "address lookup fails",
			body:   map[string]any{"email": email},
			lookup: func(string) (*authm.User, error) { return nil, fmt.Errorf("failed to get user: %w", errWireDriver) },
			want:   serviceUnavailable("", nil),
		},
		{
			name:   "BeginLogin fails",
			body:   map[string]any{"email": email},
			lookup: holder,
			beginLogin: func(*authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
				return nil, nil, fmt.Errorf("failed to load user: %w", errWireDriver)
			},
			want: serviceUnavailable("", nil),
		},
		{
			name:   "BeginLogin answers an unrouted refusal code",
			body:   map[string]any{"email": email},
			lookup: holder,
			beginLogin: func(*authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
				return nil, nil, errUnroutedRefusal
			},
			want: serviceUnavailable("", nil),
		},
		{
			name:       "email challenge cannot be stored",
			body:       map[string]any{"email": email},
			lookup:     holder,
			beginLogin: began,
			store: func(uint, *webauthn.SessionData, string) (string, error) {
				return "", fmt.Errorf("failed to store challenge: %w", errWireDriver)
			},
			want: serviceUnavailable("", nil),
		},
		{
			name: "discoverable ceremony cannot start",
			body: map[string]any{},
			beginAny: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
				return nil, nil, fmt.Errorf("failed to begin discoverable login: %w", errWireDriver)
			},
			want: serviceUnavailable("", nil),
		},
		{
			name: "discoverable challenge cannot be stored",
			body: map[string]any{},
			beginAny: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
				return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
			},
			store: func(uint, *webauthn.SessionData, string) (string, error) {
				return "", fmt.Errorf("failed to store challenge: %w", errWireDriver)
			},
			want: serviceUnavailable("", nil),
		},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			mockWA := &testhelpers.MockWebAuthnService{
				BeginLoginFn:             step.beginLogin,
				BeginDiscoverableLoginFn: step.beginAny,
				StoreChallengeFn:         step.store,
			}
			users := &testhelpers.MockUserService{GetUserByEmailFn: step.lookup}
			api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, users))

			resp := api.Post("/auth/passkey/login/begin", step.body)
			requireWireOutcome(t, resp, step.want)
			if _, hasChallenge := decodeWireBody(t, resp.Body.Bytes())["challenge_id"]; hasChallenge {
				t.Errorf("a refused or failed begin issues no challenge: %s", resp.Body.String())
			}
		})
	}
}

// login/finish for an email-specific challenge answers with the account the
// service decided on, and reads no account of its own.
func TestPasskeyWire_EmailLoginFinishAnswersTheDecidedAccount(t *testing.T) {
	_, device := softRegistration(t)
	email := "decided@example.com"
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 7, nil
		},
		FinishLoginFn: func(user *authm.User, _ *webauthn.SessionData, _ *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error) {
			return &authm.User{ID: user.ID, Email: &email, IsActive: true}, &authm.WebAuthnCredential{ID: 1}, nil
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
// failure at any step with a 503 and no session.
func TestPasskeyWire_SignupFinishErrors(t *testing.T) {
	body, _ := softRegistration(t)
	address := "new@example.com"
	consumed := func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
		return &webauthn.SessionData{}, currentSignupChallenge(address), nil
	}
	steps := []struct {
		name    string
		consume func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error)
		lookup  func(string) (*authm.User, error)
		finish  error
		mint    func(*authm.User) (string, error)
		want    wireOutcome
	}{
		{
			name: "challenge refused",
			consume: func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
				return nil, contracts.SignupChallenge{}, autherrors.NewAuthError(autherrors.CodeValidationFailed, "Invalid or expired challenge", fmt.Errorf("challenge not found"))
			},
			want: wireOutcome{"", nil, http.StatusOK, autherrors.CodeValidationFailed, "Invalid or expired challenge"},
		},
		{
			name: "challenge cannot be read",
			consume: func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
				return nil, contracts.SignupChallenge{}, fmt.Errorf("failed to consume challenge: %w", errWireDriver)
			},
			want: serviceUnavailable("", nil),
		},
		{
			name:   "address check fails",
			lookup: func(string) (*authm.User, error) { return nil, fmt.Errorf("failed to get user: %w", errWireDriver) },
			want:   serviceUnavailable("", nil),
		},
		{
			name:   "registration refused",
			finish: errRegistrationRefused,
			want:   wireOutcome{"", nil, http.StatusOK, autherrors.CodeValidationFailed, "Invalid credential response"},
		},
		{
			name:   "address taken at insert",
			finish: autherrors.ErrUserExists(address),
			want:   wireOutcome{"", nil, http.StatusOK, autherrors.CodeUserExists, "An account with this email already exists"},
		},
		{
			name:   "account cannot be created",
			finish: fmt.Errorf("failed to create user preferences: %w", errWireDriver),
			want:   serviceUnavailable("", nil),
		},
		{
			name:   "unrouted refusal code",
			finish: errUnroutedRefusal,
			want:   serviceUnavailable("", nil),
		},
		{
			name: "token cannot be minted",
			mint: func(*authm.User) (string, error) { return "", fmt.Errorf("sign token: %w", errWireDriver) },
			want: serviceUnavailable("", nil),
		},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			mockWA := &testhelpers.MockWebAuthnService{
				ConsumeSignupChallengeFn: consumed,
				FinishSignupRegistrationWithLegalFn: func(string, *webauthn.SessionData, *protocol.ParsedCredentialCreationData, string, contracts.LegalAcceptance) (*authm.User, error) {
					if step.finish != nil {
						return nil, step.finish
					}
					return &authm.User{ID: 9, Email: &address}, nil
				},
			}
			if step.consume != nil {
				mockWA.ConsumeSignupChallengeFn = step.consume
			}
			users := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil }}
			if step.lookup != nil {
				users.GetUserByEmailFn = step.lookup
			}
			jwt := &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) { return "token", nil }}
			if step.mint != nil {
				jwt.CreateTokenFn = step.mint
			}
			api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, jwt, users))

			resp := api.Post("/auth/passkey/signup/finish", signupFinishBody("c-1", body))
			requireWireOutcome(t, resp, step.want)
		})
	}
}
