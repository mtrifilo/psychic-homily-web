package routes

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"testing"
)

// Every route that can be addressed by a SHOW must have a decided position on
// the show detail route's visibility rule (PSY-1939).
//
// The gates themselves live in handlers, which makes them forgettable: a route
// added next year gets no gate and no failing test, and the leak is found by
// the next audit rather than by the build. This file is the structural answer.
// It walks the BUILT router, finds every operation whose path can name a show,
// and fails unless that exact operation appears in the table below with a
// recorded disposition.
//
// So the cost of adding a show-addressable route is one line here plus the
// judgement it forces. That is the point: the failure mode this closes is not a
// wrong decision, it is an unmade one.
//
// It pins the INVENTORY, not the behaviour. What each gated route actually
// answers is show_subresource_visibility_test.go's job, and the two are meant to
// be read together: this one says "these are all the doors", that one says "and
// these are locked".
//
// KNOWN LIMIT, stated so nobody reads this as an exhaustive sweep: it matches on
// PATH SHAPE, so it sees only routes that carry a show id in the path. THREE
// other families reach a show and are invisible here:
//
//   - routes addressed by a SUB-RESOURCE id whose handler resolves the show from
//     it (/comments/{id}, /comments/{id}/replies, /collections/{slug});
//   - routes that take the entity type as a QUERY parameter
//     (/tags/{id}/entities?entity_type=show, /auth/collections/contains);
//   - SELF-SCOPED routes addressed by the CALLER, which name no entity at all in
//     the path and yet enumerate shows in their payload — /me/comment-subscriptions,
//     /me/notifications, /me/notifications/mark-read. PSY-1983 is this family's
//     first recorded instance, and it is the one this guard is least able to
//     anticipate: the next /me/… surface that renders show titles (a digest, an
//     activity feed, an export) will trip nothing here.
//
// All three have leaked in practice. Extending the guard to them means driving it
// off the handler set rather than the path, which is the follow-up this paragraph
// exists to name.

// showRouteDisposition records why a show-addressable operation is safe.
type showRouteDisposition int

const (
	// gated: the route consults the show's visibility before answering.
	gated showRouteDisposition = iota
	// selfScoped: the route answers only about the CALLER's own relationship to
	// the show and needs authentication to say anything at all. It publishes no
	// show content.
	selfScoped
	// adminOnly: the route is registered on the admin group, which is allowed to
	// see every show.
	adminOnly
	// writeOracleDeferred: an authenticated caller can still distinguish a gated
	// show from a nonexistent one by whether the write succeeds. Deliberately
	// open pending a product decision on the write policy for this family; see
	// the ticket's scope-deviation note.
	//
	// This says NOTHING about what the surface discloses. The comment-
	// subscription family carried this disposition while it DID publish a gated
	// show's title and slug through the watching list, which was a known open
	// leak recorded here rather than a claim of safety; PSY-1983 closed it and
	// moved that family to `gated`. Read the remaining entries the same way.
	writeOracleDeferred
	// notShowAddressable: the route takes an {entity_type} segment but its
	// allowlist does not accept a show, so no show id can reach it. Recorded
	// rather than omitted so that widening the allowlist has to come here.
	notShowAddressable
)

// showAddressableRoutes is the whole inventory, keyed by "METHOD pattern" as chi
// reports it.
//
// A route missing from this map fails the test. Adding one here is a claim about
// it, so make the claim honestly: `gated` means something actually calls the
// visibility rule on that path, and the sibling behaviour test should prove it.
var showAddressableRoutes = map[string]showRouteDisposition{
	// The rule's own route.
	"GET /shows/{show_id}": gated,

	// A show's sub-resources, gated on the real viewer.
	"GET /shows/{show_id}/field-notes":                  gated,
	"POST /shows/{show_id}/field-notes":                 gated,
	"GET /entities/{entity_type}/{entity_id}/comments":  gated,
	"POST /entities/{entity_type}/{entity_id}/comments": gated,
	"GET /entities/{entity_type}/{entity_id}/tags":      gated,
	"GET /collections/entity/{entity_type}/{entity_id}": gated,
	"GET /crates/entity/{entity_type}/{entity_id}":      gated,

	// The comment-subscription family (PSY-1983). Subscribing is a standing
	// request for a show's activity, so it takes the same viewer its content
	// does; the status route answers "not subscribed" rather than refusing,
	// because refusing would confirm the id while answering truthfully would
	// publish a live comment count.
	"POST /entities/{entity_type}/{entity_id}/subscribe":       gated,
	"GET /entities/{entity_type}/{entity_id}/subscribe/status": gated,
	"POST /entities/{entity_type}/{entity_id}/mark-read":       gated,

	// Public-tier gates: these answer the same for everyone.
	"GET /shows/{show_id}/calendar.ics":               gated,
	"HEAD /shows/{show_id}/calendar.ics":              gated,
	"GET /shows/{show_id}/also-tonight":               gated,
	"GET /shows/{show_id}/timeline":                   gated,
	"GET /shows/{show_id}/saves":                      gated,
	"POST /shows/saves/batch":                         gated,
	"HEAD /entities/{entity_type}/{entity_id}/exists": gated,
	// Revision history, gated by PSY-1715 on the same rule; see
	// services/admin/revision_visibility.go.
	"GET /revisions/{entity_type}/{entity_id}": gated,
	// Addressed by venue and year rather than by a show id, but it answers
	// whether that venue had shows in that year, so it reads approved-only
	// (services/catalog/venue.go). Caught by this guard rather than by the
	// ticket's list, which is the guard working.
	"HEAD /venues/{venue_id}/shows/{year}/exists": gated,

	// Self-scoped: authenticated, and only about the caller's own row.
	"POST /saved-shows/{show_id}":        selfScoped,
	"DELETE /saved-shows/{show_id}":      selfScoped,
	"GET /saved-shows/{show_id}/check":   selfScoped,
	"GET /shows/{show_id}/my-report":     selfScoped,
	"PUT /shows/{show_id}":               selfScoped,
	"DELETE /shows/{show_id}":            selfScoped,
	"POST /shows/{show_id}/unpublish":    selfScoped,
	"POST /shows/{show_id}/make-private": selfScoped,
	"POST /shows/{show_id}/publish":      selfScoped,
	"POST /shows/{show_id}/sold-out":     selfScoped,
	"POST /shows/{show_id}/cancelled":    selfScoped,
	// Deliberately NOT gated, and self-scoped is the honest reading: it deletes
	// the caller's own row and answers identically whether one was there, so it
	// publishes nothing and offers no oracle. Gating it would only remove the
	// last direct path to a row the watching list already hides (PSY-1983).
	"DELETE /entities/{entity_type}/{entity_id}/subscribe": selfScoped,

	// The follow family. The ticket names "followers routes" as a leak, and they
	// are not one: shows are not a followable entity type. validFollowEntityTypes
	// (services/engagement/follow.go) has no show key, and the handler's own
	// allowlist (handlers/engagement/follow.go) has none either, so
	// parseEntityType refuses a show before any lookup runs. Two of these are on
	// the OPTIONAL-auth group and serve anonymous follower counts, so they are
	// emphatically not self-scoped; the reason they are safe is the allowlist,
	// and TestFollowRoutesRefuseShows pins it.
	"GET /{entity_type}/{entity_id}/follow/alerts":   notShowAddressable,
	"PATCH /{entity_type}/{entity_id}/follow/alerts": notShowAddressable,
	"GET /{entity_type}/{entity_id}/followers":       notShowAddressable,
	"GET /{entity_type}/{entity_id}/followers/list":  notShowAddressable,
	"POST /{entity_type}/{entity_id}/follow":         notShowAddressable,
	"DELETE /{entity_type}/{entity_id}/follow":       notShowAddressable,

	// Admin.
	"POST /admin/shows/{show_id}/approve":                       adminOnly,
	"POST /admin/shows/{show_id}/reject":                        adminOnly,
	"POST /admin/pipeline/enrichment/trigger/{show_id}":         adminOnly,
	"GET /admin/pending-edits/entity/{entity_type}/{entity_id}": adminOnly,

	// Deferred write oracles. Each needs a product call, not a code call.
	"POST /shows/{show_id}/report":                                   writeOracleDeferred,
	"POST /shows/{entity_id}/entity-report":                          writeOracleDeferred,
	"POST /entities/{entity_type}/{entity_id}/tags":                  writeOracleDeferred,
	"DELETE /entities/{entity_type}/{entity_id}/tags/{tag_id}":       writeOracleDeferred,
	"POST /tags/{tag_id}/entities/{entity_type}/{entity_id}/votes":   writeOracleDeferred,
	"DELETE /tags/{tag_id}/entities/{entity_type}/{entity_id}/votes": writeOracleDeferred,
}

// showAddressablePathPattern matches the path shapes a show id can travel in.
//
// Two families, and the second is why this is a regexp rather than a prefix
// check: a show reaches half these routes through a POLYMORPHIC {entity_type}
// segment, where nothing in the path says "show" at all. A guard that only
// looked for "/shows/" would have missed every comment, tag and collection route
// this ticket had to close.
var showAddressablePathPattern = regexp.MustCompile(
	`/shows/\{|\{entity_type\}|/saved-shows/\{`,
)

func TestEveryShowAddressableRouteHasADisposition(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	var undecided []string
	for method, patterns := range routes {
		for _, pattern := range patterns {
			if !showAddressablePathPattern.MatchString(pattern) {
				continue
			}
			key := method + " " + pattern
			if _, ok := showAddressableRoutes[key]; !ok {
				undecided = append(undecided, key)
			}
		}
	}

	if len(undecided) > 0 {
		sort.Strings(undecided)
		t.Errorf("%d show-addressable route(s) have no recorded position on the show "+
			"visibility rule:\n  %v\n\nEvery route that can name a show has to decide whether a "+
			"caller who cannot see that show may reach it. Add each to showAddressableRoutes with "+
			"the disposition that is TRUE of it, and if that disposition is `gated`, add the "+
			"behaviour assertion to the matching suite in this package as well: "+
			"show_subresource_visibility_test.go for routes addressed by a show id, "+
			"comment_subscription_visibility_test.go for the self-scoped /me/… family.",
			len(undecided), undecided)
	}
}

// The inventory is only a guard while it describes routes that exist. A stale
// entry is a claim about nothing, and it hides the removal of a route the
// behaviour test still thinks it is covering.
func TestShowRouteInventoryHasNoStaleEntries(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	registered := map[string]bool{}
	for method, patterns := range routes {
		for _, pattern := range patterns {
			registered[method+" "+pattern] = true
		}
	}

	var stale []string
	for key := range showAddressableRoutes {
		if !registered[key] {
			stale = append(stale, key)
		}
	}

	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d entr(ies) in showAddressableRoutes name routes that are not registered:\n  %v\n\n"+
			"Either the route was renamed or removed, or it is registered only under a build "+
			"condition. Remove the entry, or correct it to the pattern chi actually reports.",
			len(stale), stale)
	}
}

// The dev-only markdown export is registered only when ENVIRONMENT is
// "development" (routes/shows.go), so it cannot appear in the walk above and
// would otherwise sit outside the inventory entirely — which is exactly how it
// stayed ungated and unauthenticated until this ticket.
//
// Asserted as an absence rather than left unmentioned: if someone makes the
// registration unconditional, the route joins the walk, has no disposition, and
// the inventory test above fires. This test states that the omission is
// deliberate and names where the route's own gate is proven
// (handlers/catalog: TestExportShowHandler_GatedShowAnswersLikeMissing).
func TestExportRouteIsAbsentOutsideDevelopment(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	if got := matching(routes, http.MethodGet, "/shows/{}/export"); len(got) != 0 {
		t.Errorf("GET /shows/{show_id}/export is registered in a non-development test "+
			"environment: %v — it streams a show's whole contents and carries no auth "+
			"middleware, so its registration must stay conditional", got)
	}
}

// The follow family's safety rests entirely on shows being absent from the
// follow entity-type allowlist, so that absence is asserted rather than assumed.
// Adding a show key to validFollowEntityTypes would silently make six routes in
// the inventory above show-addressable with a disposition that says they are
// not; this test is what breaks first.
func TestFollowRoutesRefuseShows(t *testing.T) {
	router := newTestRouter(t)

	for _, path := range []string{
		"/shows/1/followers",
		"/shows/1/followers/list",
		"/show/1/followers",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// 400 is the allowlist refusing the entity type; 404/405 is the route not
		// matching at all. Either means no show id reached a follow lookup. A 2xx
		// or a 500 would mean it did.
		if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d, want a refusal — shows are not a followable entity "+
				"type, and six inventory entries above are recorded safe on that basis; "+
				"body: %s", path, w.Code, w.Body.String())
		}
	}
}
