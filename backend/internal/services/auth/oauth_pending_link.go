package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/markbates/goth"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	usersvc "psychic-homily-backend/internal/services/user"
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
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	linked := false
	var refusal *apperrors.AuthError
	err := s.db.Transaction(func(tx *gorm.DB) error {
		row, _, err := s.readPending(tx, pendingLinkPurpose, token, consumePendingRow)
		if err != nil {
			return err
		}
		account, err := takePendingLink(tx, row, user.ID, "")
		if errors.Is(err, errPendingLinkAccountMismatch) {
			// Another account's pending link, or one whose address has moved:
			// this sign-in proves nothing about it, so it is discarded and
			// the sign-in goes ahead on its own.
			logger.Default().Warn("oauth_pending_link_discarded_other_account",
				"provider", row.Provider,
				"user_id", user.ID)
			return nil
		}
		if refusal = asRefusal(err); refusal != nil || err != nil {
			return ignoreRefusal(err)
		}
		refusal, err = attachInSavepoint(tx, func(sp *gorm.DB) error {
			return attachPendingIdentity(sp, account.ID, row)
		})
		linked = err == nil && refusal == nil
		return err
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
		account, err := takePendingLink(tx, row, claims.UserID, claims.Email)
		if errors.Is(err, errPendingLinkAccountMismatch) {
			refusal = apperrors.ErrOAuthLinkPendingExpired(err)
			return nil
		}
		if refusal = asRefusal(err); refusal != nil || err != nil {
			return ignoreRefusal(err)
		}

		// Everything the proof changes on the account, or none of it: a
		// refusal to attach undoes the savepoint, so the password and the
		// sessions stay as they were, and the consumed pending link commits.
		refusal, err = attachInSavepoint(tx, func(sp *gorm.DB) error {
			if err := attachPendingIdentity(sp, account.ID, row); err != nil {
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
		accountID = account.ID
		returnTo = row.ReturnTo
		return err
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
	token, err := s.jwtService.CreateToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}
	logger.Default().Info("oauth_pending_link_completed", "proof", "mailbox", "user_id", user.ID)
	return &contracts.OAuthLinkCompletion{User: user, Token: token, ReturnTo: returnTo}, nil
}

// takePendingLink locks and returns the account a consumed pending link
// names, FOR UPDATE, so a deactivation, deletion or address change waits for
// the proof's transaction. It refuses with errPendingLinkAccountMismatch when
// row names an account other than userID, or one whose address is no longer
// the identity's (or, when tokenEmail is set, the emailed link's), and with
// shared.ClosedAccountRefusal when the account may not sign in.
func takePendingLink(tx *gorm.DB, row *authm.OAuthPendingSignup, userID uint, tokenEmail string) (*authm.User, error) {
	if row.UserID == nil || *row.UserID != userID {
		return nil, errPendingLinkAccountMismatch
	}
	var account authm.User
	found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).Limit(1).Find(&account)
	if found.Error != nil {
		return nil, fmt.Errorf("failed to lock account: %w", found.Error)
	}
	if found.RowsAffected == 0 || account.Email == nil ||
		!authm.SameEmailIdentity(*account.Email, row.Email) ||
		(tokenEmail != "" && !authm.SameEmailIdentity(*account.Email, tokenEmail)) {
		return nil, errPendingLinkAccountMismatch
	}
	if !account.CanSignIn() {
		return nil, shared.ClosedAccountRefusal(&account)
	}
	return &account, nil
}

// attachPendingIdentity attaches the identity row holds to userID through
// tx, on the terms every attach to an existing account follows
// (usersvc.AttachOAuthIdentityTx).
func attachPendingIdentity(tx *gorm.DB, userID uint, row *authm.OAuthPendingSignup) error {
	if row.ProviderUserID == "" {
		return fmt.Errorf("oauth provider %q returned no user id", row.Provider)
	}
	return usersvc.AttachOAuthIdentityTx(tx, userID, row.Provider, goth.User{
		UserID:    row.ProviderUserID,
		Email:     row.Email,
		Name:      row.Name,
		AvatarURL: row.AvatarURL,
	})
}

// attachInSavepoint runs attach in a savepoint of tx. A refusal attach
// returns is handed back with a nil error, after the savepoint has undone
// whatever attach wrote, so the caller's transaction can still commit; any
// other error is returned as the error.
func attachInSavepoint(tx *gorm.DB, attach func(sp *gorm.DB) error) (*apperrors.AuthError, error) {
	err := tx.Transaction(attach)
	return asRefusal(err), ignoreRefusal(err)
}

// asRefusal is the *apperrors.AuthError err is, or nil.
func asRefusal(err error) *apperrors.AuthError {
	var refusal *apperrors.AuthError
	if errors.As(err, &refusal) {
		return refusal
	}
	return nil
}

// ignoreRefusal is err unless it is a refusal, which a proof's transaction
// commits past.
func ignoreRefusal(err error) error {
	if asRefusal(err) != nil {
		return nil
	}
	return err
}
