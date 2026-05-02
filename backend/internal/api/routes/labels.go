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

	// Protected label endpoints (admin-only checks inside handlers)
	huma.Post(rc.Protected, "/labels", labelHandler.CreateLabelHandler)
	huma.Put(rc.Protected, "/labels/{label_id}", labelHandler.UpdateLabelHandler)
	huma.Delete(rc.Protected, "/labels/{label_id}", labelHandler.DeleteLabelHandler)
	huma.Post(rc.Protected, "/admin/labels/{label_id}/artists", labelHandler.AddArtistToLabelHandler)
	huma.Post(rc.Protected, "/admin/labels/{label_id}/releases", labelHandler.AddReleaseToLabelHandler)
}
