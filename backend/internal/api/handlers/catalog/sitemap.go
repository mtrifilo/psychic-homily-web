package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

type sitemapService interface {
	Entries() (*contracts.SitemapEntries, error)
}

type SitemapHandler struct {
	sitemapService sitemapService
}

func NewSitemapHandler(sitemapService sitemapService) *SitemapHandler {
	return &SitemapHandler{sitemapService: sitemapService}
}

type GetSitemapEntriesRequest struct{}

type GetSitemapEntriesResponse struct {
	Body contracts.SitemapEntries
}

// GetSitemapEntriesHandler handles GET /sitemap/entries.
//
// Errors are surfaced as 500 rather than degraded into an empty body on
// purpose. The consumer is the sitemap generator, which now fails closed and
// keeps serving its last good document — an outcome that only works if a
// failure here actually looks like a failure.
func (h *SitemapHandler) GetSitemapEntriesHandler(ctx context.Context, _ *GetSitemapEntriesRequest) (*GetSitemapEntriesResponse, error) {
	entries, err := h.sitemapService.Entries()
	if err != nil {
		logger.FromContext(ctx).Error("sitemap_entries_failed",
			"error", err.Error(),
			"request_id", logger.GetRequestID(ctx),
		)
		return nil, huma.Error500InternalServerError("Failed to collect sitemap entries", err)
	}

	logger.FromContext(ctx).Debug("sitemap_entries_success",
		"shows", len(entries.Shows),
		"artists", len(entries.Artists),
		"venues", len(entries.Venues),
	)

	return &GetSitemapEntriesResponse{Body: *entries}, nil
}
