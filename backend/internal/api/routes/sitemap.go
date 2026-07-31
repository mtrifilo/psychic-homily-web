package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

// setupSitemapRoutes registers the sitemap generator's feed.
//
// Public and unauthenticated: it exposes nothing that is not already reachable
// by crawling the site, and the generator runs without credentials.
func setupSitemapRoutes(rc RouteContext) {
	sitemapHandler := catalogh.NewSitemapHandler(rc.SC.Sitemap)

	huma.Get(rc.API, "/sitemap/entries", sitemapHandler.GetSitemapEntriesHandler)
}
