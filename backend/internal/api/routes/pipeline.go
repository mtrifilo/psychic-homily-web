package routes

import (
	"github.com/danielgtaylor/huma/v2"

	pipelineh "psychic-homily-backend/internal/api/handlers/pipeline"
)

// setupPipelineRoutes configures AI extraction pipeline admin endpoints.
// PSY-423: rc.Admin enforces auth + IsAdmin upstream so handlers don't need
// individual `shared.RequireAdmin(ctx)` calls.
func setupPipelineRoutes(rc RouteContext) {
	pipelineHandler := pipelineh.NewPipelineHandler(rc.SC.Pipeline, rc.SC.VenueSourceConfig, rc.SC.Enrichment)

	huma.Post(rc.Admin, "/admin/pipeline/extract/{venue_id}", pipelineHandler.ExtractVenueHandler)
	huma.Get(rc.Admin, "/admin/pipeline/imports", pipelineHandler.GetImportHistoryHandler)
	huma.Get(rc.Admin, "/admin/pipeline/venues", pipelineHandler.ListPipelineVenuesHandler)
	huma.Get(rc.Admin, "/admin/pipeline/venues/{venue_id}/stats", pipelineHandler.VenueRejectionStatsHandler)
	huma.Patch(rc.Admin, "/admin/pipeline/venues/{venue_id}/notes", pipelineHandler.UpdateExtractionNotesHandler)
	huma.Put(rc.Admin, "/admin/pipeline/venues/{venue_id}/config", pipelineHandler.UpdateVenueConfigHandler)
	huma.Get(rc.Admin, "/admin/pipeline/venues/{venue_id}/runs", pipelineHandler.GetVenueRunsHandler)
	huma.Post(rc.Admin, "/admin/pipeline/venues/{venue_id}/reset-render-method", pipelineHandler.ResetRenderMethodHandler)
	huma.Get(rc.Admin, "/admin/pipeline/enrichment/status", pipelineHandler.EnrichmentStatusHandler)
	huma.Post(rc.Admin, "/admin/pipeline/enrichment/trigger/{show_id}", pipelineHandler.TriggerEnrichmentHandler)
}
