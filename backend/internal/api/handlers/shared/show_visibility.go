package shared

import (
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
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
// The one call every gated polymorphic route makes. It is a thin delegation to
// services/shared.EntityVisibleTo, which owns the per-entity-type registry, and
// it is thin ON PURPOSE: the rule that decides an entity_type must be the same
// object the SQL spellings derive their allowlist from, or the handler gate and
// the row gates can disagree about what an entity type means.
//
// AN ENTITY TYPE WITH NO REGISTERED RULE IS NOT VISIBLE (PSY-1987). This used to
// pass everything that was not a show, which is how `collection` — a type with a
// real read-time rule of its own — reached six surfaces ungated. What a caller
// does with a refusal is still the caller's own no-data shape, per this file's
// header: the empty list, or the entity-not-found error a never-used id gets.
//
// One consequence worth stating because it is a contract change: a route that
// used to answer "invalid entity type" for a junk {entity_type} segment now
// refuses it as a missing entity, at the gate, before the service that produced
// that message runs. That is the fail-closed default doing its job, and it also
// removes a small oracle — the old pair of answers published which entity types
// the vocabulary contains.
func EntitySubResourceVisible(checker contracts.ShowVisibilityInterface, entityType string, entityID uint, viewer contracts.ShowViewer) bool {
	return servicesshared.EntityVisibleTo(checker, entityType, entityID, viewer)
}
