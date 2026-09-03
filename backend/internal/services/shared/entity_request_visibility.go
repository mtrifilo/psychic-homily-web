package shared

import (
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// ENTITY REQUESTS: REQUESTER OR ADMIN, AND NOBODY ELSE
// =============================================================================
//
// entity_requests rows are the contributor queue. Every route that reads the
// table serves it to one of two tiers and no other: GET /admin/entity-requests
// and the admin decide and fulfill routes to an ADMIN, and POST /entity-requests
// back to the REQUESTER who just filed it. No route serves a request row to a
// stranger or to an anonymous caller, and this rule is those two tiers written
// as a predicate.
//
// WHAT THE RULE DOES NOT DISTINGUISH IS decision_state. A PENDING row names
// content that has not been published and a REJECTED one names content that
// never will be, which is the case the rule was written for; an APPROVED and
// fulfilled row names a catalog entity that IS public, and it is refused on the
// same terms. That is the conservative answer and it has a cost: for every
// requested type but show, fulfilment records no other actor-attributed row
// (handlers/community/entity_request_fulfill.go stamps a submitter on the show
// branch alone), so a trusted contributor's created entities appear on no public
// timeline. Narrowing the refusal to rows with no created_entity_id is a
// one-line change here; it is not made because who may see that a request was
// filed, and by whom, is a product question rather than a code one.
//
// A REQUEST THAT NO LONGER EXISTS is refused to everybody, which is what keeps a
// deleted row and a withheld one one answer.
//
// The REQUESTER tier also means a requester sees, on an ADMIN's public timeline,
// that this admin decided their request and when. That is the rule as stated:
// they already hold the row, and it names no third party.

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
// An ANONYMOUS caller is refused without a probe: with no user id there is no
// requester branch to satisfy and no admin branch to take, so the condition is
// constant FALSE and binds nothing.
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
		return "FALSE", nil
	}
	return entityRequestExistsSQL(idExpr, visibleEntityRequestsAlias+".requester_id = ?"),
		[]interface{}{viewer.UserID}
}

// entityRequestExistsSQL wraps an entity_requests condition in the correlated
// EXISTS both tiers use, so they cannot correlate on different columns or cast
// the id differently from one another.
func entityRequestExistsSQL(idExpr, cond string) string {
	return entityExistsSQL("entity_requests", visibleEntityRequestsAlias,
		textIDAsBigintSQL(idExpr), cond)
}
