package contracts

import "time"

// SitemapEntry is everything the sitemap generator needs about one indexable
// URL: the slug that builds the path, and the timestamp that fills <lastmod>.
//
// Deliberately nothing else, and this is the canonical record of why — the
// service, handler and generator all reference this doc rather than restating
// it. The generator used to read these two columns off the public list
// endpoints. `GET /shows` alone answers in 4.6 MB and 15.5 s, because it
// preloads every venue and artist on every show, and the generator fetched it
// behind a 10 s abort. The abort was caught and turned into an empty slice, so
// the route rendered *successfully* with an entire entity family missing: 114
// show URLs served against 3,498 in the database, with no failure signal
// anywhere.
//
// Two invariants fall out of that, upheld across every layer:
//
//   - Fail atomically. A response missing a family is indistinguishable from
//     that family being legitimately empty, so a partial result must never be
//     published — it silently drops thousands of URLs out of the index.
//   - Empty families serialise as [], never null. The generator iterates each
//     list directly; a null would need a nil check that is easy to omit and
//     silent when forgotten.
type SitemapEntry struct {
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SitemapEntries groups indexable entries by URL family. Families are separate
// fields rather than one tagged list because the generator maps each to a
// different path prefix, changeFrequency and priority — keeping them distinct
// is what lets the frontend's mapping be type-checked against this schema.
type SitemapEntries struct {
	Shows   []SitemapEntry `json:"shows"`
	Artists []SitemapEntry `json:"artists"`
	Venues  []SitemapEntry `json:"venues"`
}

// Counts returns per-family entry counts keyed by JSON field name, for logging
// and for the freshness checks that compare served URL counts against the
// catalogue.
//
// It lives here, beside the struct, so that adding a family means editing one
// file rather than remembering to update a log statement in another package —
// an omission that would leave the new family silently uncounted.
func (e SitemapEntries) Counts() map[string]int {
	return map[string]int{
		"shows":   len(e.Shows),
		"artists": len(e.Artists),
		"venues":  len(e.Venues),
	}
}
