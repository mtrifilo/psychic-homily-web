package shared

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// ErrSessionRevoked is the cause inside the CodeTokenInvalid refusal for a
// session that predates its account's latest session revocation.
var ErrSessionRevoked = errors.New("session predates the account's latest session revocation")

// LockSessionAccount reads, through tx, the account a signed-in request acts
// for, and holds its row FOR SHARE until tx ends. A session-authenticated
// write that adds a way into the account (the Settings provider link, a
// passkey registration, a feed token) calls it first, in the transaction that
// writes. Writes no session drives (sign-up, the pending-link proofs, the
// admin-only API token) do not: a proof holds the row FOR UPDATE and judges
// the account itself.
//
// sessionsValidAfter is the acting session's revocation stamp: the
// SessionsValidAfter of the user its session validation loaded, which that
// validation honors only when it is the account's current value. nil is a
// stamp, the one a session minted before any revocation carries, not a way to
// skip the comparison.
//
// It refuses with CodeAccountInactive when the account may not sign in
// (authm.User.CanSignIn), and otherwise with CodeTokenInvalid wrapping
// ErrSessionRevoked when the account's sessions_valid_after is no longer
// sessionsValidAfter (IsSessionRefusal reports both). Every session
// revocation writes a new value, so a session revoked after its validation is
// refused here. Under the lock, a deactivation, deletion or revocation that
// committed before the read is seen by it, and one that comes later waits for
// tx, so none can land between this check and the caller's write. A missing
// account is an error, not a refusal.
func LockSessionAccount(tx *gorm.DB, userID uint, sessionsValidAfter *time.Time) error {
	var account authm.User
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&account, userID).Error; err != nil {
		return fmt.Errorf("failed to load user %d: %w", userID, err)
	}
	if !account.CanSignIn() {
		refusal := apperrors.ErrAccountInactive()
		refusal.Internal = fmt.Errorf("user %d may not sign in", account.ID)
		return refusal
	}
	if !sameRevocationStamp(account.SessionsValidAfter, sessionsValidAfter) {
		return apperrors.ErrTokenInvalid(fmt.Errorf("user %d: %w", account.ID, ErrSessionRevoked))
	}
	return nil
}

// IsSessionRefusal reports whether err is one of LockSessionAccount's
// refusals: the session no longer holds the account, because the account may
// not sign in or the session was revoked. A caller that answers a closed
// account differently from a revoked session branches on ErrSessionRevoked
// (errors.Is) for the revocation instead.
func IsSessionRefusal(err error) bool {
	var refusal *apperrors.AuthError
	if !errors.As(err, &refusal) {
		return false
	}
	return refusal.Code == apperrors.CodeAccountInactive || errors.Is(err, ErrSessionRevoked)
}

// sameRevocationStamp reports whether two sessions_valid_after values are the
// same: both NULL, or both the same instant. Both sides are reads of the
// column, so they compare exactly; session validation compares a token claim
// in whole seconds instead (JWTService.userFromSessionClaims).
func sameRevocationStamp(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
