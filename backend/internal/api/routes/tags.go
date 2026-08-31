package routes

import (
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/httprate"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
	"psychic-homily-backend/internal/api/middleware"
)

// setupTagRoutes configures tag, entity tagging, and tag voting endpoints.
// Public endpoints for browsing tags. Optional auth for entity tags (user's vote).
// Protected endpoints for tagging and voting. Admin endpoints for tag CRUD and aliases.
func setupTagRoutes(rc RouteContext) {
	tagHandler := catalogh.NewTagHandler(rc.SC.Tag, rc.SC.AuditLog, rc.SC.ShowVisibility)

	// Public tag endpoints
	huma.Get(rc.API, "/tags", tagHandler.ListTagsHandler)
	huma.Get(rc.API, "/tags/search", tagHandler.SearchTagsHandler)
	// PSY-995: cross-entity multi-tag intersection. Registered before the
	// `/tags/{tag_id}` wildcard so the literal `intersection` segment is not
	// captured as a tag_id.
	huma.Get(rc.API, "/tags/intersection", tagHandler.GetTagIntersectionHandler)
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
	//
	// PSY-1598: on the MAIN api via a Huma group. Note this group's bypass is
	// SkipRateLimitForAdmin, NOT rateLimitUnlessAPIToken — it exempts admin JWTs
	// as well as phk_ tokens, so the conversion has to preserve two escape
	// hatches, not one.
	tagCreateGroup := huma.NewGroup(rc.API, "")
	tagCreateGroup.UseMiddleware(humaFromHTTP(middleware.SkipRateLimitForAdmin(rc.SC.JWT, httprate.Limit(
		middleware.TagCreateRequestsPerHour,
		time.Hour,
		httprate.WithKeyFuncs(middleware.KeyByClientIP),
		httprate.WithLimitHandler(rateLimitHandler),
	))))
	tagCreateGroup.UseMiddleware(middleware.HumaRequestIDMiddleware)
	tagCreateGroup.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
	huma.Post(tagCreateGroup, "/entities/{entity_type}/{entity_id}/tags", tagHandler.AddTagToEntityHandler)

	// Protected: remove tag (no additional rate limiting needed)
	huma.Delete(rc.Protected, "/entities/{entity_type}/{entity_id}/tags/{tag_id}", tagHandler.RemoveTagFromEntityHandler)

	// Rate-limited tag voting: 30 requests per minute per IP.
	// Admins bypass the limit (PSY-345) for the same reason as tag creation.
	// PSY-1598: on the MAIN api via a Huma group; same two-escape-hatch bypass as
	// tag creation above. Both the POST and the DELETE share this one group, so
	// they share one limiter budget exactly as they did under the chi group.
	tagVoteGroup := huma.NewGroup(rc.API, "")
	tagVoteGroup.UseMiddleware(humaFromHTTP(middleware.SkipRateLimitForAdmin(rc.SC.JWT, httprate.Limit(
		middleware.TagVoteRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(middleware.KeyByClientIP),
		httprate.WithLimitHandler(rateLimitHandler),
	))))
	tagVoteGroup.UseMiddleware(middleware.HumaRequestIDMiddleware)
	tagVoteGroup.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
	huma.Post(tagVoteGroup, "/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", tagHandler.VoteTagHandler)
	huma.Delete(tagVoteGroup, "/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", tagHandler.RemoveTagVoteHandler)

	// Admin: tag CRUD and alias management (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Post(rc.Admin, "/tags", tagHandler.CreateTagHandler)
	huma.Put(rc.Admin, "/tags/{tag_id}", tagHandler.UpdateTagHandler)
	huma.Delete(rc.Admin, "/tags/{tag_id}", tagHandler.DeleteTagHandler)
	huma.Post(rc.Admin, "/tags/{tag_id}/aliases", tagHandler.CreateAliasHandler)
	huma.Delete(rc.Admin, "/tags/{tag_id}/aliases/{alias_id}", tagHandler.DeleteAliasHandler)
	// Admin: global alias listing + bulk CSV/JSON import (PSY-307).
	huma.Get(rc.Admin, "/admin/tags/aliases", tagHandler.ListAllAliasesHandler)
	huma.Post(rc.Admin, "/admin/tags/aliases/bulk", tagHandler.BulkImportAliasesHandler)
	// Admin: merge tags (PSY-306).
	huma.Get(rc.Admin, "/admin/tags/{source_id}/merge-preview", tagHandler.MergeTagsPreviewHandler)
	huma.Post(rc.Admin, "/admin/tags/{source_id}/merge", tagHandler.MergeTagsHandler)
	// Admin: low-quality tag review queue (PSY-310).
	huma.Get(rc.Admin, "/admin/tags/low-quality", tagHandler.ListLowQualityTagsHandler)
	huma.Post(rc.Admin, "/admin/tags/{tag_id}/snooze", tagHandler.SnoozeTagHandler)
	// Admin: bulk action on low-quality queue (PSY-487).
	huma.Post(rc.Admin, "/admin/tags/low-quality/bulk-action", tagHandler.BulkLowQualityTagsHandler)
	// Admin: genre-hierarchy editor (PSY-311).
	huma.Get(rc.Admin, "/admin/tags/hierarchy", tagHandler.GetGenreHierarchyHandler)
	huma.Patch(rc.Admin, "/admin/tags/{tag_id}/parent", tagHandler.SetTagParentHandler)
}
