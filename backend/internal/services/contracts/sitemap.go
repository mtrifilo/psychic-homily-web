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
// not be collapsed into one. ONLY THE FIRST IS FIXED HERE:
//
//  1. The fetch failed open. The generator read these two columns off the
//     public list endpoints. `GET /shows` answers in 4.6 MB and 15.5 s because
//     it preloads every venue and artist on every show, which blew the
//     generator's 10 s abort budget; the abort was caught and turned into an
//     empty slice. Shows therefore rendered as ZERO URLs, with no failure
//     signal anywhere. Fixed by this projection endpoint plus a generator that
//     throws instead of substituting an empty list.
//
//  2. The served document was stale — separately, and by a mechanism that is
//     STILL NOT ESTABLISHED, and NOT FIXED by this change. Note the shape: 114
//     stale show URLs is not what cause 1 produces (that produces zero), and
//     artists answered in 0.2 s so they never aborted at all, yet were also
//     stale — under the same per-fetch `revalidate` the generator still relies
//     on today. Whatever held that old document is not explained by the
//     fail-open and is not addressed here. A route-level `revalidate` was tried
//     as the fix, measured to change nothing, and removed rather than left in
//     as a placebo. **PSY-1644 owns this**, and carries a measured lead: the
//     sitemap route prerenders with a one-YEAR expire, so a document can go on
//     being served for that long while every revalidation fails — which is what
//     the old 15.5 s fetch behind a 10 s abort guaranteed. See the module header
//     of frontend/app/sitemap.ts for the route-mode measurements; do NOT
//     describe that route as simply "dynamic", it is conditional on whether the
//     build-time fetch succeeded.
//
//     Do not write a confident story about this without measuring first, and do
//     not read a healthy sitemap immediately after deploy as evidence it is
//     solved: this change moves the fetch to a new URL, so the cache key is
//     cold and the first document will look correct regardless. PSY-1629
//     (freshness monitoring) is what would actually catch a recurrence.
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
//
// # Composite slugs (scene-weeks, venue-years)
//
// Some families address a SLICE of an entity rather than a row, so they have
// more than one dynamic path segment and no backing table with a slug column.
// Their SitemapEntry.Slug is the whole path tail under the family's prefix, so
// the generator can keep one prefix map for every family:
//
//   - scene_weeks: "{scene-slug}/{iso-week}" (e.g. "phoenix-az/2026-W31").
//     UpdatedAt is MAX(show.updated_at) among approved shows in that week.
//   - venue_years: "{venue-slug}/shows/{year}" (e.g. "the-van-buren/shows/2025").
//     UpdatedAt is MAX(show.updated_at) among that venue's approved past shows
//     in that venue-local year.
//
// Anything mapping a URL back to a family has to disambiguate families sharing
// a prefix by segment count — see FAMILY_URL_PREFIXES in the frontend.
type SitemapEntry struct {
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SitemapEntries groups indexable entries by URL family. Families are separate
// fields rather than one tagged list because the generator maps each to a
// different path prefix, changeFrequency and priority — keeping them distinct
// is what lets the frontend's mapping be type-checked against this schema.
//
// Public collections are deliberately absent (PSY-1622): they are user-created
// and can flip private, so indexing them risks 404s in the index.
type SitemapEntries struct {
	Shows      []SitemapEntry `json:"shows"`
	Artists    []SitemapEntry `json:"artists"`
	Venues     []SitemapEntry `json:"venues"`
	VenueYears []SitemapEntry `json:"venue_years"`
	Scenes     []SitemapEntry `json:"scenes"`
	SceneWeeks []SitemapEntry `json:"scene_weeks"`
	Labels     []SitemapEntry `json:"labels"`
	Releases   []SitemapEntry `json:"releases"`
	Festivals  []SitemapEntry `json:"festivals"`
	Tags       []SitemapEntry `json:"tags"`
}

// Counts returns per-family entry counts keyed by JSON field name, for logging.
//
// HAND-MAINTAINED: adding a field to SitemapEntries does not update this map,
// and the compiler will not complain. TestSitemapEntriesCountsCoversEveryFamily
// reflects over the struct and fails if a family is missing here — that test,
// not this comment, is what actually keeps the two in sync.
func (e SitemapEntries) Counts() map[string]int {
	return map[string]int{
		"shows":       len(e.Shows),
		"artists":     len(e.Artists),
		"venues":      len(e.Venues),
		"venue_years": len(e.VenueYears),
		"scenes":      len(e.Scenes),
		"scene_weeks": len(e.SceneWeeks),
		"labels":      len(e.Labels),
		"releases":    len(e.Releases),
		"festivals":   len(e.Festivals),
		"tags":        len(e.Tags),
	}
}
