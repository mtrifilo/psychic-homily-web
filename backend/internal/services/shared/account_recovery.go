package shared

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// AccountRecoveryGracePeriod is how long a soft-deleted account stays
// recoverable before it is eligible for permanent deletion.
const AccountRecoveryGracePeriod = 30 * 24 * time.Hour

// IsAccountRecoverable reports whether user is soft-deleted (inactive with
// deleted_at set) and still inside AccountRecoveryGracePeriod.
func IsAccountRecoverable(user *authm.User) bool {
	if user == nil || user.IsActive || user.DeletedAt == nil {
		return false
	}
	return time.Now().Before(user.DeletedAt.Add(AccountRecoveryGracePeriod))
}

// ClosedAccountRefusal is the refusal for a sign-in into user, an account that
// may not sign in (authm.User.CanSignIn), by a caller who has proved its
// credential or whose provider identity resolved by its subject to the
// account: CodeAccountRecoverable when account recovery can restore it, so the
// caller can be sent there, and CodeAccountInactive otherwise.
func ClosedAccountRefusal(user *authm.User) *apperrors.AuthError {
	if recoverableByEmail(user) {
		return apperrors.ErrAccountRecoverable()
	}
	return apperrors.ErrAccountInactive()
}

// AddressMatchIsRecoverable reports whether an OAuth sign-in whose identity is
// attached to no account, and whose provider address matched account, is
// refused with CodeAccountRecoverable instead of the provider's ordinary
// address refusal.
//
// A deleted account inside its recovery window cannot be signed in to, so the
// ordinary remediation (sign in, then attach the identity) is a dead end for
// it. The refusal sends the caller to account recovery instead when both
// sides vouch for the mailbox: the provider for the caller
// (providerVouchesAddress), and this system's own verification for the
// account. Recovery mails that address, so the refusal reveals the deletion to
// no one the recovery email would not reach, and it attaches nothing.
//
// Without the provider's vouch the caller may not hold the mailbox. Without
// the account's, the account may be a squat on the caller's address, and
// recovery restores it with the squatter's password still on it. Either way
// the caller keeps the refusal that says nothing of the account's state.
func AddressMatchIsRecoverable(account *authm.User, providerVouchesAddress bool) bool {
	return providerVouchesAddress && recoverableByEmail(account) && account.EmailVerified
}

// recoverableByEmail reports whether account recovery can restore user: it is
// recoverable (IsAccountRecoverable) and has the address recovery finds it by
// and mails. An account with no address cannot be restored there, so sending
// its holder to recovery would be a dead end.
func recoverableByEmail(user *authm.User) bool {
	return IsAccountRecoverable(user) && user.Email != nil && *user.Email != ""
}
