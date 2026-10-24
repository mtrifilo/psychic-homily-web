package auth

// HasPassword reports whether the account has a password set. An empty hash is
// no password: nothing can verify against it.
func (u *User) HasPassword() bool {
	return u != nil && u.PasswordHash != nil && *u.PasswordHash != ""
}
