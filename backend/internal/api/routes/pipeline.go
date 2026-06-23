package routes

import (
	"github.com/danielgtaylor/huma/v2"

	pipelineh "psychic-homily-backend/internal/api/handlers/pipeline"
)

// setupPipelineRoutes configures the enrichment admin endpoints. The legacy
// venue-extraction routes (extract/venues/imports) were removed with the
// extraction pipeline in PSY-1165.
// PSY-423: rc.Admin enforces auth + IsAdmin upstream so handlers don't need
// individual `shared.RequireAdmin(ctx)` calls.
func setupPipelineRoutes(rc RouteContext) {
	pipelineHandler := pipelineh.NewPipelineHandler(rc.SC.Enrichment)

	huma.Get(rc.Admin, "/admin/pipeline/enrichment/status", pipelineHandler.EnrichmentStatusHandler)
	huma.Post(rc.Admin, "/admin/pipeline/enrichment/trigger/{show_id}", pipelineHandler.TriggerEnrichmentHandler)
}
