package contracts

import "time"

// SitemapEntry is everything the sitemap generator needs about one indexable
// URL: the slug that builds the path, and the timestamp that fills <lastmod>.
//
// Deliberately nothing else, and this is the canonical record of why — the
// service, handler and generator reference this doc rather than restating it.
//
// # The incident (measured 2026-07-28)
//
// The served sitemap listed 114 show URLs against 3,498 slugged shows in the
// database, and 2,520 artists against 3,591. Two DISTINCT problems, which must
// not be collapsed into one — each has its own fix, and deleting either fix
// re-opens half the bug:
//
//  1. The fetch failed open. The generator read these two columns off the
//     public list endpoints. `GET /shows` answers in 4.6 MB and 15.5 s because
//     it preloads every venue and artist on every show, which blew the
//     generator's 10 s abort budget; the abort was caught and turned into an
//     empty slice. Shows therefore rendered as ZERO URLs, with no failure
//     signal anywhere. Fixed by this projection endpoint plus a generator that
//     throws instead of substituting an empty list.
//
//  2. The served document was stale — separately, and by a mechanism that was
//     NOT fully established. Note the shape: 114 stale show URLs is not what
//     cause 1 produces (that produces zero), and artists answered in 0.2 s so
//     they never aborted at all, yet were also stale. Whatever held that old
//     document is not explained by the fail-open. Do not write a confident
//     story about it here without measuring first.
//
// # Invariants this contract upholds
//
//   - Fail atomically. A response missing a family is indistinguishable from
//     that family being legitimately empty, so a partial result must never be
//     published — it silently drops thousands of URLs out of the index. The
//     generator enforces this on its side too: it rejects a body whose family
//     is null or absent rather than coercing it to empty.
//   - Empty families serialise as [], never null. Asserted over real HTTP in
//     the routes integration test.
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

// Counts returns per-family entry counts keyed by JSON field name, for logging.
//
// HAND-MAINTAINED: adding a field to SitemapEntries does not update this map,
// and the compiler will not complain. TestSitemapEntriesCountsCoversEveryFamily
// reflects over the struct and fails if a family is missing here — that test,
// not this comment, is what actually keeps the two in sync.
func (e SitemapEntries) Counts() map[string]int {
	return map[string]int{
		"shows":   len(e.Shows),
		"artists": len(e.Artists),
		"venues":  len(e.Venues),
	}
}
