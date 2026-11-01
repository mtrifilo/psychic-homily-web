package shared

import (
	"errors"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
)

// The lookups of a users row by a username or email address a caller supplies
// live here, so each holds the encoding guard (authm.PostgresRejectsText) and
// its account-state rule in one place. A sign-in that matches an identity
// provider's email claim refuses an unstorable claim first
// (ValidateProviderEmailClaim) and queries for itself. Each reads through db, which may carry the
// caller's Preload or Select clauses, and answers (nil, nil) when no row
// matches, including for a value Postgres rejects as text, for which no query
// runs. A failed query returns its error unwrapped.

// usernameWhere is the WHERE fragment that matches a users row by username.
const usernameWhere = "username = ?"

// LiveUserByUsername finds the live account (authm.LiveAccountSQL) holding
// username. A deactivated or soft-deleted holder is not found, so a caller
// answers a closed account exactly as an unknown username.
func LiveUserByUsername(db *gorm.DB, username string) (*authm.User, error) {
	return findUser(db.Where(authm.LiveAccountSQL("users")), usernameWhere, username)
}

// UserByUsernameIncludingClosed finds the account holding username whatever
// its state, deactivated and soft-deleted included.
func UserByUsernameIncludingClosed(db *gorm.DB, username string) (*authm.User, error) {
	return findUser(db, usernameWhere, username)
}

// UserByEmail finds the account holding email whatever its state,
// deactivated and soft-deleted included. The match is case-insensitive
// (authm.EmailIdentityWhere).
func UserByEmail(db *gorm.DB, email string) (*authm.User, error) {
	return findUser(db, authm.EmailIdentityWhere, email)
}

// findUser loads the first users row matching where, which binds value as
// its one argument.
func findUser(db *gorm.DB, where, value string) (*authm.User, error) {
	if authm.PostgresRejectsText(value) {
		return nil, nil
	}
	var user authm.User
	err := db.Where(where, value).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
