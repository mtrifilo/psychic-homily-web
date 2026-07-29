package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

type sitemapService interface {
	Entries(ctx context.Context) (*contracts.SitemapEntries, error)
}

type SitemapHandler struct {
	sitemapService sitemapService
}

func NewSitemapHandler(sitemapService sitemapService) *SitemapHandler {
	return &SitemapHandler{sitemapService: sitemapService}
}

type GetSitemapEntriesRequest struct{}

type GetSitemapEntriesResponse struct {
	// The body is identical for every caller and its only consumer refetches
	// hourly, so let shared caches absorb the load — this is three unbounded
	// projections behind an unauthenticated route.
	CacheControl string `header:"Cache-Control"`
	Body         contracts.SitemapEntries
}

// GetSitemapEntriesHandler handles GET /sitemap/entries.
//
// Errors are surfaced as 500 rather than degraded into an empty body on
// purpose: the generator fails closed, which only works if a failure here
// actually looks like a failure. See contracts.SitemapEntry for the incident.
func (h *SitemapHandler) GetSitemapEntriesHandler(ctx context.Context, _ *GetSitemapEntriesRequest) (*GetSitemapEntriesResponse, error) {
	entries, err := h.sitemapService.Entries(ctx)
	if err != nil {
		logger.FromContext(ctx).Error("sitemap_entries_failed",
			"error", err.Error(),
			"request_id", logger.GetRequestID(ctx),
		)
		// The error is deliberately NOT passed to huma: it would be serialised
		// into the response body, handing an unauthenticated caller raw driver
		// text (table names, SQLSTATE codes). It is already logged above with
		// the request ID, which is how to correlate a report to a cause.
		return nil, huma.Error500InternalServerError("Failed to collect sitemap entries")
	}

	// Counts come from the contract type, kept honest by a reflection test, so
	// a newly added family is logged without anyone remembering this line.
	counts := entries.Counts()
	attrs := make([]any, 0, len(counts)*2)
	for family, n := range counts {
		attrs = append(attrs, family, n)
	}
	logger.FromContext(ctx).Debug("sitemap_entries_success", attrs...)

	return &GetSitemapEntriesResponse{
		CacheControl: "public, max-age=300, stale-while-revalidate=3600",
		Body:         *entries,
	}, nil
}
