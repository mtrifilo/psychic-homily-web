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
 * SITE-WIDE PAGINATION INDEXING POLICY (PSY-1767).
 *
 * THE RULE: every page and filter variant of an ordered list declares the list
 * root as its canonical. `?page=2`, `?window=`, `?scene=`, `?sort=`, `?year=`
 * and the rest are CRAWLABLE, since the pagers emit real anchors and the rows
 * behind them stay reachable, but they are not themselves indexable documents.
 * The rows are the content. The slice is only an address for reaching them.
 *
 * WHY NOT self-referencing per-page canonicals, the alternative PSY-1767
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
 * be plumbed in on purpose.
 *
 * SURFACES ROUTED THROUGH THIS HELPER. Not an inventory of every paginated
 * list on the site, see the note below. Add to it when a list adopts the
 * helper:
 *   - `/releases` plus `?page=` and every filter param (app/releases/page.tsx).
 *   - `/charts/{module}` drilldowns plus `?page=`, `?window=`, `?scene=`. This
 *     was the one real outlier: it declared no canonical at all until
 *     PSY-1767, so every slice was offered as its own document.
 *   - `/charts/{year}` and `/charts/{year}/{quarter}` calendar archives.
 *
 * TWO SURFACES FOLLOW THE POLICY WITHOUT CALLING THIS, and both are fine:
 *   - `/venues/{slug}/shows/{year}` reached this posture first under PSY-1756.
 *     Its URL comes from `archiveUrl` in `features/venues/yearArchivePage.tsx`,
 *     which also feeds the breadcrumb, so it is a page-URL builder rather than
 *     a canonical builder. It is blind to `?page=` for the same structural
 *     reason as above.
 *   - `/artists`, `/venues`, `/shows` and `/labels` each inline a root-pinned
 *     literal. They already agree with the policy; sweeping them onto the
 *     helper is its own change, per the note on SITE_URL above.
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
