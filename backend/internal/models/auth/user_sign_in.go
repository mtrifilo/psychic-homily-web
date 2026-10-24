package auth

import "time"

// SessionIssuedBeforeRevocation reports whether a session issued at issuedAt
// predates the account's SessionsValidAfter, and so may no longer be honored.
// Nil SessionsValidAfter revokes nothing.
func (u *User) SessionIssuedBeforeRevocation(issuedAt time.Time) bool {
	return u != nil && u.SessionsValidAfter != nil && issuedAt.Before(*u.SessionsValidAfter)
}

// CanSignIn reports whether the account may hold a session. A deactivated
// account (IsActive false) and a soft-deleted one (DeletedAt set) may not, and
// either field alone is enough to refuse.
//
// Sign-in asks it after the caller has proved the credential and before a
// session is minted. Session, API-token and calendar-token validation ask it
// after loading the account, so none of those credentials is honored for an
// account that may not be issued a session.
func (u *User) CanSignIn() bool {
	return u != nil && u.IsActive && u.DeletedAt == nil
}

// LiveAccountSQL is the WHERE fragment that keeps, from the users table under
// alias, exactly the accounts CanSignIn accepts.
//
// It reads is_active alone because users_live_account_check refuses is_active
// true with deleted_at set, so every soft-deleted row is already inactive. It
// binds no arguments, so it fits a raw SQL string and a GORM Where alike.
func LiveAccountSQL(alias string) string {
	return alias + ".is_active = TRUE"
}

// liveAccountExistsAlias is the alias LiveAccountExistsSQL binds the users
// table to, distinct from any alias a calling query uses so the correlation on
// userIDExpr cannot be shadowed by the subquery.
const liveAccountExistsAlias = "live_account"

// LiveAccountExistsSQL is the correlated EXISTS condition, true when the users
// row userIDExpr names is an account CanSignIn accepts. It is LiveAccountSQL for
// a query that holds a user id in another table's column rather than joining
// users itself, and like it binds no arguments.
//
// userIDExpr is SQL the caller controls and must be a literal in the calling
// code.
func LiveAccountExistsSQL(userIDExpr string) string {
	return "EXISTS (SELECT 1 FROM users " + liveAccountExistsAlias +
		" WHERE " + liveAccountExistsAlias + ".id = " + userIDExpr +
		" AND " + LiveAccountSQL(liveAccountExistsAlias) + ")"
}
