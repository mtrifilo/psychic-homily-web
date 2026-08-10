/**
 * Sitemap XML parsing and URL-family classification.
 *
 * Deliberately dependency-free and regex-based rather than a real XML parser.
 * The only documents this reads are the ones this repo emits (Next's
 * `MetadataRoute.Sitemap` serializer and app/sitemap-index/route.ts), which are
 * machine-generated, namespace-plain and entity-free in practice. Adding an XML
 * dependency to the frontend bundle graph to parse two known shapes is not
 * worth it. If the sitemap ever gains a namespace prefix (`<ns:loc>`) or
 * embedded markup, replace this — do not widen the regexes further.
 *
 * `<sitemap\b` cannot match `<sitemapindex` because there is no word boundary
 * between `sitemap` and `index`, so the block matchers do not need to guard
 * against the root element.
 */

import {
  FAMILY_SHARD_IDS,
  FAMILY_URL_PREFIXES,
  PAGES_SHARD_ID,
  type Family,
} from '@/app/sitemap-shards'

/**
 * Which document shape was served.
 *
 * `index` is what production serves now that the generateSitemaps() sharding
 * is deployed; `urlset` is the older single-document shape. The monitor
 * supports both because it has to keep working across that deploy boundary —
 * see `walkSitemap` in fetch.ts.
 */
export type SitemapShape = 'index' | 'urlset'

/**
 * The bucket a `<loc>` is counted under.
 *
 * `pages` is the static/MDX shard, which has no API counterpart to compare
 * against. `other` exists so an unrecognised URL is reported rather than
 * silently folded into `pages` — silent bucketing is the failure mode this
 * whole monitor exists to catch.
 */
export type LocBucket = Family | typeof PAGES_SHARD_ID | 'other'

const URL_BLOCK = /<url\b[^>]*>([\s\S]*?)<\/url\s*>/gi
const SITEMAP_BLOCK = /<sitemap\b[^>]*>([\s\S]*?)<\/sitemap\s*>/gi
const LOC_TAG = /<loc\b[^>]*>([\s\S]*?)<\/loc\s*>/i
const SHOW_SLUG_DATE = /\/shows\/(\d{4}-\d{2}-\d{2})-/

/** The five predefined XML entities. Sitemap `<loc>` values must escape these. */
function decodeEntities(value: string): string {
  // Sitemap locs essentially never contain entities, and this runs on every one
  // of ~33k URLs; one indexOf beats five full regex scans in the common case.
  if (!value.includes('&')) return value
  return value
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    // `&amp;` last: decoding it first would let `&amp;lt;` collapse to `<`.
    .replace(/&amp;/g, '&')
}

function extractLoc(block: string): string | undefined {
  const match = LOC_TAG.exec(block)
  if (!match) return undefined
  const value = decodeEntities(match[1].trim())
  return value.length > 0 ? value : undefined
}

/**
 * Identify the root element.
 *
 * Throws rather than guessing: a document that is neither shape is a served
 * error page or a redirect body, and counting zero URLs from it would look
 * exactly like a catastrophically empty sitemap. Failing loudly keeps the two
 * apart.
 */
export function detectShape(xml: string): SitemapShape {
  if (/<sitemapindex\b/i.test(xml)) return 'index'
  if (/<urlset\b/i.test(xml)) return 'urlset'
  throw new Error(
    'sitemap document has neither a <sitemapindex> nor a <urlset> root element'
  )
}

/** The child-document `<loc>` values listed by a `<sitemapindex>`. */
export function parseSitemapIndex(xml: string): string[] {
  const locs: string[] = []
  for (const [, block] of xml.matchAll(SITEMAP_BLOCK)) {
    const loc = extractLoc(block)
    if (loc) locs.push(loc)
  }
  return locs
}

/**
 * The `<loc>` values of a `<urlset>` document.
 *
 * `<lastmod>` is deliberately not returned. It carries `updated_at`, which
 * cannot answer the freshness question — a bulk re-ingest of historical rows
 * refreshes every `<lastmod>` while the catalogue stays entirely in the past.
 * `showDateFromLoc` reads the event date instead, so parsing lastmod would be
 * pure cost across ~33k entries.
 */
export function parseUrlset(xml: string): string[] {
  const locs: string[] = []
  for (const [, block] of xml.matchAll(URL_BLOCK)) {
    const loc = extractLoc(block)
    if (loc) locs.push(loc)
  }
  return locs
}

/**
 * Path prefixes that belong to the `pages` shard.
 *
 * Listing pages (`/shows`, `/venues`, …) are single-segment and handled by the
 * segment-count rule in `classifyLoc`, so only the multi-segment page families
 * need naming here.
 */
const PAGE_PREFIXES = new Set(['blog', 'dj-sets'])

/**
 * First path segment → the families claiming it, derived from the shared
 * FAMILY_URL_PREFIXES table rather than restated.
 *
 * Most prefixes have exactly one claimant. `/scenes` and `/venues` have two
 * each, which is why this is a list: the collision is DETECTED here rather than
 * assumed, so a future family sharing an existing prefix surfaces in
 * `SHARED_CLAIMANTS` (and fails the guard test in parse.test.ts) instead of
 * being silently misbucketed.
 */
const FAMILIES_BY_PREFIX = new Map<string, Family[]>()
for (const family of FAMILY_SHARD_IDS) {
  const prefix = FAMILY_URL_PREFIXES[family].replace(/^\//, '')
  const claimants = FAMILIES_BY_PREFIX.get(prefix)
  if (claimants) claimants.push(family)
  else FAMILIES_BY_PREFIX.set(prefix, [family])
}

/**
 * Every shared prefix and the exact families claiming it, sorted so the guard
 * test does not depend on FAMILY_SHARD_IDS' declaration order.
 *
 * The CLAIMANTS, not just the prefix list: `classifyLoc` needs a rule per
 * family under a shared prefix, not per prefix. A list of prefixes alone stops
 * catching anything once a prefix is already shared — a third family under
 * `/venues` would leave it unchanged, and the new family would silently
 * classify as `other` with every test still green.
 */
export const SHARED_CLAIMANTS: Record<string, Family[]> = Object.fromEntries(
  [...FAMILIES_BY_PREFIX]
    .filter(([, claimants]) => claimants.length > 1)
    .map(([prefix, claimants]) => [prefix, [...claimants].sort()])
)

/** The `/scenes` prefix without its slash, as `classifyLoc` compares segments. */
const bareScenesPrefix = FAMILY_URL_PREFIXES.scenes.replace(/^\//, '')

/** The `/venues` prefix without its slash. Same comparison, same reason. */
const bareVenuesPrefix = FAMILY_URL_PREFIXES.venues.replace(/^\//, '')

/**
 * The segment a venue-year URL carries between the venue slug and the year:
 * `/venues/{slug}/shows/{year}`. Checked rather than assumed, so a future
 * `/venues/{slug}/{something-else}` route does not silently count as an archive.
 */
const VENUE_YEAR_SEGMENT = 'shows'

/** A four-digit calendar year, the only tail a venue-year URL may end in. */
const VENUE_YEAR_PATTERN = /^\d{4}$/

/**
 * Bucket a `<loc>` by its URL path.
 *
 * Only needed for the single-document shape, where every family shares one
 * `<urlset>`. When the target serves a sitemap index the shard id is the
 * family and this is not consulted — which is why the sharded path cannot be
 * fooled by a classification gap here.
 */
export function classifyLoc(loc: string): LocBucket {
  let path: string
  try {
    path = new URL(loc).pathname
  } catch {
    return 'other'
  }

  const segments = path.split('/').filter(Boolean)

  // The bare origin and every single-segment listing page belong to `pages`.
  if (segments.length <= 1) return PAGES_SHARD_ID

  const [prefix] = segments
  if (PAGE_PREFIXES.has(prefix)) return PAGES_SHARD_ID

  const claimants = FAMILIES_BY_PREFIX.get(prefix)
  if (!claimants) return 'other'

  if (claimants.length === 1) {
    return segments.length === 2 ? claimants[0] : 'other'
  }

  // The shared prefixes. Keyed off the shared table rather than the literals,
  // so renaming a prefix moves the generator and this rule together — which is
  // the whole point of FAMILY_URL_PREFIXES having one owner.

  // `/scenes/{city}` is a scene, `/scenes/{city}/{week}` a scene week.
  if (prefix === bareScenesPrefix) {
    return segments.length === 2 ? 'scenes' : 'scene_weeks'
  }

  // `/venues/{slug}` is a venue, `/venues/{slug}/shows/{year}` a year archive
  // (PSY-1756). Stricter than the scenes rule on purpose: `/venues` has room for
  // other child routes, so anything that is not exactly the archive shape is
  // 'other' rather than being counted as an archive the generator never emitted.
  if (prefix === bareVenuesPrefix) {
    if (segments.length === 2) return 'venues'
    if (
      segments.length === 4 &&
      segments[2] === VENUE_YEAR_SEGMENT &&
      VENUE_YEAR_PATTERN.test(segments[3])
    ) {
      return 'venue_years'
    }
    return 'other'
  }

  return 'other'
}

/**
 * The event date encoded in a show slug (`2026-03-20-lonna-kelley-at-…`).
 *
 * The slug prefix is the only event-date signal in the document — see the note
 * on `parseUrlset` about why `<lastmod>` cannot serve here.
 *
 * Returns undefined for the minority of show slugs with no date prefix
 * (measured: 10 of 1458 on stage), which are skipped rather than treated as
 * malformed.
 */
export function showDateFromLoc(loc: string): string | undefined {
  return SHOW_SLUG_DATE.exec(loc)?.[1]
}
