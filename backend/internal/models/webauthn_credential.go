package models

import (
	"time"
)

// WebAuthnCredential represents a WebAuthn/passkey credential
type WebAuthnCredential struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"index;not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// WebAuthn credential data
	CredentialID []byte `json:"-" gorm:"type:bytea;not null"`          // The credential ID (hidden from JSON for security)
	PublicKey    []byte `json:"-" gorm:"type:bytea;not null"`          // The public key (hidden from JSON for security)
	SignCount    uint32 `json:"sign_count" gorm:"default:0"`           // Counter to prevent replay attacks
	AAGUID       string `json:"aaguid" gorm:"type:varchar(36)"`        // Authenticator Attestation GUID
	CloneWarning bool   `json:"clone_warning" gorm:"default:false"`    // Flag if credential may be cloned

	// Attestation and transport info
	AttestationType string `json:"attestation_type" gorm:"type:varchar(50)"` // e.g., "none", "direct", "indirect"
	Transports      string `json:"transports" gorm:"type:text"`              // JSON array of transports (usb, nfc, ble, internal)

	// Backup state (for passkey sync)
	BackupEligible bool `json:"backup_eligible" gorm:"default:false"` // Whether credential can be backed up
	BackupState    bool `json:"backup_state" gorm:"default:false"`    // Whether credential is currently backed up

	// User-friendly info
	DisplayName string     `json:"display_name" gorm:"type:varchar(255)"` // User-provided name for the credential
	LastUsedAt  *time.Time `json:"last_used_at"`                          // Last time this credential was used

	// Relationships
	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for WebAuthnCredential
func (WebAuthnCredential) TableName() string {
	return "webauthn_credentials"
}

// WebAuthnChallenge represents a temporary challenge for WebAuthn operations
// This can be stored in session/cache with TTL instead of database
type WebAuthnChallenge struct {
	ID          string    `json:"id" gorm:"primaryKey;type:varchar(64)"`
	UserID      uint      `json:"user_id" gorm:"index;not null"`
	Challenge   []byte    `json:"-" gorm:"type:bytea;not null"` // Hidden from JSON
	SessionData []byte    `json:"-" gorm:"type:bytea"`          // WebAuthn session data (hidden)
	Operation   string    `json:"operation" gorm:"type:varchar(20);not null"` // "registration" or "authentication"
	ExpiresAt   time.Time `json:"expires_at" gorm:"not null"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName specifies the table name for WebAuthnChallenge
func (WebAuthnChallenge) TableName() string {
	return "webauthn_challenges"
}

// IsExpired checks if the challenge has expired
func (c *WebAuthnChallenge) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}
