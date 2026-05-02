package routes

import (
	"github.com/danielgtaylor/huma/v2"

	pipelineh "psychic-homily-backend/internal/api/handlers/pipeline"
)

// setupPipelineRoutes configures AI extraction pipeline admin endpoints.
// Admin check is performed inside handlers, JWT auth is required via protected group.
func setupPipelineRoutes(rc RouteContext) {
	pipelineHandler := pipelineh.NewPipelineHandler(rc.SC.Pipeline, rc.SC.VenueSourceConfig, rc.SC.Enrichment)

	huma.Post(rc.Protected, "/admin/pipeline/extract/{venue_id}", pipelineHandler.ExtractVenueHandler)
	huma.Get(rc.Protected, "/admin/pipeline/imports", pipelineHandler.GetImportHistoryHandler)
	huma.Get(rc.Protected, "/admin/pipeline/venues", pipelineHandler.ListPipelineVenuesHandler)
	huma.Get(rc.Protected, "/admin/pipeline/venues/{venue_id}/stats", pipelineHandler.VenueRejectionStatsHandler)
	huma.Patch(rc.Protected, "/admin/pipeline/venues/{venue_id}/notes", pipelineHandler.UpdateExtractionNotesHandler)
	huma.Put(rc.Protected, "/admin/pipeline/venues/{venue_id}/config", pipelineHandler.UpdateVenueConfigHandler)
	huma.Get(rc.Protected, "/admin/pipeline/venues/{venue_id}/runs", pipelineHandler.GetVenueRunsHandler)
	huma.Post(rc.Protected, "/admin/pipeline/venues/{venue_id}/reset-render-method", pipelineHandler.ResetRenderMethodHandler)
	huma.Get(rc.Protected, "/admin/pipeline/enrichment/status", pipelineHandler.EnrichmentStatusHandler)
	huma.Post(rc.Protected, "/admin/pipeline/enrichment/trigger/{show_id}", pipelineHandler.TriggerEnrichmentHandler)
}
