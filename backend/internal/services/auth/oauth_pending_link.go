package auth

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// A pending link holds a provider identity whose verified address belongs to
// an account it is not attached to. It joins that account only after one of
// two proofs that the person holds the account: its password
// (LinkOAuthWithPassword) or its mailbox (an emailed link,
// CompleteOAuthLinkByEmail). Each proof consumes the pending link in the
// transaction that acts on it, so whichever lands first is the only one that
// acts, and the other is refused.

var errPendingLinkAccountMismatch = errors.New("pending link names another account or address")

// StartOAuthLink stores identity as a pending link to userID, with returnTo,
// for contracts.OAuthPendingLinkTTL, and returns the token that names it.
func (s *AuthService) StartOAuthLink(identity contracts.OAuthIdentity, userID uint, returnTo string) (string, error) {
	row := pendingRowFromIdentity(identity, returnTo)
	row.UserID = &userID
	return s.storePending(row, pendingLinkPurpose, contracts.OAuthPendingLinkTTL)
}

// PeekOAuthLink returns the pending link token names, leaving it in place.
func (s *AuthService) PeekOAuthLink(token string) (*contracts.OAuthPendingLink, error) {
	row, hash, err := s.readPending(s.db, pendingLinkPurpose, token, peekPendingRow)
	if err != nil {
		return nil, err
	}
	link := &contracts.OAuthPendingLink{
		Identity:  identityFromPendingRow(row),
		ReturnTo:  row.ReturnTo,
		ExpiresAt: row.ExpiresAt,
		Ref:       hash,
	}
	if row.UserID != nil {
		link.UserID = *row.UserID
	}
	return link, nil
}

// LinkOAuthWithPassword finishes the pending link token names for user, whose
// password the caller has just verified (contracts.AuthServiceInterface).
func (s *AuthService) LinkOAuthWithPassword(token string, user *authm.User) (bool, error) {
	if user == nil {
		return false, fmt.Errorf("user cannot be nil")
	}
	hash, err := verifyPendingToken(s.pendingSignupSecret(), oauthPendingLinkDomain, token)
	if err != nil {
		return false, apperrors.ErrOAuthLinkPendingExpired(err)
	}
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	linked := false
	var refusal *apperrors.AuthError
	err = s.db.Transaction(func(tx *gorm.DB) error {
		row, err := readPendingByHash(tx, pendingLinkPurpose, hash, consumePendingRow)
		if err != nil {
			return err
		}
		account, err := lockPendingLinkAccount(tx, row, user.ID, "")
		if errors.Is(err, errPendingLinkAccountMismatch) {
			// Another account's pending link, or one whose address has moved:
			// this sign-in proves nothing about it, so it is discarded and
			// the sign-in goes ahead on its own.
			logger.Default().Warn("oauth_pending_link_discarded_other_account",
				"provider", row.Provider,
				"user_id", user.ID)
			return nil
		}
		if err != nil {
			return err
		}
		if !account.CanSignIn() {
			refusal = shared.ClosedAccountRefusal(account)
			return nil
		}
		// A refusal to attach undoes only the savepoint; the consumed pending
		// link commits.
		refusal, err = attachInSavepoint(tx, func(sp *gorm.DB) error {
			return attachPendingIdentity(sp, account.ID, identityFromPendingRow(row))
		})
		if err != nil {
			return err
		}
		linked = refusal == nil
		return nil
	})
	if err != nil {
		return false, err
	}
	if refusal != nil {
		return false, refusal
	}
	if linked {
		logger.Default().Info("oauth_pending_link_completed", "proof", "password", "user_id", user.ID)
	}
	return linked, nil
}

// CompleteOAuthLinkByEmail finishes the pending link an emailed sign-in link
// names (contracts.AuthServiceInterface).
func (s *AuthService) CompleteOAuthLinkByEmail(claims *contracts.OAuthLinkTokenClaims) (*contracts.OAuthLinkCompletion, error) {
	if claims == nil {
		return nil, fmt.Errorf("claims cannot be nil")
	}
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var accountID uint
	var returnTo string
	var refusal *apperrors.AuthError
	err := s.db.Transaction(func(tx *gorm.DB) error {
		row, err := readPendingByHash(tx, pendingLinkPurpose, claims.PendingLink, consumePendingRow)
		if err != nil {
			return err
		}
		account, err := lockPendingLinkAccount(tx, row, claims.UserID, claims.Email)
		if errors.Is(err, errPendingLinkAccountMismatch) {
			refusal = apperrors.ErrOAuthLinkPendingExpired(err)
			return nil
		}
		if err != nil {
			return err
		}
		if !account.CanSignIn() {
			refusal = shared.ClosedAccountRefusal(account)
			return nil
		}

		// Everything the proof changes on the account, or none of it: a
		// refusal to attach undoes the savepoint, so the password and the
		// sessions stay as they were, and the consumed pending link commits.
		refusal, err = attachInSavepoint(tx, func(sp *gorm.DB) error {
			if err := attachPendingIdentity(sp, account.ID, identityFromPendingRow(row)); err != nil {
				return err
			}
			// The mailbox proves the address and says nothing about whoever
			// set the password: a password set by someone who registered the
			// address first must stop working, so it is cleared.
			if err := sp.Model(&authm.User{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
				"password_hash":         nil,
				"email_verified":        true,
				"failed_login_attempts": 0,
				"locked_until":          nil,
			}).Error; err != nil {
				return fmt.Errorf("failed to clear password: %w", err)
			}
			return RevokeAllSessions(sp, account, time.Now())
		})
		if err != nil {
			return err
		}
		accountID = account.ID
		returnTo = row.ReturnTo
		return nil
	})
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		return nil, refusal
	}

	user, err := s.userService.GetUserByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to load linked account: %w", err)
	}
	token, err := s.jwtService.createTokenAfterRevocation(user)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}
	logger.Default().Info("oauth_pending_link_completed", "proof", "mailbox", "user_id", user.ID)
	return &contracts.OAuthLinkCompletion{User: user, Token: token, ReturnTo: returnTo}, nil
}

// attachInSavepoint runs attach in a savepoint of tx. A refusal attach
// returns is handed back with a nil error, after the savepoint has undone
// whatever attach wrote, so the caller's transaction can still commit; any
// other error is returned as the error.
func attachInSavepoint(tx *gorm.DB, attach func(sp *gorm.DB) error) (*apperrors.AuthError, error) {
	err := tx.Transaction(attach)
	var refusal *apperrors.AuthError
	if errors.As(err, &refusal) {
		return refusal, nil
	}
	return nil, err
}

// lockPendingLinkAccount locks and returns the account row names, FOR UPDATE,
// so a deactivation, deletion or address change waits for the proof's
// transaction. It refuses with errPendingLinkAccountMismatch when row names
// an account other than userID, or when that account's address is no longer
// the identity's (or, when tokenEmail is set, the emailed link's).
func lockPendingLinkAccount(tx *gorm.DB, row *authm.OAuthPendingSignup, userID uint, tokenEmail string) (*authm.User, error) {
	if row.UserID == nil || *row.UserID != userID {
		return nil, errPendingLinkAccountMismatch
	}
	var account authm.User
	found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).Limit(1).Find(&account)
	if found.Error != nil {
		return nil, fmt.Errorf("failed to lock account: %w", found.Error)
	}
	if found.RowsAffected == 0 {
		return nil, errPendingLinkAccountMismatch
	}
	if account.Email == nil || !authm.SameEmailIdentity(*account.Email, row.Email) {
		return nil, errPendingLinkAccountMismatch
	}
	if tokenEmail != "" && !authm.SameEmailIdentity(*account.Email, tokenEmail) {
		return nil, errPendingLinkAccountMismatch
	}
	return &account, nil
}

// attachPendingIdentity attaches identity to userID through tx, refusing with
// CodeOAuthIdentityInUse when another account holds the identity and with
// CodeOAuthProviderAlreadyLinked when userID holds a different identity from
// the provider. An identity userID already holds is left as it is. The
// unique indexes on oauth_accounts decide a concurrent attach; the loser gets
// the refusal for whichever now holds.
func attachPendingIdentity(tx *gorm.DB, userID uint, identity contracts.OAuthIdentity) error {
	if identity.Subject == "" {
		return fmt.Errorf("oauth provider %q returned no user id", identity.Provider)
	}
	held, err := checkPendingIdentityAttach(tx, userID, identity)
	if err != nil || held {
		return err
	}

	account := &authm.OAuthAccount{
		UserID:            userID,
		Provider:          identity.Provider,
		ProviderUserID:    identity.Subject,
		ProviderEmail:     &identity.Email,
		ProviderName:      &identity.Name,
		ProviderAvatarURL: &identity.AvatarURL,
	}
	if err := tx.Transaction(func(sp *gorm.DB) error { return sp.Create(account).Error }); err != nil {
		if !errors.Is(err, gorm.ErrDuplicatedKey) {
			return fmt.Errorf("failed to create OAuth account: %w", err)
		}
		if _, recheck := checkPendingIdentityAttach(tx, userID, identity); recheck != nil {
			return recheck
		}
		return apperrors.ErrOAuthProviderAlreadyLinked(identity.Provider)
	}
	return nil
}

// checkPendingIdentityAttach reports whether userID already holds identity,
// or the refusal attaching it meets now as the error. The identity's row,
// when there is one, is locked FOR UPDATE, so an unlink of it waits.
func checkPendingIdentityAttach(tx *gorm.DB, userID uint, identity contracts.OAuthIdentity) (bool, error) {
	var bySubject authm.OAuthAccount
	found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("provider = ? AND provider_user_id = ?", identity.Provider, identity.Subject).
		Limit(1).Find(&bySubject)
	if found.Error != nil {
		return false, fmt.Errorf("database error: %w", found.Error)
	}
	if found.RowsAffected > 0 {
		if bySubject.UserID != userID {
			return false, apperrors.ErrOAuthIdentityInUse(identity.Provider)
		}
		return true, nil
	}
	var byProvider authm.OAuthAccount
	found = tx.Where("user_id = ? AND provider = ?", userID, identity.Provider).Limit(1).Find(&byProvider)
	if found.Error != nil {
		return false, fmt.Errorf("database error: %w", found.Error)
	}
	if found.RowsAffected > 0 {
		return false, apperrors.ErrOAuthProviderAlreadyLinked(identity.Provider)
	}
	return false, nil
}
