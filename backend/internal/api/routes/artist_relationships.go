package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
	"psychic-homily-backend/internal/api/middleware"
)

// setupArtistRelationshipRoutes configures artist relationship and similar artist endpoints.
func setupArtistRelationshipRoutes(rc RouteContext) {
	relHandler := catalogh.NewArtistRelationshipHandler(rc.SC.ArtistRelationship, rc.SC.AuditLog)

	// Public: get related artists with optional auth (for user's votes)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/artists/{artist_id}/related", relHandler.GetRelatedArtistsHandler)
	huma.Get(optionalAuthGroup, "/artists/{artist_id}/graph", relHandler.GetArtistGraphHandler)
	huma.Get(optionalAuthGroup, "/artists/{artist_id}/bill-composition", relHandler.GetArtistBillCompositionHandler)

	// Protected: create relationships and vote
	huma.Post(rc.Protected, "/artists/relationships", relHandler.CreateRelationshipHandler)
	huma.Post(rc.Protected, "/artists/relationships/{source_id}/{target_id}/vote", relHandler.VoteHandler)
	huma.Delete(rc.Protected, "/artists/relationships/{source_id}/{target_id}/vote", relHandler.RemoveVoteHandler)

	// Admin: delete relationships
	huma.Delete(rc.Protected, "/artists/relationships/{source_id}/{target_id}", relHandler.DeleteRelationshipHandler)

	// Admin: trigger relationship derivation
	huma.Post(rc.Protected, "/admin/artist-relationships/derive", relHandler.DeriveRelationshipsHandler)
}
