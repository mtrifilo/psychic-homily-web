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
// purpose: the generator fails closed and keeps serving its last good document,
// which only works if a failure here actually looks like a failure. See
// contracts.SitemapEntry for the incident this guards against.
func (h *SitemapHandler) GetSitemapEntriesHandler(ctx context.Context, _ *GetSitemapEntriesRequest) (*GetSitemapEntriesResponse, error) {
	entries, err := h.sitemapService.Entries()
	if err != nil {
		logger.FromContext(ctx).Error("sitemap_entries_failed",
			"error", err.Error(),
			"request_id", logger.GetRequestID(ctx),
		)
		return nil, huma.Error500InternalServerError("Failed to collect sitemap entries", err)
	}

	// Counts come from the contract type so a newly added family is logged
	// without anyone having to remember this line exists.
	attrs := make([]any, 0, len(entries.Counts())*2)
	for family, n := range entries.Counts() {
		attrs = append(attrs, family, n)
	}
	logger.FromContext(ctx).Debug("sitemap_entries_success", attrs...)

	return &GetSitemapEntriesResponse{Body: *entries}, nil
}
