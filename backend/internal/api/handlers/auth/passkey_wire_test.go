package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// The browser sends what @simplewebauthn/browser v13 serializes, and huma
// validates request bodies against the operation's schema before a handler
// runs, so these post that exact shape through a huma API: a field the
// library adds and the server does not model must not turn into a 422.

const wireOrigin = "http://localhost:3000"

func wireWebAuthn(t *testing.T) *webauthn.WebAuthn {
	t.Helper()
	w, err := webauthn.New(&webauthn.Config{RPDisplayName: "Test", RPID: "localhost", RPOrigins: []string{wireOrigin}})
	if err != nil {
		t.Fatalf("webauthn.New: %v", err)
	}
	return w
}

// softRegistration returns a RegistrationResponseJSON body for a fresh
// credential, plus the authenticator that holds it.
func softRegistration(t *testing.T) (json.RawMessage, *softauthn.Authenticator) {
	t.Helper()
	options, _, err := wireWebAuthn(t).BeginRegistration(&authm.User{ID: 1})
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	device := softauthn.New(wireOrigin)
	body, err := device.Register(options)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return body, device
}

// softAssertion returns an AuthenticationResponseJSON body signed by device.
func softAssertion(t *testing.T, device *softauthn.Authenticator) json.RawMessage {
	t.Helper()
	options, _, err := wireWebAuthn(t).BeginDiscoverableLogin()
	if err != nil {
		t.Fatalf("BeginDiscoverableLogin: %v", err)
	}
	body, err := device.Assert(options)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	return body
}

func newPasskeyWireAPI(t *testing.T, h *PasskeyHandler) humatest.TestAPI {
	t.Helper()
	api, withUser := newWireTestAPI(t)
	huma.Post(api, "/auth/passkey/login/begin", h.BeginLoginHandler)
	huma.Post(api, "/auth/passkey/login/finish", h.FinishLoginHandler)
	huma.Post(api, "/auth/passkey/signup/finish", h.FinishSignupHandler)
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

	resp := api.Post("/auth/passkey/login/begin", map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d: %s", resp.Code, resp.Body.String())
	}
	options, _ := decodeWireBody(t, resp.Body.Bytes())["options"].(map[string]any)
	publicKey, _ := options["publicKey"].(map[string]any)
	if challenge, _ := publicKey["challenge"].(string); challenge == "" {
		t.Fatalf("expected options.publicKey.challenge, got %s", resp.Body.String())
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
			return &authm.User{ID: 1}, &authm.WebAuthnCredential{ID: 1}, nil
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
		GetChallengeWithEmailFn: func(string, string) (*webauthn.SessionData, string, error) {
			return &webauthn.SessionData{}, "new@example.com", nil
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
		"terms_version":    "v1",
		"privacy_version":  "v1",
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
