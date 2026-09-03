package shared

import (
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// ENTITY REQUESTS: FULFILLED IS PUBLIC, UNFULFILLED IS REQUESTER OR ADMIN
// =============================================================================
//
// entity_requests rows are the contributor queue, and created_entity_id is what
// splits them. THE COLUMN IS THE RULE, not decision_state:
//
//   - created_entity_id IS NOT NULL: the request produced a catalog entity that
//     is itself public, so that this contributor asked for it is public too.
//     EVERY tier sees the row.
//   - created_entity_id IS NULL: the request names content that has not been
//     published and may never be. Only the REQUESTER and an ADMIN see it.
//
// KEYED ON THE COLUMN RATHER THAN ON decision_state because the two disagree in
// the case that matters. An approved show request whose fulfilment deferred
// (handlers/community/entity_request.go leaves it approved-but-unfulfilled for
// the PSY-1088 rescue queue) is approved and has created nothing, so it stays
// private until the rescue endpoint fills the column in. Reading the state
// instead would publish a row naming a show that does not exist.
//
// WHY THE PUBLIC TIER EXISTS AT ALL: POST /entity-requests is the only route by
// which a non-admin creates an artist, venue, label, release or festival, and
// fulfilment stamps a submitter on the SHOW branch alone
// (handlers/community/entity_request_fulfill.go), so for every other type this
// audit row is the sole record that the contributor made the thing. Refusing it
// to strangers would erase their public contribution history.
//
// WHAT THE PUBLIC TIER DISCLOSES is bounded by what the row still carries once
// the timeline is done with it: the action, the requested type, the request id
// and a timestamp. The name is suppressed, because a request id is not a catalog
// id, and the metadata is withheld by the allowlist, so nothing here publishes
// the payload, the requester id, or the superseded-submission digest.
//
// A REQUEST THAT NO LONGER EXISTS is refused to everybody, which is what keeps a
// deleted row and a withheld one one answer.
//
// The REQUESTER tier also means a requester sees, on an ADMIN's public timeline,
// that this admin decided their own request and when. That is the rule as
// stated: they already hold the row, and it names no third party.

// visibleEntityRequestsAlias is the alias the spelling below binds the
// entity_requests table to.
//
// Deliberately not `er` or `entity_requests`, for the reason visibleShowsAlias
// gives: an alias declared in this subquery SHADOWS an outer one of the same
// name, so an idExpr qualified with a colliding alias would self-correlate and
// the EXISTS would be true whenever any request exists at all. Callers must
// never pass an expression qualified with this alias.
const visibleEntityRequestsAlias = "visible_entity_request"

// VisibleEntityRequestTextIDExistsSQL returns a correlated EXISTS condition,
// true when the entity request whose id is written as TEXT in idExpr is one
// viewer may see, plus its bind arguments.
//
// TEXT rather than a typed column because the id a caller can trust sits inside
// the audit row's JSON metadata. An entity-request audit row carries the request
// id in entity_id too, but THAT COLUMN IS REWRITTEN BY CATALOG MERGES:
// repointEntityRefs (services/catalog/entity_ref_repoint.go) updates audit_logs
// keyed on (entity_type, entity_id) alone, and these rows store the REQUESTED
// catalog type in entity_type, so merging the entity whose id equals a request's
// id moves that request's audit rows onto the canonical entity's number. A
// metadata key is not in that statement's reach.
//
// THE ADMIN TIER STILL PAYS THE EXISTENCE PROBE rather than short-circuiting to
// a bare TRUE, which is where this differs from VisibleShowExistsSQL. A row
// whose request has been deleted names nothing, and an admin seeing it while
// everyone else does not would make the pair of answers an oracle over the
// request id space.
//
// An ANONYMOUS caller STILL PAYS THE PROBE, because the public tier is a
// property of the request rather than of the caller: with no user id there is no
// requester branch to add, so the condition is the fulfilled test alone and binds
// nothing.
//
// A row whose idExpr names no request is NOT visible, which is what lets a
// caller answer the same for a refused request and a deleted one. That holds for
// EVERY tier here, including the admin one, which is the difference from
// VisibleShowExistsSQL noted above. textIDAsBigintSQL's digits-only guard makes
// a non-numeric value answer "no such request" rather than raising inside the
// statement it sits in.
//
// idExpr is SQL the CALLER controls and must be a literal in the calling code.
// Nothing derived from a request may reach it.
func VisibleEntityRequestTextIDExistsSQL(idExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	if viewer.IsAdmin {
		return entityRequestExistsSQL(idExpr, "TRUE"), nil
	}
	if viewer.UserID == 0 {
		return entityRequestExistsSQL(idExpr, entityRequestFulfilledSQL), nil
	}
	// Built by concatenation rather than written whole so the requester branch
	// DISAPPEARS for a caller with no id instead of comparing against user 0,
	// which is VisibleShowPredicateSQL's reason for the same shape. The
	// parentheses are written here and not left to the caller: this OR sits
	// inside an EXISTS whose other conjunct is the id correlation, and a form
	// where that correlation bound inside the OR would answer true whenever any
	// fulfilled request existed at all.
	return entityRequestExistsSQL(idExpr,
			"("+entityRequestFulfilledSQL+" OR "+visibleEntityRequestsAlias+".requester_id = ?)"),
		[]interface{}{viewer.UserID}
}

// entityRequestFulfilledSQL is the public-tier test: the request created a
// catalog entity, so the row is answerable to everyone.
//
// ONE SPELLING, spliced into both non-admin tiers, so the anonymous answer and
// the authenticated one cannot come to differ about what fulfilled means.
const entityRequestFulfilledSQL = visibleEntityRequestsAlias + ".created_entity_id IS NOT NULL"

// entityRequestExistsSQL wraps an entity_requests condition in the correlated
// EXISTS all three tiers use, so they cannot correlate on different columns or
// cast the id differently from one another.
func entityRequestExistsSQL(idExpr, cond string) string {
	return entityExistsSQL("entity_requests", visibleEntityRequestsAlias,
		textIDAsBigintSQL(idExpr), cond)
}
