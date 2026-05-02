package routes

import (
	"github.com/danielgtaylor/huma/v2"

	communityh "psychic-homily-backend/internal/api/handlers/community"
	"psychic-homily-backend/internal/api/middleware"
)

// setupCollectionRoutes configures collection endpoints.
// Both /collections/ and /crates/ paths are registered for backward compatibility.
// Public endpoints use optional auth (for private collection access checks).
// CRUD, item management, and subscription endpoints require authentication.
func setupCollectionRoutes(rc RouteContext) {
	collectionHandler := communityh.NewCollectionHandler(rc.SC.Collection, rc.SC.AuditLog)

	// Public collection endpoints with optional auth
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))

	// Canonical /collections/ paths
	huma.Get(optionalAuthGroup, "/collections", collectionHandler.ListCollectionsHandler)
	huma.Get(optionalAuthGroup, "/collections/{slug}", collectionHandler.GetCollectionHandler)
	huma.Get(optionalAuthGroup, "/collections/{slug}/stats", collectionHandler.GetCollectionStatsHandler)

	// Legacy /crates/ paths (backward compat)
	huma.Get(optionalAuthGroup, "/crates", collectionHandler.ListCollectionsHandler)
	huma.Get(optionalAuthGroup, "/crates/{slug}", collectionHandler.GetCollectionHandler)
	huma.Get(optionalAuthGroup, "/crates/{slug}/stats", collectionHandler.GetCollectionStatsHandler)

	// Protected collection endpoints — canonical /collections/ paths
	huma.Post(rc.Protected, "/collections", collectionHandler.CreateCollectionHandler)
	huma.Put(rc.Protected, "/collections/{slug}", collectionHandler.UpdateCollectionHandler)
	huma.Delete(rc.Protected, "/collections/{slug}", collectionHandler.DeleteCollectionHandler)
	// Clone/fork (PSY-351). Auth required, no trust-tier gate.
	huma.Post(rc.Protected, "/collections/{slug}/clone", collectionHandler.CloneCollectionHandler)

	// Protected collection endpoints — legacy /crates/ paths (backward compat)
	huma.Post(rc.Protected, "/crates", collectionHandler.CreateCollectionHandler)
	huma.Put(rc.Protected, "/crates/{slug}", collectionHandler.UpdateCollectionHandler)
	huma.Delete(rc.Protected, "/crates/{slug}", collectionHandler.DeleteCollectionHandler)
	huma.Post(rc.Protected, "/crates/{slug}/clone", collectionHandler.CloneCollectionHandler)

	// Collection item management — canonical /collections/ paths
	huma.Post(rc.Protected, "/collections/{slug}/items", collectionHandler.AddItemHandler)
	huma.Patch(rc.Protected, "/collections/{slug}/items/{item_id}", collectionHandler.UpdateItemHandler)
	huma.Delete(rc.Protected, "/collections/{slug}/items/{item_id}", collectionHandler.RemoveItemHandler)
	huma.Put(rc.Protected, "/collections/{slug}/items/reorder", collectionHandler.ReorderItemsHandler)

	// Collection item management — legacy /crates/ paths (backward compat)
	huma.Post(rc.Protected, "/crates/{slug}/items", collectionHandler.AddItemHandler)
	huma.Patch(rc.Protected, "/crates/{slug}/items/{item_id}", collectionHandler.UpdateItemHandler)
	huma.Delete(rc.Protected, "/crates/{slug}/items/{item_id}", collectionHandler.RemoveItemHandler)
	huma.Put(rc.Protected, "/crates/{slug}/items/reorder", collectionHandler.ReorderItemsHandler)

	// Collection subscription — canonical /collections/ paths
	huma.Post(rc.Protected, "/collections/{slug}/subscribe", collectionHandler.SubscribeHandler)
	huma.Delete(rc.Protected, "/collections/{slug}/subscribe", collectionHandler.UnsubscribeHandler)

	// Collection subscription — legacy /crates/ paths (backward compat)
	huma.Post(rc.Protected, "/crates/{slug}/subscribe", collectionHandler.SubscribeHandler)
	huma.Delete(rc.Protected, "/crates/{slug}/subscribe", collectionHandler.UnsubscribeHandler)

	// PSY-352: collection like/unlike. Idempotent — POST creates or no-ops if
	// already liked, DELETE removes or no-ops if not liked. Returns the
	// post-mutation aggregate count + caller's like state.
	collectionLikeHandler := communityh.NewCollectionLikeHandler(rc.SC.Collection)
	huma.Post(rc.Protected, "/collections/{slug}/like", collectionLikeHandler.LikeCollectionHandler)
	huma.Delete(rc.Protected, "/collections/{slug}/like", collectionLikeHandler.UnlikeCollectionHandler)
	huma.Post(rc.Protected, "/crates/{slug}/like", collectionLikeHandler.LikeCollectionHandler)
	huma.Delete(rc.Protected, "/crates/{slug}/like", collectionLikeHandler.UnlikeCollectionHandler)

	// PSY-354: collection tag management. Same edit-access rule as
	// AddItem (creator OR collaborative-and-authenticated). Both
	// /collections/ and /crates/ paths registered for backward compat.
	huma.Post(rc.Protected, "/collections/{slug}/tags", collectionHandler.AddCollectionTagHandler)
	huma.Delete(rc.Protected, "/collections/{slug}/tags/{tag_id}", collectionHandler.RemoveCollectionTagHandler)
	huma.Post(rc.Protected, "/crates/{slug}/tags", collectionHandler.AddCollectionTagHandler)
	huma.Delete(rc.Protected, "/crates/{slug}/tags/{tag_id}", collectionHandler.RemoveCollectionTagHandler)

	// Admin: feature/unfeature collections — canonical /collections/ paths
	// (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Put(rc.Admin, "/collections/{slug}/feature", collectionHandler.SetFeaturedHandler)

	// Admin: feature/unfeature collections — legacy /crates/ paths (backward compat)
	huma.Put(rc.Admin, "/crates/{slug}/feature", collectionHandler.SetFeaturedHandler)

	// Entity collections — public, find collections containing a given entity
	huma.Get(optionalAuthGroup, "/collections/entity/{entity_type}/{entity_id}", collectionHandler.GetEntityCollectionsHandler)
	huma.Get(optionalAuthGroup, "/crates/entity/{entity_type}/{entity_id}", collectionHandler.GetEntityCollectionsHandler)

	// User's public collections — public, for profile pages
	huma.Get(optionalAuthGroup, "/users/{username}/collections", collectionHandler.GetUserPublicCollectionsHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/crates", collectionHandler.GetUserPublicCollectionsHandler)

	// User's own collections (created + subscribed)
	huma.Get(rc.Protected, "/auth/collections", collectionHandler.GetUserCollectionsHandler)

	// Legacy user collections path (backward compat)
	huma.Get(rc.Protected, "/auth/crates", collectionHandler.GetUserCollectionsHandler)
}
