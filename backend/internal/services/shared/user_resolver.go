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
// NOT FOR PUBLIC RESPONSES. Tier 4 publishes a fragment of the account's email
// address, which no logged-out surface may render (owner decision 2026-08-29).
// Use this on admin-only views, on internal channels, and in messages addressed
// to the user themselves; everywhere else reach for ResolvePublicUserName, or
// ResolvePublicContributorCredit when the surface credits a CONTRIBUTION and can
// omit the byline. See public_attribution.go.
//
// Pair with ResolveUserUsername when you also want a profile-link slug — the
// username form returns *string so consumers can omit the link when no username
// is set.
//
// CALLERS USING A COLUMN-RESTRICTED Select MUST LOAD EVERY CHAIN COLUMN:
// id, username, display_name, first_name, last_name, email. Omitting one
// silently disables that branch (the field scans as nil) — this bit two
// call sites when display_name was added (PSY-1063).
func ResolveUserName(user *authm.User) string {
	if name := resolvePublicNameTiers(user); name != "" {
		return name
	}
	if user != nil && user.ID != 0 && user.Email != nil && *user.Email != "" {
		if idx := strings.Index(*user.Email, "@"); idx > 0 {
			return (*user.Email)[:idx]
		}
	}
	return AnonymousUserName
}

// resolvePublicNameTiers walks the tiers of the chain that a PUBLIC surface may
// render, and returns "" when the user has none of them.
//
// This is the primitive, not a copy: ResolveUserName is this plus the email
// tier plus the terminal, ResolvePublicUserName is this plus the terminal, and
// HasPublicName is this being non-empty. Spelling the shared prefix ONCE is what
// makes "the public chain is the canonical chain minus its last tier" true by
// construction rather than by comment. Two functions each listing the tiers
// would let a later edit reorder or insert one in a way that silently promotes
// the email tier above a public one, which is precisely the leak PSY-1940 closed.
//
// So: a new tier goes HERE if a logged-out visitor may see it, and in
// ResolveUserName's own body if not. Nowhere else.
//
// A tier holding only WHITESPACE counts as unset, and what a tier does return
// is trimmed. Registration stores first_name raw (services/user/user.go; only
// the profile PATCH trims), so " " and "\t" and "\n" are all storable today.
// Untrimmed they are non-empty strings that satisfy every `!= ""` gate above
// and every `name &&` guard on the frontend, and the byline renders as a
// dangling "added Jul 12 by " with an empty span. Trimming here rather than at
// each gate is the point of having one primitive: the callers ask "is there a
// name" and get a truthful answer, once.
//
// This is unicode.IsSpace only. A name that is invisible without being space —
// a lone zero-width space, a bidi override — still resolves, and deliberately
// so: the format-character category that would catch it also holds joiners
// that legitimate names in several scripts require. Rejecting those belongs to
// input validation at the write side, not to a resolver that must render
// whatever was stored.
//
// nil-safe: returns "" for a nil or ID-0 user, which every caller above turns
// into its own terminal.
func resolvePublicNameTiers(user *authm.User) string {
	if user == nil || user.ID == 0 {
		return ""
	}
	if name := trimmedDeref(user.DisplayName); name != "" {
		return name
	}
	if name := trimmedDeref(user.Username); name != "" {
		return name
	}
	if first := trimmedDeref(user.FirstName); first != "" {
		if last := trimmedDeref(user.LastName); last != "" {
			return first + " " + last
		}
		return first
	}
	return ""
}

// trimmedDeref unwraps a nullable text column to its trimmed value, or "" for
// nil. "" therefore means "nothing a surface could render", covering NULL, the
// empty string, and whitespace alike.
func trimmedDeref(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
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

// NOTE: there is deliberately no batch form of ResolveUserName. PSY-1940 moved
// every batch caller to BatchResolvePublicUserNames, and a batch resolver that
// publishes email local-parts is the most convenient wrong answer for the next
// list endpoint that needs names — an unused one sitting here would be a trap,
// not a convenience. If an ADMIN list surface ever needs the canonical chain in
// bulk, add it then, with its own "not for public responses" warning and a
// named caller.

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

// ShowArtistRef is one band on a bill, reduced to what a linked row needs: the
// name that labels it and the slug that builds its href.
//
// Slug is "" — never nil — for an unslugged artist: `artists.slug` is NULLABLE
// (hence the COALESCE below) and GenerateSlug can return an empty string, so
// such a band is a real row and is carried, not filtered. What a CONSUMER must
// do with it is the wire contract's business — see
// contracts.SceneShowSummary.Artists.
type ShowArtistRef struct {
	Name string
	Slug string
}

// BatchResolveShowArtists returns the billing-ordered bill for multiple shows in
// a single query, grouped by show ID. Avoids the per-show artist query that an
// N+1 caller would otherwise issue on a hot path.
//
// Returns an empty map (not nil) when showIDs is empty so callers can index
// without nil-check guards. Shows with no artists are absent from the map.
func BatchResolveShowArtists(db *gorm.DB, showIDs []uint) (map[uint][]ShowArtistRef, error) {
	byShowID := make(map[uint][]ShowArtistRef, len(showIDs))
	if len(showIDs) == 0 {
		return byShowID, nil
	}

	var rows []struct {
		ShowID uint
		Name   string
		Slug   string
	}
	// artists.id tie-break: same-position bill entries otherwise come back in
	// planner order, which can flip between runs (PSY-1325).
	//
	// COALESCE on slug because the column is NULLABLE — see ShowArtistRef.Slug.
	if err := db.Raw(`SELECT show_artists.show_id, artists.name, COALESCE(artists.slug, '') AS slug FROM show_artists
		JOIN artists ON show_artists.artist_id = artists.id
		WHERE show_artists.show_id IN (?)
		ORDER BY show_artists.show_id, show_artists.position, artists.id`, showIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		byShowID[row.ShowID] = append(byShowID[row.ShowID], ShowArtistRef{Name: row.Name, Slug: row.Slug})
	}
	return byShowID, nil
}

// BatchResolveShowArtistNames is the names-only projection of
// BatchResolveShowArtists, for callers that render a bill as plain text and have
// no link to build.
//
// It delegates rather than issuing its own query so that these two helpers
// cannot drift apart on billing order — especially the artists.id tie-break
// above. The cost is one extra text column on the read, far cheaper than a
// second copy of an ordering rule.
//
// This is NOT the only bill-ordering query in the backend, and the others do
// not agree: catalog/venue_rail.go tie-breaks on artists.name, and
// catalog/charts_service.go has no tie-break at all — the planner-order
// nondeterminism PSY-1325 fixed here, still live there. Pointing both at this
// helper is a worthwhile follow-up; until then, do not read "one definition"
// into this package.
func BatchResolveShowArtistNames(db *gorm.DB, showIDs []uint) (map[uint][]string, error) {
	artistsByShow, err := BatchResolveShowArtists(db, showIDs)
	if err != nil {
		return nil, err
	}

	byShowID := make(map[uint][]string, len(artistsByShow))
	for showID, artists := range artistsByShow {
		names := make([]string, len(artists))
		for i, a := range artists {
			names[i] = a.Name
		}
		byShowID[showID] = names
	}
	return byShowID, nil
}
