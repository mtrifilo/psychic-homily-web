package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

func setupLabelRoutes(rc RouteContext) {
	labelHandler := catalogh.NewLabelHandler(rc.SC.Label, rc.SC.AuditLog, rc.SC.Revision)

	// Public label endpoints
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/labels", labelHandler.ListLabelsHandler)
	huma.Get(rc.API, "/labels/search", labelHandler.SearchLabelsHandler)
	huma.Get(rc.API, "/labels/{label_id}", labelHandler.GetLabelHandler)
	huma.Get(rc.API, "/labels/{label_id}/artists", labelHandler.GetLabelRosterHandler)
	huma.Get(rc.API, "/labels/{label_id}/releases", labelHandler.GetLabelCatalogHandler)

	// Admin-only label endpoints (PSY-423: route-gated by HumaAdminMiddleware).
	// Non-admins must use the pending-edit flow (suggest-edit) which routes
	// through community/pending_entity_edits.
	huma.Post(rc.Admin, "/labels", labelHandler.CreateLabelHandler)
	huma.Put(rc.Admin, "/labels/{label_id}", labelHandler.UpdateLabelHandler)
	huma.Delete(rc.Admin, "/labels/{label_id}", labelHandler.DeleteLabelHandler)
	huma.Post(rc.Admin, "/admin/labels/{label_id}/artists", labelHandler.AddArtistToLabelHandler)
	huma.Post(rc.Admin, "/admin/labels/{label_id}/releases", labelHandler.AddReleaseToLabelHandler)
}
