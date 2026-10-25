package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/markbates/goth"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	catalogsvc "psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	engagementsvc "psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
)

// Account lockout constants
const (
	MaxFailedLoginAttempts = 5
	AccountLockDuration    = 15 * time.Minute
)

// ListUsers returns a paginated list of users for the admin console
func (s *UserService) ListUsers(limit, offset int, filters contracts.AdminUserFilters) ([]*contracts.AdminUserResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Base query
	query := s.db.Model(&authm.User{})

	// Apply search filter
	if filters.Search != "" {
		searchPattern := shared.LikePattern(filters.Search)
		query = query.Where("(email ILIKE ? OR username ILIKE ?)", searchPattern, searchPattern)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// Fetch users with OAuth accounts preloaded
	var users []authm.User
	if err := query.
		Preload("OAuthAccounts").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	if len(users) == 0 {
		return []*contracts.AdminUserResponse{}, total, nil
	}

	// Collect user IDs for batch queries
	userIDs := make([]uint, len(users))
	for i, u := range users {
		userIDs[i] = u.ID
	}

	// Batch load passkey counts
	type passkeyCount struct {
		UserID uint
		Count  int64
	}
	var passkeyCounts []passkeyCount
	s.db.Model(&authm.WebAuthnCredential{}).
		Select("user_id, COUNT(*) as count").
		Where("user_id IN ?", userIDs).
		Group("user_id").
		Scan(&passkeyCounts)

	passkeyMap := make(map[uint]int64, len(passkeyCounts))
	for _, pc := range passkeyCounts {
		passkeyMap[pc.UserID] = pc.Count
	}

	// Batch load show stats
	type showStat struct {
		SubmittedBy uint
		Status      string
		Count       int64
	}
	var showStats []showStat
	s.db.Model(&catalogm.Show{}).
		Select("submitted_by, status, COUNT(*) as count").
		Where("submitted_by IN ?", userIDs).
		Group("submitted_by, status").
		Scan(&showStats)

	statsMap := make(map[uint]contracts.UserSubmissionStats, len(users))
	for _, ss := range showStats {
		stats := statsMap[ss.SubmittedBy]
		switch ss.Status {
		case "approved":
			stats.Approved = ss.Count
		case "pending":
			stats.Pending = ss.Count
		case "rejected":
			stats.Rejected = ss.Count
		}
		stats.Total += ss.Count
		statsMap[ss.SubmittedBy] = stats
	}

	// Build response
	result := make([]*contracts.AdminUserResponse, len(users))
	for i, u := range users {
		// Derive auth methods
		var authMethods []string
		if u.PasswordHash != nil && *u.PasswordHash != "" {
			authMethods = append(authMethods, "password")
		}
		for _, oauth := range u.OAuthAccounts {
			authMethods = append(authMethods, oauth.Provider)
		}
		if passkeyMap[u.ID] > 0 {
			authMethods = append(authMethods, "passkey")
		}

		result[i] = &contracts.AdminUserResponse{
			ID:              u.ID,
			Email:           u.Email,
			Username:        u.Username,
			DisplayName:     u.DisplayName,
			FirstName:       u.FirstName,
			LastName:        u.LastName,
			AvatarURL:       u.AvatarURL,
			IsActive:        u.IsActive,
			IsAdmin:         u.IsAdmin,
			EmailVerified:   u.EmailVerified,
			AuthMethods:     authMethods,
			SubmissionStats: statsMap[u.ID],
			CreatedAt:       u.CreatedAt,
			DeletedAt:       u.DeletedAt,
		}
	}

	return result, total, nil
}

// UserService handles user-related business logic
type UserService struct {
	db                  *gorm.DB
	savedReleaseService contracts.SavedReleaseServiceInterface

	// incrementFailedAttemptsFn is a test-only seam. When non-nil,
	// AuthenticateUserWithPassword routes the lockout-counter increment through
	// this function instead of s.IncrementFailedAttempts directly. The seam
	// exists to deterministically exercise the fail-closed path on counter-
	// write failure without resorting to DB-state manipulation that would race
	// with the rest of the integration suite. Leave nil in production.
	incrementFailedAttemptsFn func(userID uint) error
}

// NewUserService creates a new user service
func NewUserService(database *gorm.DB) *UserService {
	if database == nil {
		database = db.GetDB()
	}
	releaseService := catalogsvc.NewReleaseService(database)
	return &UserService{
		db:                  database,
		savedReleaseService: engagementsvc.NewSavedReleaseService(database, releaseService),
	}
}

// FindOrCreateUserWithConsent returns the account the OAuth identity is
// attached to, or creates one from the identity when consent passes
// shared.ValidateOAuthSignupConsent.
//
// Returns a typed *apperrors.AuthError for each refusal: CodeOAuthLinkRefused
// when the address already belongs to an account the identity is not attached
// to, CodeUserExists when a concurrent signup claims the address first,
// CodeAccountInactive when the identity resolves to an account that may not
// sign in (authm.User.CanSignIn), CodeAccountRecoverable in place of either of
// those two when the account was deleted and is inside its recovery window
// (shared.ClosedAccountRefusal and shared.AddressMatchIsRecoverable), the
// refusal of shared.ValidateProviderEmailClaim before any query, and the
// consent refusals of shared.ValidateOAuthSignupConsent when an account would
// be created. Callers that render a refusal differently from a fault must
// discriminate on the code.
//
// With a nil consent, an identity that would need an account created is
// refused with *contracts.OAuthSignupRequiredError carrying the identity. The
// CodeOAuthLinkRefused refusal arrives wrapped in
// *contracts.OAuthLinkRequiredError when the provider verified the address
// and is the authority for it (contracts.OAuthIdentity.EmailAuthoritative),
// and the account holding it may sign in and holds no identity from the
// provider.
func (s *UserService) FindOrCreateUserWithConsent(gothUser goth.User, provider string, consent *contracts.OAuthSignupConsent) (*authm.User, error) {
	return s.FindOrCreateUserFromOAuthIdentity(OAuthIdentityFromGoth(gothUser, provider), consent, nil)
}

// OAuthIdentityFromGoth is what gothUser asserts, as the provider named
// provider: email_verified is set only when the provider's own claim says so.
// The provider's tokens are left out; nothing reads them.
func OAuthIdentityFromGoth(gothUser goth.User, provider string) contracts.OAuthIdentity {
	return contracts.OAuthIdentity{
		Provider:      provider,
		Subject:       gothUser.UserID,
		Email:         gothUser.Email,
		EmailVerified: providerAssertsEmailVerified(gothUser),
		// Read here, while RawData is at hand; the pending tables do not
		// keep it.
		EmailAuthoritative: providerIsAuthoritativeForEmail(provider, gothUser),
		Name:               gothUser.Name,
		FirstName:          gothUser.FirstName,
		LastName:           gothUser.LastName,
		AvatarURL:          gothUser.AvatarURL,
	}
}

// FindOrCreateUserFromOAuthIdentity returns the account identity is attached
// to, or creates one from it, with the refusals FindOrCreateUserWithConsent
// documents. An account it creates is named by the provider's first and last
// names when displayName is nil, takes a non-blank displayName as its display
// name, and has no name at all for a blank one. An account it finds is
// returned unchanged.
func (s *UserService) FindOrCreateUserFromOAuthIdentity(identity contracts.OAuthIdentity, consent *contracts.OAuthSignupConsent, displayName *string) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	provider := identity.Provider

	// provider_user_id is the identity this whole function resolves on, and the
	// column permits the empty string. Without this, a provider returning no
	// subject would match any row stored with an empty one and sign in as its
	// owner, ahead of every check below.
	if identity.Subject == "" {
		return nil, fmt.Errorf("oauth provider %q returned no user id", provider)
	}

	if err := shared.ValidateProviderEmailClaim(provider, identity.Email); err != nil {
		return nil, err
	}

	// First, try to find existing OAuth account
	var oauthAccount authm.OAuthAccount

	result := s.db.
		Where("provider = ? AND provider_user_id = ?", provider, identity.Subject).
		First(&oauthAccount)

	if result.Error == nil {
		// OAuth account exists, get the user
		var user authm.User
		if err := s.db.Preload("OAuthAccounts").Preload("Preferences").First(&user, oauthAccount.UserID).Error; err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}
		if !user.CanSignIn() {
			refusal := shared.ClosedAccountRefusal(&user)
			logger.Default().Warn("oauth_signin_refused_account_inactive",
				"provider", provider,
				"user_id", user.ID,
				"code", refusal.Code)
			return nil, refusal
		}
		return &user, nil
	}

	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database error: %w", result.Error)
	}

	// OAuth account doesn't exist, check if user exists by email
	var existingUser authm.User
	if identity.Email != "" {
		result.Error = s.db.Where(authm.EmailIdentityWhere, identity.Email).First(&existingUser).Error
		if result.Error == nil {
			// A matching address is not evidence that this provider identity
			// belongs to the account holding it, and no verification signal
			// makes it evidence. The provider's claim says who owns the
			// MAILBOX; it says nothing about who owns the ACCOUNT, and those
			// come apart the moment someone creates an account on an address
			// they do not own. Attaching an identity by address alone is how
			// the real owner of a squatted address gets joined into the
			// squatter's account.
			//
			// So there is no by-address link at all. An identity is attached
			// only from a session that already holds the account, at
			// /auth/link/{provider}, which the refusal names, or after the
			// person proves the account by its password or its mailbox, which
			// OAuthLinkRequiredError offers a caller.
			//
			// A recoverable deleted account is the exception, for the reasons
			// shared.AddressMatchIsRecoverable states.
			if shared.AddressMatchIsRecoverable(&existingUser, identity.EmailVerified) {
				logger.Default().Warn("oauth_signin_refused_account_recoverable",
					"provider", provider,
					"user_id", existingUser.ID)
				return nil, apperrors.ErrAccountRecoverable()
			}
			// The proofs can join this identity to the account only when the
			// provider is the authority for the mailbox, not only a witness
			// that it was verified once: the mailbox proof is a link sent
			// there, and an identity that does not hold the mailbox now must
			// not be joined by whoever does clicking it. An account that may
			// not sign in, or that already holds an identity from this
			// provider, keeps the plain refusal.
			if existingUser.CanSignIn() && identity.EmailVerified && identity.EmailAuthoritative {
				holds, err := s.accountHoldsProviderIdentity(existingUser.ID, provider)
				if err != nil {
					return nil, err
				}
				if !holds {
					// Whether a pending link is actually offered is the
					// caller's decision (a CLI sign-in has nowhere to show
					// one), so this names the possibility, not the outcome.
					logger.Default().Warn("oauth_signin_address_belongs_to_account_link_possible",
						"provider", provider,
						"user_id", existingUser.ID,
						"email_hash", logger.HashEmail(identity.Email))
					return nil, &contracts.OAuthLinkRequiredError{
						Identity: identity,
						UserID:   existingUser.ID,
						Refusal:  apperrors.ErrOAuthLinkRefused(identity.Email),
					}
				}
			}
			logger.Default().Warn("oauth_signin_refused_address_belongs_to_account",
				"provider", provider,
				"email_hash", logger.HashEmail(identity.Email))
			return nil, apperrors.ErrOAuthLinkRefused(identity.Email)
		}
		if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("database error: %w", result.Error)
		}
	}

	if consent == nil {
		return nil, &contracts.OAuthSignupRequiredError{
			Identity: identity,
			Refusal:  apperrors.ErrTermsAcceptanceRequired("new oauth identity arrived without consent"),
		}
	}
	return s.createNewUserOauthWithConsent(identity, consent, displayName)
}

// accountHoldsProviderIdentity reports whether userID already holds an
// identity from provider.
func (s *UserService) accountHoldsProviderIdentity(userID uint, provider string) (bool, error) {
	var count int64
	if err := s.db.Model(&authm.OAuthAccount{}).
		Where("user_id = ? AND provider = ?", userID, provider).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("database error: %w", err)
	}
	return count > 0, nil
}

// CreateUserWithPasswordWithLegal creates a new user and records legal acceptance metadata.
// An acceptance that names no terms version records no consent, so it is
// refused with CodeTermsAcceptanceRequired and no account is created.
func (s *UserService) CreateUserWithPasswordWithLegal(
	email,
	password,
	firstName,
	lastName string,
	acceptance contracts.LegalAcceptance,
) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if acceptance.TermsVersion == "" {
		return nil, apperrors.ErrTermsAcceptanceRequired("password signup acceptance names no terms version")
	}

	existingUser, err := s.GetUserByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if existingUser != nil {
		return nil, apperrors.ErrUserExists(email)
	}

	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	// Create user
	user := &authm.User{
		Email:         &email,
		PasswordHash:  &hashedPassword,
		FirstName:     &firstName,
		LastName:      &lastName,
		IsActive:      true,
		EmailVerified: false, // Email verification required for password users
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

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Load relationships
	if err := s.db.Preload("Preferences").First(user, user.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load user relationships: %w", err)
	}

	return user, nil
}

func (s *UserService) AuthenticateUserWithPassword(email, password string) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	user, err := s.GetUserByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// To prevent timing attacks that reveal whether an email exists,
	// always perform a bcrypt comparison even if user doesn't exist
	if user == nil {
		// Use a dummy hash to maintain constant time
		// This will always fail but takes the same time as a real verification
		dummyHash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy" // bcrypt hash of "dummy"
		// Intentionally discard the result: this call exists ONLY for its CPU-time
		// side effect (constant-time response). We must not branch on the outcome,
		// since branching would re-introduce the timing oracle this code defends
		// against.
		//nolint:errcheck // see above; called for timing equalization, not validation
		bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, apperrors.ErrInvalidCredentials(nil)
	}

	// The gate is per request and the failure count is exact: a request that
	// loaded the row above before the locking failure committed still gets one
	// password check, and that failure is counted.
	if s.IsAccountLocked(user) {
		remaining := s.GetLockTimeRemaining(user)
		minutes := int(remaining.Minutes()) + 1 // Round up
		return nil, apperrors.ErrAccountLockedWithMinutes(minutes)
	}

	if user.PasswordHash == nil {
		// Also do dummy verification for OAuth-only accounts so attackers can't
		// distinguish "no password set" from "wrong password" by timing.
		dummyHash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
		//nolint:errcheck // see above; called for timing equalization, not validation
		bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, apperrors.ErrInvalidCredentials(nil)
	}

	if err := s.VerifyPassword(*user.PasswordHash, password); err != nil {
		// Increment failed attempts on password failure. Fail closed if the
		// counter write itself fails: silently returning ErrInvalidCredentials
		// when Increment errors would let an attacker brute-force without ever
		// tripping the lockout (the invariant ">= N failures locks the account"
		// only holds if every failure is recorded). A 503 is the right
		// alternative — generic message, no account-state leak; a legitimate
		// user during a DB hiccup retries until DB recovers.
		incrementFn := s.IncrementFailedAttempts
		if s.incrementFailedAttemptsFn != nil {
			incrementFn = s.incrementFailedAttemptsFn
		}
		if incErr := incrementFn(user.ID); incErr != nil {
			logger.Default().Error("user_increment_failed_attempts_failed",
				"user_id", user.ID,
				"error", incErr,
			)
			return nil, apperrors.ErrServiceUnavailable("user_increment_failed_attempts", incErr)
		}
		return nil, apperrors.ErrInvalidCredentials(nil)
	}

	if !user.CanSignIn() {
		return nil, shared.ClosedAccountRefusal(user)
	}

	// Reset failed attempts on successful login. Log + continue on failure:
	// the user is authenticated either way; a stale counter is a minor
	// degradation, not a correctness bug.
	if resetErr := s.ResetFailedAttempts(user.ID); resetErr != nil {
		logger.Default().Error("user_reset_failed_attempts_failed",
			"user_id", user.ID,
			"error", resetErr,
		)
	}

	return user, nil
}

func (s *UserService) createNewUserOauthWithConsent(
	identity contracts.OAuthIdentity,
	consent *contracts.OAuthSignupConsent,
	displayName *string,
) (*authm.User, error) {
	acceptance, err := shared.ValidateOAuthSignupConsent(consent)
	if err != nil {
		return nil, err
	}

	// Start transaction
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create user. Email is left NULL when the provider supplied none: an empty
	// string is a real value that collides with every other addressless account
	// under the users_lower_email_uniq index, while NULLs do not collide.
	// GitHub returns no address for a user with none public.
	firstName, lastName := identity.FirstName, identity.LastName
	if displayName != nil && *displayName == "" {
		firstName, lastName, displayName = "", "", nil
	}
	user := &authm.User{
		FirstName: &firstName,
		LastName:  &lastName,
		AvatarURL: &identity.AvatarURL,
		IsActive:  true,
		// The column carries what the provider actually asserted. An account
		// created from an address no provider vouched for stays unverified,
		// which closes every capability gated on the flag until this system's
		// own verification email opens them.
		EmailVerified: identity.EmailVerified,
	}
	if identity.Email != "" {
		user.Email = &identity.Email
	}
	if displayName != nil {
		user.DisplayName = displayName
	}
	shared.StampLegalAcceptance(user, acceptance)

	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		if dup := shared.UserExistsIfDuplicate(identity.Email, err); dup != nil {
			return nil, dup
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create OAuth account
	oauthAccount := &authm.OAuthAccount{
		UserID:            user.ID,
		Provider:          identity.Provider,
		ProviderUserID:    identity.Subject,
		ProviderEmail:     &identity.Email,
		ProviderName:      &identity.Name,
		ProviderAvatarURL: &identity.AvatarURL,
	}

	if err := tx.Create(oauthAccount).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create OAuth account: %w", err)
	}

	// Create default preferences
	preferences := &authm.UserPreferences{
		UserID: user.ID,
	}

	if err := tx.Create(preferences).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user preferences: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Load relationships
	if err := s.db.Preload("OAuthAccounts").Preload("Preferences").First(user, user.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load user relationships: %w", err)
	}

	return user, nil
}

// LinkOAuthAccountToUser attaches a provider identity to the account named by
// userID, which the caller has already authenticated. The provider's address
// plays no part: the session is the proof of ownership, so an address that
// matches nothing, matches another account, or is absent entirely all link the
// same way.
//
// It refuses three shapes rather than resolving them, because each one would
// otherwise silently retarget who can sign in as whom:
//
//   - the identity is attached to a different account (CodeOAuthIdentityInUse)
//   - this account already holds another identity from the same provider
//     (CodeOAuthProviderAlreadyLinked)
//   - the provider returned no subject, the value the identity resolves on
//
// It does not touch users.email or users.email_verified. A provider account is
// not evidence about the mailbox on the account it is being attached to.
func (s *UserService) LinkOAuthAccountToUser(userID uint, gothUser goth.User, provider string) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if gothUser.UserID == "" {
		return nil, fmt.Errorf("oauth provider %q returned no user id", provider)
	}

	// Everything from the account check to the write runs in ONE transaction;
	// AttachOAuthIdentityTx states how its races are decided.
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// The account has to exist before an identity is attached to it. The
	// oauth_accounts foreign key would refuse the write anyway, but a refusal
	// by design keeps this correct if that constraint is ever relaxed, and it
	// gives the caller an error naming the user rather than a driver message.
	//
	// It also has to be one that may sign in, judged in the transaction that
	// writes the link, so an account closed after any earlier check is still
	// refused. FOR SHARE: a deactivation or deletion of the row waits for this
	// transaction, so the account cannot close between this read and the write.
	var account authm.User
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&account, userID).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if !account.CanSignIn() {
		tx.Rollback()
		logger.Default().Warn("oauth_link_refused_account_inactive",
			"provider", provider,
			"user_id", userID)
		return nil, apperrors.ErrAccountInactive()
	}

	if err := AttachOAuthIdentityTx(tx, userID, provider, gothUser); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	linked, err := s.reloadUserWithRelations(userID)
	if err != nil {
		return nil, err
	}
	logger.Default().Info("oauth_link_succeeded",
		"provider", provider,
		"user_id", userID)
	return linked, nil
}

// AttachOAuthIdentityTx attaches gothUser's identity, from provider, to
// userID through tx, which the caller commits. Every path that joins an
// identity to an existing account goes through it, so they refuse the same
// shapes:
//
//   - the identity is attached to a different account (CodeOAuthIdentityInUse)
//   - userID already holds another identity from the provider
//     (CodeOAuthProviderAlreadyLinked)
//
// An identity userID already holds is refreshed, so a repeat attach is
// idempotent rather than an error. The caller has already checked that the
// account may sign in, and gothUser carries a subject.
//
// The checks alone do not serialize under READ COMMITTED: two concurrent
// attaches can both read "unclaimed". Two unique indexes decide those races:
//
//	oauth_accounts (provider, provider_user_id)  one account per identity
//	oauth_accounts_user_provider_uniq            one identity per provider
//	                                             per account
//
// Postgres reports either as the same duplicate-key error once GORM's
// TranslateError has collapsed it, so the insert runs in a savepoint and the
// loser's situation is established by reading back which of the two now
// holds.
func AttachOAuthIdentityTx(tx *gorm.DB, userID uint, provider string, gothUser goth.User) error {
	held, err := checkOAuthIdentityAttach(tx, userID, provider, gothUser.UserID)
	if err != nil {
		return err
	}
	if held != nil {
		// The write names the columns applyGothUserToOAuthAccount set (GORM
		// adds updated_at) rather than using Save: Save writes back every
		// column it read, and turns an update that matches no row into an
		// insert.
		refreshed := applyGothUserToOAuthAccount(held, gothUser)
		if err := tx.Model(held).Select(refreshed).Updates(held).Error; err != nil {
			return fmt.Errorf("failed to update OAuth account: %w", err)
		}
		return nil
	}

	oauthAccount := &authm.OAuthAccount{
		UserID:         userID,
		Provider:       provider,
		ProviderUserID: gothUser.UserID,
	}
	applyGothUserToOAuthAccount(oauthAccount, gothUser)
	err = tx.Transaction(func(sp *gorm.DB) error { return sp.Create(oauthAccount).Error })
	if err == nil || !errors.Is(err, gorm.ErrDuplicatedKey) {
		if err != nil {
			return fmt.Errorf("failed to create OAuth account: %w", err)
		}
		return nil
	}

	// The loser of a concurrent attach. When the winner attached this very
	// identity to this account, the loser asked for exactly that and
	// succeeds; otherwise it gets the refusal for whichever index now holds.
	held, err = checkOAuthIdentityAttach(tx, userID, provider, gothUser.UserID)
	if err != nil {
		return err
	}
	if held != nil {
		logger.Default().Info("oauth_link_race_resolved_idempotent",
			"provider", provider,
			"user_id", userID)
		return nil
	}
	logger.Default().Warn("oauth_link_refused_provider_already_linked_race",
		"provider", provider,
		"user_id", userID)
	return apperrors.ErrOAuthProviderAlreadyLinked(provider)
}

// checkOAuthIdentityAttach returns the row when userID already holds the
// identity, nil when the identity may be attached, and the refusal attaching
// it meets as the error. The identity's row, when there is one, is locked FOR
// UPDATE, so a concurrent connect or unlink of it waits for tx. A failed
// lookup is an error, never proof that the identity is unclaimed.
func checkOAuthIdentityAttach(tx *gorm.DB, userID uint, provider, subject string) (*authm.OAuthAccount, error) {
	var bySubject authm.OAuthAccount
	found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("provider = ? AND provider_user_id = ?", provider, subject).
		Limit(1).Find(&bySubject)
	if found.Error != nil {
		return nil, fmt.Errorf("database error: %w", found.Error)
	}
	if found.RowsAffected > 0 {
		if bySubject.UserID != userID {
			logger.Default().Warn("oauth_link_refused_identity_in_use",
				"provider", provider,
				"user_id", userID)
			return nil, apperrors.ErrOAuthIdentityInUse(provider)
		}
		return &bySubject, nil
	}

	var byProvider authm.OAuthAccount
	found = tx.Where("user_id = ? AND provider = ?", userID, provider).Limit(1).Find(&byProvider)
	if found.Error != nil {
		return nil, fmt.Errorf("database error: %w", found.Error)
	}
	if found.RowsAffected > 0 {
		logger.Default().Warn("oauth_link_refused_provider_already_linked",
			"provider", provider,
			"user_id", userID)
		return nil, apperrors.ErrOAuthProviderAlreadyLinked(provider)
	}
	return nil, nil
}

// applyGothUserToOAuthAccount copies the provider's profile and tokens onto an
// oauth_accounts row and returns the columns it set. It deliberately leaves
// ProviderUserID alone: the subject is the identity a row is keyed on, and
// rewriting it retargets which provider account signs in as the row's owner.
//
// ExpiresAt is set only when the provider supplied one, so a provider that
// stops sending an expiry does not blank a stored value.
func applyGothUserToOAuthAccount(account *authm.OAuthAccount, gothUser goth.User) []string {
	account.ProviderEmail = &gothUser.Email
	account.ProviderName = &gothUser.Name
	account.ProviderAvatarURL = &gothUser.AvatarURL
	account.AccessToken = &gothUser.AccessToken
	account.RefreshToken = &gothUser.RefreshToken
	columns := []string{"provider_email", "provider_name", "provider_avatar_url", "access_token", "refresh_token"}
	if !gothUser.ExpiresAt.IsZero() {
		account.ExpiresAt = &gothUser.ExpiresAt
		columns = append(columns, "expires_at")
	}
	return columns
}

// reloadUserWithRelations returns the user as callers of the OAuth paths
// expect it: with the associations a session response renders.
func (s *UserService) reloadUserWithRelations(userID uint) (*authm.User, error) {
	var user authm.User
	if err := s.db.Preload("OAuthAccounts").Preload("Preferences").First(&user, userID).Error; err != nil {
		return nil, fmt.Errorf("failed to load user: %w", err)
	}
	return &user, nil
}

// GetUserByID retrieves a user by ID
func (s *UserService) GetUserByID(userID uint) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user authm.User

	result := s.db.Preload("OAuthAccounts").
		Preload("Preferences").
		First(&user, userID)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return &user, nil
}

// GetUserByEmail retrieves a user by email. The match is case-insensitive
// (authm.EmailIdentityWhere), so the caller may pass the address in whatever
// casing its owner typed. An address Postgres rejects as text
// (authm.PostgresRejectsText) is not found.
func (s *UserService) GetUserByEmail(email string) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if authm.PostgresRejectsText(email) {
		return nil, nil
	}

	var user authm.User

	result := s.db.Where(authm.EmailIdentityWhere, email).
		Preload("OAuthAccounts").
		Preload("Preferences").
		First(&user)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return &user, nil
}

// GetUserByUsername retrieves a user by username. A username Postgres rejects
// as text (authm.PostgresRejectsText) is not found.
func (s *UserService) GetUserByUsername(username string) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if authm.PostgresRejectsText(username) {
		return nil, nil
	}

	var user authm.User

	result := s.db.Where("username = ?", username).
		Preload("OAuthAccounts").
		Preload("Preferences").
		First(&user)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return &user, nil
}

// UpdateUser updates user information
func (s *UserService) UpdateUser(userID uint, updates map[string]any) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user authm.User

	result := s.db.Model(&user).
		Where("id = ?", userID).
		Updates(updates)

	if result.Error != nil {
		// A unique-constraint violation on profile update means the chosen
		// username is taken. Translate the typed DB error into the user-facing
		// CodeUsernameTaken so handlers discriminate on a code instead of the
		// raw driver message.
		if shared.IsDuplicateKey(result.Error) {
			return nil, apperrors.ErrUsernameTaken(result.Error)
		}
		return nil, fmt.Errorf("failed to update user: %w", result.Error)
	}

	return s.GetUserByID(userID)
}

func (s *UserService) HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hashedBytes), nil
}

func (s *UserService) VerifyPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// IsAccountLocked checks if the user account is currently locked
func (s *UserService) IsAccountLocked(user *authm.User) bool {
	return user.LockedUntil != nil && time.Now().Before(*user.LockedUntil)
}

// GetLockTimeRemaining returns the duration until the account is unlocked
func (s *UserService) GetLockTimeRemaining(user *authm.User) time.Duration {
	if user.LockedUntil == nil {
		return 0
	}
	remaining := time.Until(*user.LockedUntil)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IncrementFailedAttempts increments the counter and locks the account for
// AccountLockDuration when the incremented count reaches MaxFailedLoginAttempts.
//
// It is one UPDATE of the two lockout columns (and updated_at, which GORM adds):
// Postgres evaluates every SET expression against the row it locked, so
// concurrent failures are all counted and is_active and deleted_at are never
// written.
func (s *UserService) IncrementFailedAttempts(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	lockUntil := time.Now().Add(AccountLockDuration)
	result := s.db.Model(&authm.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"failed_login_attempts": gorm.Expr("failed_login_attempts + 1"),
			"locked_until": gorm.Expr(
				"CASE WHEN failed_login_attempts + 1 >= ? THEN ? ELSE locked_until END",
				MaxFailedLoginAttempts, lockUntil,
			),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to increment failed attempts: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("failed to increment failed attempts: %w", gorm.ErrRecordNotFound)
	}

	return nil
}

// ResetFailedAttempts clears the counter and unlocks the account
func (s *UserService) ResetFailedAttempts(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&authm.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"failed_login_attempts": 0,
			"locked_until":          nil,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to reset failed attempts: %w", result.Error)
	}

	return nil
}

// UpdatePassword updates a user's password after verifying the current password
func (s *UserService) UpdatePassword(userID uint, currentPassword, newPassword string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Get user with current password hash
	user, err := s.GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user has a password (not OAuth-only)
	if user.PasswordHash == nil {
		return apperrors.ErrNoPasswordSet()
	}

	// Verify current password
	if err := s.VerifyPassword(*user.PasswordHash, currentPassword); err != nil {
		return apperrors.ErrInvalidCredentials(nil)
	}

	// Hash the new password
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Only over the hash just verified: a password cleared or changed while
	// this request ran (the mailbox proof of a pending provider link clears
	// it to evict whoever set it) is not put back by a request that proved
	// the old one.
	result := s.db.Model(&authm.User{}).
		Where("id = ? AND password_hash = ?", userID, *user.PasswordHash).
		Update("password_hash", hashedPassword)

	if result.Error != nil {
		return fmt.Errorf("failed to update password: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return apperrors.ErrInvalidCredentials(nil)
	}

	return nil
}

// SetEmailVerified updates the email_verified status for a user
func (s *UserService) SetEmailVerified(userID uint, verified bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&authm.User{}).
		Where("id = ?", userID).
		Update("email_verified", verified)

	if result.Error != nil {
		return fmt.Errorf("failed to update email verified status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// DeletionSummary contains counts of user data that will be affected by deletion

// GetDeletionSummary returns counts of data that will be affected by account deletion
func (s *UserService) GetDeletionSummary(userID uint) (*contracts.DeletionSummary, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	summary := &contracts.DeletionSummary{}

	// Count shows submitted by this user
	if err := s.db.Model(&catalogm.Show{}).Where("submitted_by = ?", userID).Count(&summary.ShowsCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count shows: %w", err)
	}

	// Count saved shows (bookmarks with entity_type='show' and action='save')
	if err := s.db.Model(&engagementm.UserBookmark{}).Where("user_id = ? AND entity_type = ? AND action = ?", userID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave).Count(&summary.SavedShowsCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count saved shows: %w", err)
	}

	if err := s.db.Table("user_bookmarks").
		Joins("JOIN releases ON releases.id = user_bookmarks.entity_id").
		Where(
			"user_id = ? AND entity_type = ? AND action = ?",
			userID, engagementm.BookmarkEntityRelease, engagementm.BookmarkActionReleaseSave,
		).Count(&summary.SavedReleasesCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count saved releases: %w", err)
	}

	// Count passkeys
	if err := s.db.Model(&authm.WebAuthnCredential{}).Where("user_id = ?", userID).Count(&summary.PasskeysCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count passkeys: %w", err)
	}

	return summary, nil
}

// SoftDeleteAccount performs a soft delete on the user account
// Sets is_active=false, deleted_at=now, and stores optional deletion reason
func (s *UserService) SoftDeleteAccount(userID uint, reason *string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	updates := map[string]interface{}{
		"is_active":  false,
		"deleted_at": now,
	}

	if reason != nil && *reason != "" {
		updates["deletion_reason"] = *reason
	}

	result := s.db.Model(&authm.User{}).
		Where("id = ?", userID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to soft delete user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// ExportUserData exports all user data in a portable JSON format
// This supports GDPR Article 20 - Right to data portability
func (s *UserService) ExportUserData(userID uint) (*contracts.UserDataExport, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get user with all relationships
	var user authm.User
	if err := s.db.
		Preload("OAuthAccounts").
		Preload("PasskeyCredentials").
		First(&user, userID).Error; err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	export := &contracts.UserDataExport{
		ExportedAt:    time.Now().UTC(),
		ExportVersion: "1.0",
		Profile: contracts.UserProfileExport{
			ID:            user.ID,
			Email:         user.Email,
			Username:      user.Username,
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			AvatarURL:     user.AvatarURL,
			Bio:           user.Bio,
			EmailVerified: user.EmailVerified,
			DateOfBirth:   exportedDateOfBirth(user.DateOfBirth),
			AgeAttestedAt: user.AgeAttestedAt,
			CreatedAt:     user.CreatedAt,
			UpdatedAt:     user.UpdatedAt,
		},
	}

	// Export OAuth accounts (without tokens)
	for _, oauth := range user.OAuthAccounts {
		export.OAuthAccounts = append(export.OAuthAccounts, contracts.OAuthAccountExport{
			Provider:      oauth.Provider,
			ProviderEmail: oauth.ProviderEmail,
			ProviderName:  oauth.ProviderName,
			LinkedAt:      oauth.CreatedAt,
		})
	}

	// Export passkey metadata (no keys)
	for _, passkey := range user.PasskeyCredentials {
		export.Passkeys = append(export.Passkeys, contracts.PasskeyExport{
			DisplayName:    passkey.DisplayName,
			CreatedAt:      passkey.CreatedAt,
			LastUsedAt:     passkey.LastUsedAt,
			BackupEligible: passkey.BackupEligible,
			BackupState:    passkey.BackupState,
		})
	}

	// Get saved shows with venue info (from user_bookmarks)
	var savedBookmarks []engagementm.UserBookmark
	if err := s.db.Where("user_id = ? AND entity_type = ? AND action = ?", userID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave).Find(&savedBookmarks).Error; err != nil {
		return nil, fmt.Errorf("failed to get saved shows: %w", err)
	}

	for _, sb := range savedBookmarks {
		var show catalogm.Show
		if err := s.db.Preload("Venues").First(&show, sb.EntityID).Error; err != nil {
			continue // Skip if show not found
		}

		savedExport := contracts.SavedShowExport{
			ShowID:    show.ID,
			Title:     show.Title,
			EventDate: show.EventDate,
			City:      show.City,
			SavedAt:   sb.CreatedAt,
		}

		if len(show.Venues) > 0 {
			savedExport.Venue = &show.Venues[0].Name
		}

		export.SavedShows = append(export.SavedShows, savedExport)
	}

	if s.savedReleaseService == nil {
		return nil, fmt.Errorf("saved release service not initialized")
	}
	savedReleases, _, err := s.savedReleaseService.GetUserSavedReleases(userID, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get saved releases: %w", err)
	}
	for _, release := range savedReleases {
		export.SavedReleases = append(export.SavedReleases, contracts.SavedReleaseExport{
			ReleaseID:   release.ID,
			Title:       release.Title,
			ReleaseType: release.ReleaseType,
			ReleaseYear: release.ReleaseYear,
			SavedAt:     release.SavedAt,
		})
	}

	// Export submitted shows with details
	var submittedShows []catalogm.Show
	if err := s.db.
		Preload("Venues").
		Preload("Artists").
		Where("submitted_by = ?", userID).
		Find(&submittedShows).Error; err != nil {
		return nil, fmt.Errorf("failed to get submitted shows: %w", err)
	}

	for _, show := range submittedShows {
		submittedExport := contracts.SubmittedShowExport{
			ShowID:      show.ID,
			Title:       show.Title,
			EventDate:   show.EventDate,
			Status:      string(show.Status),
			SubmittedAt: show.CreatedAt,
			City:        show.City,
		}

		if len(show.Venues) > 0 {
			submittedExport.Venue = &show.Venues[0].Name
		}

		for _, artist := range show.Artists {
			submittedExport.Artists = append(submittedExport.Artists, artist.Name)
		}

		export.SubmittedShows = append(export.SubmittedShows, submittedExport)
	}

	return export, nil
}

// exportedDateOfBirth is the stored date of birth as its calendar date, nil
// when none is stored.
func exportedDateOfBirth(dateOfBirth *time.Time) *string {
	if dateOfBirth == nil {
		return nil
	}
	formatted := legal.FormatDateOfBirth(*dateOfBirth)
	return &formatted
}

// ExportUserDataJSON exports user data as a JSON byte slice
func (s *UserService) ExportUserDataJSON(userID uint) ([]byte, error) {
	export, err := s.ExportUserData(userID)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(export, "", "  ")
}

// GetFavoriteCities returns a user's favorite cities from their preferences
func (s *UserService) GetFavoriteCities(userID uint) ([]authm.FavoriteCity, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var prefs authm.UserPreferences
	if err := s.db.Where("user_id = ?", userID).First(&prefs).Error; err != nil {
		return nil, fmt.Errorf("failed to get user preferences: %w", err)
	}

	if prefs.FavoriteCities == nil {
		return []authm.FavoriteCity{}, nil
	}

	var cities []authm.FavoriteCity
	if err := json.Unmarshal(*prefs.FavoriteCities, &cities); err != nil {
		return nil, fmt.Errorf("failed to unmarshal favorite cities: %w", err)
	}

	return cities, nil
}

// SetFavoriteCities replaces a user's favorite cities list
func (s *UserService) SetFavoriteCities(userID uint, cities []authm.FavoriteCity) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Validate max 20 cities
	if len(cities) > 20 {
		return fmt.Errorf("maximum 20 favorite cities allowed")
	}

	// Validate each city has non-empty values
	for _, c := range cities {
		if c.City == "" || c.State == "" {
			return fmt.Errorf("city and state must not be empty")
		}
	}

	data, err := json.Marshal(cities)
	if err != nil {
		return fmt.Errorf("failed to marshal favorite cities: %w", err)
	}

	raw := json.RawMessage(data)

	// Upsert preferences row
	result := s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update("favorite_cities", &raw)

	if result.Error != nil {
		return fmt.Errorf("failed to update favorite cities: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		// No preferences row yet — create one
		prefs := &authm.UserPreferences{
			UserID:         userID,
			FavoriteCities: &raw,
		}
		if err := s.db.Create(prefs).Error; err != nil {
			return fmt.Errorf("failed to create user preferences: %w", err)
		}
	}

	return nil
}

// SetChartDefaults replaces a user's /charts landing defaults (PSY-1423).
// Pass nil to clear. Window must be month|quarter|all_time; scene is an optional
// metro code (nil/empty = all scenes).
func (s *UserService) SetChartDefaults(userID uint, defaults *authm.ChartDefaults) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var raw *json.RawMessage
	if defaults != nil {
		switch defaults.Window {
		case "month", "quarter", "all_time":
		default:
			return fmt.Errorf("invalid chart window: %s", defaults.Window)
		}
		if defaults.Scene != nil {
			scene := strings.TrimSpace(*defaults.Scene)
			if scene == "" {
				defaults.Scene = nil
			} else if !chartSceneMetroRe.MatchString(scene) {
				return fmt.Errorf("invalid chart scene metro: %s", scene)
			} else {
				defaults.Scene = &scene
			}
		}
		data, err := json.Marshal(defaults)
		if err != nil {
			return fmt.Errorf("failed to marshal chart defaults: %w", err)
		}
		msg := json.RawMessage(data)
		raw = &msg
	}

	result := s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update("chart_defaults", raw)

	if result.Error != nil {
		return fmt.Errorf("failed to update chart defaults: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		prefs := &authm.UserPreferences{
			UserID:        userID,
			ChartDefaults: raw,
		}
		if err := s.db.Create(prefs).Error; err != nil {
			return fmt.Errorf("failed to create user preferences: %w", err)
		}
	}

	return nil
}

var chartSceneMetroRe = regexp.MustCompile(`^[0-9]{1,10}$`)

// SetShowReminders enables or disables show reminders for a user
func (s *UserService) SetShowReminders(userID uint, enabled bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update("show_reminders", enabled)

	if result.Error != nil {
		return fmt.Errorf("failed to update show reminders: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		prefs := &authm.UserPreferences{
			UserID:        userID,
			ShowReminders: enabled,
		}
		if err := s.db.Create(prefs).Error; err != nil {
			return fmt.Errorf("failed to create user preferences: %w", err)
		}
	}

	return nil
}

// SetDefaultReplyPermission sets the user's default reply_permission value
// applied to new top-level comments. PSY-296.
func (s *UserService) SetDefaultReplyPermission(userID uint, permission string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if !engagementm.IsValidReplyPermission(permission) {
		return apperrors.ErrInvalidReplyPermission(permission)
	}

	result := s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update("default_reply_permission", permission)

	if result.Error != nil {
		return fmt.Errorf("failed to update default reply permission: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		prefs := &authm.UserPreferences{
			UserID:                 userID,
			DefaultReplyPermission: permission,
		}
		if err := s.db.Create(prefs).Error; err != nil {
			return fmt.Errorf("failed to create user preferences: %w", err)
		}
	}

	return nil
}

// SetNotifyOnCommentSubscription toggles the comment-subscription email
// preference. PSY-289. Always-upsert semantics: if a preferences row doesn't
// yet exist, one is created with the passed value plus the other comment
// notification pref defaulted to true.
func (s *UserService) SetNotifyOnCommentSubscription(userID uint, enabled bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return s.setCommentNotificationPref(userID, map[string]interface{}{
		"notify_on_comment_subscription": enabled,
	})
}

// SetNotifyOnMention toggles the mention-notification email preference.
// PSY-289. Upserts — see SetNotifyOnCommentSubscription.
func (s *UserService) SetNotifyOnMention(userID uint, enabled bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return s.setCommentNotificationPref(userID, map[string]interface{}{
		"notify_on_mention": enabled,
	})
}

// SetNotifyOnCollectionDigest toggles the collection digest email
// preference (PSY-350). Always-upsert: if no preferences row exists, create
// one with this flag and the other notification flags at their defaults.
//
// Defaults differ by notification type:
//   - notify_on_comment_subscription / notify_on_mention default TRUE
//     (opt-OUT) — single email per direct interaction.
//   - notify_on_collection_digest defaults FALSE (opt-IN) — subscribing
//     to a collection implicitly subscribes to every future addition
//     by anyone, so we keep the firehose explicit.
func (s *UserService) SetNotifyOnCollectionDigest(userID uint, enabled bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update("notify_on_collection_digest", enabled)
	if result.Error != nil {
		return fmt.Errorf("failed to update notify_on_collection_digest: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No prefs row exists yet — create one. The GORM bool zero-value
		// gotcha only bites when the desired value collides with the column
		// default. NotifyOnCommentSubscription / NotifyOnMention default to
		// TRUE at the column level, so passing TRUE is fine. NotifyOnCollectionDigest
		// defaults to FALSE (opt-IN), so passing FALSE is fine, but passing
		// TRUE would also be skipped by GORM's zero-value omit only if it
		// were false; since we're passing the caller's `enabled`, we follow
		// up with an explicit Update only when enabling — matching the
		// pattern used in ToggleCollectionDigest tests for false toggles.
		prefs := &authm.UserPreferences{
			UserID:                      userID,
			NotifyOnCommentSubscription: true,
			NotifyOnMention:             true,
			NotifyOnCollectionDigest:    enabled,
		}
		if err := s.db.Create(prefs).Error; err != nil {
			return fmt.Errorf("failed to create user preferences: %w", err)
		}
		// If the caller asked to enable but the column default is false,
		// GORM's Create writes `true` correctly because true is non-zero.
		// If the caller asked to disable, the column default is false and
		// GORM skipped the column → row reflects false. No follow-up Update
		// is needed in either branch.
	}
	return nil
}

// SetNotifyOnSceneDigest toggles the weekly scene-digest email preference
// (PSY-1342). Opt-IN (column default FALSE). Upserts — see setNotificationFlagPref.
func (s *UserService) SetNotifyOnSceneDigest(userID uint, enabled bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return s.setNotificationFlagPref(userID, "notify_on_scene_digest", enabled)
}

// SetNotifyOnTierNotifications toggles the tier-change email preference.
// Upserts — see setNotificationFlagPref.
func (s *UserService) SetNotifyOnTierNotifications(userID uint, enabled bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return s.setNotificationFlagPref(userID, "notify_on_tier_notifications", enabled)
}

// SetNotifyOnEditNotifications toggles the edit-review email preference.
// Upserts — see setNotificationFlagPref.
func (s *UserService) SetNotifyOnEditNotifications(userID uint, enabled bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return s.setNotificationFlagPref(userID, "notify_on_edit_notifications", enabled)
}

// setCommentNotificationPref updates one or both of the PSY-289 preference
// flags on the user_preferences row, creating the row if it doesn't exist.
func (s *UserService) setCommentNotificationPref(userID uint, updates map[string]interface{}) error {
	result := s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update notification preferences: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// No preferences row yet — create one. Both prefs default to TRUE,
		// then apply the incoming override(s).
		prefs := &authm.UserPreferences{
			UserID:                      userID,
			NotifyOnCommentSubscription: true,
			NotifyOnMention:             true,
		}
		if v, ok := updates["notify_on_comment_subscription"].(bool); ok {
			prefs.NotifyOnCommentSubscription = v
		}
		if v, ok := updates["notify_on_mention"].(bool); ok {
			prefs.NotifyOnMention = v
		}
		if err := s.db.Create(prefs).Error; err != nil {
			return fmt.Errorf("failed to create user preferences: %w", err)
		}
	}
	return nil
}

// setNotificationFlagPref updates a single default-TRUE notification flag,
// creating the preferences row if it doesn't exist yet. Used by the tier/edit
// toggles. The created row carries the other default-TRUE flags
// (comment/mention/tier/edit) at TRUE and applies the one incoming override.
//
// GORM bool gotcha: a freshly Create-d struct skips false fields (zero value),
// but every column here defaults TRUE, so a "disable" path (enabled=false on
// the target column) needs an explicit follow-up Update. We just rebuild the
// targeted column with Update after Create when disabling.
func (s *UserService) setNotificationFlagPref(userID uint, column string, enabled bool) error {
	result := s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update(column, enabled)
	if result.Error != nil {
		return fmt.Errorf("failed to update %s: %w", column, result.Error)
	}
	if result.RowsAffected == 0 {
		prefs := &authm.UserPreferences{
			UserID:                      userID,
			NotifyOnCommentSubscription: true,
			NotifyOnMention:             true,
			NotifyOnTierNotifications:   true,
			NotifyOnEditNotifications:   true,
		}
		if err := s.db.Create(prefs).Error; err != nil {
			return fmt.Errorf("failed to create user preferences: %w", err)
		}
		// Create skipped the column if `enabled` is false (zero value) and the
		// column default is TRUE — apply the disable explicitly.
		if !enabled {
			if err := s.db.Model(&authm.UserPreferences{}).
				Where("user_id = ?", userID).
				Update(column, false).Error; err != nil {
				return fmt.Errorf("failed to update %s: %w", column, err)
			}
		}
	}
	return nil
}

// GetOAuthAccounts returns all OAuth accounts linked to a user
func (s *UserService) GetOAuthAccounts(userID uint) ([]authm.OAuthAccount, error) {
	var accounts []authm.OAuthAccount
	err := s.db.Where("user_id = ?", userID).Find(&accounts).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth accounts: %w", err)
	}
	return accounts, nil
}

// GetUserByEmailIncludingDeleted retrieves a user by email, including
// soft-deleted accounts. The match is case-insensitive
// (authm.EmailIdentityWhere). An address Postgres rejects as text
// (authm.PostgresRejectsText) is not found.
func (s *UserService) GetUserByEmailIncludingDeleted(email string) (*authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if authm.PostgresRejectsText(email) {
		return nil, nil
	}

	var user authm.User

	// Query without filtering by is_active to include soft-deleted accounts
	result := s.db.Where(authm.EmailIdentityWhere, email).
		Preload("OAuthAccounts").
		Preload("Preferences").
		First(&user)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return &user, nil
}

// IsAccountRecoverable reports whether user is a soft-deleted account still
// inside its recovery window (shared.IsAccountRecoverable).
func (s *UserService) IsAccountRecoverable(user *authm.User) bool {
	return shared.IsAccountRecoverable(user)
}

// GetDaysUntilPermanentDeletion returns the number of days until an account is permanently deleted
// Returns 0 if account is not recoverable
func (s *UserService) GetDaysUntilPermanentDeletion(user *authm.User) int {
	if user == nil || user.DeletedAt == nil || user.IsActive {
		return 0
	}

	gracePeriodEnd := user.DeletedAt.Add(shared.AccountRecoveryGracePeriod)
	remaining := time.Until(gracePeriodEnd)

	if remaining <= 0 {
		return 0
	}

	// Round up to nearest day
	days := int(remaining.Hours()/24) + 1
	return days
}

// RestoreAccount restores a soft-deleted account
func (s *UserService) RestoreAccount(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&authm.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"is_active":       true,
			"deleted_at":      nil,
			"deletion_reason": nil,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to restore account: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// GetExpiredDeletedAccounts returns users whose accounts have been soft-deleted
// for longer than the grace period and are ready for permanent deletion
func (s *UserService) GetExpiredDeletedAccounts() ([]authm.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	cutoffDate := time.Now().Add(-shared.AccountRecoveryGracePeriod)

	var users []authm.User
	result := s.db.
		Where("is_active = ? AND deleted_at IS NOT NULL AND deleted_at < ?", false, cutoffDate).
		Find(&users)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get expired deleted accounts: %w", result.Error)
	}

	return users, nil
}

// PermanentlyDeleteUser hard-deletes a user and all associated data
// This should only be called for accounts that have exceeded the grace period
func (s *UserService) PermanentlyDeleteUser(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Set shows.submitted_by to NULL for shows submitted by this user
		// (this is already handled by ON DELETE SET NULL in the FK, but being explicit)
		if err := tx.Model(&catalogm.Show{}).
			Where("submitted_by = ?", userID).
			Update("submitted_by", nil).Error; err != nil {
			return fmt.Errorf("failed to nullify show submissions: %w", err)
		}

		// Hard delete the user (cascades will handle related data like OAuth accounts,
		// preferences, passkeys, saved shows, favorite venues)
		if err := tx.Unscoped().Delete(&authm.User{}, userID).Error; err != nil {
			return fmt.Errorf("failed to permanently delete user: %w", err)
		}

		return nil
	})
}

// CanUnlinkOAuthAccount checks if a user can safely unlink an OAuth account
// Returns (canUnlink, reason, error)
func (s *UserService) CanUnlinkOAuthAccount(userID uint, provider string) (bool, string, error) {
	// Get user
	var user authm.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return false, "", fmt.Errorf("failed to get user: %w", err)
	}

	// Check if this OAuth account exists
	var oauthAccount authm.OAuthAccount
	err := s.db.Where("user_id = ? AND provider = ?", userID, provider).First(&oauthAccount).Error
	if err != nil {
		return false, "OAuth account not found", nil
	}

	// Count alternative auth methods
	hasPassword := user.PasswordHash != nil && *user.PasswordHash != ""

	// Count other OAuth accounts
	var otherOAuthCount int64
	s.db.Model(&authm.OAuthAccount{}).Where("user_id = ? AND provider != ?", userID, provider).Count(&otherOAuthCount)

	// Count passkeys
	var passkeyCount int64
	s.db.Model(&authm.WebAuthnCredential{}).Where("user_id = ?", userID).Count(&passkeyCount)

	// User must have at least one other auth method
	if !hasPassword && otherOAuthCount == 0 && passkeyCount == 0 {
		return false, "Cannot unlink - this is your only sign-in method. Add a password or passkey first.", nil
	}

	return true, "", nil
}

// UnlinkOAuthAccount removes an OAuth account from a user
func (s *UserService) UnlinkOAuthAccount(userID uint, provider string) error {
	return UnlinkOAuthAccountTx(s.db, userID, provider)
}

// UnlinkOAuthAccountTx removes userID's identity from provider through tx,
// which the caller commits. It refuses when the account holds none. Whether
// the account keeps a way to sign in is the caller's check
// (CanUnlinkOAuthAccount); this removes the identity either way.
func UnlinkOAuthAccountTx(tx *gorm.DB, userID uint, provider string) error {
	result := tx.Where("user_id = ? AND provider = ?", userID, provider).Delete(&authm.OAuthAccount{})
	if result.Error != nil {
		return fmt.Errorf("failed to unlink OAuth account: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("OAuth account not found")
	}
	return nil
}
