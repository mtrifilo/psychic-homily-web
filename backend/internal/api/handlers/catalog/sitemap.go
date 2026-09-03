package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

type sitemapService interface {
	Entries(ctx context.Context, family string) (*contracts.SitemapEntries, error)
}

type SitemapHandler struct {
	sitemapService sitemapService
}

func NewSitemapHandler(sitemapService sitemapService) *SitemapHandler {
	return &SitemapHandler{sitemapService: sitemapService}
}

// sitemapEntriesCacheControl bounds repeat hits from callers that are NOT the
// sitemap generator.
//
// It buys nothing for the generator itself: that path is already bounded to one
// origin hit per hour by Next's fetch Data Cache, and crawlers never reach this
// endpoint — they fetch /sitemap.xml from the frontend. What it covers is
// everything else that can point at a public URL: the freshness monitor
// (PSY-1629), a second consumer, a misbehaving client. The response is
// byte-identical for every caller of a given family filter, and serving it
// costs unbounded projections, so any intermediary willing to hold it is worth
// having.
//
// Kept short on purpose: a shared cache must not become the reason a changed
// catalogue is invisible. 5 minutes fresh, with no stale-while-revalidate —
// swr=3600 would have allowed a conforming cache to serve up to ~65 minutes
// old, which is ABOVE the generator's hourly cycle and would have made this the
// binding constraint rather than a backstop.
const sitemapEntriesCacheControl = "public, max-age=300"

type GetSitemapEntriesRequest struct {
	// Family, when set, returns only that family's entries (others empty). The
	// frontend shards by family so each Data Cache entry stays under Next's
	// ~1.50 MiB effective budget (PSY-1622). Omit to receive every family.
	//
	// The values after the ten family names are SUB-SHARDS, each returning one
	// bucket of a family that does not fit a single cache entry: `shows-b3` is
	// the shows whose primary key is 3 modulo 8. They ride in this parameter
	// rather than a second one because an unrecognised value here is a 422 the
	// generator degrades on, whereas an unrecognised extra parameter would be
	// silently ignored. See catalog.SitemapService.Entries.
	//
	// This literal cannot be built from catalog.SitemapFamilyValues() — huma
	// reads it off the struct tag — so TestSitemapFamilyEnumMatchesTheService
	// asserts the two agree.
	Family string `query:"family" required:"false" enum:"shows,artists,venues,venue_years,scenes,scene_weeks,labels,releases,festivals,tags,shows-b0,shows-b1,shows-b2,shows-b3,shows-b4,shows-b5,shows-b6,shows-b7,artists-b0,artists-b1,artists-b2,artists-b3,artists-b4,artists-b5,artists-b6,artists-b7,releases-b0,releases-b1,releases-b2,releases-b3,releases-b4,releases-b5,releases-b6,releases-b7" doc:"When set, only this family is populated; others are empty arrays."`
}

type GetSitemapEntriesResponse struct {
	CacheControl string `header:"Cache-Control"`
	Body         contracts.SitemapEntries
}

// GetSitemapEntriesHandler handles GET /sitemap/entries.
//
// Errors are surfaced as 500 rather than degraded into an empty body on
// purpose: the generator fails closed, which only works if a failure here
// actually looks like a failure. See contracts.SitemapEntry for the incident.
// An unknown family is a 400 — the enum on the request usually rejects it
// before the handler runs, but the service also guards.
func (h *SitemapHandler) GetSitemapEntriesHandler(ctx context.Context, req *GetSitemapEntriesRequest) (*GetSitemapEntriesResponse, error) {
	entries, err := h.sitemapService.Entries(ctx, req.Family)
	// nil-without-error cannot happen with *SitemapService, but this handler
	// depends on the interface, and a nil here would panic on the deref below
	// rather than fail closed.
	if err == nil && entries == nil {
		err = fmt.Errorf("sitemap service returned no entries and no error")
	}
	if err != nil {
		logger.FromContext(ctx).Error("sitemap_entries_failed",
			"error", err.Error(),
			"family", req.Family,
			"request_id", logger.GetRequestID(ctx),
		)
		if strings.HasPrefix(err.Error(), "unknown sitemap family") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		// The error is deliberately NOT passed to huma: it would be serialised
		// into the response body, handing an unauthenticated caller raw driver
		// text (table names, SQLSTATE codes). It is already logged above with
		// the request ID, which is how to correlate a report to a cause.
		return nil, huma.Error500InternalServerError("Failed to collect sitemap entries")
	}

	// Counts come from the contract type, kept honest by a reflection test, so
	// a newly added family is logged without anyone remembering this line.
	counts := entries.Counts()
	attrs := make([]any, 0, len(counts)*2+2)
	attrs = append(attrs, "family_filter", req.Family)
	for family, n := range counts {
		attrs = append(attrs, family, n)
	}
	logger.FromContext(ctx).Debug("sitemap_entries_success", attrs...)

	return &GetSitemapEntriesResponse{
		CacheControl: sitemapEntriesCacheControl,
		Body:         *entries,
	}, nil
}
