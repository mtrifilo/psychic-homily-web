package routes

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every route that can be addressed by a COLLECTION must have a decided
// position on the collection detail route's visibility rule.
//
// The show inventory in show_visibility_route_inventory_test.go is keyed on
// shows and says so: a route reading `gated` there means it calls the
// polymorphic gate, which can be true of shows and false of collections at the
// same time. Path shape keyed on shows cannot see a route that leaks a
// different entity type, and the `/collections/{slug}/...` family never appears
// in it at all. This file is the collection half.
//
// It walks the BUILT router, finds every operation whose path can name a
// collection, and fails unless that exact operation appears below with a
// recorded disposition. The cost of adding a collection-addressable route is one
// line here plus the judgement it forces.
//
// It pins the INVENTORY, not the behaviour. What each gated route answers is
// collection_subscription_visibility_test.go's job and the service suites'.
//
// KNOWN LIMIT: it matches on PATH SHAPE, so it sees only routes that carry a
// collection slug or id in the path. Routes addressed by the CALLER
// (/auth/collections, /auth/collections/contains), by a sub-resource id
// (/comments/{id}), or by a tag (/tags/{id}/entities) reach collections and are
// invisible here; so is every background job. Those are decided in SQL by
// services/shared's spellings, and the spellings-agree suites are what keep them
// in step.

// collectionRouteDisposition records why a collection-addressable operation is
// safe.
type collectionRouteDisposition int

const (
	// collectionGated: the route consults the collection's visibility before
	// answering, and refuses a private collection as a MISSING one.
	//
	// Not-found rather than forbidden across the whole family, including the
	// slug-addressed routes. A slug is derived from the title, so "you may not
	// see X" over a guessable name discloses the name, and the detail route is
	// reachable by a dense integer id as well as by the slug
	// (GetCollectionHandler parses the segment as a uint first), which makes the
	// 403/404 pair walkable.
	collectionGated collectionRouteDisposition = iota
	// collectionCreatorOnly: the route is a write that already refuses anybody
	// but the creator, or a contributor on a collaborative collection. The
	// collaborative branch carries no visibility test of its own, which is the
	// divergence the map's item-write entries name below.
	collectionCreatorOnly
	// collectionSelfScoped: the route answers only about the CALLER's own
	// relationship to the collection and publishes no collection content.
	collectionSelfScoped
	// collectionAdminOnly: registered on the admin group. Admins hold moderation
	// powers over private collections that no non-admin surface grants; see
	// services/shared/collection_visibility.go.
	collectionAdminOnly
	// collectionNotAddressable: the route takes a polymorphic segment but its
	// allowlist does not accept a collection, so no collection can reach it.
	// Recorded rather than omitted so widening the allowlist has to come here.
	collectionNotAddressable
)

// collectionAddressableRoutes is the whole inventory, keyed by "METHOD pattern"
// as chi reports it. A route missing from this map fails the test, and adding
// one here is a claim about it.
var collectionAddressableRoutes = map[string]collectionRouteDisposition{
	// The rule's own route and its read siblings. `/crates/...` is the same
	// handler under the product's other name for the same object, so every
	// entry is duplicated and the two must never diverge.
	"GET /collections/{slug}":       collectionGated,
	"GET /crates/{slug}":            collectionGated,
	"GET /collections/{slug}/graph": collectionGated,
	"GET /crates/{slug}/graph":      collectionGated,
	"GET /collections/{slug}/stats": collectionGated,
	"GET /crates/{slug}/stats":      collectionGated,

	// Polymorphic sub-resources, gated through services/shared's registry.
	"GET /entities/{entity_type}/{entity_id}/comments":               collectionGated,
	"POST /entities/{entity_type}/{entity_id}/comments":              collectionGated,
	"GET /entities/{entity_type}/{entity_id}/tags":                   collectionGated,
	"POST /entities/{entity_type}/{entity_id}/tags":                  collectionGated,
	"DELETE /entities/{entity_type}/{entity_id}/tags/{tag_id}":       collectionGated,
	"POST /tags/{tag_id}/entities/{entity_type}/{entity_id}/votes":   collectionGated,
	"DELETE /tags/{tag_id}/entities/{entity_type}/{entity_id}/votes": collectionGated,
	"POST /entities/{entity_type}/{entity_id}/subscribe":             collectionGated,
	"GET /entities/{entity_type}/{entity_id}/subscribe/status":       collectionGated,
	"POST /entities/{entity_type}/{entity_id}/mark-read":             collectionGated,
	// Deletes the caller's own subscription row and answers the same whether one
	// was there, so it publishes nothing and offers no oracle.
	"DELETE /entities/{entity_type}/{entity_id}/subscribe": collectionSelfScoped,

	// The slug-addressed tag writes. canEditCollectionTags refuses a collection
	// the caller cannot see before it considers `collaborative`, so these answer
	// the same rule as their polymorphic twins above.
	"POST /collections/{slug}/tags":                     collectionGated,
	"POST /crates/{slug}/tags":                          collectionGated,
	"DELETE /collections/{slug}/tags/{tag_id}":          collectionGated,
	"DELETE /crates/{slug}/tags/{tag_id}":               collectionGated,
	"POST /collections/{entity_id}/report":              collectionGated,
	"POST /collections/{slug}/subscribe":                collectionGated,
	"POST /crates/{slug}/subscribe":                     collectionGated,
	"POST /collections/{slug}/like":                     collectionGated,
	"POST /crates/{slug}/like":                          collectionGated,
	"DELETE /collections/{slug}/like":                   collectionGated,
	"DELETE /crates/{slug}/like":                        collectionGated,
	"POST /collections/{slug}/clone":                    collectionGated,
	"POST /crates/{slug}/clone":                         collectionGated,
	"GET /collections/entity/{entity_type}/{entity_id}": collectionGated,
	"GET /crates/entity/{entity_type}/{entity_id}":      collectionGated,

	// Unsubscribing is the caller's own row, like its polymorphic twin.
	"DELETE /collections/{slug}/subscribe": collectionSelfScoped,
	"DELETE /crates/{slug}/subscribe":      collectionSelfScoped,

	// Owner writes. Each refuses a caller who is neither the creator nor, on a
	// collaborative collection, a contributor, and each refuses a collection the
	// caller cannot see before it considers `collaborative`.
	"PUT /collections/{slug}":                    collectionCreatorOnly,
	"PUT /crates/{slug}":                         collectionCreatorOnly,
	"DELETE /collections/{slug}":                 collectionCreatorOnly,
	"DELETE /crates/{slug}":                      collectionCreatorOnly,
	"POST /collections/{slug}/items":             collectionCreatorOnly,
	"POST /crates/{slug}/items":                  collectionCreatorOnly,
	"POST /collections/{slug}/items/bulk":        collectionCreatorOnly,
	"POST /crates/{slug}/items/bulk":             collectionCreatorOnly,
	"PATCH /collections/{slug}/items/{item_id}":  collectionCreatorOnly,
	"PATCH /crates/{slug}/items/{item_id}":       collectionCreatorOnly,
	"DELETE /collections/{slug}/items/{item_id}": collectionCreatorOnly,
	"DELETE /crates/{slug}/items/{item_id}":      collectionCreatorOnly,
	"PUT /collections/{slug}/items/reorder":      collectionCreatorOnly,
	"PUT /crates/{slug}/items/reorder":           collectionCreatorOnly,

	// Admin.
	"PUT /collections/{slug}/feature": collectionAdminOnly,
	"PUT /crates/{slug}/feature":      collectionAdminOnly,

	// Polymorphic routes a collection cannot reach.
	"GET /revisions/{entity_type}/{entity_id}":                  collectionNotAddressable,
	"GET /admin/pending-edits/entity/{entity_type}/{entity_id}": collectionNotAddressable,
	"HEAD /entities/{entity_type}/{entity_id}/exists":           collectionNotAddressable,
	"GET /{entity_type}/{entity_id}/follow/alerts":              collectionNotAddressable,
	"PATCH /{entity_type}/{entity_id}/follow/alerts":            collectionNotAddressable,
	"GET /{entity_type}/{entity_id}/followers":                  collectionNotAddressable,
	"GET /{entity_type}/{entity_id}/followers/list":             collectionNotAddressable,
	"POST /{entity_type}/{entity_id}/follow":                    collectionNotAddressable,
	"DELETE /{entity_type}/{entity_id}/follow":                  collectionNotAddressable,
}

// collectionAddressablePathPattern matches the path shapes a collection can
// travel in: its own slug family, its report route's id, and every polymorphic
// {entity_type} segment.
var collectionAddressablePathPattern = regexp.MustCompile(
	`/collections/\{|/crates/\{|\{entity_type\}`,
)

func TestEveryCollectionAddressableRouteHasADisposition(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	var undecided []string
	for method, patterns := range routes {
		for _, pattern := range patterns {
			if !collectionAddressablePathPattern.MatchString(pattern) {
				continue
			}
			key := method + " " + pattern
			if _, ok := collectionAddressableRoutes[key]; !ok {
				undecided = append(undecided, key)
			}
		}
	}

	if len(undecided) > 0 {
		sort.Strings(undecided)
		t.Errorf("%d collection-addressable route(s) have no recorded position on the "+
			"collection visibility rule:\n  %v\n\nEvery route that can name a collection has to "+
			"decide whether a caller who cannot see that collection may reach it. Add each to "+
			"collectionAddressableRoutes with the disposition that is TRUE of it, and if that "+
			"disposition is `collectionGated`, add the behaviour assertion to "+
			"collection_subscription_visibility_test.go or the owning service suite.",
			len(undecided), undecided)
	}
}

// The inventory is only a guard while it describes routes that exist. A stale
// entry is a claim about nothing, and it hides the removal of a route a
// behaviour test still thinks it is covering.
func TestCollectionRouteInventoryHasNoStaleEntries(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	registered := map[string]bool{}
	for method, patterns := range routes {
		for _, pattern := range patterns {
			registered[method+" "+pattern] = true
		}
	}

	var stale []string
	for key := range collectionAddressableRoutes {
		if !registered[key] {
			stale = append(stale, key)
		}
	}

	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d entr(ies) in collectionAddressableRoutes name routes that are not "+
			"registered:\n  %v\n\nEither the route was renamed or removed, or it is registered "+
			"only under a build condition. Remove the entry, or correct it to the pattern chi "+
			"actually reports.", len(stale), stale)
	}
}

// `/crates/...` is the same handler under the product's other name, so a
// disposition recorded for one and not the other is a claim that two spellings
// of one route behave differently.
func TestCollectionAndCrateRoutesAgree(t *testing.T) {
	// BOTH DIRECTIONS. Walking only the collections spellings would miss a
	// crates-only entry whose collections twin is recorded differently, which is
	// the same drift seen from the other side.
	for key, disposition := range collectionAddressableRoutes {
		var twin string
		switch {
		case strings.Contains(key, "/collections/"):
			twin = strings.Replace(key, "/collections/", "/crates/", 1)
		case strings.Contains(key, "/crates/"):
			twin = strings.Replace(key, "/crates/", "/collections/", 1)
		default:
			continue
		}
		twinDisposition, ok := collectionAddressableRoutes[twin]
		if !ok {
			// Not every route has a twin: the report route is registered under
			// /collections only. A twin that should exist and does not is the
			// stale-entry test's business, not this one's.
			continue
		}
		if twinDisposition != disposition {
			t.Errorf("%q is recorded %v but its twin %q is recorded %v; they are one handler "+
				"and must answer alike", key, disposition, twin, twinDisposition)
		}
	}
}
