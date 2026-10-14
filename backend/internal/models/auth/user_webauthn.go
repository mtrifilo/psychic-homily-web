package auth

import (
	"encoding/binary"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

// WebAuthnID returns the user's ID as bytes for WebAuthn
// Implements webauthn.User interface
func (u *User) WebAuthnID() []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(u.ID))
	return buf
}

// UserIDFromWebAuthnID decodes a user handle produced by WebAuthnID back to
// the users.id it encodes. It reports false for a handle of any other length
// or one that encodes zero.
func UserIDFromWebAuthnID(handle []byte) (uint, bool) {
	if len(handle) != 8 {
		return 0, false
	}
	id := binary.BigEndian.Uint64(handle)
	if id == 0 {
		return 0, false
	}
	return uint(id), true
}

// WebAuthnName returns the user's email or username for WebAuthn
// Implements webauthn.User interface
func (u *User) WebAuthnName() string {
	if u.Email != nil && *u.Email != "" {
		return *u.Email
	}
	if u.Username != nil && *u.Username != "" {
		return *u.Username
	}
	return ""
}

// WebAuthnDisplayName returns the user's display name for WebAuthn
// Implements webauthn.User interface
func (u *User) WebAuthnDisplayName() string {
	if u.FirstName != nil && *u.FirstName != "" {
		name := *u.FirstName
		if u.LastName != nil && *u.LastName != "" {
			name += " " + *u.LastName
		}
		return name
	}
	return u.WebAuthnName()
}

// WebAuthnCredentials returns the user's WebAuthn credentials
// Implements webauthn.User interface
func (u *User) WebAuthnCredentials() []webauthn.Credential {
	credentials := make([]webauthn.Credential, len(u.PasskeyCredentials))
	for i, cred := range u.PasskeyCredentials {
		credentials[i] = webauthn.Credential{
			ID:              cred.CredentialID,
			PublicKey:       cred.PublicKey,
			AttestationType: cred.AttestationType,
			Authenticator: webauthn.Authenticator{
				AAGUID:       aaguidBytes(cred.AAGUID),
				SignCount:    cred.SignCount,
				CloneWarning: cred.CloneWarning,
			},
			Flags: webauthn.CredentialFlags{
				BackupEligible: cred.BackupEligible,
				BackupState:    cred.BackupState,
			},
		}
	}
	return credentials
}

// WebAuthnIcon returns an optional icon URL for the user
// Implements webauthn.User interface
func (u *User) WebAuthnIcon() string {
	if u.AvatarURL != nil {
		return *u.AvatarURL
	}
	return ""
}

// ToWebAuthnCredential converts a webauthn.Credential to a WebAuthnCredential model
func ToWebAuthnCredential(userID uint, cred *webauthn.Credential, displayName string) *WebAuthnCredential {
	transports := ""
	if len(cred.Transport) > 0 {
		// Convert transports to JSON array string
		transports = "["
		for i, t := range cred.Transport {
			if i > 0 {
				transports += ","
			}
			transports += "\"" + string(t) + "\""
		}
		transports += "]"
	}

	return &WebAuthnCredential{
		UserID:          userID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		SignCount:       cred.Authenticator.SignCount,
		AAGUID:          aaguidString(cred.Authenticator.AAGUID),
		CloneWarning:    cred.Authenticator.CloneWarning,
		AttestationType: cred.AttestationType,
		Transports:      transports,
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
		DisplayName:     displayName,
	}
}

// aaguidString renders an authenticator's 16-byte AAGUID in the canonical
// UUID form the aaguid column holds. A value that is not 16 bytes has no such
// form and is stored as empty.
func aaguidString(raw []byte) string {
	id, err := uuid.FromBytes(raw)
	if err != nil {
		return ""
	}
	return id.String()
}

// aaguidBytes is the inverse of aaguidString; an empty or unparseable value
// yields nil.
func aaguidBytes(s string) []byte {
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return id[:]
}

// GetTransports parses the transports JSON and returns protocol.AuthenticatorTransport slice
func (c *WebAuthnCredential) GetTransports() []protocol.AuthenticatorTransport {
	// Simple JSON parsing for transport array
	if c.Transports == "" || c.Transports == "[]" {
		return nil
	}

	// This is a simplified parser - you may want to use json.Unmarshal for production
	transports := []protocol.AuthenticatorTransport{}

	// Check for common transports
	if contains(c.Transports, "usb") {
		transports = append(transports, protocol.USB)
	}
	if contains(c.Transports, "nfc") {
		transports = append(transports, protocol.NFC)
	}
	if contains(c.Transports, "ble") {
		transports = append(transports, protocol.BLE)
	}
	if contains(c.Transports, "internal") {
		transports = append(transports, protocol.Internal)
	}
	if contains(c.Transports, "hybrid") {
		transports = append(transports, protocol.Hybrid)
	}

	return transports
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
