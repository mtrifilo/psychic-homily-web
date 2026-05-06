// Package shared exposes cross-cutting service helpers that don't belong to a
// single domain (collection, comment, request, etc.). It is the canonical home
// for resolution chains, name formatters, and other zero-dependency utilities
// that need to render identically regardless of which surface looks them up.
//
// User attribution (PSY-612 / PSY-598):
//
//	Resolve* helpers return the same display name for a given user no matter
//	which handler asks. Prior to consolidation, five backend sites had their
//	own slightly-different chains: some stopped at "Unknown", some leaked the
//	raw email address, and some omitted the email-prefix step entirely. See
//	docs/research/user-attribution-audit.md for the full classification.
package shared

import (
	"strings"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
)

// ResolveUserName returns the display name for a user. Never empty.
//
// Resolution chain, in order:
//  1. user.Username                            (preferred, also the URL slug)
//  2. user.FirstName [+ " " + user.LastName]   (human full name)
//  3. local-part of user.Email (before "@")    (last-resort handle)
//  4. "Anonymous"                              (terminal fallback)
//
// nil-safe: returns "Anonymous" when the user is nil or has ID 0.
//
// Use this whenever a backend response needs a label for a user. Pair with
// ResolveUserUsername when you also want a profile-link slug — the username
// form returns *string so consumers can omit the link when no username is set.
func ResolveUserName(user *authm.User) string {
	if user == nil || user.ID == 0 {
		return "Anonymous"
	}
	if user.Username != nil && *user.Username != "" {
		return *user.Username
	}
	if user.FirstName != nil && *user.FirstName != "" {
		name := *user.FirstName
		if user.LastName != nil && *user.LastName != "" {
			name += " " + *user.LastName
		}
		return name
	}
	if user.Email != nil && *user.Email != "" {
		if idx := strings.Index(*user.Email, "@"); idx > 0 {
			return (*user.Email)[:idx]
		}
	}
	return "Anonymous"
}

// ResolveUserUsername returns the user's username for /users/:username links,
// or nil when the user has no username set. Distinct from ResolveUserName,
// which falls back to first/last/email and so cannot be safely used in a URL
// slug.
//
// nil-safe: returns nil when the user is nil or has ID 0.
//
// Callers should treat nil as "render unlinked" — emit plain text with no
// anchor — and a non-nil pointer as "render <Link href={'/users/' + *u}>".
func ResolveUserUsername(user *authm.User) *string {
	if user == nil || user.ID == 0 {
		return nil
	}
	if user.Username == nil || *user.Username == "" {
		return nil
	}
	username := *user.Username
	return &username
}

// BatchResolveUserNames resolves display names for multiple user IDs in a
// single query. Returns a map keyed by user ID; missing users are absent
// from the map (callers can default to "Anonymous" via ResolveUserName(nil)
// or by checking the map directly).
//
// Returns an empty map (not nil) when userIDs is empty so callers can index
// without nil-check guards.
func BatchResolveUserNames(db *gorm.DB, userIDs []uint) (map[uint]string, error) {
	result := make(map[uint]string)
	if len(userIDs) == 0 {
		return result, nil
	}

	var users []authm.User
	if err := db.Select("id, username, first_name, last_name, email").
		Where("id IN ?", userIDs).
		Find(&users).Error; err != nil {
		return nil, err
	}

	for i := range users {
		result[users[i].ID] = ResolveUserName(&users[i])
	}
	return result, nil
}

// BatchResolveUserUsernames resolves usernames for multiple user IDs in a
// single query. Map values are nil-pointer when the user has no username —
// callers should treat that as "render unlinked".
//
// Returns an empty map (not nil) when userIDs is empty so callers can index
// without nil-check guards.
func BatchResolveUserUsernames(db *gorm.DB, userIDs []uint) (map[uint]*string, error) {
	result := make(map[uint]*string)
	if len(userIDs) == 0 {
		return result, nil
	}

	var users []authm.User
	if err := db.Select("id, username").
		Where("id IN ?", userIDs).
		Find(&users).Error; err != nil {
		return nil, err
	}

	for i := range users {
		result[users[i].ID] = ResolveUserUsername(&users[i])
	}
	return result, nil
}
