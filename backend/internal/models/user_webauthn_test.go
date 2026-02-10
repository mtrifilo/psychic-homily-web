package models

import (
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// WebAuthnID Tests
// =============================================================================

func TestWebAuthnID(t *testing.T) {
	user := &User{ID: 42}
	id := user.WebAuthnID()
	assert.Len(t, id, 8)
	// Big-endian encoding of 42: last byte should be 42
	assert.Equal(t, byte(42), id[7])
	assert.Equal(t, byte(0), id[0])
}

func TestWebAuthnID_Zero(t *testing.T) {
	user := &User{ID: 0}
	id := user.WebAuthnID()
	assert.Len(t, id, 8)
	for _, b := range id {
		assert.Equal(t, byte(0), b)
	}
}

func TestWebAuthnID_LargeValue(t *testing.T) {
	user := &User{ID: 256}
	id := user.WebAuthnID()
	assert.Len(t, id, 8)
	assert.Equal(t, byte(1), id[6])
	assert.Equal(t, byte(0), id[7])
}

// =============================================================================
// WebAuthnName Tests
// =============================================================================

func TestWebAuthnName_Email(t *testing.T) {
	email := "user@test.com"
	user := &User{Email: &email}
	assert.Equal(t, "user@test.com", user.WebAuthnName())
}

func TestWebAuthnName_Username(t *testing.T) {
	username := "testuser"
	user := &User{Username: &username}
	assert.Equal(t, "testuser", user.WebAuthnName())
}

func TestWebAuthnName_Both(t *testing.T) {
	email := "user@test.com"
	username := "testuser"
	user := &User{Email: &email, Username: &username}
	// Prefers email
	assert.Equal(t, "user@test.com", user.WebAuthnName())
}

func TestWebAuthnName_Neither(t *testing.T) {
	user := &User{}
	assert.Equal(t, "", user.WebAuthnName())
}

func TestWebAuthnName_EmptyEmail(t *testing.T) {
	email := ""
	username := "testuser"
	user := &User{Email: &email, Username: &username}
	// Falls back to username when email is empty string
	assert.Equal(t, "testuser", user.WebAuthnName())
}

func TestWebAuthnName_EmptyBoth(t *testing.T) {
	email := ""
	username := ""
	user := &User{Email: &email, Username: &username}
	assert.Equal(t, "", user.WebAuthnName())
}

// =============================================================================
// WebAuthnDisplayName Tests
// =============================================================================

func TestWebAuthnDisplayName_FullName(t *testing.T) {
	first := "John"
	last := "Doe"
	user := &User{FirstName: &first, LastName: &last}
	assert.Equal(t, "John Doe", user.WebAuthnDisplayName())
}

func TestWebAuthnDisplayName_FirstOnly(t *testing.T) {
	first := "John"
	user := &User{FirstName: &first}
	assert.Equal(t, "John", user.WebAuthnDisplayName())
}

func TestWebAuthnDisplayName_NoName_FallsBackToEmail(t *testing.T) {
	email := "user@test.com"
	user := &User{Email: &email}
	assert.Equal(t, "user@test.com", user.WebAuthnDisplayName())
}

func TestWebAuthnDisplayName_EmptyFirstName(t *testing.T) {
	first := ""
	email := "user@test.com"
	user := &User{FirstName: &first, Email: &email}
	// Empty FirstName falls back to WebAuthnName
	assert.Equal(t, "user@test.com", user.WebAuthnDisplayName())
}

func TestWebAuthnDisplayName_FirstAndEmptyLast(t *testing.T) {
	first := "John"
	last := ""
	user := &User{FirstName: &first, LastName: &last}
	// Empty LastName is ignored, returns just first
	assert.Equal(t, "John", user.WebAuthnDisplayName())
}

// =============================================================================
// WebAuthnCredentials Tests
// =============================================================================

func TestWebAuthnCredentials_Empty(t *testing.T) {
	user := &User{}
	creds := user.WebAuthnCredentials()
	assert.Empty(t, creds)
	assert.Len(t, creds, 0)
}

func TestWebAuthnCredentials_Multiple(t *testing.T) {
	user := &User{
		PasskeyCredentials: []WebAuthnCredential{
			{
				CredentialID:    []byte("cred1"),
				PublicKey:       []byte("pk1"),
				AttestationType: "none",
				AAGUID:          "test-aaguid-1",
				SignCount:       5,
				CloneWarning:    false,
				BackupEligible:  true,
				BackupState:     true,
			},
			{
				CredentialID:    []byte("cred2"),
				PublicKey:       []byte("pk2"),
				AttestationType: "direct",
				AAGUID:          "test-aaguid-2",
				SignCount:       10,
				CloneWarning:    true,
				BackupEligible:  false,
				BackupState:     false,
			},
		},
	}
	creds := user.WebAuthnCredentials()
	assert.Len(t, creds, 2)

	assert.Equal(t, []byte("cred1"), creds[0].ID)
	assert.Equal(t, []byte("pk1"), creds[0].PublicKey)
	assert.Equal(t, "none", creds[0].AttestationType)
	assert.Equal(t, []byte("test-aaguid-1"), creds[0].Authenticator.AAGUID)
	assert.Equal(t, uint32(5), creds[0].Authenticator.SignCount)
	assert.False(t, creds[0].Authenticator.CloneWarning)
	assert.True(t, creds[0].Flags.BackupEligible)
	assert.True(t, creds[0].Flags.BackupState)

	assert.Equal(t, []byte("cred2"), creds[1].ID)
	assert.Equal(t, uint32(10), creds[1].Authenticator.SignCount)
	assert.True(t, creds[1].Authenticator.CloneWarning)
}

// =============================================================================
// WebAuthnIcon Tests
// =============================================================================

func TestWebAuthnIcon_WithAvatar(t *testing.T) {
	url := "https://example.com/avatar.png"
	user := &User{AvatarURL: &url}
	assert.Equal(t, "https://example.com/avatar.png", user.WebAuthnIcon())
}

func TestWebAuthnIcon_NoAvatar(t *testing.T) {
	user := &User{}
	assert.Equal(t, "", user.WebAuthnIcon())
}

// =============================================================================
// ToWebAuthnCredential Tests
// =============================================================================

func TestToWebAuthnCredential_Basic(t *testing.T) {
	cred := &webauthn.Credential{
		ID:              []byte("test-cred-id"),
		PublicKey:       []byte("test-public-key"),
		AttestationType: "none",
		Authenticator: webauthn.Authenticator{
			AAGUID:       []byte("test-aaguid"),
			SignCount:    42,
			CloneWarning: false,
		},
		Flags: webauthn.CredentialFlags{
			BackupEligible: true,
			BackupState:    false,
		},
	}

	result := ToWebAuthnCredential(123, cred, "My Passkey")

	assert.Equal(t, uint(123), result.UserID)
	assert.Equal(t, []byte("test-cred-id"), result.CredentialID)
	assert.Equal(t, []byte("test-public-key"), result.PublicKey)
	assert.Equal(t, "none", result.AttestationType)
	assert.Equal(t, "test-aaguid", result.AAGUID)
	assert.Equal(t, uint32(42), result.SignCount)
	assert.False(t, result.CloneWarning)
	assert.True(t, result.BackupEligible)
	assert.False(t, result.BackupState)
	assert.Equal(t, "My Passkey", result.DisplayName)
	assert.Equal(t, "", result.Transports) // No transports
}

func TestToWebAuthnCredential_WithTransports(t *testing.T) {
	cred := &webauthn.Credential{
		ID:        []byte("cred"),
		PublicKey: []byte("pk"),
		Transport: []protocol.AuthenticatorTransport{
			protocol.USB,
			protocol.Internal,
		},
		Authenticator: webauthn.Authenticator{},
	}

	result := ToWebAuthnCredential(1, cred, "Key")
	assert.Equal(t, `["usb","internal"]`, result.Transports)
}

func TestToWebAuthnCredential_SingleTransport(t *testing.T) {
	cred := &webauthn.Credential{
		ID:        []byte("cred"),
		PublicKey: []byte("pk"),
		Transport: []protocol.AuthenticatorTransport{protocol.BLE},
		Authenticator: webauthn.Authenticator{},
	}

	result := ToWebAuthnCredential(1, cred, "Key")
	assert.Equal(t, `["ble"]`, result.Transports)
}

func TestToWebAuthnCredential_NoTransports(t *testing.T) {
	cred := &webauthn.Credential{
		ID:            []byte("cred"),
		PublicKey:     []byte("pk"),
		Authenticator: webauthn.Authenticator{},
	}

	result := ToWebAuthnCredential(1, cred, "Key")
	assert.Equal(t, "", result.Transports)
}

// =============================================================================
// GetTransports Tests
// =============================================================================

func TestGetTransports_Empty(t *testing.T) {
	cred := &WebAuthnCredential{Transports: ""}
	assert.Nil(t, cred.GetTransports())
}

func TestGetTransports_EmptyArray(t *testing.T) {
	cred := &WebAuthnCredential{Transports: "[]"}
	assert.Nil(t, cred.GetTransports())
}

func TestGetTransports_USB(t *testing.T) {
	cred := &WebAuthnCredential{Transports: `["usb"]`}
	transports := cred.GetTransports()
	assert.Len(t, transports, 1)
	assert.Equal(t, protocol.USB, transports[0])
}

func TestGetTransports_Internal(t *testing.T) {
	cred := &WebAuthnCredential{Transports: `["internal"]`}
	transports := cred.GetTransports()
	assert.Len(t, transports, 1)
	assert.Equal(t, protocol.Internal, transports[0])
}

func TestGetTransports_Hybrid(t *testing.T) {
	cred := &WebAuthnCredential{Transports: `["hybrid"]`}
	transports := cred.GetTransports()
	assert.Len(t, transports, 1)
	assert.Equal(t, protocol.Hybrid, transports[0])
}

func TestGetTransports_NFC(t *testing.T) {
	cred := &WebAuthnCredential{Transports: `["nfc"]`}
	transports := cred.GetTransports()
	assert.Len(t, transports, 1)
	assert.Equal(t, protocol.NFC, transports[0])
}

func TestGetTransports_BLE(t *testing.T) {
	cred := &WebAuthnCredential{Transports: `["ble"]`}
	transports := cred.GetTransports()
	assert.Len(t, transports, 1)
	assert.Equal(t, protocol.BLE, transports[0])
}

func TestGetTransports_Multiple(t *testing.T) {
	cred := &WebAuthnCredential{Transports: `["usb","internal","hybrid"]`}
	transports := cred.GetTransports()
	assert.Len(t, transports, 3)
	assert.Contains(t, transports, protocol.USB)
	assert.Contains(t, transports, protocol.Internal)
	assert.Contains(t, transports, protocol.Hybrid)
}

func TestGetTransports_AllFive(t *testing.T) {
	cred := &WebAuthnCredential{Transports: `["usb","nfc","ble","internal","hybrid"]`}
	transports := cred.GetTransports()
	assert.Len(t, transports, 5)
}

// =============================================================================
// Round-trip: ToWebAuthnCredential → GetTransports
// =============================================================================

func TestTransportRoundTrip(t *testing.T) {
	cred := &webauthn.Credential{
		ID:        []byte("cred"),
		PublicKey: []byte("pk"),
		Transport: []protocol.AuthenticatorTransport{
			protocol.USB,
			protocol.Internal,
			protocol.Hybrid,
		},
		Authenticator: webauthn.Authenticator{},
	}

	model := ToWebAuthnCredential(1, cred, "test")
	transports := model.GetTransports()
	assert.Len(t, transports, 3)
	assert.Contains(t, transports, protocol.USB)
	assert.Contains(t, transports, protocol.Internal)
	assert.Contains(t, transports, protocol.Hybrid)
}
