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
	"gorm.io/gorm/clause"

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

// FinishRegistration verifies a registration response for user and stores
// the credential it creates.
//
// A refusal is an *apperrors.AuthError: CodeValidationFailed for a response
// that fails verification or whose credential id an account already holds,
// CodeAccountInactive when the account may not sign in
// (authm.User.CanSignIn) as the credential is written. A refusal stores
// nothing. Any other error means the credential could not be stored.
func (s *WebAuthnService) FinishRegistration(user *authm.User, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*authm.WebAuthnCredential, error) {
	credential, err := s.webAuthn.CreateCredential(user, *session, response)
	if err != nil {
		return nil, apperrors.ErrInvalidCredentialResponse(fmt.Errorf("create credential: %w", err))
	}

	webauthnCred := authm.ToWebAuthnCredential(user.ID, credential, displayName)

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// The account is judged in the transaction that writes the
		// credential, so one closed after the caller's check is still
		// refused. FOR SHARE: a deactivation or deletion of the row waits for
		// this transaction, so the account cannot close between this read and
		// the write.
		var account authm.User
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&account, user.ID).Error; err != nil {
			return fmt.Errorf("failed to load user: %w", err)
		}
		if !account.CanSignIn() {
			return refuseInactiveAccount(account.ID)
		}
		if err := tx.Create(webauthnCred).Error; err != nil {
			return credentialSaveError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return webauthnCred, nil
}

// credentialSaveError is the error for a failed webauthn_credentials insert:
// the invalid-credential-response refusal for a duplicate key, which is a
// credential some account already holds because credential_id is the only
// unique column the insert writes (id is assigned by its sequence), and a
// failure otherwise.
func credentialSaveError(err error) error {
	if shared.IsDuplicateKey(err) {
		return apperrors.ErrInvalidCredentialResponse(fmt.Errorf("credential already registered: %w", err))
	}
	return fmt.Errorf("failed to save credential: %w", err)
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
// CodeInvalidCredentials refusals FinishDiscoverableLogin returns.
const invalidPasskeyMessage = "Invalid passkey"

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

// loadSignInAccount loads account userID for a passkey sign-in: with its
// passkey credentials, which validation needs, and with its OAuth accounts and
// preferences, which the signed-in account is returned with.
func (s *WebAuthnService) loadSignInAccount(userID uint) (*authm.User, error) {
	var account authm.User
	if err := s.db.Preload("PasskeyCredentials").Preload("OAuthAccounts").Preload("Preferences").
		First(&account, userID).Error; err != nil {
		return nil, err
	}
	return &account, nil
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

// FinishDiscoverableLogin completes a discoverable passkey login and returns
// the account it signs in, as loaded for the decision with its OAuth accounts
// and preferences and without its credentials, and the credential it used.
//
// Its errors come in two kinds. A refusal is an *apperrors.AuthError:
// CodeInvalidCredentials when the assertion does not prove a credential of an
// existing account (it fails validation, carries another account's user
// handle, comes from a session that is not a discoverable login's, or names a
// credential or account that does not exist), and CodeAccountInactive when it
// does but the account may not sign in (authm.User.CanSignIn). Any other
// error means no decision was reached: a lookup or the credential update
// failed.
//
// The account's state is checked only after the assertion validated, so only
// the credential's holder learns it, and a refused sign-in writes nothing. A
// clone warning (a sign count that did not advance) is recorded on the
// credential and does not refuse the sign-in.
func (s *WebAuthnService) FinishDiscoverableLogin(session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error) {
	var webauthnCred authm.WebAuthnCredential
	if err := s.db.Where("credential_id = ?", response.RawID).First(&webauthnCred).Error; err != nil {
		return nil, nil, signInLookupError("credential", err)
	}

	user, err := s.loadSignInAccount(webauthnCred.UserID)
	if err != nil {
		return nil, nil, signInLookupError("user", err)
	}

	// Validation refuses an assertion whose user handle is not this account's.
	credential, err := s.webAuthn.ValidateDiscoverableLogin(
		func(rawID, userHandle []byte) (webauthn.User, error) {
			return user, nil
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
	return user, &webauthnCred, nil
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

// DeleteCredential removes userID's passkey credential credentialID.
//
// A refusal is a CodeValidationFailed *apperrors.AuthError for a credential
// userID does not hold. Any other error means the delete could not run.
func (s *WebAuthnService) DeleteCredential(userID uint, credentialID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	result := s.db.Where("id = ? AND user_id = ?", credentialID, userID).Delete(&authm.WebAuthnCredential{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete credential: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrCredentialNotHeld(fmt.Errorf("credential %d not held by user %d", credentialID, userID))
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

// ConsumeChallenge removes a WebAuthn challenge and returns its session. Every
// later call for the same id fails, whether or not the ceremony the first
// caller runs with it succeeds.
//
// A refusal is a CodeValidationFailed *apperrors.AuthError for a challenge
// that does not exist or has expired. Any other error means the challenge
// could not be read, or operation is contracts.WebAuthnRegistrationOperation,
// whose challenges only ConsumeRegistrationChallenge consumes.
func (s *WebAuthnService) ConsumeChallenge(challengeID string, operation string) (*webauthn.SessionData, error) {
	challenge, err := s.consumeOperationChallenge(challengeID, operation)
	if err != nil {
		return nil, err
	}
	return decodeChallengeSession(challenge)
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

// deleteExpiredChallenges deletes every challenge in database past its expiry
// and returns how many rows it deleted.
func deleteExpiredChallenges(ctx context.Context, database *gorm.DB) (int64, error) {
	result := database.WithContext(ctx).Where("expires_at < ?", time.Now()).Delete(&authm.WebAuthnChallenge{})
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
// It returns the account as created, with its preferences. A refusal is an
// *apperrors.AuthError: CodeTermsAcceptanceRequired for an acceptance that
// names no terms version, which records no consent, CodeValidationFailed for
// a response that fails verification or whose credential id an account
// already holds, CodeUserExists for an address an account already holds. A
// refusal creates no account. Any other error means the account was not
// created.
func (s *WebAuthnService) FinishSignupRegistrationWithLegal(
	email string,
	session *webauthn.SessionData,
	response *protocol.ParsedCredentialCreationData,
	displayName string,
	acceptance contracts.LegalAcceptance,
) (*authm.User, error) {
	if acceptance.TermsVersion == "" {
		return nil, apperrors.ErrTermsAcceptanceRequired("passkey signup acceptance names no terms version")
	}

	reservedID, ok := authm.UserIDFromWebAuthnID(session.UserID)
	if !ok {
		return nil, fmt.Errorf("signup session carries no reserved user id")
	}

	credential, err := s.webAuthn.CreateCredential(signupAccount(reservedID, email), *session, response)
	if err != nil {
		return nil, apperrors.ErrInvalidCredentialResponse(fmt.Errorf("create credential: %w", err))
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
	shared.StampLegalAcceptance(user, acceptance)

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
		return nil, credentialSaveError(err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	user.Preferences = preferences
	return user, nil
}
