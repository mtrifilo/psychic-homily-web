package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// The browser sends what @simplewebauthn/browser v13 serializes, and huma
// validates request bodies against the operation's schema before a handler
// runs, so these post that exact shape through a huma API: a field the
// library adds and the server does not model must not turn into a 422.

func newPasskeyWireAPI(t *testing.T, h *PasskeyHandler) humatest.TestAPI {
	t.Helper()
	api, withUser := newWireTestAPI(t)
	huma.Post(api, "/auth/passkey/login/begin", h.BeginLoginHandler)
	huma.Post(api, "/auth/passkey/login/finish", h.FinishLoginHandler)
	huma.Post(api, "/auth/passkey/signup/begin", h.BeginSignupHandler)
	huma.Post(api, "/auth/passkey/signup/finish", h.FinishSignupHandler)
	freshSession := huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithValue(ctx, middleware.SessionAuthTimeContextKey, time.Now()))
	}}
	huma.Register(api, huma.Operation{
		OperationID: "passkey-register-begin",
		Method:      http.MethodPost,
		Path:        "/auth/passkey/register/begin",
		Middlewares: append(withUser, freshSession...),
	}, h.BeginRegisterHandler)
	huma.Register(api, huma.Operation{
		OperationID: "passkey-register-finish",
		Method:      http.MethodPost,
		Path:        "/auth/passkey/register/finish",
		Middlewares: withUser,
	}, h.FinishRegisterHandler)
	return api
}

func decodeWireBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return body
}

func requireWireSuccess(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d: %s", resp.Code, resp.Body.String())
	}
	if success, _ := decodeWireBody(t, resp.Body.Bytes())["success"].(bool); !success {
		t.Fatalf("expected success: %s", resp.Body.String())
	}
}

// wirePublicKey returns options.publicKey from a begin response, failing the
// test when the wrapper or its challenge is missing.
func wirePublicKey(t *testing.T, resp *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	requireWireSuccess(t, resp)
	options, _ := decodeWireBody(t, resp.Body.Bytes())["options"].(map[string]any)
	publicKey, _ := options["publicKey"].(map[string]any)
	if challenge, _ := publicKey["challenge"].(string); challenge == "" {
		t.Fatalf("expected options.publicKey.challenge, got %s", resp.Body.String())
	}
	return publicKey
}

// The frontend hands options.publicKey to the browser library, so the begin
// endpoints must keep the CredentialAssertion/CredentialCreation wrapper.
func TestPasskeyWire_LoginBeginOptionsCarryPublicKeyWrapper(t *testing.T) {
	mockWA := &testhelpers.MockWebAuthnService{
		BeginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return wireWebAuthn(t).BeginDiscoverableLogin()
		},
		StoreChallengeFn: func(uint, *webauthn.SessionData, string) (string, error) { return "c-1", nil },
	}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{}))

	wirePublicKey(t, api.Post("/auth/passkey/login/begin", map[string]any{}))
}

func TestPasskeyWire_RegisterBeginOptionsCarryPublicKeyWrapper(t *testing.T) {
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationFn: func(user *authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return wireWebAuthn(t).BeginRegistration(user)
		},
		StoreChallengeFn: func(uint, *webauthn.SessionData, string) (string, error) { return "c-1", nil },
	}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{}))

	wirePublicKey(t, api.Post("/auth/passkey/register/begin", map[string]any{"display_name": "Laptop"}))
}

// signup/begin passes the service's user handle to the client as base64url
// that decodes back to the account id. That the service reserves the id is
// TestSignupHandleCarriesNoEmail in the service suite.
func TestPasskeyWire_SignupBeginEncodesUserHandleAsBase64URL(t *testing.T) {
	reserved := &authm.User{ID: 77}
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationForEmailFn: func(string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return wireWebAuthn(t).BeginRegistration(reserved)
		},
		StoreSignupChallengeFn: func(contracts.SignupChallenge, *webauthn.SessionData, string) (string, error) { return "c-1", nil },
	}
	users := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil }}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, users))

	publicKey := wirePublicKey(t, api.Post("/auth/passkey/signup/begin", map[string]any{
		"email":            "new@example.com",
		"terms_accepted":   true,
		"terms_version":    legal.CurrentTermsVersion,
		"privacy_version":  legal.CurrentPrivacyVersion,
		"age_confirmed":    true,
		"min_age_attested": 16,
	}))
	user, _ := publicKey["user"].(map[string]any)
	encoded, _ := user["id"].(string)
	handle, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode user.id %q: %v", encoded, err)
	}
	if id, ok := authm.UserIDFromWebAuthnID(handle); !ok || id != reserved.ID {
		t.Fatalf("expected user.id to encode %d, got %q", reserved.ID, encoded)
	}
}

// An invalid name is refused at begin; finish stores the default instead of
// failing after the authenticator has written the credential.
func TestPasskeyWire_RegisterBeginRefusesInvalidName(t *testing.T) {
	for name, displayName := range map[string]string{
		"too long":          strings.Repeat("x", 101),
		"control character": "Laptop\u0000",
	} {
		t.Run(name, func(t *testing.T) {
			var began bool
			mockWA := &testhelpers.MockWebAuthnService{
				BeginRegistrationFn: func(*authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
					began = true
					return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
				},
			}
			api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{}))

			resp := api.Post("/auth/passkey/register/begin", map[string]any{"display_name": displayName})
			body := decodeWireBody(t, resp.Body.Bytes())
			if success, _ := body["success"].(bool); success || began {
				t.Fatalf("expected a refusal before any challenge: %s", resp.Body.String())
			}
			if code, _ := body["error_code"].(string); code != autherrors.CodeValidationFailed {
				t.Errorf("expected %s, got %s", autherrors.CodeValidationFailed, resp.Body.String())
			}
		})
	}
}

func TestPasskeyName(t *testing.T) {
	for raw, want := range map[string]string{
		"":                       defaultPasskeyName,
		"   ":                    defaultPasskeyName,
		"  Laptop  ":             "Laptop",
		strings.Repeat("x", 101): defaultPasskeyName,
		"Laptop\u0000":           defaultPasskeyName,
	} {
		if got := passkeyName(raw); got != want {
			t.Errorf("passkeyName(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestPasskeyWire_LoginFinishAcceptsLibraryResponseShape(t *testing.T) {
	_, device := softRegistration(t)
	var parsedID string
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 0, nil
		},
		FinishDiscoverableLoginFn: func(_ *webauthn.SessionData, parsed *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error) {
			parsedID = parsed.ID
			return &authm.User{ID: 1, IsActive: true}, &authm.WebAuthnCredential{ID: 1}, nil
		},
	}
	jwt := &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) { return "token", nil }}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, jwt, &testhelpers.MockUserService{}))

	resp := api.Post("/auth/passkey/login/finish", map[string]any{
		"challenge_id": "c-1",
		"response":     softAssertion(t, device),
	})
	requireWireSuccess(t, resp)
	if parsedID == "" {
		t.Error("expected the parsed assertion to reach the service")
	}
}

func TestPasskeyWire_SignupFinishWithoutDisplayName(t *testing.T) {
	body, _ := softRegistration(t)
	var displayName string
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeSignupChallengeFn: func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
			return &webauthn.SessionData{}, currentSignupChallenge("new@example.com"), nil
		},
		FinishSignupRegistrationWithLegalFn: func(_ string, _ *webauthn.SessionData, _ *protocol.ParsedCredentialCreationData, name string, _ contracts.LegalAcceptance) (*authm.User, error) {
			displayName = name
			email := "new@example.com"
			return &authm.User{ID: 9, Email: &email}, nil
		},
	}
	users := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil }}
	jwt := &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) { return "token", nil }}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, jwt, users))

	resp := api.Post("/auth/passkey/signup/finish", map[string]any{
		"challenge_id":     "c-1",
		"response":         body,
		"terms_accepted":   true,
		"terms_version":    legal.CurrentTermsVersion,
		"privacy_version":  legal.CurrentPrivacyVersion,
		"age_confirmed":    true,
		"min_age_attested": 16,
	})
	requireWireSuccess(t, resp)
	if displayName != "My Passkey" {
		t.Errorf("expected the default display name, got %q", displayName)
	}
}

func TestPasskeyWire_RegisterFinishWithoutDisplayName(t *testing.T) {
	body, _ := softRegistration(t)
	var displayName string
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeRegistrationChallengeFn: func(string, uint) (*webauthn.SessionData, error) {
			return &webauthn.SessionData{}, nil
		},
		FinishRegistrationFn: func(user *authm.User, _ *webauthn.SessionData, _ *protocol.ParsedCredentialCreationData, name string) (*authm.WebAuthnCredential, error) {
			displayName = name
			return &authm.WebAuthnCredential{ID: 3, UserID: user.ID}, nil
		},
	}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{}))

	resp := api.Post("/auth/passkey/register/finish", map[string]any{
		"challenge_id": "c-1",
		"response":     body,
	})
	requireWireSuccess(t, resp)
	if displayName != "My Passkey" {
		t.Errorf("expected the default display name, got %q", displayName)
	}
}

// login/finish answers what the service decided: a refusal with the refusal's
// own code and message and no session, and an error that is not a refusal
// with a 5xx that carries none of the cause, on both ceremonies.
func TestPasskeyWire_LoginFinishErrorClasses(t *testing.T) {
	inactive := autherrors.ErrAccountInactive()

	outcomes := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "assertion refused",
			err:         autherrors.NewAuthError(autherrors.CodeInvalidCredentials, "Invalid passkey", errWireDriver),
			wantStatus:  http.StatusOK,
			wantCode:    autherrors.CodeInvalidCredentials,
			wantMessage: "Invalid passkey",
		},
		{
			name:        "account may not sign in",
			err:         inactive,
			wantStatus:  http.StatusOK,
			wantCode:    autherrors.CodeAccountInactive,
			wantMessage: inactive.UserMessage(),
		},
		{
			name:        "service failure",
			err:         fmt.Errorf("failed to update credential: %w", errWireDriver),
			wantStatus:  http.StatusServiceUnavailable,
			wantCode:    autherrors.CodeServiceUnavailable,
			wantMessage: "Service temporarily unavailable",
		},
		{
			// A refusal code login/finish does not route is answered as a
			// failure, not with the code's own status (409 here).
			name:        "unrouted refusal code",
			err:         autherrors.ErrUserExists("someone@example.com"),
			wantStatus:  http.StatusServiceUnavailable,
			wantCode:    autherrors.CodeServiceUnavailable,
			wantMessage: "Service temporarily unavailable",
		},
	}
	// The challenge's user id decides which ceremony runs: zero is a
	// discoverable login, anything else names the account up front.
	ceremonies := []struct {
		name          string
		challengeUser uint
	}{
		{name: "email", challengeUser: 7},
		{name: "discoverable", challengeUser: 0},
	}

	_, device := softRegistration(t)
	for _, ceremony := range ceremonies {
		for _, outcome := range outcomes {
			t.Run(ceremony.name+"/"+outcome.name, func(t *testing.T) {
				mockWA := &testhelpers.MockWebAuthnService{
					ConsumeChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
						return &webauthn.SessionData{}, ceremony.challengeUser, nil
					},
					FinishLoginFn: func(*authm.User, *webauthn.SessionData, *protocol.ParsedCredentialAssertionData) (*authm.WebAuthnCredential, error) {
						return nil, outcome.err
					},
					FinishDiscoverableLoginFn: func(*webauthn.SessionData, *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error) {
						return nil, nil, outcome.err
					},
				}
				users := &testhelpers.MockUserService{
					GetUserByIDFn: func(id uint) (*authm.User, error) { return &authm.User{ID: id, IsActive: true}, nil },
				}
				jwt := &testhelpers.MockJWTService{CreateTokenFn: func(*authm.User) (string, error) {
					t.Error("no session may be minted")
					return "token", nil
				}}
				api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, jwt, users))

				resp := api.Post("/auth/passkey/login/finish", map[string]any{
					"challenge_id": "c-1",
					"response":     softAssertion(t, device),
				})

				if resp.Code != outcome.wantStatus {
					t.Fatalf("status = %d, want %d; body: %s", resp.Code, outcome.wantStatus, resp.Body.String())
				}
				body := decodeWireBody(t, resp.Body.Bytes())
				if success, _ := body["success"].(bool); success {
					t.Errorf("expected success=false: %s", resp.Body.String())
				}
				if code, _ := body["error_code"].(string); code != outcome.wantCode {
					t.Errorf("error_code = %q, want %q", code, outcome.wantCode)
				}
				if message, _ := body["message"].(string); message != outcome.wantMessage {
					t.Errorf("message = %q, want %q", message, outcome.wantMessage)
				}
				if _, hasUser := body["user"]; hasUser {
					t.Errorf("a refused or failed sign-in returns no account: %s", resp.Body.String())
				}
				if cookie := resp.Header().Get("Set-Cookie"); cookie != "" {
					t.Errorf("a refused or failed sign-in sets no cookie, got %q", cookie)
				}
				assertNoInternalText(t, resp.Body.String(), "failed to update credential", "duplicate email")
			})
		}
	}
}

// requireWireServiceUnavailable asserts a 503 SERVICE_UNAVAILABLE that
// carries none of errWireDriver's text.
func requireWireServiceUnavailable(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", resp.Code, resp.Body.String())
	}
	if code := decodeErrorCode(t, resp.Body.Bytes()); code != autherrors.CodeServiceUnavailable {
		t.Errorf("error_code = %q, want %q", code, autherrors.CodeServiceUnavailable)
	}
	assertNoInternalText(t, resp.Body.String())
}

// An email-specific login whose account cannot be loaded is a failure; only
// an account that does not exist is refused.
func TestPasskeyWire_LoginFinishUserLoadFailure(t *testing.T) {
	_, device := softRegistration(t)
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 7, nil
		},
		FinishLoginFn: func(*authm.User, *webauthn.SessionData, *protocol.ParsedCredentialAssertionData) (*authm.WebAuthnCredential, error) {
			t.Error("a failed account load must not reach the ceremony")
			return nil, nil
		},
	}
	users := &testhelpers.MockUserService{
		GetUserByIDFn: func(uint) (*authm.User, error) { return nil, fmt.Errorf("failed to get user: %w", errWireDriver) },
	}
	api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, users))

	requireWireServiceUnavailable(t, api.Post("/auth/passkey/login/finish", map[string]any{
		"challenge_id": "c-1",
		"response":     softAssertion(t, device),
	}))
}

// register/finish answers a refused challenge with the refusal's code and
// message, and a challenge that could not be read with a 5xx.
func TestPasskeyWire_RegisterFinishChallengeErrors(t *testing.T) {
	body, _ := softRegistration(t)
	outcomes := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"issued to another user", autherrors.NewAuthError(autherrors.CodeUnauthorized, "Challenge belongs to different user", nil),
			http.StatusOK, autherrors.CodeUnauthorized, "Challenge belongs to different user"},
		{"absent or expired", autherrors.NewAuthError(autherrors.CodeValidationFailed, "Invalid or expired challenge", nil),
			http.StatusOK, autherrors.CodeValidationFailed, "Invalid or expired challenge"},
		{"could not be read", fmt.Errorf("failed to consume challenge: %w", errWireDriver),
			http.StatusServiceUnavailable, autherrors.CodeServiceUnavailable, "Service temporarily unavailable"},
		{"unrouted refusal code", autherrors.ErrUserExists("someone@example.com"),
			http.StatusServiceUnavailable, autherrors.CodeServiceUnavailable, "Service temporarily unavailable"},
	}
	for _, outcome := range outcomes {
		t.Run(outcome.name, func(t *testing.T) {
			mockWA := &testhelpers.MockWebAuthnService{
				ConsumeRegistrationChallengeFn: func(string, uint) (*webauthn.SessionData, error) {
					return nil, outcome.err
				},
				FinishRegistrationFn: func(*authm.User, *webauthn.SessionData, *protocol.ParsedCredentialCreationData, string) (*authm.WebAuthnCredential, error) {
					t.Error("a refused or unreadable challenge must not reach registration")
					return nil, nil
				},
			}
			api := newPasskeyWireAPI(t, testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{}))

			resp := api.Post("/auth/passkey/register/finish", map[string]any{"challenge_id": "c-1", "response": body})

			if resp.Code != outcome.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", resp.Code, outcome.wantStatus, resp.Body.String())
			}
			decoded := decodeWireBody(t, resp.Body.Bytes())
			if code, _ := decoded["error_code"].(string); code != outcome.wantCode {
				t.Errorf("error_code = %q, want %q", code, outcome.wantCode)
			}
			if message, _ := decoded["message"].(string); message != outcome.wantMessage {
				t.Errorf("message = %q, want %q", message, outcome.wantMessage)
			}
			assertNoInternalText(t, resp.Body.String(), "duplicate email")
		})
	}
}
