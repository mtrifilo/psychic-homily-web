package contracts

import (
	"context"
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

// MagicLinkTokenTTL is how long an emailed sign-in link works after it is
// sent, for a plain magic link and for one that proves the mailbox for a
// pending provider link alike. The frontend's sent-state copy states it
// (MAGIC_LINK_EXPIRY_MINUTES in frontend/lib/magic-link.ts).
const MagicLinkTokenTTL = 15 * time.Minute

// OAuthLinkTokenClaims contains JWT claims for the emailed sign-in link that
// proves the mailbox for a pending provider link. PendingLink is that pending
// link's reference (OAuthPendingLink.Ref).
type OAuthLinkTokenClaims struct {
	UserID      uint   `json:"user_id"`
	Email       string `json:"email"`
	PendingLink string `json:"pending_link"`
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

// ParseEmailVerifiedClaim reads one provider's email-verification flag. A JSON
// bool and the strings "true"/"false" both decode, because AppleIdentityToken-
// Claims.EmailVerified is typed any for exactly that reason.
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

// AppleKeyNotFoundError refuses an Apple identity token whose key id Apple's
// key set does not hold. KeyID is the key id exactly as the token's header
// carries it, which is caller-supplied; Error leaves it out, so whoever logs
// the error decides how much of it to show.
type AppleKeyNotFoundError struct{ KeyID string }

func (e *AppleKeyNotFoundError) Error() string {
	return "apple public key not found for the token's key id"
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
	// AgeConfirmedAt is when the age was given and MinAgeAttested the age in
	// years: computed from DateOfBirth on the password, passkey and "Create
	// your account" paths, attested on Apple's. A zero MinAgeAttested means no
	// age was recorded.
	AgeConfirmedAt time.Time
	MinAgeAttested int
	// DateOfBirth is the date the age was computed from, nil on a path that
	// asks for an attestation rather than a date.
	DateOfBirth *time.Time
}

// SignupChallenge is what a passkey signup's begin step hands its finish step
// through the stored challenge: the address the account is created for and
// the consent begin validated. Finish records exactly these and reads neither
// from its own request.
type SignupChallenge struct {
	Email      string
	Acceptance LegalAcceptance
}

// OAuthSignupConsent carries consent details from OAuth signup initiation.
type OAuthSignupConsent struct {
	TermsAccepted  bool
	TermsVersion   string
	PrivacyVersion string
	AcceptedAt     time.Time
	// AgeConfirmed / MinAgeAttested capture the age an OAuth sign-up gave, in
	// years, mirroring TermsAccepted.
	AgeConfirmed   bool
	MinAgeAttested int
	// DateOfBirth is the date MinAgeAttested was computed from, nil when the
	// age was attested rather than computed.
	DateOfBirth *time.Time
}

// OAuthIdentity is what a provider asserted about the person who completed a
// handshake: the subject the identity resolves on, the address and whether
// the provider vouched for it, and the names and avatar it reported.
type OAuthIdentity struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	// EmailAuthoritative is set when the provider is the authority for the
	// mailbox (it hosts the address), not only a witness that it verified it
	// once. Only such an identity is offered a pending link to the account
	// that holds its address. It is read at the callback and not stored.
	EmailAuthoritative bool
	Name               string
	FirstName          string
	LastName           string
	AvatarURL          string
}

// OAuthSignupRequiredError is the refusal for an identity attached to no
// account, whose address no account holds, that arrived with no consent. It
// carries the identity, so a caller that can collect the consent itself can
// hold on to it, and it unwraps to Refusal, the typed consent refusal a caller
// that cannot collect consent reports as it is.
type OAuthSignupRequiredError struct {
	Identity OAuthIdentity
	Refusal  error
}

func (e *OAuthSignupRequiredError) Error() string {
	return "new oauth identity needs consent before an account is created"
}

func (e *OAuthSignupRequiredError) Unwrap() error {
	return e.Refusal
}

// OAuthLinkRequiredError is the refusal for an identity attached to no
// account whose address, which the provider verified and is the authority
// for (OAuthIdentity.EmailAuthoritative), belongs to an account UserID that
// may sign in and holds no identity from that provider. Refusal is the
// CodeOAuthLinkRefused AuthError, so a caller that offers no way to prove the
// account answers with it unchanged; a caller that does holds Identity as a
// pending link for that account.
type OAuthLinkRequiredError struct {
	Identity OAuthIdentity
	UserID   uint
	Refusal  error
}

func (e *OAuthLinkRequiredError) Error() string {
	return "oauth identity's address belongs to an account it is not attached to"
}

func (e *OAuthLinkRequiredError) Unwrap() error {
	return e.Refusal
}

// OAuthPendingSignupTTL is how long a pending sign-up stays usable, and so the
// lifetime of the cookie that names it.
const OAuthPendingSignupTTL = 10 * time.Minute

// OAuthPendingLinkTTL is how long a pending link stays usable from the
// browser that started it, and so the lifetime of the cookie that names it.
// Asking for its emailed link keeps the pending link alive for as long as
// that link can be delivered and followed (HoldOAuthLinkForEmail); the
// browser's cookie still lapses at OAuthPendingLinkTTL.
const OAuthPendingLinkTTL = OAuthPendingSignupTTL

// OAuthPendingLink is a provider identity held until the person proves they
// hold the account UserID names. Ref names it to the emailed sign-in link
// without being able to read or consume it from a browser.
type OAuthPendingLink struct {
	Identity  OAuthIdentity
	UserID    uint
	ExpiresAt time.Time
	Ref       string
}

// OAuthLinkCompletion is a pending link finished by the mailbox proof: the
// account the identity joined, the new session issued for it, and the
// sanitised destination the sign-in named.
type OAuthLinkCompletion struct {
	User     *authm.User
	Token    string
	ReturnTo string
}

// OAuthPendingSignup is a provider identity held for the "Create your
// account" step, with the sanitised destination its sign-in named.
type OAuthPendingSignup struct {
	Identity  OAuthIdentity
	ReturnTo  string
	ExpiresAt time.Time
}

// ──────────────────────────────────────────────
// Auth Service Interface
// ──────────────────────────────────────────────

// SignInFactors is what an account can sign in with besides its linked
// providers. It carries no secret: a password's presence, never its hash, and
// a passkey count, never the credentials.
type SignInFactors struct {
	HasPassword  bool
	PasskeyCount int
}

// AuthServiceInterface defines the contract for authentication operations.
type AuthServiceInterface interface {
	OAuthLogin(w http.ResponseWriter, r *http.Request, provider string) error
	// OAuthCallback finishes the provider handshake and signs in the account
	// the identity resolves to. An identity that would need an account created
	// is refused with an error wrapping *OAuthSignupRequiredError, whose
	// Refusal is the CodeTermsAcceptanceRequired AuthError; the other
	// refusals are UserServiceInterface.FindOrCreateUserWithConsent's.
	OAuthCallback(w http.ResponseWriter, r *http.Request, provider string) (*authm.User, string, error)
	// StartOAuthSignup stores identity and returnTo as a pending sign-up for
	// OAuthPendingSignupTTL and returns the signed token that names it, the
	// only value that can read or consume it.
	StartOAuthSignup(identity OAuthIdentity, returnTo string) (token string, err error)
	// PeekOAuthSignup returns the pending sign-up token names without
	// consuming it. A token that fails its signature, names no row, or names
	// an expired one is refused with a typed AuthError carrying
	// CodeOAuthSignupExpired; any other error means it could not be read.
	PeekOAuthSignup(token string) (*OAuthPendingSignup, error)
	// ConsumeOAuthSignup removes the pending sign-up token names and returns
	// it; every later call for the same token is refused. Refusals are
	// PeekOAuthSignup's.
	ConsumeOAuthSignup(token string) (*OAuthPendingSignup, error)
	// StartOAuthLink stores identity as a pending link to the account userID
	// names, with returnTo, for OAuthPendingLinkTTL, and returns the signed
	// token that names it. A pending sign-up's token never reads a pending
	// link, nor the reverse.
	StartOAuthLink(identity OAuthIdentity, userID uint, returnTo string) (token string, err error)
	// PeekOAuthLink returns the pending link token names without consuming
	// it. A token that fails its signature, names no pending link, or names an
	// expired one is refused with a typed AuthError carrying
	// CodeOAuthLinkPendingExpired; any other error means it could not be read.
	PeekOAuthLink(token string) (*OAuthPendingLink, error)
	// HoldOAuthLinkForEmail is PeekOAuthLink for the mailbox proof's send: it
	// also keeps the pending link alive for as long as the emailed link it is
	// about to be named by can be delivered and followed. Refusals are
	// PeekOAuthLink's.
	HoldOAuthLinkForEmail(token string) (*OAuthPendingLink, error)
	// LinkOAuthWithPassword finishes the pending link token names for user,
	// whose password the caller has just verified. It consumes the pending
	// link whatever it names. When it names user, and user's address is still
	// the identity's, the identity is attached to user in the transaction that
	// consumes it and linked is true. A pending link for another account is
	// discarded and linked is false. Refusals: PeekOAuthLink's, and
	// CodeOAuthIdentityInUse or CodeOAuthProviderAlreadyLinked when the
	// identity or the provider was attached since the pending link began.
	LinkOAuthWithPassword(token string, user *authm.User) (linked bool, err error)
	// CompleteOAuthLinkByEmail finishes the pending link an emailed sign-in
	// link names. In one transaction it consumes the pending link, attaches
	// the identity, clears the account's password, marks its address
	// verified, and revokes every session the account holds; then it issues a
	// new session, which the revocation does not reach. Refusals:
	// CodeOAuthLinkPendingExpired when the pending link is used, expired, or
	// not the claims' account, shared.ClosedAccountRefusal when the account
	// may not sign in, and LinkOAuthWithPassword's link refusals.
	CompleteOAuthLinkByEmail(claims *OAuthLinkTokenClaims) (*OAuthLinkCompletion, error)
	// CompleteOAuthSignup resolves or creates the account for a consumed
	// pending sign-up through
	// UserServiceInterface.FindOrCreateUserFromOAuthIdentity, and issues a
	// session for it. Refusals are that method's.
	CompleteOAuthSignup(signup *OAuthPendingSignup, consent *OAuthSignupConsent, displayName *string) (*authm.User, string, error)
	// CompleteOAuthLink attaches the handshake's identity to an already
	// authenticated userID and issues no session. Refuses with a typed
	// AuthError carrying CodeAccountInactive when the account may not sign in
	// (authm.User.CanSignIn), and with the typed AuthErrors
	// UserServiceInterface.LinkOAuthAccountToUser documents.
	CompleteOAuthLink(w http.ResponseWriter, r *http.Request, provider string, userID uint) (*authm.User, error)
	GetUserProfile(userID uint) (*authm.User, error)
	// GetSignInFactors reports the sign-in factors user holds besides its
	// linked providers. Passkeys are counted with a query, never loaded.
	GetSignInFactors(user *authm.User) (SignInFactors, error)
	// RefreshUserToken renews a session, carrying authAt through unchanged.
	RefreshUserToken(user *authm.User, authAt time.Time) (string, error)
	Logout(w http.ResponseWriter, r *http.Request) error
	SetOAuthCompleter(completer OAuthCompleter)
}

// ──────────────────────────────────────────────
// JWT Service Interface
// ──────────────────────────────────────────────

// JWTServiceInterface defines the contract for JWT token operations.
type JWTServiceInterface interface {
	// CreateToken mints a session for a caller that has just completed an
	// authentication factor, stamping auth_at with the current time.
	CreateToken(user *authm.User) (string, error)
	// RenewSessionToken mints a session for a caller presenting an existing
	// session rather than a factor, carrying authAt through unchanged.
	RenewSessionToken(user *authm.User, authAt time.Time) (string, error)
	// ValidateSession is the reader that also yields the session's
	// authentication time; ValidateToken discards it.
	ValidateSession(tokenString string) (*authm.User, time.Time, error)
	ValidateSessionLenient(tokenString string, gracePeriod time.Duration) (*authm.User, time.Time, error)
	ValidateToken(tokenString string) (*authm.User, error)
	CreateVerificationToken(userID uint, email string) (string, error)
	ValidateVerificationToken(tokenString string) (*VerificationTokenClaims, error)
	CreateMagicLinkToken(userID uint, email string) (string, error)
	ValidateMagicLinkToken(tokenString string) (*MagicLinkTokenClaims, error)
	// ValidateOAuthLinkToken accepts only the emailed sign-in link that
	// proves the mailbox for a pending provider link; ValidateMagicLinkToken
	// refuses that link and this refuses a plain one.
	ValidateOAuthLinkToken(tokenString string) (*OAuthLinkTokenClaims, error)
	CreateAccountRecoveryToken(userID uint, email string) (string, error)
	ValidateAccountRecoveryToken(tokenString string) (*AccountRecoveryTokenClaims, error)
}

// ──────────────────────────────────────────────
// Apple Auth Service Interface
// ──────────────────────────────────────────────

// AppleAuthServiceInterface defines the contract for Apple authentication operations.
type AppleAuthServiceInterface interface {
	// Returns ctx's error when ctx ends while the lookup waits on a key-set
	// fetch. A token whose key id Apple's set lacks fails with an error that
	// wraps *AppleKeyNotFoundError. No error text carries the token or text
	// taken from its header or claims.
	ValidateIdentityToken(ctx context.Context, identityToken string) (*AppleIdentityTokenClaims, error)
	// Refuses with a typed AuthError carrying CodeOAuthLinkRefused when the
	// claims' address already belongs to an account this identity may not join
	// on the address alone, with CodeAccountRecoverable instead when
	// shared.AddressMatchIsRecoverable holds, and with
	// shared.ClosedAccountRefusal when the identity resolves to an account that
	// may not sign in. Creates with email_verified from the claim, so an
	// address Apple will not vouch for produces an unverified account. claims
	// must already be verified by the caller.
	//
	// consent is read only when the call would create an account: a known
	// Apple identity signs in whatever consent carries, nil included. A
	// creation whose consent fails shared.ValidateOAuthSignupConsent is refused
	// with that typed AuthError (CodeTermsAcceptanceRequired or
	// CodeAgeConfirmationRequired) and writes nothing. An email claim Postgres
	// rejects as text is refused with CodeValidationFailed before any query.
	FindOrCreateAppleUser(claims *AppleIdentityTokenClaims, firstName, lastName string, consent *OAuthSignupConsent) (*authm.User, error)
	GenerateToken(user *authm.User) (string, error)
}

// ──────────────────────────────────────────────
// WebAuthn Service Interface
// ──────────────────────────────────────────────

// WebAuthnRegistrationOperation is the operation a passkey registration's
// challenge is stored under. Only ConsumeRegistrationChallenge consumes it.
const WebAuthnRegistrationOperation = "registration"

// WebAuthnServiceInterface defines the contract for WebAuthn/passkey operations.
type WebAuthnServiceInterface interface {
	BeginRegistration(user *authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error)
	// FinishRegistration answers a registration response that fails
	// verification with an *errors.AuthError coded VALIDATION_FAILED, and an
	// account that may not sign in when the credential is written with one
	// coded ACCOUNT_INACTIVE; each message is the client's copy. Any other
	// error means the credential could not be stored.
	FinishRegistration(user *authm.User, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*authm.WebAuthnCredential, error)
	BeginDiscoverableLogin() (*protocol.CredentialAssertion, *webauthn.SessionData, error)
	// FinishDiscoverableLogin answers a refused sign-in with an
	// *errors.AuthError coded INVALID_CREDENTIALS, or with
	// shared.ClosedAccountRefusal (ACCOUNT_RECOVERABLE or ACCOUNT_INACTIVE)
	// when the assertion proves a credential of an account that may not sign
	// in; each message is the client's copy. Any other error means no decision
	// was reached. On success it returns the account the sign-in was decided on.
	FinishDiscoverableLogin(session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error)
	GetUserCredentials(userID uint) ([]authm.WebAuthnCredential, error)
	DeleteCredential(userID uint, credentialID uint) error
	UpdateCredentialName(userID uint, credentialID uint, displayName string) error
	StoreChallenge(userID uint, session *webauthn.SessionData, operation string) (string, error)
	// ConsumeChallenge answers a challenge that does not exist or has expired
	// with an *errors.AuthError coded VALIDATION_FAILED, whose message is the
	// client's copy. Any other error means the challenge could not be read, or
	// the operation was WebAuthnRegistrationOperation, whose challenges only
	// ConsumeRegistrationChallenge consumes.
	ConsumeChallenge(challengeID string, operation string) (*webauthn.SessionData, error)
	// ConsumeRegistrationChallenge spends a registration challenge only for the
	// user it was issued to. It answers a refusal with an *errors.AuthError
	// coded UNAUTHORIZED (an unexpired challenge issued to another user) or
	// VALIDATION_FAILED (absent or expired), whose message is the client's
	// copy. Any other error means the challenge could not be read.
	ConsumeRegistrationChallenge(challengeID string, userID uint) (*webauthn.SessionData, error)
	BeginRegistrationForEmail(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error)
	StoreSignupChallenge(signup SignupChallenge, session *webauthn.SessionData, operation string) (string, error)
	// ConsumeSignupChallenge's errors are ConsumeChallenge's.
	ConsumeSignupChallenge(challengeID string, operation string) (*webauthn.SessionData, SignupChallenge, error)
	// FinishSignupRegistrationWithLegal answers a refusal with an
	// *errors.AuthError coded TERMS_ACCEPTANCE_REQUIRED (an acceptance that
	// names no terms version), VALIDATION_FAILED (a registration response that
	// fails verification, or whose credential id an account already holds) or
	// USER_EXISTS (an address an account already holds), whose message is the
	// client's copy. Any other error means the account was not created.
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

// ──────────────────────────────────────────────
// Auth Email Outbox Interface
// ──────────────────────────────────────────────

// AuthEmailOutboxInterface queues a magic-link, account-recovery or
// verification email for an account. A background poller sends it, so a request handler that calls
// Enqueue never waits on the email provider.
type AuthEmailOutboxInterface interface {
	// Enqueue queues one email of kind for userID. A call while one is still
	// queued for the same account and kind is a no-op that returns nil. It
	// returns an error when the row could not be written or the outbox is
	// switched off; in both cases no email is sent for this call.
	Enqueue(ctx context.Context, kind authm.AuthEmailKind, userID uint) error
	// EnqueueOAuthLink queues the sign-in link that proves userID's mailbox
	// for the pending link ref names (OAuthPendingLink.Ref), on Enqueue's
	// terms: one queued per account, and an error means no email for this
	// call.
	EnqueueOAuthLink(ctx context.Context, userID uint, ref string) error
}
