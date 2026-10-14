package auth

import (
	"encoding/json"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/softauthn"
)

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
