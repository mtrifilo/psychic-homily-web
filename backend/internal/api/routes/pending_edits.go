package routes

import (
	"github.com/danielgtaylor/huma/v2"

	adminh "psychic-homily-backend/internal/api/handlers/admin"
)

// setupPendingEditRoutes configures pending entity edit endpoints.
// Protected endpoints for suggesting edits and managing own edits.
// Admin endpoints for reviewing, approving, and rejecting edits.
func setupPendingEditRoutes(rc RouteContext) {
	pendingEditHandler := adminh.NewPendingEditHandler(rc.SC.PendingEdit, rc.SC.AuditLog)

	// Protected: suggest edits (creates pending or auto-applies for trusted users)
	huma.Put(rc.Protected, "/artists/{entity_id}/suggest-edit", pendingEditHandler.SuggestArtistEditHandler)
	huma.Put(rc.Protected, "/venues/{entity_id}/suggest-edit", pendingEditHandler.SuggestVenueEditHandler)
	huma.Put(rc.Protected, "/festivals/{entity_id}/suggest-edit", pendingEditHandler.SuggestFestivalEditHandler)
	huma.Put(rc.Protected, "/releases/{entity_id}/suggest-edit", pendingEditHandler.SuggestReleaseEditHandler)
	huma.Put(rc.Protected, "/labels/{entity_id}/suggest-edit", pendingEditHandler.SuggestLabelEditHandler)

	// Protected: user's own pending edits
	huma.Get(rc.Protected, "/my/pending-edits", pendingEditHandler.GetMyPendingEditsHandler)
	huma.Delete(rc.Protected, "/my/pending-edits/{edit_id}", pendingEditHandler.CancelMyPendingEditHandler)

	// Admin: review queue
	huma.Get(rc.Protected, "/admin/pending-edits", pendingEditHandler.AdminListPendingEditsHandler)
	huma.Get(rc.Protected, "/admin/pending-edits/{edit_id}", pendingEditHandler.AdminGetPendingEditHandler)
	huma.Post(rc.Protected, "/admin/pending-edits/{edit_id}/approve", pendingEditHandler.AdminApprovePendingEditHandler)
	huma.Post(rc.Protected, "/admin/pending-edits/{edit_id}/reject", pendingEditHandler.AdminRejectPendingEditHandler)
	huma.Get(rc.Protected, "/admin/pending-edits/entity/{entity_type}/{entity_id}", pendingEditHandler.AdminGetEntityPendingEditsHandler)
}
