package auth

import (
	"encoding/base64"
	"encoding/json"
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
		GetChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
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
		GetSignupChallengeFn: func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
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
		GetChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 1, nil
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
