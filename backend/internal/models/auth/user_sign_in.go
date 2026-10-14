package auth

// CanSignIn reports whether the account may hold a session. A deactivated
// account (IsActive false) and a soft-deleted one (DeletedAt set) may not, and
// either field alone is enough to refuse.
//
// Sign-in asks it after the caller has proved the credential and before a
// session is minted, and session validation that loads the account asks it
// too, so such a session is valid for exactly the accounts that may be issued
// one.
func (u *User) CanSignIn() bool {
	return u != nil && u.IsActive && u.DeletedAt == nil
}
