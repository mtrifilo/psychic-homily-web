package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// WebAuthnService handles WebAuthn/passkey operations
type WebAuthnService struct {
	webAuthn *webauthn.WebAuthn
	db       *gorm.DB
	config   *config.Config
}

// NewWebAuthnService creates a new WebAuthn service
func NewWebAuthnService(database *gorm.DB, cfg *config.Config) (*WebAuthnService, error) {
	if database == nil {
		database = db.GetDB()
	}

	// Get RPID and origins from config
	rpID := cfg.WebAuthn.RPID
	if rpID == "" {
		rpID = "localhost"
	}

	rpOrigins := cfg.WebAuthn.RPOrigins
	if len(rpOrigins) == 0 {
		rpOrigins = []string{cfg.Email.FrontendURL}
	}

	wconfig := &webauthn.Config{
		RPDisplayName: cfg.WebAuthn.RPDisplayName,
		RPID:          rpID,
		RPOrigins:     rpOrigins,
		// Attestation preference - "none" is fine for most use cases
		AttestationPreference: protocol.PreferNoAttestation,
		// AuthenticatorAttachment is left unset so the browser offers every
		// kind of authenticator: platform ones (Touch ID, Face ID, Windows
		// Hello) as well as security keys and phones. Setting it to either
		// value hides the other kind from the ceremony.
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationPreferred,
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
		},
	}

	w, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create webauthn: %w", err)
	}

	return &WebAuthnService{
		webAuthn: w,
		db:       database,
		config:   cfg,
	}, nil
}

// BeginRegistration starts the passkey registration process
func (s *WebAuthnService) BeginRegistration(user *authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	// Get existing credentials to exclude
	var existingCreds []authm.WebAuthnCredential
	if err := s.db.Where("user_id = ?", user.ID).Find(&existingCreds).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to get existing credentials: %w", err)
	}

	// Convert to webauthn credentials for exclusion
	excludeList := make([]protocol.CredentialDescriptor, len(existingCreds))
	for i, cred := range existingCreds {
		excludeList[i] = protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: cred.CredentialID,
			Transport:    cred.GetTransports(),
		}
	}

	// Create registration options
	options, session, err := s.webAuthn.BeginRegistration(
		user,
		webauthn.WithExclusions(excludeList),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin registration: %w", err)
	}

	return options, session, nil
}

// invalidRegistrationMessage is the user-facing message of the
// CodeValidationFailed refusals FinishRegistration and
// FinishSignupRegistrationWithLegal return for a registration that fails
// verification.
const invalidRegistrationMessage = "Invalid credential response"

// refuseRegistration is the refusal for a registration response that fails
// verification against its session: a signature, origin, challenge, relying
// party or flag the ceremony does not accept.
func refuseRegistration(internal error) *apperrors.AuthError {
	return apperrors.NewAuthError(apperrors.CodeValidationFailed, invalidRegistrationMessage, internal)
}

// FinishRegistration verifies a registration response for user and stores
// the credential it creates.
//
// A refusal is a CodeValidationFailed *apperrors.AuthError for a response
// that fails verification. Any other error means the credential could not be
// stored.
func (s *WebAuthnService) FinishRegistration(user *authm.User, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*authm.WebAuthnCredential, error) {
	credential, err := s.webAuthn.CreateCredential(user, *session, response)
	if err != nil {
		return nil, refuseRegistration(fmt.Errorf("create credential: %w", err))
	}

	// Convert to our model
	webauthnCred := authm.ToWebAuthnCredential(user.ID, credential, displayName)

	// Save to database
	if err := s.db.Create(webauthnCred).Error; err != nil {
		return nil, fmt.Errorf("failed to save credential: %w", err)
	}

	return webauthnCred, nil
}

// noPasskeysMessage is the user-facing message of BeginLogin's refusal.
const noPasskeysMessage = "No passkeys registered for this account"

func refuseLoginBegin(internal error) *apperrors.AuthError {
	return apperrors.NewAuthError(apperrors.CodeInvalidCredentials, noPasskeysMessage, internal)
}

// BeginLogin starts an email-specific passkey login for user, loading the
// account's credentials into user so the options name them.
//
// A refusal is a CodeInvalidCredentials *apperrors.AuthError for an account
// that does not exist or holds no passkey. Any other error means the account
// could not be read or the ceremony could not start.
func (s *WebAuthnService) BeginLogin(user *authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	if user == nil {
		return nil, nil, fmt.Errorf("no user to begin login for")
	}
	if err := s.db.Preload("PasskeyCredentials").First(user, user.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, refuseLoginBegin(fmt.Errorf("user %d not found: %w", user.ID, err))
		}
		return nil, nil, fmt.Errorf("failed to load user: %w", err)
	}

	if len(user.PasskeyCredentials) == 0 {
		return nil, nil, refuseLoginBegin(fmt.Errorf("user %d has no registered passkeys", user.ID))
	}

	// Create login options
	options, session, err := s.webAuthn.BeginLogin(user)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin login: %w", err)
	}

	return options, session, nil
}

// BeginDiscoverableLogin starts a discoverable (usernameless) login
func (s *WebAuthnService) BeginDiscoverableLogin() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	options, session, err := s.webAuthn.BeginDiscoverableLogin()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin discoverable login: %w", err)
	}

	return options, session, nil
}

// invalidPasskeyMessage is the user-facing message of the
// CodeInvalidCredentials refusals FinishLogin and FinishDiscoverableLogin
// return, except FinishLogin's for an account that does not exist, which is
// accountNotFoundMessage.
const invalidPasskeyMessage = "Invalid passkey"

// accountNotFoundMessage is the user-facing message of FinishLogin's refusal
// for an account that does not exist.
const accountNotFoundMessage = "User not found"

func refusePasskey(internal error) *apperrors.AuthError {
	return apperrors.NewAuthError(apperrors.CodeInvalidCredentials, invalidPasskeyMessage, internal)
}

// refuseInactiveAccount is apperrors.ErrAccountInactive carrying the refused
// account's id in its internal error, which callers log and clients never see.
func refuseInactiveAccount(userID uint) *apperrors.AuthError {
	inactive := apperrors.ErrAccountInactive()
	inactive.Internal = fmt.Errorf("user %d may not sign in", userID)
	return inactive
}

// signInLookupError is the error for a sign-in lookup of what that returned
// err: a refusal when the row does not exist, a failure otherwise.
func signInLookupError(what string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return refusePasskey(fmt.Errorf("%s not found: %w", what, err))
	}
	return fmt.Errorf("failed to load %s: %w", what, err)
}

// FinishLogin completes an email-specific passkey login for the account
// user.ID and returns that account, as loaded for the decision with its
// OAuth accounts and preferences and without its credentials, and the
// credential it used.
//
// Its errors come in two kinds. A refusal is an *apperrors.AuthError:
// CodeInvalidCredentials when the assertion does not prove a credential of an
// existing account (it fails validation, carries another account's user
// handle, or names a credential or account that does not exist), and
// CodeAccountInactive when it does but the account may not sign in
// (authm.User.CanSignIn). Any other error means no decision was reached: a
// lookup or the credential update failed.
//
// The account's state is checked only after the assertion validated, so only
// the credential's holder learns it, and a refused sign-in writes nothing. A
// clone warning (a sign count that did not advance) is recorded on the
// credential and does not refuse the sign-in.
func (s *WebAuthnService) FinishLogin(user *authm.User, session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error) {
	// Validation checks the assertion against the account's stored
	// credentials, which the caller's copy of the user need not hold, so it
	// runs against a fresh load and leaves the caller's copy untouched. The
	// account state the sign-in is decided on, and the account returned, are
	// this load's.
	var owner authm.User
	if err := s.db.Preload("PasskeyCredentials").Preload("OAuthAccounts").Preload("Preferences").
		First(&owner, user.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apperrors.NewAuthError(apperrors.CodeInvalidCredentials, accountNotFoundMessage, err)
		}
		return nil, nil, signInLookupError("user", err)
	}

	credential, err := s.webAuthn.ValidateLogin(&owner, *session, response)
	if err != nil {
		return nil, nil, refusePasskey(fmt.Errorf("validate login: %w", err))
	}
	if !owner.CanSignIn() {
		return nil, nil, refuseInactiveAccount(owner.ID)
	}

	var webauthnCred authm.WebAuthnCredential
	if err := s.db.Where("user_id = ? AND credential_id = ?", owner.ID, credential.ID).First(&webauthnCred).Error; err != nil {
		return nil, nil, signInLookupError("credential", err)
	}
	if err := s.recordCredentialUse(&webauthnCred, credential); err != nil {
		return nil, nil, err
	}

	owner.PasskeyCredentials = nil
	return &owner, &webauthnCred, nil
}

// recordCredentialUse writes what a validated assertion changes on the stored
// credential (sign count, clone warning, backup state, last use) and nothing
// else. Backup eligibility cannot change: validation refuses an assertion
// whose eligibility differs from the stored value. A credential deleted since
// it was read is a refusal: it may no longer sign in.
func (s *WebAuthnService) recordCredentialUse(stored *authm.WebAuthnCredential, validated *webauthn.Credential) error {
	now := time.Now()
	stored.SignCount = validated.Authenticator.SignCount
	stored.CloneWarning = validated.Authenticator.CloneWarning
	stored.BackupState = validated.Flags.BackupState
	stored.LastUsedAt = &now
	result := s.db.Model(stored).Select("sign_count", "clone_warning", "backup_state", "last_used_at").Updates(stored)
	if result.Error != nil {
		return fmt.Errorf("failed to update credential: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return refusePasskey(fmt.Errorf("credential %d was deleted during sign-in", stored.ID))
	}
	return nil
}

// FinishDiscoverableLogin completes a discoverable login and returns the
// account it signs in and the credential it used. Its errors, and when it
// checks the account's state, are FinishLogin's.
func (s *WebAuthnService) FinishDiscoverableLogin(session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error) {
	var webauthnCred authm.WebAuthnCredential
	if err := s.db.Where("credential_id = ?", response.RawID).First(&webauthnCred).Error; err != nil {
		return nil, nil, signInLookupError("credential", err)
	}

	var user authm.User
	if err := s.db.Preload("PasskeyCredentials").First(&user, webauthnCred.UserID).Error; err != nil {
		return nil, nil, signInLookupError("user", err)
	}

	// Validation refuses an assertion whose user handle is not this account's.
	credential, err := s.webAuthn.ValidateDiscoverableLogin(
		func(rawID, userHandle []byte) (webauthn.User, error) {
			return &user, nil
		},
		*session,
		response,
	)
	if err != nil {
		return nil, nil, refusePasskey(fmt.Errorf("validate discoverable login: %w", err))
	}
	if !user.CanSignIn() {
		return nil, nil, refuseInactiveAccount(user.ID)
	}

	if err := s.recordCredentialUse(&webauthnCred, credential); err != nil {
		return nil, nil, err
	}

	// The credentials were loaded for validation only; the returned user is
	// what the login response carries, so it goes back without them.
	user.PasskeyCredentials = nil
	return &user, &webauthnCred, nil
}

// GetUserCredentials returns all passkey credentials for a user
func (s *WebAuthnService) GetUserCredentials(userID uint) ([]authm.WebAuthnCredential, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var credentials []authm.WebAuthnCredential
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&credentials).Error; err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}
	return credentials, nil
}

// DeleteCredential removes a passkey credential
func (s *WebAuthnService) DeleteCredential(userID uint, credentialID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	result := s.db.Where("id = ? AND user_id = ?", credentialID, userID).Delete(&authm.WebAuthnCredential{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete credential: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("credential not found")
	}
	return nil
}

// UpdateCredentialName updates the display name of a credential
func (s *WebAuthnService) UpdateCredentialName(userID uint, credentialID uint, displayName string) error {
	result := s.db.Model(&authm.WebAuthnCredential{}).
		Where("id = ? AND user_id = ?", credentialID, userID).
		Update("display_name", displayName)
	if result.Error != nil {
		return fmt.Errorf("failed to update credential: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("credential not found")
	}
	return nil
}

// Challenge storage helpers (for session management)

// StoreChallenge saves a WebAuthn challenge to the database
func (s *WebAuthnService) StoreChallenge(userID uint, session *webauthn.SessionData, operation string) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	sessionData, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}

	challenge := authm.WebAuthnChallenge{
		ID:          uuid.New().String(),
		UserID:      userID,
		Challenge:   []byte(session.Challenge),
		SessionData: sessionData,
		Operation:   operation,
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}

	if err := s.db.Create(&challenge).Error; err != nil {
		return "", fmt.Errorf("failed to store challenge: %w", err)
	}

	return challenge.ID, nil
}

// consumeChallenge deletes the challenge stored under challengeID for
// operation and returns the row it held. The delete and the read are one
// statement, so a challenge reaches at most one caller however many requests
// carry its id. An expired challenge is deleted all the same and answers as
// expired.
func (s *WebAuthnService) consumeChallenge(challengeID string, operation string) (*authm.WebAuthnChallenge, error) {
	var challenge authm.WebAuthnChallenge
	result := s.db.Raw(
		"DELETE FROM webauthn_challenges WHERE id = ? AND operation = ? RETURNING *",
		challengeID, operation,
	).Scan(&challenge)
	return checkConsumed(&challenge, result)
}

// consumeChallengeIssuedTo is consumeChallenge for a challenge stored for
// userID: one stored for any other user is neither returned nor removed.
func (s *WebAuthnService) consumeChallengeIssuedTo(challengeID string, operation string, userID uint) (*authm.WebAuthnChallenge, error) {
	var challenge authm.WebAuthnChallenge
	result := s.db.Raw(
		"DELETE FROM webauthn_challenges WHERE id = ? AND operation = ? AND user_id = ? RETURNING *",
		challengeID, operation, userID,
	).Scan(&challenge)
	return checkConsumed(&challenge, result)
}

var (
	// errChallengeNotFound is the error for a challenge id that selects no row.
	errChallengeNotFound = errors.New("challenge not found")
	// errChallengeExpired is the error for a challenge consumed after it expired.
	errChallengeExpired = errors.New("challenge expired")
)

// invalidChallengeMessage is the user-facing message of the
// CodeValidationFailed refusals ConsumeChallenge, ConsumeSignupChallenge and
// ConsumeRegistrationChallenge return.
const invalidChallengeMessage = "Invalid or expired challenge"

// refuseChallenge is err as a CodeValidationFailed refusal when it says the
// challenge does not exist or has expired, and err unchanged otherwise.
func refuseChallenge(err error) error {
	if errors.Is(err, errChallengeNotFound) || errors.Is(err, errChallengeExpired) {
		return apperrors.NewAuthError(apperrors.CodeValidationFailed, invalidChallengeMessage, err)
	}
	return err
}

// isChallengeID reports whether challengeID has the form StoreChallenge
// issues. Any other value names no challenge and never reaches a query.
func isChallengeID(challengeID string) bool {
	_, err := uuid.Parse(challengeID)
	return err == nil
}

// errRegistrationOperation is the error the operation-generic consumes return
// for the registration operation. It is not a refusal: asking for it is a
// caller's mistake, not a client's.
var errRegistrationOperation = fmt.Errorf("%s challenges are consumed by ConsumeRegistrationChallenge", contracts.WebAuthnRegistrationOperation)

// checkConsumed is the outcome of a challenge DELETE ... RETURNING that
// scanned into challenge: errChallengeNotFound when it removed no row and
// errChallengeExpired when the row it removed had expired.
func checkConsumed(challenge *authm.WebAuthnChallenge, result *gorm.DB) (*authm.WebAuthnChallenge, error) {
	if result.Error != nil {
		return nil, fmt.Errorf("failed to consume challenge: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, errChallengeNotFound
	}
	if challenge.IsExpired() {
		return nil, errChallengeExpired
	}
	return challenge, nil
}

// consumeOperationChallenge is consumeChallenge with its outcome classified
// for the operation-generic consumes: errRegistrationOperation for the
// registration operation, a CodeValidationFailed refusal for a malformed id
// or a challenge that does not exist or has expired, any other error when the
// challenge could not be read.
func (s *WebAuthnService) consumeOperationChallenge(challengeID string, operation string) (*authm.WebAuthnChallenge, error) {
	if operation == contracts.WebAuthnRegistrationOperation {
		return nil, errRegistrationOperation
	}
	if !isChallengeID(challengeID) {
		return nil, refuseChallenge(errChallengeNotFound)
	}
	challenge, err := s.consumeChallenge(challengeID, operation)
	if err != nil {
		return nil, refuseChallenge(err)
	}
	return challenge, nil
}

func decodeChallengeSession(challenge *authm.WebAuthnChallenge) (*webauthn.SessionData, error) {
	var session webauthn.SessionData
	if err := json.Unmarshal(challenge.SessionData, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}
	return &session, nil
}

// ConsumeChallenge removes a WebAuthn challenge and returns its session and
// the user it was issued to. Every later call for the same id fails, whether
// or not the ceremony the first caller runs with it succeeds.
//
// A refusal is a CodeValidationFailed *apperrors.AuthError for a challenge
// that does not exist or has expired. Any other error means the challenge
// could not be read, or operation is contracts.WebAuthnRegistrationOperation,
// whose challenges only ConsumeRegistrationChallenge consumes.
func (s *WebAuthnService) ConsumeChallenge(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
	challenge, err := s.consumeOperationChallenge(challengeID, operation)
	if err != nil {
		return nil, 0, err
	}
	session, err := decodeChallengeSession(challenge)
	if err != nil {
		return nil, 0, err
	}
	return session, challenge.UserID, nil
}

// ConsumeRegistrationChallenge removes the registration challenge stored
// under challengeID for userID and returns its session. Every later call for
// the same id fails. Only userID can spend it: for a challenge issued to
// another user it removes nothing.
//
// A refusal is an *apperrors.AuthError: CodeUnauthorized for an unexpired
// challenge issued to another user, CodeValidationFailed for one that does
// not exist or has expired. Any other error means the challenge could not be
// read.
func (s *WebAuthnService) ConsumeRegistrationChallenge(challengeID string, userID uint) (*webauthn.SessionData, error) {
	if !isChallengeID(challengeID) {
		return nil, refuseChallenge(errChallengeNotFound)
	}
	operation := contracts.WebAuthnRegistrationOperation
	challenge, err := s.consumeChallengeIssuedTo(challengeID, operation, userID)
	if errors.Is(err, errChallengeNotFound) {
		issuedToAnother, probeErr := s.liveChallengeExists(challengeID, operation)
		if probeErr != nil {
			return nil, probeErr
		}
		if issuedToAnother {
			return nil, apperrors.NewAuthError(
				apperrors.CodeUnauthorized,
				"Challenge belongs to different user",
				fmt.Errorf("registration challenge %s was issued to another user", challengeID),
			)
		}
	}
	if err != nil {
		return nil, refuseChallenge(err)
	}
	return decodeChallengeSession(challenge)
}

// liveChallengeExists reports whether an unexpired challenge is stored under
// challengeID for operation.
func (s *WebAuthnService) liveChallengeExists(challengeID string, operation string) (bool, error) {
	var count int64
	if err := s.db.Model(&authm.WebAuthnChallenge{}).
		Where("id = ? AND operation = ? AND expires_at > ?", challengeID, operation, time.Now()).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to look up challenge: %w", err)
	}
	return count > 0, nil
}

// CleanupExpiredChallenges deletes every challenge past its expiry and returns
// how many rows it deleted. ChallengeSweep runs it on a schedule.
func (s *WebAuthnService) CleanupExpiredChallenges(ctx context.Context) (int64, error) {
	result := s.db.WithContext(ctx).Where("expires_at < ?", time.Now()).Delete(&authm.WebAuthnChallenge{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete expired challenges: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// --- Passkey Signup (passkey-first registration) methods ---

// A passkey's user handle is always its account's users.id in the
// authm.User.WebAuthnID encoding, because login checks the handle the
// authenticator returns against exactly that value. A passkey minted at
// signup is created before the account's row exists, so signup reserves the
// id from the users sequence at begin, mints the credential with that id's
// handle, and inserts the row with that same id at finish. A signup that is
// abandoned after begin leaves a gap in users.id and no row.

// reserveUserID draws the next users.id without inserting a row.
func (s *WebAuthnService) reserveUserID() (uint, error) {
	var id int64
	if err := s.db.Raw("SELECT nextval(pg_get_serial_sequence('users', 'id'))").Scan(&id).Error; err != nil {
		return 0, fmt.Errorf("failed to reserve user id: %w", err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("failed to reserve user id: sequence returned %d", id)
	}
	return uint(id), nil
}

// signupAccount is the account a signup ceremony is for, before its row exists.
func signupAccount(id uint, email string) *authm.User {
	return &authm.User{ID: id, Email: &email}
}

// signupSessionData is the stored form of a signup challenge. Its JSON keys
// are a storage format, kept apart from the contracts type so a rename there
// cannot change what an already stored challenge decodes to. A challenge
// stored without an acceptance decodes to a zero one, which carries no
// version and no age confirmation.
//
// storedSignupAcceptance has exactly the fields of contracts.LegalAcceptance,
// differing only in tags: the two convert into each other, and the conversion
// stops compiling when either gains a field the other lacks.
type signupSessionData struct {
	Email      string                 `json:"email"`
	Acceptance storedSignupAcceptance `json:"acceptance"`
	Session    *webauthn.SessionData  `json:"session"`
}

type storedSignupAcceptance struct {
	TermsAcceptedAt time.Time `json:"terms_accepted_at"`
	TermsVersion    string    `json:"terms_version"`
	PrivacyVersion  string    `json:"privacy_version"`
	AgeConfirmedAt  time.Time `json:"age_confirmed_at"`
	MinAgeAttested  int       `json:"min_age_attested"`
}

// BeginRegistrationForEmail starts passkey registration for a new account
// identified only by its email, reserving the account's id for the handle.
// The returned session's UserID carries that reserved id.
func (s *WebAuthnService) BeginRegistrationForEmail(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	reservedID, err := s.reserveUserID()
	if err != nil {
		return nil, nil, err
	}

	options, session, err := s.webAuthn.BeginRegistration(
		signupAccount(reservedID, email),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin registration: %w", err)
	}

	return options, session, nil
}

// StoreSignupChallenge saves a signup ceremony's challenge together with the
// address and the consent its finish step records.
func (s *WebAuthnService) StoreSignupChallenge(signup contracts.SignupChallenge, session *webauthn.SessionData, operation string) (string, error) {
	wrapped := signupSessionData{
		Email:      signup.Email,
		Acceptance: storedSignupAcceptance(signup.Acceptance),
		Session:    session,
	}

	sessionData, err := json.Marshal(wrapped)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}

	challenge := authm.WebAuthnChallenge{
		ID:          uuid.New().String(),
		UserID:      0, // No user yet for signup
		Challenge:   []byte(session.Challenge),
		SessionData: sessionData,
		Operation:   operation,
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}

	if err := s.db.Create(&challenge).Error; err != nil {
		return "", fmt.Errorf("failed to store challenge: %w", err)
	}

	return challenge.ID, nil
}

// ConsumeSignupChallenge removes a signup challenge and returns it with the
// address and the consent StoreSignupChallenge stored beside it. Every later
// call for the same id fails, as with ConsumeChallenge. Its errors are
// ConsumeChallenge's.
func (s *WebAuthnService) ConsumeSignupChallenge(challengeID string, operation string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
	challenge, err := s.consumeOperationChallenge(challengeID, operation)
	if err != nil {
		return nil, contracts.SignupChallenge{}, err
	}

	var wrapped signupSessionData
	if err := json.Unmarshal(challenge.SessionData, &wrapped); err != nil {
		return nil, contracts.SignupChallenge{}, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return wrapped.Session, contracts.SignupChallenge{
		Email:      wrapped.Email,
		Acceptance: contracts.LegalAcceptance(wrapped.Acceptance),
	}, nil
}

// FinishSignupRegistrationWithLegal verifies a signup's registration
// response, then creates the account under the id its session reserved, with
// the acceptance recorded, its default preferences and the credential.
//
// A refusal is an *apperrors.AuthError: CodeValidationFailed for a response
// that fails verification, CodeUserExists for an address an account already
// holds. Any other error means the account was not created, or was created
// and could not be read back.
func (s *WebAuthnService) FinishSignupRegistrationWithLegal(
	email string,
	session *webauthn.SessionData,
	response *protocol.ParsedCredentialCreationData,
	displayName string,
	acceptance contracts.LegalAcceptance,
) (*authm.User, error) {
	reservedID, ok := authm.UserIDFromWebAuthnID(session.UserID)
	if !ok {
		return nil, fmt.Errorf("signup session carries no reserved user id")
	}

	credential, err := s.webAuthn.CreateCredential(signupAccount(reservedID, email), *session, response)
	if err != nil {
		return nil, refuseRegistration(fmt.Errorf("create credential: %w", err))
	}

	// Start a transaction to create user and credential atomically
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create user without password, under the id its passkey's handle encodes
	user := &authm.User{
		ID:            reservedID,
		Email:         &email,
		IsActive:      true,
		EmailVerified: false,
	}
	if acceptance.TermsVersion != "" {
		acceptedAt := acceptance.TermsAcceptedAt
		if acceptedAt.IsZero() {
			acceptedAt = time.Now().UTC()
		}
		termsVersion := acceptance.TermsVersion
		privacyVersion := acceptance.PrivacyVersion
		user.TermsAcceptedAt = &acceptedAt
		user.TermsVersion = &termsVersion
		user.PrivacyVersion = &privacyVersion
	}
	// Record the minimum-age confirmation alongside terms (PSY-1023). Mirrors the
	// terms block: only applied when an age was attested.
	if acceptance.MinAgeAttested > 0 {
		ageConfirmedAt := acceptance.AgeConfirmedAt
		if ageConfirmedAt.IsZero() {
			ageConfirmedAt = time.Now().UTC()
		}
		minAge := acceptance.MinAgeAttested
		user.AgeConfirmedAt = &ageConfirmedAt
		user.MinAgeAttested = &minAge
	}

	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		if dup := shared.UserExistsIfDuplicate(email, err); dup != nil {
			return nil, dup
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create default preferences
	preferences := &authm.UserPreferences{
		UserID: user.ID,
	}

	if err := tx.Create(preferences).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user preferences: %w", err)
	}

	// Convert and save credential with real user ID
	webauthnCred := authm.ToWebAuthnCredential(user.ID, credential, displayName)

	if err := tx.Create(webauthnCred).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to save credential: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Load relationships
	if err := s.db.Preload("Preferences").First(user, user.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load user relationships: %w", err)
	}

	return user, nil
}
