package routes

import (
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
	"psychic-homily-backend/internal/api/middleware"
)

// setupTagRoutes configures tag, entity tagging, and tag voting endpoints.
// Public endpoints for browsing tags. Optional auth for entity tags (user's vote).
// Protected endpoints for tagging and voting. Admin endpoints for tag CRUD and aliases.
func setupTagRoutes(rc RouteContext) {
	tagHandler := catalogh.NewTagHandler(rc.SC.Tag, rc.SC.AuditLog)

	// Public tag endpoints
	huma.Get(rc.API, "/tags", tagHandler.ListTagsHandler)
	huma.Get(rc.API, "/tags/search", tagHandler.SearchTagsHandler)
	huma.Get(rc.API, "/tags/{tag_id}", tagHandler.GetTagHandler)
	huma.Get(rc.API, "/tags/{tag_id}/detail", tagHandler.GetTagDetailHandler)
	huma.Get(rc.API, "/tags/{tag_id}/aliases", tagHandler.ListAliasesHandler)
	huma.Get(rc.API, "/tags/{tag_id}/entities", tagHandler.ListTagEntitiesHandler)

	// Entity tags with optional auth (includes user's vote if authenticated)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/entities/{entity_type}/{entity_id}/tags", tagHandler.ListEntityTagsHandler)

	// Rate-limited tag creation: 20 requests per hour per IP.
	// Admins bypass the limit (PSY-345) so bulk-tagging sessions don't
	// collide with a limiter meant for anonymous/IP-level abuse.
	rc.Router.Group(func(r chi.Router) {
		r.Use(middleware.SkipRateLimitForAdmin(rc.SC.JWT, httprate.Limit(
			middleware.TagCreateRequestsPerHour,
			time.Hour,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		)))
		tagCreateAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Tag Create", "1.0.0"))
		tagCreateAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		tagCreateAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(tagCreateAPI, "/entities/{entity_type}/{entity_id}/tags", tagHandler.AddTagToEntityHandler)
	})

	// Protected: remove tag (no additional rate limiting needed)
	huma.Delete(rc.Protected, "/entities/{entity_type}/{entity_id}/tags/{tag_id}", tagHandler.RemoveTagFromEntityHandler)

	// Rate-limited tag voting: 30 requests per minute per IP.
	// Admins bypass the limit (PSY-345) for the same reason as tag creation.
	rc.Router.Group(func(r chi.Router) {
		r.Use(middleware.SkipRateLimitForAdmin(rc.SC.JWT, httprate.Limit(
			middleware.TagVoteRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		)))
		tagVoteAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Tag Vote", "1.0.0"))
		tagVoteAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		tagVoteAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(tagVoteAPI, "/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", tagHandler.VoteTagHandler)
		huma.Delete(tagVoteAPI, "/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", tagHandler.RemoveTagVoteHandler)
	})

	// Admin: tag CRUD and alias management
	huma.Post(rc.Protected, "/tags", tagHandler.CreateTagHandler)
	huma.Put(rc.Protected, "/tags/{tag_id}", tagHandler.UpdateTagHandler)
	huma.Delete(rc.Protected, "/tags/{tag_id}", tagHandler.DeleteTagHandler)
	huma.Post(rc.Protected, "/tags/{tag_id}/aliases", tagHandler.CreateAliasHandler)
	huma.Delete(rc.Protected, "/tags/{tag_id}/aliases/{alias_id}", tagHandler.DeleteAliasHandler)
	// Admin: global alias listing + bulk CSV/JSON import (PSY-307).
	huma.Get(rc.Protected, "/admin/tags/aliases", tagHandler.ListAllAliasesHandler)
	huma.Post(rc.Protected, "/admin/tags/aliases/bulk", tagHandler.BulkImportAliasesHandler)
	// Admin: merge tags (PSY-306).
	huma.Get(rc.Protected, "/admin/tags/{source_id}/merge-preview", tagHandler.MergeTagsPreviewHandler)
	huma.Post(rc.Protected, "/admin/tags/{source_id}/merge", tagHandler.MergeTagsHandler)
	// Admin: low-quality tag review queue (PSY-310).
	huma.Get(rc.Protected, "/admin/tags/low-quality", tagHandler.ListLowQualityTagsHandler)
	huma.Post(rc.Protected, "/admin/tags/{tag_id}/snooze", tagHandler.SnoozeTagHandler)
	// Admin: bulk action on low-quality queue (PSY-487).
	huma.Post(rc.Protected, "/admin/tags/low-quality/bulk-action", tagHandler.BulkLowQualityTagsHandler)
	// Admin: genre-hierarchy editor (PSY-311).
	huma.Get(rc.Protected, "/admin/tags/hierarchy", tagHandler.GetGenreHierarchyHandler)
	huma.Patch(rc.Protected, "/admin/tags/{tag_id}/parent", tagHandler.SetTagParentHandler)
}
