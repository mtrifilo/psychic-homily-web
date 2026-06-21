package routes

import (
	"github.com/danielgtaylor/huma/v2"

	sourcesh "psychic-homily-backend/internal/api/handlers/sources"
)

// setupSourceRoutes configures the source-config registry admin endpoints
// (PSY-1164). rc.Admin enforces auth + IsAdmin upstream (PSY-423), so the
// handlers don't repeat the admin check.
func setupSourceRoutes(rc RouteContext) {
	h := sourcesh.NewSourceHandler(rc.SC.SourceConfig)

	huma.Get(rc.Admin, "/admin/sources", h.ListStaleHandler)
	huma.Put(rc.Admin, "/admin/sources", h.RegisterHandler)
	huma.Post(rc.Admin, "/admin/sources/refresh", h.RefreshHandler)
	huma.Post(rc.Admin, "/admin/sources/failure", h.FailHandler)
}
