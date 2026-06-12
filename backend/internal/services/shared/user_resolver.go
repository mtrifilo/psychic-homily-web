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

// AnonymousUserName is the terminal label ResolveUserName returns when a user
// has no username, name, or email to resolve (and for nil / ID-0 users).
//
// Exported so surfaces that layer their own channel-specific empty-state copy
// (e.g. a Discord embed's "Not provided") can detect "the chain bottomed out"
// without re-deriving the chain or hardcoding the sentinel string.
const AnonymousUserName = "Anonymous"

// ResolveUserName returns the display name for a user. Never empty.
//
// Resolution chain, in order:
//  1. user.DisplayName                         (explicitly user-chosen, PSY-1063)
//  2. user.Username                            (also the URL slug)
//  3. user.FirstName [+ " " + user.LastName]   (human full name)
//  4. local-part of user.Email (before "@")    (last-resort handle)
//  5. AnonymousUserName                        (terminal fallback)
//
// nil-safe: returns AnonymousUserName when the user is nil or has ID 0.
//
// Use this whenever a backend response needs a label for a user. Pair with
// ResolveUserUsername when you also want a profile-link slug — the username
// form returns *string so consumers can omit the link when no username is set.
//
// CALLERS USING A COLUMN-RESTRICTED Select MUST LOAD EVERY CHAIN COLUMN:
// id, username, display_name, first_name, last_name, email. Omitting one
// silently disables that branch (the field scans as nil) — this bit two
// call sites when display_name was added (PSY-1063).
func ResolveUserName(user *authm.User) string {
	if user == nil || user.ID == 0 {
		return AnonymousUserName
	}
	if user.DisplayName != nil && *user.DisplayName != "" {
		return *user.DisplayName
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
	return AnonymousUserName
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
	if err := db.Select("id, username, display_name, first_name, last_name, email").
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

// BatchResolveShowArtistNames returns the billing-ordered artist names for
// multiple shows in a single query, grouped by show ID. Avoids the per-show
// artist query that an N+1 caller would otherwise issue on a hot path.
//
// Returns an empty map (not nil) when showIDs is empty so callers can index
// without nil-check guards. Shows with no artists are absent from the map.
func BatchResolveShowArtistNames(db *gorm.DB, showIDs []uint) (map[uint][]string, error) {
	byShowID := make(map[uint][]string, len(showIDs))
	if len(showIDs) == 0 {
		return byShowID, nil
	}

	var rows []struct {
		ShowID uint
		Name   string
	}
	if err := db.Raw(`SELECT show_artists.show_id, artists.name FROM show_artists
		JOIN artists ON show_artists.artist_id = artists.id
		WHERE show_artists.show_id IN (?)
		ORDER BY show_artists.show_id, show_artists.position`, showIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		byShowID[row.ShowID] = append(byShowID[row.ShowID], row.Name)
	}
	return byShowID, nil
}
