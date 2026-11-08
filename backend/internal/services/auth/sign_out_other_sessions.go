package auth

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
)

// SignOutOtherSessions revokes every session user's account holds and its
// calendar-feed token (revokeSessionsAndFeedToken) and returns the session
// the caller goes on with: a renewal for the account as the revocation left
// it, carrying authAt through unchanged. user is the principal the caller's
// session validation loaded; its SessionsValidAfter is that session's
// revocation stamp.
//
// The account is read FOR UPDATE and judged with shared.CheckSessionAccount
// before anything is written, so a session revoked, or an account closed,
// after validation is refused with that check's refusals and revokes
// nothing: a revoked session cannot use this to mint itself a current one.
// The renewal is minted inside the transaction that writes the stamp, so the
// stamp commits only once the session that replaces the caller's exists; a
// mint failure rolls the revocation back.
//
// API tokens are not sessions and are left in place.
func (s *AuthService) SignOutOtherSessions(user *authm.User, authAt time.Time) (string, error) {
	return signOutOtherSessions(s.db, user, authAt, time.Now(), s.jwtService.RenewSessionToken)
}

// sessionMint mints a session for account carrying authAt as its
// authentication time.
type sessionMint func(account *authm.User, authAt time.Time) (string, error)

// signOutOtherSessions is SignOutOtherSessions with the database, the clock
// and the mint supplied.
func signOutOtherSessions(db *gorm.DB, user *authm.User, authAt, now time.Time, mint sessionMint) (string, error) {
	var token string
	err := db.Transaction(func(tx *gorm.DB) error {
		// FOR UPDATE rather than shared.LockSessionAccount's FOR SHARE: the
		// revocation writes this row, and two calls that each held it FOR
		// SHARE and then both wrote it would deadlock. Held FOR UPDATE, a
		// second call waits for the first to end and then reads the stamp the
		// first wrote, which its own session no longer names.
		var account authm.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, user.ID).Error; err != nil {
			return fmt.Errorf("failed to load user %d: %w", user.ID, err)
		}
		if err := shared.CheckSessionAccount(&account, user.SessionsValidAfter); err != nil {
			return err
		}
		if err := revokeSessionsAndFeedToken(tx, &account, now); err != nil {
			return err
		}
		// The revocation set account.SessionsValidAfter to the stamp it
		// wrote, so the renewal names the stamp this transaction commits.
		minted, err := mint(&account, authAt)
		if err != nil {
			return fmt.Errorf("failed to mint the replacement session: %w", err)
		}
		token = minted
		return nil
	})
	return token, err
}
