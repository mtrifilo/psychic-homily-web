package shared

import (
	"encoding/json"
	"strings"

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
// AnonymousUserName. Answered by running the public tiers rather than by
// re-listing them, and asked of the SOURCE columns rather than of a resolved
// string, because an email-derived name is a bare local-part with nothing in it
// to pattern-match on.
//
// nil-safe: false for a nil or ID-0 user.
func HasPublicName(user *authm.User) bool {
	return resolvePublicNameTiers(user) != ""
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
// The two share resolvePublicNameTiers, so this cannot drift into a different
// ORDERING of the tiers it does publish — only into a different terminal, which
// is the whole point of the pair.
//
// Never empty, so it is a drop-in for the surfaces whose author slot must
// render something. Surfaces that can OMIT the name instead should use
// ResolvePublicContributorCredit, which also applies the privacy gate.
func ResolvePublicUserName(user *authm.User) string {
	if name := resolvePublicNameTiers(user); name != "" {
		return name
	}
	return AnonymousUserName
}

// BatchResolvePublicUserNames resolves names for many ids in one query, each
// through ResolvePublicUserName.
//
// THIS APPLIES RULE 1 ONLY. It cuts the email tier; it does NOT read
// privacy_settings.contributions, and its Select does not even load the column.
// It is therefore right for the authored-content family (comment authors,
// curators, requesters) and WRONG for a list of CONTRIBUTION bylines, which
// needs ResolvePublicContributorCredit's gate. There is no batch form of that
// yet: the first list endpoint to need one should add
// BatchResolvePublicContributorCredits here, with privacy_settings and
// profile_visibility in its Select, rather than reaching for this because the
// name looks safe.
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
//
// Trims before testing, belt-and-braces with resolvePublicNameTiers: a
// whitespace-only name is not a name, and the failure it produces is a byline
// reading "added Jul 12 by " with nothing after the "by".
func (c PublicContributorCredit) Renderable() bool { return strings.TrimSpace(c.Name) != "" }

// ContributorProfileLink returns the /users/{username} slug to hang a credit on,
// or nil when there is no profile to link to.
//
// Two ways to get nil, and they are the same answer for different reasons: the
// account has no username, or its profile_visibility is "private".
//
// The private case is NOT a privacy grant, and that is why this is a separate
// function rather than a branch inside ResolvePublicContributorCredit. It is a
// fact about whether the URL resolves: /users/{username} 404s for a private
// profile for EVERYONE, admins included (community/contributor_profile.go). So
// an admin-tier byline that skipped this gate would not be seeing more, it would
// be getting a link that is guaranteed to break.
//
// SCOPE, precisely: every CONTRIBUTION byline goes through this, both tiers.
// The authored-content family does not yet — comment_service, request,
// collection, charts and the notification feed still call ResolveUserUsername
// (or its batch form) directly, so a private profile keeps a dead link there.
// That is pre-existing and is its own follow-up; do not read this function as a
// site-wide invariant.
func ContributorProfileLink(user *authm.User) *string {
	if user == nil || user.ProfileVisibility == "private" {
		return nil
	}
	return ResolveUserUsername(user)
}

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
//
//     "hidden" is the ONLY level that suppresses, and that is an open question
//     rather than a settled rule. The owner decided "hidden" on 2026-08-29
//     (PSY-1866); "count_only" was not put to them. It is defensible to credit
//     a count_only contributor — a byline is one edit, not an aggregate — and
//     it is defensible not to, because their own profile answers
//     /users/{username}/contributions with an empty list while this names them
//     on a specific edit. Crediting them is the pre-existing behaviour, so this
//     preserves it rather than deciding. ASK BEFORE CHANGING EITHER WAY, and
//     note that the same question sits in leaderboard.go's raw SQL, which
//     likewise tests only for 'hidden'.
//
//  2. No public name tier omits the credit — see HasPublicName. This also
//     swallows the chain's terminal AnonymousUserName, which carried no
//     information worth a byline anyway.
//
//  3. No reachable profile drops the LINK and keeps the name, so the credit
//     renders as plain text — see ContributorProfileLink, which every tier
//     shares because a dead link is dead for admins too. Only gate 1 suppresses
//     the person.
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

	// Gate 3. NOTE this is not like the two above: it never clears the name, it
	// only decides whether there is a link to hang it on.
	return PublicContributorCredit{
		Name:     ResolvePublicUserName(user),
		Username: ContributorProfileLink(user),
	}
}
