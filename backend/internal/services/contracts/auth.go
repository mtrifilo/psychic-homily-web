package contracts

import (
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"
	"github.com/markbates/goth"

	authm "psychic-homily-backend/internal/models/auth"
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

// IsEmailVerified reports whether Apple asserted it verified the address.
func (c *AppleIdentityTokenClaims) IsEmailVerified() bool {
	verified, _ := ParseEmailVerifiedClaim(c.EmailVerified)
	return verified
}

// ParseEmailVerifiedClaim reads one provider's email-verification flag. It
// arrives as a JSON bool from an OIDC userinfo response and as the string
// "true"/"false" from an Apple identity token, so both shapes decode here.
//
// present is false for a nil claim and for any other shape, which keeps an
// unreadable value from being mistaken for an assertion in either direction.
// Callers that treat "the provider said nothing" the same as "the provider
// said no" can ignore it.
func ParseEmailVerifiedClaim(raw any) (verified bool, present bool) {
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		switch v {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
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
	// AgeConfirmedAt / MinAgeAttested mirror the terms fields for the required
	// "I am at least N years old" confirmation (PSY-1023). A zero MinAgeAttested
	// means the caller did not record an age confirmation.
	AgeConfirmedAt time.Time
	MinAgeAttested int
}

// OAuthSignupConsent carries consent details from OAuth signup initiation.
type OAuthSignupConsent struct {
	TermsAccepted  bool
	TermsVersion   string
	PrivacyVersion string
	AcceptedAt     time.Time
	// AgeConfirmed / MinAgeAttested capture the required age confirmation for
	// OAuth signups (PSY-1023), mirroring TermsAccepted.
	AgeConfirmed   bool
	MinAgeAttested int
}

// ──────────────────────────────────────────────
// Auth Service Interface
// ──────────────────────────────────────────────

// AuthServiceInterface defines the contract for authentication operations.
type AuthServiceInterface interface {
	OAuthLogin(w http.ResponseWriter, r *http.Request, provider string) error
	OAuthCallback(w http.ResponseWriter, r *http.Request, provider string) (*authm.User, string, error)
	OAuthCallbackWithConsent(w http.ResponseWriter, r *http.Request, provider string, consent *OAuthSignupConsent) (*authm.User, string, error)
	GetUserProfile(userID uint) (*authm.User, error)
	RefreshUserToken(user *authm.User) (string, error)
	Logout(w http.ResponseWriter, r *http.Request) error
	SetOAuthCompleter(completer OAuthCompleter)
}

// ──────────────────────────────────────────────
// JWT Service Interface
// ──────────────────────────────────────────────

// JWTServiceInterface defines the contract for JWT token operations.
type JWTServiceInterface interface {
	CreateToken(user *authm.User) (string, error)
	ValidateToken(tokenString string) (*authm.User, error)
	RefreshToken(tokenString string) (string, error)
	ValidateTokenLenient(tokenString string, gracePeriod time.Duration) (*authm.User, error)
	CreateVerificationToken(userID uint, email string) (string, error)
	ValidateVerificationToken(tokenString string) (*VerificationTokenClaims, error)
	CreateMagicLinkToken(userID uint, email string) (string, error)
	ValidateMagicLinkToken(tokenString string) (*MagicLinkTokenClaims, error)
	CreateAccountRecoveryToken(userID uint, email string) (string, error)
	ValidateAccountRecoveryToken(tokenString string) (*AccountRecoveryTokenClaims, error)
}

// ──────────────────────────────────────────────
// Apple Auth Service Interface
// ──────────────────────────────────────────────

// AppleAuthServiceInterface defines the contract for Apple authentication operations.
type AppleAuthServiceInterface interface {
	ValidateIdentityToken(identityToken string) (*AppleIdentityTokenClaims, error)
	// Refuses with a typed AuthError carrying CodeUserExists when the claims'
	// address already belongs to an account and they do not assert Apple
	// verified it. claims must already be verified by the caller.
	FindOrCreateAppleUser(claims *AppleIdentityTokenClaims, firstName, lastName string) (*authm.User, error)
	GenerateToken(user *authm.User) (string, error)
}

// ──────────────────────────────────────────────
// WebAuthn Service Interface
// ──────────────────────────────────────────────

// WebAuthnServiceInterface defines the contract for WebAuthn/passkey operations.
type WebAuthnServiceInterface interface {
	BeginRegistration(user *authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error)
	FinishRegistration(user *authm.User, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*authm.WebAuthnCredential, error)
	BeginLogin(user *authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error)
	BeginDiscoverableLogin() (*protocol.CredentialAssertion, *webauthn.SessionData, error)
	FinishLogin(user *authm.User, session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*authm.WebAuthnCredential, error)
	FinishDiscoverableLogin(session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error)
	GetUserCredentials(userID uint) ([]authm.WebAuthnCredential, error)
	DeleteCredential(userID uint, credentialID uint) error
	UpdateCredentialName(userID uint, credentialID uint, displayName string) error
	StoreChallenge(userID uint, session *webauthn.SessionData, operation string) (string, error)
	GetChallenge(challengeID string, operation string) (*webauthn.SessionData, uint, error)
	DeleteChallenge(challengeID string) error
	CleanupExpiredChallenges() error
	BeginRegistrationForEmail(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error)
	StoreChallengeWithEmail(email string, session *webauthn.SessionData, operation string) (string, error)
	GetChallengeWithEmail(challengeID string, operation string) (*webauthn.SessionData, string, error)
	FinishSignupRegistration(email string, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*authm.User, error)
	FinishSignupRegistrationWithLegal(email string, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string, acceptance LegalAcceptance) (*authm.User, error)
}

// ──────────────────────────────────────────────
// Password Validator Interface
// ──────────────────────────────────────────────

// PasswordValidatorInterface defines the contract for password validation operations.
type PasswordValidatorInterface interface {
	ValidatePassword(password string) (*PasswordValidationResult, error)
	IsBreached(password string) (bool, error)
	IsCommonPassword(password string) bool
}
