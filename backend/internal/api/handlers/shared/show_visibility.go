package shared

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
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

// ShowRowVisible is ShowSubResourceVisible for a route that has ALREADY LOADED
// the show it is deciding about.
//
// The detail route is the one caller: it has the row in hand, so asking the
// checker by id would re-read the two columns it is already holding. It takes no
// checker for the same reason, and so there is no nil-checker tier to fail
// closed on; what makes it safe is that it cannot answer from a row it does not
// have.
//
// Here rather than called directly from the handler so that every show gate in
// the handler layer goes through one door, and a reader comparing two routes in
// the same file finds one vocabulary.
//
// status is catalogm.ShowStatus rather than a bare string, and the conversion is
// the CALLER's, for the reason the predicate below it gives: a response DTO
// carries the status as a plain string, and a door that took one would decide a
// security boundary on whatever that field holds.
func ShowRowVisible(status catalogm.ShowStatus, submittedBy *uint, viewer contracts.ShowViewer) bool {
	return servicesshared.LoadedShowVisibleTo(status, submittedBy, viewer)
}

// EntitySubResourceVisible is ShowSubResourceVisible for a polymorphic route,
// where the entity may not be a show at all.
//
// The one call every gated polymorphic route makes. A thin delegation to
// services/shared.EntityVisibleTo, whose doc is the contract; it is thin ON
// PURPOSE, because the rule that decides an entity_type must be the same object
// the SQL spellings derive their allowlist from, or the handler gate and the row
// gates can disagree about what an entity type means.
//
// AN ENTITY TYPE WITH NO REGISTERED RULE IS NOT VISIBLE, so a junk
// {entity_type} segment is refused here rather than reaching the service that
// would have answered "invalid entity type". What a caller does with a refusal
// is still the caller's own no-data shape, per this file's header: the empty
// list, or the entity-not-found error a never-used id gets. One answer for a
// gated entity, a missing one and an unknown type is the point: the pairs that
// differ are the oracle.
//
// A CALLER THAT VALIDATES THE VOCABULARY FIRST answers its own error before this
// runs, and community.GetEntityCollectionsHandler does exactly that with a 422.
// That is its own contract, not this one.
func EntitySubResourceVisible(checker contracts.ShowVisibilityInterface, entityType string, entityID uint, viewer contracts.ShowViewer) bool {
	return servicesshared.EntityVisibleTo(checker, entityType, entityID, viewer)
}
