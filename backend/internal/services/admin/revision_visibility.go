package admin

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
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
// It is spelled in THREE functions in this file, and a change to the policy has
// to touch all three or the routes will disagree with each other:
// requireEntityVisible (the entity-history route's 404), revisionVisibleTo (the
// single-revision route), and visibleRevisionsOnly (the SQL both listings
// filter with). TestTheGoPredicateAndTheSQLFilterAgree pins the last two
// against each other; nothing but this sentence pins the first.
//
// The mirror also stops at revision history. Other public routes that serve a
// show's content by id — field notes, comments, tags, collections, followers —
// do NOT consult shows.status today. This file closes the revision half of that
// gap, not the whole of it.
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
// under. Named so the gate and the query it filters cannot drift onto different
// spellings.
//
// Case matters and needs no defence: revisions are written with this exact
// value, the entity_type comparison in Postgres is case-sensitive, and a caller
// passing "Show" is rejected by the handler's entity-type allowlist before it
// reaches here. Were it not, it would select zero rows rather than bypass the
// gate.
const entityTypeShow = "show"

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

	// Built by concatenation rather than written whole because the submitter
	// branch must disappear for an anonymous caller instead of comparing
	// against user id 0. No caller input reaches the string; the only variable
	// part is whether one fixed fragment is present.
	//
	// The OUTER parentheses are written here rather than left to the query
	// builder, and showVisibleTo does the same. GORM does wrap raw conditions
	// containing OR today, so neither is a workaround: they are a refusal to let
	// a security boundary rest on framework behaviour that could change. Written
	// out, the binding this pins is `author = x AND (type <> 'show' OR <visible
	// show>)` — never `author = x AND type <> 'show' OR <visible show>`, which
	// would serve every visible show revision by every author.
	cond := "(revisions.entity_type <> ? OR (" +
		"revisions.from_gated_show = FALSE AND EXISTS (" +
		"SELECT 1 FROM shows s WHERE s.id = revisions.entity_id AND (s.status = ?"
	args := []interface{}{entityTypeShow, catalogm.ShowStatusApproved}
	if viewer.UserID != 0 {
		cond += " OR s.submitted_by = ?"
		args = append(args, viewer.UserID)
	}
	cond += "))))"

	return q.Where(cond, args...)
}

// showVisibleTo answers the detail route's question for one show: may this
// viewer see it at all.
//
// Callers have already established that viewer is not an admin. Returns false
// when the show does not exist, which is the same answer GET /shows/{id} gives
// by 404ing, and false on any lookup failure.
func (s *RevisionService) showVisibleTo(showID uint, viewer contracts.RevisionViewer) bool {
	if s.db == nil || showID == 0 {
		return false
	}

	// One condition carrying its own parentheses, for the same reason
	// visibleRevisionsOnly writes its own: GORM does parenthesize a raw
	// condition containing OR today, so this is a deliberate refusal to let a
	// security boundary rest on framework behaviour, not a workaround for a bug.
	// Written out, the binding this pins is `id = X AND (status = 'approved' OR
	// submitted_by = Y)` — never `id = X AND status = 'approved' OR
	// submitted_by = Y`, which would answer yes for a gated show whenever the
	// caller submitted ANY show at all.
	q := s.db.Model(&catalogm.Show{})
	if viewer.UserID != 0 {
		q = q.Where("id = ? AND (status = ? OR submitted_by = ?)",
			showID, catalogm.ShowStatusApproved, viewer.UserID)
	} else {
		q = q.Where("id = ? AND status = ?", showID, catalogm.ShowStatusApproved)
	}

	var count int64
	if err := q.Count(&count).Error; err != nil {
		logger.Default().Error("revision_visibility_show_lookup_failed",
			"show_id", showID,
			"error", err.Error(),
		)
		return false
	}
	return count > 0
}
