package contracts

import (
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/markbates/goth"
)

// ──────────────────────────────────────────────
// Auth / JWT / Apple / WebAuthn / Password types
// ──────────────────────────────────────────────

// OAuthCompleter interface for mocking OAuth completion
type OAuthCompleter interface {
	CompleteUserAuth(w http.ResponseWriter, r *http.Request) (goth.User, error)
}

// VerificationTokenClaims contains JWT claims for email verification tokens.
type VerificationTokenClaims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// MagicLinkTokenClaims contains JWT claims for magic link login tokens.
type MagicLinkTokenClaims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// AccountRecoveryTokenClaims contains JWT claims for account recovery tokens.
type AccountRecoveryTokenClaims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// AppleIdentityTokenClaims represents the claims in an Apple identity token
type AppleIdentityTokenClaims struct {
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"` // Apple sends this as string "true" or bool
	jwt.RegisteredClaims
}

// IsEmailVerified returns whether the email is verified, handling both string and bool types
func (c *AppleIdentityTokenClaims) IsEmailVerified() bool {
	switch v := c.EmailVerified.(type) {
	case bool:
		return v
	case string:
		return v == "true"
	default:
		return false
	}
}

// PasswordValidationResult contains the result of password validation
type PasswordValidationResult struct {
	Valid    bool
	Errors   []string
	Warnings []string
}

// LegalAcceptance records terms/privacy agreement metadata at account creation.
type LegalAcceptance struct {
	TermsAcceptedAt time.Time
	TermsVersion    string
	PrivacyVersion  string
}

// OAuthSignupConsent carries consent details from OAuth signup initiation.
type OAuthSignupConsent struct {
	TermsAccepted  bool
	TermsVersion   string
	PrivacyVersion string
	AcceptedAt     time.Time
}
