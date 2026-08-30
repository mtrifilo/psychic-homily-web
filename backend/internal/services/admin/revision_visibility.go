package admin

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// =============================================================================
// READ-TIME ENTITY VISIBILITY
// =============================================================================
//
// This file is the ENTITY-level half of revision privacy. Its FIELD-level twin
// lives beside it in revision.go (applyPrivacyRedaction), and the two answer
// different questions:
//
//   - Field masking serves the revision and hides some values inside it. An
//     unverified venue's address history is still auditable as an edit.
//   - Entity gating does not serve the revision at all. There is no masked
//     variant, because the thing being withheld is the existence of the edit
//     as much as its values.
//
// One entity uses the second kind today: shows. GET /shows/{id} answers 404 for
// a show whose status is not approved unless the caller is an admin or the
// show's own submitter (handlers/catalog.GetShowHandler). Revision history
// mirrors that rule, because it publishes the show's title, event date, city,
// state, price, ticket url and description — the payload the 404 exists to
// withhold. Unpublishing a show has to hide its history too, or the gate on the
// live payload only costs the reader one extra request.
//
// ONE documented departure: a row a show merge stamped is admin-only, even for
// the surviving show's submitter, because the submitted_by that would have
// answered was deleted by that merge. See FromGatedShow below; do not read the
// paragraphs above as promising submitter parity for those rows.
//
// Mirroring, not approximating, is the point. The rule is keyed on the same two
// facts the detail route reads (shows.status and shows.submitted_by), so a
// change to what "visible" means has one obvious second site rather than a rule
// that silently drifts out of agreement.
//
// THREE functions in this file decide WHERE the rule is applied:
// requireEntityVisible (the entity-history route's 404), revisionVisibleTo (the
// single-revision route), and visibleRevisionsOnly (the SQL both listings
// filter with). All three now delegate the rule itself to services/shared, so a
// change to what "visible" means is made once, there.
// TestTheGoPredicateAndTheSQLFilterAgree pins the last two against each other;
// nothing but this sentence pins the first.
//
// The mirror no longer stops at revision history. The sibling routes that serve
// a show's content by id — field notes, comments, tags, collections, saves —
// were closed by PSY-1939, and the rule they all evaluate is the one in
// services/shared/show_visibility.go. This file holds what is SPECIFIC to
// revisions: which reads gate, what a gated read answers, and the merge
// provenance stamp below. The predicate itself is no longer spelled here.
//
// It fails closed at every step. A missing show row, a nil db and a failed
// lookup all resolve to "not visible" for a non-admin, which withholds history
// that may well have been publishable. Withholding a publishable edit is
// recoverable; publishing an unpublished show's history is not.
//
// A revision pointing at a show that no longer exists is therefore hidden from
// the public and still readable by an admin. That is deliberate: revisions are
// polymorphic and carry no foreign key, so a deleted show leaves its history
// behind with nothing left to gate on, and the audit trail is exactly what an
// admin needs at that point.
//
// Nothing is scrubbed: the stored rows keep their real values either way, which
// is what rollback reads. Re-approving a show restores its history to everyone,
// the same way verifying a venue restores its addresses, because the gate is
// evaluated at read time against the show's current status.
//
// The ONE exception is a merge-stamped row. FromGatedShow only ever goes TRUE,
// nothing clears it, and the gate below honors it whatever the current show's
// status says — so re-approving the surviving show does NOT bring a stamped row
// back, and no product action will. That is the price of a marker that cannot be
// laundered by a chain of merges; recovering such a row means a deliberate
// database update, not a status change.
//
// Merges are the case a status lookup cannot answer on its own, and they are
// handled by a provenance stamp rather than by this lookup. See
// adminm.Revision.FromGatedShow and catalog.MergeDuplicateShow.

// entityTypeShow is the polymorphic entity_type value show revisions are stored
// under.
//
// The canonical spelling lives in services/shared beside the rest of the rule,
// because the contributor profile filters revisions by the same value. This
// name is the local alias for it, not a second definition.
//
// A caller passing "Show" is rejected by the handler's entity-type allowlist
// before it reaches here. Were it not, the case-sensitive comparison would
// select zero rows rather than bypass the gate.
const entityTypeShow = shared.RevisionEntityTypeShow

// requireEntityVisible reports whether a whole entity's revision history may be
// served to viewer, returning contracts.ErrRevisionEntityHidden when it may not.
//
// Only the entity-history route calls this, because only it names an entity the
// caller may not be allowed to know exists. The other two reads name a revision
// or an author and filter rows instead; see visibleRevisionsOnly.
func (s *RevisionService) requireEntityVisible(entityType string, entityID uint, viewer contracts.RevisionViewer) error {
	if viewer.IsAdmin {
		return nil
	}
	if entityType != entityTypeShow {
		return nil
	}
	if s.showVisibleTo(entityID, viewer) {
		return nil
	}
	return contracts.ErrRevisionEntityHidden
}

// requireAuthorContributionsVisible reports whether a whole per-AUTHOR revision
// listing may be served, returning contracts.ErrRevisionEntityHidden when it may
// not.
//
// Only GET /users/{user_id}/revisions calls this, and it is the one read that is
// indexed BY a person rather than by an entity. That makes it a contributions
// listing, which is exactly what privacy_settings.contributions governs — and
// every sibling in that family already refuses it: /users/{username}/contributions
// 404s for "hidden", the activity heatmap answers empty, the rankings 404, and
// the leaderboard filters the row out in SQL. This route was the one that did
// not, reachable by swapping the username in the URL for the numeric id.
//
// Suppressing the BYLINE is not enough on its own here, which is why this gate
// exists rather than leaning on the response mapper. An anonymous caller could
// read an entity's history, take a revision id whose author was suppressed, and
// scan this route until a page contained that id — recovering the author id, and
// from there the name, off any public payload that publishes an id beside a
// display name. Withholding user_id from the response raises that cost; only
// refusing the listing removes it.
//
// The OWNER always sees their own, and so does an admin. Fails closed on
// everything else, including a lookup error: a user row we cannot read is one we
// cannot clear.
//
// 404, not an empty 200, to match the profile route's answer for the same
// setting and to stay indistinguishable from a user id that does not exist.
func (s *RevisionService) requireAuthorContributionsVisible(userID uint, viewer contracts.RevisionViewer) error {
	if viewer.IsAdmin || (viewer.UserID != 0 && viewer.UserID == userID) {
		return nil
	}

	var author authm.User
	if err := s.db.Select("id, privacy_settings").First(&author, userID).Error; err != nil {
		// A missing author has no contributions to list, and a failed read is
		// not a clearance. Both answer the same 404 the caller would get for a
		// user id that never existed.
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Default().Error("revision_author_privacy_lookup_failed",
				"user_id", userID,
				"error", err.Error(),
			)
		}
		return contracts.ErrRevisionEntityHidden
	}

	privacy := contracts.DefaultPrivacySettings()
	if author.PrivacySettings != nil {
		_ = json.Unmarshal(*author.PrivacySettings, &privacy)
	}
	if privacy.Contributions == contracts.PrivacyHidden {
		return contracts.ErrRevisionEntityHidden
	}
	return nil
}

// revisionVisibleTo reports whether one already-loaded revision may be served.
//
// The single-revision route's gate. It checks the provenance stamp BEFORE the
// status lookup, because a stamped row's original show is deleted and the
// lookup would answer for the show it was merged into.
func (s *RevisionService) revisionVisibleTo(r *adminm.Revision, viewer contracts.RevisionViewer) bool {
	if viewer.IsAdmin {
		return true
	}
	if r.EntityType != entityTypeShow {
		return true
	}
	if r.FromGatedShow {
		return false
	}
	return s.showVisibleTo(r.EntityID, viewer)
}

// visibleRevisionsOnly narrows a revisions query to the rows viewer may read.
//
// It exists as a query filter rather than a post-load drop so that the TOTAL a
// paginated listing reports counts the same rows the page contains. Filtering
// after the fact would return a short page beside a total that announces how
// many rows were withheld, which is this leak stated as a number.
//
// KNOWN GAP, and the reason that sentence says "this leak" and not "the leak":
// the same number is still derivable elsewhere. user.ContributorProfileService
// counts revisions_made as an unfiltered COUNT(*) over revisions.user_id and
// serves it on the public profile, so differencing it against this total yields
// how many of an author's edits sit on shows the caller cannot see. Closing that
// means threading a viewer through the profile stats, which is a change to a
// different service's contract and is deliberately not made here. The
// counterpart note lives at that count.
//
// The condition is the SQL spelling of revisionVisibleTo, and the two have to
// stay in agreement. It is one correlated EXISTS on the shows primary key, so
// the cost is one index probe per show revision on the page, not a join that
// multiplies rows.
//
// Non-show revisions pass untouched, including every entity type that has no
// entity-level gate. That is a deliberate default-open on the ENTITY question
// only: field masking still runs afterwards in applyPrivacyRedaction.
func visibleRevisionsOnly(q *gorm.DB, viewer contracts.RevisionViewer) *gorm.DB {
	if viewer.IsAdmin {
		return q
	}

	// The condition is the shared one, and it is shared precisely so the total
	// this listing reports and the revisions_made count on the public profile
	// are the same number. See shared.VisibleShowRevisionsSQL for the three
	// terms and for why the parentheses are written rather than left to the
	// query builder.
	cond, args := shared.VisibleShowRevisionsSQL(viewer)

	return q.Where(cond, args...)
}

// showVisibleTo answers the detail route's question for one show: may this
// viewer see it at all.
//
// A one-line delegation to the shared rule, kept as a named method so the two
// call sites above read as one gate rather than as two spellings of a database
// lookup.
func (s *RevisionService) showVisibleTo(showID uint, viewer contracts.RevisionViewer) bool {
	return shared.ShowVisibleTo(s.db, showID, viewer)
}
