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

// ErrSessionAccountClosed is the cause inside the CodeAccountInactive refusal
// LockSessionAccount gives a session whose account may no longer sign in.
var ErrSessionAccountClosed = errors.New("the session's account may no longer sign in")

// LockSessionAccount reads, through tx, the account a signed-in request acts
// for, and holds its row FOR SHARE until tx ends. A session-authenticated
// write that adds a way into the account (the Settings provider link, a
// passkey registration, a feed token) calls it first, in the transaction that
// writes. Sign-up and the pending-link proofs, which no session drives, do
// not (a proof holds the row FOR UPDATE and judges the account itself); nor
// does the admin-only API-token mint, which is outside this check's scope.
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
	return CheckSessionAccount(&account, sessionsValidAfter)
}

// CheckSessionAccount judges account, a read of the account a signed-in
// request acts for, against sessionsValidAfter, the acting session's
// revocation stamp, and refuses as LockSessionAccount does: CodeAccountInactive
// wrapping ErrSessionAccountClosed when the account may not sign in, and
// otherwise CodeTokenInvalid wrapping ErrSessionRevoked when its
// sessions_valid_after is no longer sessionsValidAfter. It holds no lock, so
// its answer covers only what committed before account was read.
func CheckSessionAccount(account *authm.User, sessionsValidAfter *time.Time) error {
	if !account.CanSignIn() {
		refusal := apperrors.ErrAccountInactive()
		refusal.Internal = fmt.Errorf("user %d: %w", account.ID, ErrSessionAccountClosed)
		return refusal
	}
	if !sameRevocationStamp(account.SessionsValidAfter, sessionsValidAfter) {
		return apperrors.ErrTokenInvalid(fmt.Errorf("user %d: %w", account.ID, ErrSessionRevoked))
	}
	return nil
}

// IsSessionRefusal reports whether err is one of CheckSessionAccount's
// refusals (which LockSessionAccount gives too), by their causes: the account may not sign in
// (ErrSessionAccountClosed) or the session was revoked (ErrSessionRevoked).
// An ACCOUNT_INACTIVE from anywhere else is not one. A caller that answers a
// closed account differently from a revoked session branches on
// ErrSessionRevoked (errors.Is) for the revocation instead.
func IsSessionRefusal(err error) bool {
	return errors.Is(err, ErrSessionRevoked) || errors.Is(err, ErrSessionAccountClosed)
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
