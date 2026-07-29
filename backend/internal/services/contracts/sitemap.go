package contracts

import "time"

// SitemapEntry is everything the sitemap generator needs about one indexable
// URL: the slug that builds the path, and the timestamp that fills <lastmod>.
//
// Deliberately nothing else. The generator used to read these two columns off
// the full public list endpoints — /shows alone answers in 4.6 MB and 15.5 s
// because it preloads every venue and artist on every show — which exceeded the
// generator's fetch budget and silently produced a sitemap with no shows in it.
type SitemapEntry struct {
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SitemapEntries groups indexable entries by URL family. Families are separate
// fields rather than one tagged list because the generator maps each to a
// different path prefix and a different changeFrequency.
type SitemapEntries struct {
	Shows   []SitemapEntry `json:"shows"`
	Artists []SitemapEntry `json:"artists"`
	Venues  []SitemapEntry `json:"venues"`
}
