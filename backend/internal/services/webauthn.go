package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
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
		// Authenticator selection - prefer platform authenticators (Touch ID, Face ID, Windows Hello)
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			AuthenticatorAttachment: protocol.CrossPlatform,
			UserVerification:        protocol.VerificationPreferred,
			ResidentKey:             protocol.ResidentKeyRequirementPreferred,
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
func (s *WebAuthnService) BeginRegistration(user *models.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	// Get existing credentials to exclude
	var existingCreds []models.WebAuthnCredential
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

// FinishRegistration completes the passkey registration process
func (s *WebAuthnService) FinishRegistration(user *models.User, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*models.WebAuthnCredential, error) {
	// Complete the registration
	credential, err := s.webAuthn.CreateCredential(user, *session, response)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	// Convert to our model
	webauthnCred := models.ToWebAuthnCredential(user.ID, credential, displayName)

	// Save to database
	if err := s.db.Create(webauthnCred).Error; err != nil {
		return nil, fmt.Errorf("failed to save credential: %w", err)
	}

	return webauthnCred, nil
}

// BeginLogin starts the passkey login process
func (s *WebAuthnService) BeginLogin(user *models.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	// Load user's credentials
	if err := s.db.Preload("PasskeyCredentials").First(user, user.ID).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to load user: %w", err)
	}

	if len(user.PasskeyCredentials) == 0 {
		return nil, nil, fmt.Errorf("user has no registered passkeys")
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

// FinishLogin completes the passkey login process
func (s *WebAuthnService) FinishLogin(user *models.User, session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*models.WebAuthnCredential, error) {
	// Complete the login
	credential, err := s.webAuthn.ValidateLogin(user, *session, response)
	if err != nil {
		return nil, fmt.Errorf("failed to validate login: %w", err)
	}

	// Find and update the credential
	var webauthnCred models.WebAuthnCredential
	if err := s.db.Where("user_id = ? AND credential_id = ?", user.ID, credential.ID).First(&webauthnCred).Error; err != nil {
		return nil, fmt.Errorf("credential not found: %w", err)
	}

	// Update sign count and last used
	now := time.Now()
	webauthnCred.SignCount = credential.Authenticator.SignCount
	webauthnCred.LastUsedAt = &now
	webauthnCred.CloneWarning = credential.Authenticator.CloneWarning

	if err := s.db.Save(&webauthnCred).Error; err != nil {
		return nil, fmt.Errorf("failed to update credential: %w", err)
	}

	return &webauthnCred, nil
}

// FinishDiscoverableLogin completes a discoverable login and returns the user
func (s *WebAuthnService) FinishDiscoverableLogin(session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*models.User, *models.WebAuthnCredential, error) {
	// Find the credential by ID
	var webauthnCred models.WebAuthnCredential
	if err := s.db.Where("credential_id = ?", response.RawID).Preload("User").First(&webauthnCred).Error; err != nil {
		return nil, nil, fmt.Errorf("credential not found: %w", err)
	}

	// Load the user with all credentials
	var user models.User
	if err := s.db.Preload("PasskeyCredentials").First(&user, webauthnCred.UserID).Error; err != nil {
		return nil, nil, fmt.Errorf("user not found: %w", err)
	}

	// Validate the login
	credential, err := s.webAuthn.ValidateDiscoverableLogin(
		func(rawID, userHandle []byte) (webauthn.User, error) {
			return &user, nil
		},
		*session,
		response,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate login: %w", err)
	}

	// Update the credential
	now := time.Now()
	webauthnCred.SignCount = credential.Authenticator.SignCount
	webauthnCred.LastUsedAt = &now
	webauthnCred.CloneWarning = credential.Authenticator.CloneWarning

	if err := s.db.Save(&webauthnCred).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to update credential: %w", err)
	}

	return &user, &webauthnCred, nil
}

// GetUserCredentials returns all passkey credentials for a user
func (s *WebAuthnService) GetUserCredentials(userID uint) ([]models.WebAuthnCredential, error) {
	var credentials []models.WebAuthnCredential
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&credentials).Error; err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}
	return credentials, nil
}

// DeleteCredential removes a passkey credential
func (s *WebAuthnService) DeleteCredential(userID uint, credentialID uint) error {
	result := s.db.Where("id = ? AND user_id = ?", credentialID, userID).Delete(&models.WebAuthnCredential{})
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
	result := s.db.Model(&models.WebAuthnCredential{}).
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
	sessionData, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}

	challenge := models.WebAuthnChallenge{
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

// GetChallenge retrieves and validates a WebAuthn challenge
func (s *WebAuthnService) GetChallenge(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
	var challenge models.WebAuthnChallenge
	if err := s.db.Where("id = ? AND operation = ?", challengeID, operation).First(&challenge).Error; err != nil {
		return nil, 0, fmt.Errorf("challenge not found: %w", err)
	}

	// Check expiry
	if challenge.IsExpired() {
		s.db.Delete(&challenge)
		return nil, 0, fmt.Errorf("challenge expired")
	}

	// Unmarshal session data
	var session webauthn.SessionData
	if err := json.Unmarshal(challenge.SessionData, &session); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, challenge.UserID, nil
}

// DeleteChallenge removes a used challenge
func (s *WebAuthnService) DeleteChallenge(challengeID string) error {
	return s.db.Where("id = ?", challengeID).Delete(&models.WebAuthnChallenge{}).Error
}

// CleanupExpiredChallenges removes expired challenges (can be run periodically)
func (s *WebAuthnService) CleanupExpiredChallenges() error {
	return s.db.Where("expires_at < ?", time.Now()).Delete(&models.WebAuthnChallenge{}).Error
}

// --- Passkey Signup (passkey-first registration) methods ---

// signupUser is a temporary user struct for passkey signup (before user exists in DB)
type signupUser struct {
	email string
}

func (u *signupUser) WebAuthnID() []byte {
	// Use email hash as temporary ID
	return []byte(u.email)
}

func (u *signupUser) WebAuthnName() string {
	return u.email
}

func (u *signupUser) WebAuthnDisplayName() string {
	return u.email
}

func (u *signupUser) WebAuthnCredentials() []webauthn.Credential {
	return nil // No existing credentials for new signup
}

func (u *signupUser) WebAuthnIcon() string {
	return ""
}

// signupSessionData wraps session data with email for signup flow
type signupSessionData struct {
	Email   string                `json:"email"`
	Session *webauthn.SessionData `json:"session"`
}

// BeginRegistrationForEmail starts passkey registration for a new user with just email
func (s *WebAuthnService) BeginRegistrationForEmail(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	tempUser := &signupUser{email: email}

	// Create registration options
	options, session, err := s.webAuthn.BeginRegistration(
		tempUser,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin registration: %w", err)
	}

	return options, session, nil
}

// StoreChallengeWithEmail saves a WebAuthn challenge with email for signup flow
func (s *WebAuthnService) StoreChallengeWithEmail(email string, session *webauthn.SessionData, operation string) (string, error) {
	// Wrap session with email
	wrapped := signupSessionData{
		Email:   email,
		Session: session,
	}

	sessionData, err := json.Marshal(wrapped)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}

	challenge := models.WebAuthnChallenge{
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

// GetChallengeWithEmail retrieves a signup challenge and returns session + email
func (s *WebAuthnService) GetChallengeWithEmail(challengeID string, operation string) (*webauthn.SessionData, string, error) {
	var challenge models.WebAuthnChallenge
	if err := s.db.Where("id = ? AND operation = ?", challengeID, operation).First(&challenge).Error; err != nil {
		return nil, "", fmt.Errorf("challenge not found: %w", err)
	}

	// Check expiry
	if challenge.IsExpired() {
		s.db.Delete(&challenge)
		return nil, "", fmt.Errorf("challenge expired")
	}

	// Unmarshal wrapped session data
	var wrapped signupSessionData
	if err := json.Unmarshal(challenge.SessionData, &wrapped); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return wrapped.Session, wrapped.Email, nil
}

// FinishSignupRegistration completes registration, creates user, and stores credential.
// Legacy path without legal acceptance metadata.
func (s *WebAuthnService) FinishSignupRegistration(email string, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*models.User, error) {
	return s.FinishSignupRegistrationWithLegal(email, session, response, displayName, LegalAcceptance{})
}

// FinishSignupRegistrationWithLegal completes registration and records legal acceptance metadata.
func (s *WebAuthnService) FinishSignupRegistrationWithLegal(
	email string,
	session *webauthn.SessionData,
	response *protocol.ParsedCredentialCreationData,
	displayName string,
	acceptance LegalAcceptance,
) (*models.User, error) {
	// Use the same temp user for credential creation
	tempUser := &signupUser{email: email}

	// Complete the registration
	credential, err := s.webAuthn.CreateCredential(tempUser, *session, response)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	// Start a transaction to create user and credential atomically
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create user without password
	user := &models.User{
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

	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create default preferences
	preferences := &models.UserPreferences{
		UserID: user.ID,
	}

	if err := tx.Create(preferences).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user preferences: %w", err)
	}

	// Convert and save credential with real user ID
	webauthnCred := models.ToWebAuthnCredential(user.ID, credential, displayName)

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
