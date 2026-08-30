package shared

import (
	"strings"

	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// SHOW VISIBILITY AT THE HANDLER BOUNDARY
// =============================================================================
//
// The rule lives in services/shared/show_visibility.go. This file is the
// handler-side half of it: how a route resolves WHO is asking, and what a route
// answers when the answer is no (PSY-1939).
//
// What a gated show must look like is the whole design constraint. It must look
// like a show that does not exist — not like a show the caller lacks permission
// for. A 403, a distinguishable message, an empty page beside a non-zero total,
// or a 404 whose body differs from the ordinary one all restore the enumeration
// oracle the detail route's 404 exists to remove. So there is no shared "deny"
// response here: the caller answers with ITS OWN no-data shape, which is why
// each gated route below returns its empty list or zero count rather than
// calling a helper that decides the status code for it.

// showEntityTypes are the spellings of "show" this gate recognises in an
// {entity_type} path segment.
//
// Every gated route today emits the SINGULAR: the comment, tag and collection
// allowlists all spell it "show". The plural is here anyway because the codebase
// uses it elsewhere for the same concept (catalog.EntityExistenceService keys on
// "shows"), so a route wired to that vocabulary would otherwise reach a gate
// that silently waves it through. Recognising a spelling no route emits costs
// nothing; failing to recognise one that does costs the whole gate.
//
// Case is folded rather than compared exactly for the same reason: an
// entity_type is caller-supplied text, and a gate that "show" passes but "Show"
// slips through is not a gate. The underlying queries are case-sensitive and
// would return nothing for the odd spelling anyway; this makes that a property
// of the gate rather than a lucky property of Postgres.
var showEntityTypes = map[string]bool{
	"show":  true,
	"shows": true,
}

// isShowEntityType reports whether an {entity_type} path segment names a show.
func isShowEntityType(entityType string) bool {
	return showEntityTypes[strings.ToLower(strings.TrimSpace(entityType))]
}

// ShowSubResourceVisible reports whether a show-scoped sub-resource read may be
// answered with real data for viewer.
//
// The one call every gated show sub-resource route makes.
//
// A nil checker answers FALSE, which is why every handler holding one documents
// its field as required rather than optional: a handler constructed without a
// gate refuses every show-scoped read instead of serving it, so a construction
// bug on this boundary fails closed.
//
// The viewer arrives as a value rather than being read out of a context here,
// because this package must not import the middleware that plants it:
// internal/api/middleware imports services/admin, and services/admin's internal
// tests import THIS package, so an import of middleware here closes a cycle in
// the admin test binary. middleware.GetShowViewerFromContext is the resolver.
func ShowSubResourceVisible(checker contracts.ShowVisibilityInterface, showID uint, viewer contracts.ShowViewer) bool {
	if checker == nil {
		return false
	}
	return checker.ShowVisibleTo(showID, viewer)
}

// EntitySubResourceVisible is ShowSubResourceVisible for a polymorphic route,
// where the entity may not be a show at all.
//
// Non-show entity types pass untouched. That is a deliberate default-open on
// entity types that have no read-time visibility rule of their own; adding one
// means adding it here, beside this sentence, not at each caller.
func EntitySubResourceVisible(checker contracts.ShowVisibilityInterface, entityType string, entityID uint, viewer contracts.ShowViewer) bool {
	if !isShowEntityType(entityType) {
		return true
	}
	return ShowSubResourceVisible(checker, entityID, viewer)
}
