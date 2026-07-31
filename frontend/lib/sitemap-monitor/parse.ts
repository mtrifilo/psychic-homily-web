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

import { FAMILY_SHARD_IDS, PAGES_SHARD_ID, type Family } from '@/app/sitemap-shards'

/** One `<url>` entry from a `<urlset>` document. */
export interface SitemapUrl {
  loc: string
  lastModified?: string
}

/**
 * Which document shape was served.
 *
 * `index` is what production serves once the generateSitemaps() sharding is
 * deployed; `urlset` is the older single-document shape. The monitor supports
 * both because it has to keep working across that deploy boundary — see
 * `walkSitemap` in fetch.ts.
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
const LASTMOD_TAG = /<lastmod\b[^>]*>([\s\S]*?)<\/lastmod\s*>/i

/** The five predefined XML entities. Sitemap `<loc>` values must escape these. */
function decodeEntities(value: string): string {
  return value
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    // `&amp;` last: decoding it first would let `&amp;lt;` collapse to `<`.
    .replace(/&amp;/g, '&')
}

function extract(block: string, tag: RegExp): string | undefined {
  const match = tag.exec(block)
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
    const loc = extract(block, LOC_TAG)
    if (loc) locs.push(loc)
  }
  return locs
}

/** The `<url>` entries of a `<urlset>` document. */
export function parseUrlset(xml: string): SitemapUrl[] {
  const urls: SitemapUrl[] = []
  for (const [, block] of xml.matchAll(URL_BLOCK)) {
    const loc = extract(block, LOC_TAG)
    if (!loc) continue
    urls.push({ loc, lastModified: extract(block, LASTMOD_TAG) })
  }
  return urls
}

/**
 * Path prefixes that belong to the `pages` shard.
 *
 * Listing pages (`/shows`, `/venues`, …) are single-segment and handled by the
 * segment-count rule below, so only the multi-segment page families need to be
 * named here.
 */
const PAGE_PREFIXES = new Set(['blog', 'dj-sets'])

/**
 * First path segment → family, for the families whose URLs are exactly
 * `/{prefix}/{slug}`.
 *
 * Typed as a total record over the shard ids so that adding a family to
 * sitemap-shards.ts stops this file compiling until the new family is
 * classified — the same compile-time guard app/sitemap.ts uses on
 * FAMILY_ROUTES. `scenes` and `scene_weeks` share the `/scenes` prefix and are
 * split by segment count instead, so they are mapped to `null` here.
 */
const PREFIX_TO_FAMILY: Record<Family, string | null> = {
  shows: 'shows',
  artists: 'artists',
  venues: 'venues',
  labels: 'labels',
  releases: 'releases',
  festivals: 'festivals',
  tags: 'tags',
  scenes: null,
  scene_weeks: null,
}

const SIMPLE_PREFIXES = new Map<string, Family>(
  FAMILY_SHARD_IDS.flatMap(family => {
    const prefix = PREFIX_TO_FAMILY[family]
    return prefix ? ([[prefix, family]] as [string, Family][]) : []
  })
)

/**
 * Bucket a `<loc>` by its URL path.
 *
 * Only needed for the single-document shape, where every family shares one
 * `<urlset>`. When the target serves a sitemap index the shard id is the
 * family and this is not consulted — which is why the sharded path cannot be
 * fooled by a classification gap here.
 *
 * Mirrors FAMILY_ROUTES in app/sitemap.ts. Kept as a separate declaration on
 * purpose: importing app/sitemap.ts would drag Sentry and the MDX blog loader
 * into a CLI that needs neither. classify.test.ts asserts the two stay in step.
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

  // `/scenes/{city}` is a scene; `/scenes/{city}/{iso-week}` is a scene week.
  if (prefix === 'scenes') {
    return segments.length === 2 ? 'scenes' : 'scene_weeks'
  }

  if (segments.length === 2) {
    const family = SIMPLE_PREFIXES.get(prefix)
    if (family) return family
  }

  return 'other'
}

/**
 * The event date encoded in a show slug (`2026-03-20-lonna-kelley-at-…`).
 *
 * Show `<lastmod>` carries `updated_at`, not the event date, so it cannot
 * answer "does this sitemap contain upcoming shows" — a bulk re-ingest of
 * historical rows would refresh every `<lastmod>` while the catalogue stayed
 * entirely in the past. The slug prefix is the only event-date signal in the
 * document.
 *
 * Returns undefined for the minority of show slugs with no date prefix
 * (measured: 10 of 1458 on stage), which are skipped rather than treated as
 * malformed.
 */
export function showDateFromLoc(loc: string): string | undefined {
  const match = /\/shows\/(\d{4}-\d{2}-\d{2})-/.exec(loc)
  return match?.[1]
}
