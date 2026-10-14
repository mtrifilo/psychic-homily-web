package auth

// CanSignIn reports whether the account may hold a session. A deactivated
// account (IsActive false) and a soft-deleted one (DeletedAt set) may not, and
// either field alone is enough to refuse.
//
// Sign-in paths ask it after the caller has proved the credential and before a
// session is minted, and answer a refusal with the typed account-inactive
// error. Session validation asks it too, so a session is valid for exactly the
// accounts that may be issued one.
func (u *User) CanSignIn() bool {
	return u != nil && u.IsActive && u.DeletedAt == nil
}
