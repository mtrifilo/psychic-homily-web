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
	engagementsvc "psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
	usersvc "psychic-homily-backend/internal/services/user"
)

// A pending link holds a provider identity whose verified address belongs to
// an account it is not attached to. It joins that account only after one of
// two proofs that the person holds the account: its password
// (LinkOAuthWithPassword) or its mailbox (an emailed link,
// CompleteOAuthLinkByEmail). Each proof consumes the pending link in the
// transaction that acts on it, so whichever lands first is the only one that
// acts, and the other is refused. A password proof refused to attach the
// identity rolls its consumption back, so the pending link is still there for
// the mailbox proof, which on an address never verified succeeds where the
// password proof is refused (authm.User.MailboxProofReplacesIdentity).

var errPendingLinkAccountMismatch = errors.New("pending link names another account or address")

// StartOAuthLink stores identity as a pending link to userID, with returnTo,
// for contracts.OAuthPendingLinkTTL, and returns the token that names it.
func (s *AuthService) StartOAuthLink(identity contracts.OAuthIdentity, userID uint, returnTo string) (string, error) {
	row := pendingRowFromIdentity(identity, returnTo)
	row.UserID = &userID
	return s.storePending(row, pendingLinkPurpose, contracts.OAuthPendingLinkTTL)
}

// PeekOAuthLink returns the pending link token names, leaving it in place
// (contracts.AuthServiceInterface).
func (s *AuthService) PeekOAuthLink(token string) (*contracts.OAuthPendingLink, error) {
	link, err := s.peekPendingLink(token)
	if err != nil {
		return nil, err
	}
	link.ReplacesIdentity, err = s.mailboxProofReplacesIdentity(link)
	if err != nil {
		return nil, err
	}
	return link, nil
}

// mailboxProofReplacesIdentity reports whether the mailbox proof of link,
// were it to land now, would remove an identity the account holds from
// link's provider (CompleteOAuthLinkByEmail): the account's address was
// never verified and it holds an identity from the provider other than
// link's. It reads without locking, so it describes the account as it is
// now, and the proof decides again under its own lock.
func (s *AuthService) mailboxProofReplacesIdentity(link *contracts.OAuthPendingLink) (bool, error) {
	var account authm.User
	found := s.db.Where("id = ?", link.UserID).Limit(1).Find(&account)
	if found.Error != nil {
		return false, fmt.Errorf("failed to read pending link account: %w", found.Error)
	}
	if found.RowsAffected == 0 || !account.MailboxProofReplacesIdentity() {
		return false, nil
	}
	var held int64
	if err := s.db.Model(&authm.OAuthAccount{}).
		Where("user_id = ? AND provider = ? AND provider_user_id <> ?",
			link.UserID, link.Identity.Provider, link.Identity.Subject).
		Count(&held).Error; err != nil {
		return false, fmt.Errorf("failed to read provider identities: %w", err)
	}
	return held > 0, nil
}

// peekPendingLink reads the pending link token names, leaving it in place.
func (s *AuthService) peekPendingLink(token string) (*contracts.OAuthPendingLink, error) {
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

// oauthLinkEmailedLifetime is how long a pending link lives once its emailed
// link is asked for: the longest the outbox may hold the email before it is
// dropped unsent (authEmailMaxAge) plus the emailed link's own lifetime, so a
// link sent at any moment the outbox may send it works for its whole life.
const oauthLinkEmailedLifetime = authEmailMaxAge + contracts.MagicLinkTokenTTL

// HoldOAuthLinkForEmail returns the pending link token names and keeps it for
// oauthLinkEmailedLifetime from now, or longer if it already was
// (contracts.AuthServiceInterface).
func (s *AuthService) HoldOAuthLinkForEmail(token string) (*contracts.OAuthPendingLink, error) {
	link, err := s.peekPendingLink(token)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	res := s.db.Model(&authm.OAuthPendingSignup{}).
		Where("token_hash = ? AND purpose = ? AND expires_at > ?", link.Ref, authm.OAuthPendingPurposeLink, now).
		Update("expires_at", gorm.Expr("GREATEST(expires_at, ?)", now.Add(oauthLinkEmailedLifetime)))
	if res.Error != nil {
		return nil, fmt.Errorf("failed to hold pending link: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Used or expired between the read and the write.
		return nil, apperrors.ErrOAuthLinkPendingExpired(errPendingExpired)
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
	// A refusal returned from the transaction rolls it back, consumption
	// included, so a refused password proof leaves the pending link as it
	// found it.
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
		if err != nil {
			return err
		}
		if err := attachPendingIdentity(tx, account.ID, row); err != nil {
			return err
		}
		linked = true
		return nil
	})
	if err != nil {
		return false, err
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
	var replaced *authm.OAuthAccount
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
			if account.MailboxProofReplacesIdentity() {
				evicted, err := evictUnprovenIdentity(sp, account.ID, row.Provider, row.ProviderUserID)
				if err != nil {
					return err
				}
				replaced = evicted
			}
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
			if err := RevokeAllSessions(sp, account, time.Now()); err != nil {
				return err
			}
			return evictOtherCredentials(sp, account.ID, row.Provider)
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
	if replaced != nil {
		// The removed row is gone from oauth_accounts, so this line is the
		// record of which identity the proof took off the account.
		logger.Default().Warn("oauth_pending_link_replaced_identity",
			"user_id", user.ID,
			"provider", replaced.Provider,
			"replaced_oauth_account_id", replaced.ID,
			"replaced_provider_email_hash", logger.HashEmail(shared.DerefOrEmpty(replaced.ProviderEmail)))
	}
	logger.Default().Info("oauth_pending_link_completed", "proof", "mailbox", "user_id", user.ID,
		"replaced_identity", replaced != nil)
	return &contracts.OAuthLinkCompletion{User: user, Token: token, ReturnTo: returnTo}, nil
}

// evictOtherCredentials removes, through tx, the credentials besides the
// password and sessions that whoever set the password could have attached:
// the calendar-feed token, and every provider identity other than
// keepProvider's, through the paths Settings revokes and disconnects them by.
// The mailbox proof leaves the account with the mailbox owner's identity
// from keepProvider, its passkeys (which are not evicted) and no password.
func evictOtherCredentials(tx *gorm.DB, userID uint, keepProvider string) error {
	if _, err := engagementsvc.DeleteCalendarTokensTx(tx, userID); err != nil {
		return err
	}
	// FOR UPDATE: a concurrent disconnect of one of these rows waits for the
	// proof, so each unlink below finds the row it was listed for. One row
	// per provider per account (oauth_accounts_user_provider_uniq).
	var providers []string
	if err := tx.Model(&authm.OAuthAccount{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND provider <> ?", userID, keepProvider).
		Pluck("provider", &providers).Error; err != nil {
		return fmt.Errorf("failed to list provider identities: %w", err)
	}
	for _, provider := range providers {
		if err := usersvc.UnlinkOAuthAccountTx(tx, userID, provider); err != nil {
			return err
		}
	}
	return nil
}

// evictUnprovenIdentity removes, through tx, userID's identity from provider
// when it is not provenSubject, the identity the mailbox proof attaches, and
// returns the row it removed, or nil when there was none. The caller has
// userID's row locked and has read it as one the mailbox proof replaces an
// identity on (authm.User.MailboxProofReplacesIdentity). An account holds at
// most one row per
// provider (oauth_accounts_user_provider_uniq), so the row locked FOR UPDATE
// here is the one UnlinkOAuthAccountTx, the path Settings disconnects by,
// removes.
func evictUnprovenIdentity(tx *gorm.DB, userID uint, provider, provenSubject string) (*authm.OAuthAccount, error) {
	var held authm.OAuthAccount
	found := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND provider = ? AND provider_user_id <> ?", userID, provider, provenSubject).
		Limit(1).Find(&held)
	if found.Error != nil {
		return nil, fmt.Errorf("failed to read the provider identity: %w", found.Error)
	}
	if found.RowsAffected == 0 {
		return nil, nil
	}
	if err := usersvc.UnlinkOAuthAccountTx(tx, userID, provider); err != nil {
		return nil, err
	}
	return &held, nil
}

// takePendingLink locks and returns the account a consumed pending link
// names, FOR UPDATE, so a deactivation, deletion or address change waits for
// the proof's transaction. It refuses with errPendingLinkAccountMismatch when
// row names an account other than userID, or one whose address is no longer
// the identity's (or, when tokenEmail is set, the emailed link's), and with
// shared.ClosedAccountRefusal when the account may not sign in.
//
// It is not shared.LockSessionAccount: no session drives a proof, so there is
// no revocation stamp to compare. It holds the row FOR UPDATE, which conflicts
// with the FOR SHARE a session's credential write takes there, so such a
// write either commits before the proof reads the account, or waits for the
// proof to commit and then judges the account as the proof left it.
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
