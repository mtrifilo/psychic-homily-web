package shared

import (
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// ENTITY REQUESTS — REQUESTER OR ADMIN, AND NOBODY ELSE
// =============================================================================
//
// entity_requests rows are the contributor queue: a pending row carries a
// payload naming content that has not been published, and a rejected one names
// content that never will be. Every route that reads the table serves it to one
// of two tiers and no other: GET /admin/entity-requests and the admin decide and
// fulfill routes to an ADMIN, and POST /entity-requests back to the REQUESTER
// who just filed it. No route serves a request row to a stranger or to an
// anonymous caller, and this rule is those two tiers written as a predicate.
//
// The rule this file spells is therefore the narrowest one that still lets the
// two parties who already hold the row see it: the request's own REQUESTER, and
// an ADMIN. Everybody else is refused, and a request that no longer exists is
// refused to everybody, which is what keeps a deleted row and a withheld one one
// answer.

// visibleEntityRequestsAlias is the alias VisibleEntityRequestExistsSQL binds
// the entity_requests table to.
//
// Deliberately not `er` or `entity_requests`, for the reason visibleShowsAlias
// gives: an alias declared in this subquery SHADOWS an outer one of the same
// name, so an idExpr qualified with a colliding alias would self-correlate and
// the EXISTS would be true whenever any request exists at all. Callers must
// never pass an expression qualified with this alias.
const visibleEntityRequestsAlias = "visible_entity_request"

// VisibleEntityRequestExistsSQL returns a correlated EXISTS condition, true when
// the entity request named by requestIDExpr is one viewer may see, plus its bind
// arguments.
//
// For a query that holds an entity_requests id in some other table's column. The
// contributions timeline is the caller that needs it: its audit rows record the
// REQUESTED entity type in entity_type and a REQUEST id in entity_id, so nothing
// on the row identifies the table its id belongs to except the action.
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
// requestIDExpr is SQL the CALLER controls and must be a literal in the calling
// code. Nothing derived from a request may reach it.
func VisibleEntityRequestExistsSQL(requestIDExpr string, viewer contracts.ShowViewer) (string, []interface{}) {
	if viewer.IsAdmin {
		return entityExistsSQL("entity_requests", visibleEntityRequestsAlias, requestIDExpr, "TRUE"), nil
	}
	if viewer.UserID == 0 {
		return "FALSE", nil
	}
	return entityExistsSQL("entity_requests", visibleEntityRequestsAlias, requestIDExpr,
			visibleEntityRequestsAlias+".requester_id = ?"),
		[]interface{}{viewer.UserID}
}
