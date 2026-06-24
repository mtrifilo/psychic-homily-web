package routes

import (
	"github.com/danielgtaylor/huma/v2"

	aiextractionh "psychic-homily-backend/internal/api/handlers/aiextraction"
)

// setupAIExtractionRoutes registers the per-user rate-limit gate the Next.js
// BFF extract routes call before doing any Anthropic work (PSY-855).
//
// Registered on rc.Protected so the caller is identified by the JWT in the
// forwarded auth_token cookie — the user_id is never taken from the request
// body and cannot be spoofed. Admin bypass is enforced inside the handler
// (IsAdmin on the authenticated user, PSY-345), not at the route level, because
// the same endpoint serves both admins (always allowed) and regular users
// (counted).
func setupAIExtractionRoutes(rc RouteContext) {
	h := aiextractionh.NewThrottleHandler(rc.SC.AIExtractionThrottle)

	huma.Post(rc.Protected, "/ai-extraction/throttle", h.CheckThrottleHandler)
}
