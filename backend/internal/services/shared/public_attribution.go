package shared

import (
	"encoding/json"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// This file holds the PUBLIC narrowing of the canonical resolution chain in
// user_resolver.go. Two rules, decided by the owner on 2026-08-29 (PSY-1866,
// generalised by PSY-1940) and binding on every surface a logged-out visitor
// can read:
//
//  1. No public surface renders an email-derived name. ResolveUserName's
//     last-resort tier returns the local part of the account's email address,
//     which is a fragment of a private identifier the person never chose to
//     publish. ResolvePublicUserName stops one tier short of it.
//  2. Contribution attribution is FAIL-CLOSED on privacy_settings.contributions.
//     A surface that names somebody FOR A CONTRIBUTION honours "hidden" by
//     omitting the credit — see ResolvePublicContributorCredit.
//
// The two rules are separate because their terminals differ, and the
// difference is deliberate. A comment or a curated collection is authored
// content whose author slot must say SOMETHING, so rule 1 alone applies there
// and bottoms out at AnonymousUserName. A contribution byline is an optional
// garnish on somebody else's record, so it can and does disappear entirely.
// Do not "unify" them by giving contributor credits an Anonymous terminal:
// "added Jul 12 by Anonymous" claims a person where the honest reading is that
// we may not say.

// HasPublicName reports whether a user has a name tier that a public surface
// may publish: display_name, username, or first_name — the three tiers a
// person chose as a public identity.
//
// False means ResolveUserName would fall through to the email local-part or to
// AnonymousUserName. Checked on the SOURCE columns rather than by inspecting a
// resolved string, because an email-derived name is a bare local-part with
// nothing in it to pattern-match on.
//
// nil-safe: false for a nil or ID-0 user.
func HasPublicName(user *authm.User) bool {
	if user == nil || user.ID == 0 {
		return false
	}
	return (user.DisplayName != nil && *user.DisplayName != "") ||
		(user.Username != nil && *user.Username != "") ||
		(user.FirstName != nil && *user.FirstName != "")
}

// ResolvePublicUserName is ResolveUserName with the email tier removed: a user
// with no public name tier resolves to AnonymousUserName rather than to the
// local part of their email address.
//
// Use this instead of ResolveUserName in any response an unauthenticated
// caller can read. ResolveUserName stays correct for admin-only views and for
// messages addressed to the user themselves (an approval email's greeting),
// where the email tier names somebody who already knows their own address.
//
// Never empty, so it is a drop-in for the surfaces whose author slot must
// render something. Surfaces that can OMIT the name instead should use
// ResolvePublicContributorCredit, which also applies the privacy gate.
func ResolvePublicUserName(user *authm.User) string {
	if !HasPublicName(user) {
		return AnonymousUserName
	}
	// Guaranteed by HasPublicName to resolve on one of the first three tiers.
	return ResolveUserName(user)
}

// BatchResolvePublicUserNames is BatchResolveUserNames under the public rule:
// one query for many ids, resolving each through ResolvePublicUserName.
//
// The Select deliberately omits `email`, departing from the
// load-every-chain-column rule stated on ResolveUserName. That rule exists so a
// missing column cannot silently disable a tier; here disabling the email tier
// is the POINT, and not loading the column means an email cannot reach a
// response even if the tier check were later removed. Structural, not
// incidental — keep it that way, and keep every OTHER chain column listed.
//
// Returns an empty map (not nil) when userIDs is empty so callers can index
// without nil-check guards. Missing users are absent from the map.
func BatchResolvePublicUserNames(db *gorm.DB, userIDs []uint) (map[uint]string, error) {
	result := make(map[uint]string)
	if len(userIDs) == 0 {
		return result, nil
	}

	var users []authm.User
	if err := db.Select("id, username, display_name, first_name, last_name").
		Where("id IN ?", userIDs).
		Find(&users).Error; err != nil {
		return nil, err
	}

	for i := range users {
		result[users[i].ID] = ResolvePublicUserName(&users[i])
	}
	return result, nil
}

// PublicContributorCredit is the credit a public contribution byline may
// publish for a user: a display name, plus the profile slug to link it with
// when there is a reachable profile.
//
// The ZERO VALUE means "this contribution may not be credited". Callers must
// then omit the credit entirely — no placeholder, no "Anonymous", no "hidden
// contributor". "added Jul 12" claims nothing about who, which is the honest
// rendering of "we may not say"; a placeholder would assert a person.
//
// A non-empty Name with a nil Username means "credit as plain text": the person
// is named, but there is no profile to link to.
type PublicContributorCredit struct {
	Name     string
	Username *string
}

// Renderable reports whether the credit may be published at all.
func (c PublicContributorCredit) Renderable() bool { return c.Name != "" }

// ResolvePublicContributorCredit resolves the credit a PUBLIC contribution
// byline may publish for user — the show submitter byline (PSY-1866), revision
// author attribution (PSY-1940), and anything later that names a person for an
// edit or a submission.
//
// Three fail-closed gates, in order. The first two drop the credit whole; the
// third keeps the name and drops only the link.
//
//  1. privacy_settings.contributions = "hidden" omits the credit. This is a
//     real user-facing setting, already honoured by the contributor leaderboard
//     (services/user/leaderboard.go), the activity heatmap and the profile's
//     contribution stats.
//  2. No public name tier omits the credit — see HasPublicName. This also
//     swallows the chain's terminal AnonymousUserName, which carried no
//     information worth a byline anyway.
//  3. profile_visibility = "private" keeps the NAME and drops the username, so
//     the credit renders as plain text. The profile route 404s a private
//     profile (community/contributor_profile.go), so linking would be a dead
//     link. Only gate 1 suppresses the person.
//
// Deliberately NOT viewer-dependent: the same public rule applies to everyone,
// the contributor included, and the leaderboard's own gate is likewise
// viewerless. A caller that serves an ADMIN tier decides that ABOVE this
// function, by calling ResolveUserName directly — see the revision handler.
//
// CALLERS USING A COLUMN-RESTRICTED Select MUST LOAD: id, username,
// display_name, first_name, last_name, privacy_settings, profile_visibility.
// `email` is not needed and should not be selected (see
// BatchResolvePublicUserNames on why omitting it is a feature).
func ResolvePublicContributorCredit(user *authm.User) PublicContributorCredit {
	if user == nil || user.ID == 0 {
		return PublicContributorCredit{}
	}

	// Gate 1. Falling back to the DEFAULTS, which have Contributions visible,
	// is not fail-closed by itself — but it matches how every other reader of
	// this column behaves, and a divergent local rule would be the surprise.
	// Both ways in are near-unreachable by the schema, which is what makes that
	// safe: the column is NOT NULL with a default, so the nil branch is purely
	// defensive, and it is jsonb, so Postgres rejects malformed JSON at write
	// time and only a well-formed blob of the wrong SHAPE can fail to unmarshal.
	privacy := contracts.DefaultPrivacySettings()
	if user.PrivacySettings != nil {
		_ = json.Unmarshal(*user.PrivacySettings, &privacy)
	}
	if privacy.Contributions == contracts.PrivacyHidden {
		return PublicContributorCredit{}
	}

	// Gate 2.
	if !HasPublicName(user) {
		return PublicContributorCredit{}
	}

	credit := PublicContributorCredit{Name: ResolveUserName(user)}

	// Gate 3. NOTE this return is not like the two above: the name is already
	// assigned and STAYS. A private profile loses only its link. Do not move
	// this above the assignment.
	if user.ProfileVisibility == "private" {
		return credit
	}
	credit.Username = ResolveUserUsername(user)
	return credit
}
