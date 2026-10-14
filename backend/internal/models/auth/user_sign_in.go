package auth

// CanSignIn reports whether the account may be issued a session. A deactivated
// account (IsActive false) and a soft-deleted one (DeletedAt set) may not, and
// either field alone is enough to refuse.
//
// Sign-in paths ask it after the caller has proved the credential and before a
// session is minted, and answer a refusal with the typed account-inactive
// error.
func (u *User) CanSignIn() bool {
	return u != nil && u.IsActive && u.DeletedAt == nil
}
