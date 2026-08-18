/**
 * Canonical, site-wide description string.
 *
 * Consumed by the root layout metadata (the fallback for every page that sets
 * no description of its own), the homepage metadata (plain + Open Graph), and
 * the Organization JSON-LD that search engines and AI assistants read.
 *
 * These four copies are centralized here because they had already drifted: the
 * layout fallback and the Organization schema still described the site as
 * Arizona-scoped long after the homepage copy had been rewritten, so a share
 * card, a search result, and the page itself could each claim a different
 * scope. One constant makes that class of drift impossible rather than merely
 * unlikely.
 */
export const SITE_DESCRIPTION =
  'Show lists for your city, real-time freeform radio playlists, and your music knowledge graph for artists, labels, venues, and releases around the world.'

/**
 * Canonical public origin, with no trailing slash.
 *
 * Adoption is PARTIAL and deliberately so: `lib/seo/jsonld.ts` and the
 * scene-week page build from this, while most of `app/**` still inlines the
 * literal in its `alternates.canonical` and its `generateBreadcrumbSchema`
 * calls. Prefer this in new code; sweeping the rest is its own change, not a
 * side effect of an unrelated one. Do not read the existence of this constant
 * as proof that no literals remain.
 */
export const SITE_URL = 'https://psychichomily.com'

/**
 * The absolute canonical URL for a paginated list surface, given the path of
 * its list ROOT (page one, no filters applied).
 *
 * SITE-WIDE PAGINATION INDEXING POLICY (PSY-1767). This function is the policy;
 * the doc comment is the reasoning. Read both before changing either.
 *
 * THE RULE: every page and filter variant of an ordered list declares the list
 * root as its canonical. `?page=2`, `?window=`, `?scene=`, `?sort=`, `?year=`
 * and the rest are CRAWLABLE (the pagers emit real anchors, so the rows behind
 * them are reachable and their outbound links are followed) but they are not
 * themselves indexable documents. The rows are the content. The slice is only
 * an address for reaching them.
 *
 * WHY NOT self-referencing per-page canonicals, the alternative this ticket
 * weighed and rejected: a deep page of a list is a near duplicate of its
 * neighbours. Same heading, same chrome, same description, differing only in
 * which rows the offset happened to land on. Indexing those competes a surface
 * against itself for its own query and spreads its ranking signal across an
 * unbounded page set whose contents shift every time a row is added, which for
 * these lists is daily.
 *
 * The rule holds STRUCTURALLY rather than by discipline. Every caller is a
 * static `metadata` object or a `generateMetadata` that reads `params` only,
 * and neither can see the query string, so a per-page canonical would have to
 * be plumbed in on purpose. That is the point: the cheap path is the correct
 * one.
 *
 * GOVERNED SURFACES (this list is the policy's inventory, keep it current):
 *   - `/releases` plus `?page=` and every filter param (app/releases/page.tsx).
 *   - `/charts/{module}` drilldowns plus `?page=`, `?window=`, `?scene=`
 *     (app/charts/[module]/page.tsx).
 *   - `/charts/{year}` and `/charts/{year}/{quarter}` calendar archives
 *     (app/charts/[module]/page.tsx, app/charts/[module]/[period]/page.tsx).
 *   - `/venues/{slug}/shows/{year}` year archives, which reached this posture
 *     first under PSY-1756. Its canonical is built in
 *     `features/venues/yearArchivePage.tsx` from the year path and is blind to
 *     `?page=` for exactly the structural reason above. It is not routed
 *     through this helper because its URL is assembled by `venueArchiveHref`,
 *     which owns the shape of that path.
 *
 * OPEN FOLLOW-UP, and not a hole in the policy: under `cacheComponents`, Next
 * streams `<title>` and this `<link rel="canonical">` into the BODY rather than
 * the `<head>` on routes whose metadata is DYNAMIC. Measured on PSY-1767
 * against `next build` + `next start` + curl, which splits the surfaces here:
 *
 *   - `/charts/{module}?page=2` and `/charts/2026/q2?page=2` emit the canonical
 *     AFTER `</head>`, because both use `generateMetadata`.
 *   - `/releases?page=3&type=album` emits it INSIDE `<head>`, because its
 *     metadata is a static object resolved at build time.
 *
 * So the exposure is the charts family, not the whole policy, and it is about
 * PLACEMENT only. Every tag above carries the right href. Whether Google honors
 * a body-streamed canonical has to be read off Search Console once the venue
 * year archives (same shape, shipped first under PSY-1756) have actually been
 * crawled. That has not happened yet. Until it has there is nothing here to
 * revisit, and pre-emptively reshaping a shipped posture on no signal would
 * trade a measured unknown for an unmeasured one.
 */
export function listRootCanonical(rootPath: string): string {
  return `${SITE_URL}${rootPath}`
}
