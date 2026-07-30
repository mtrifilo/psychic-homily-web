/**
 * The sitemap generator.
 *
 * HOW THIS ROUTE CACHES — measured against `next build` in this repo on Next
 * 16.1.4 with `cacheComponents: true`. Do not reason about it from the Next
 * docs; three confident readings were wrong before this was measured, and the
 * first two measurements were themselves wrong because they only tested ONE of
 * the two cases below. Re-measure BOTH after any Next upgrade.
 *
 * With `generateSitemaps()` (PSY-1622) the route shards by family. Each shard
 * fetches `GET /sitemap/entries?family=…`, so each Next Data Cache entry stays
 * under the ~1.5 MB effective budget (2 MB cap, body base64-encoded). The
 * index lives at `/sitemap.xml`; children at `/sitemap/{id}.xml`.
 *
 * The route mode is CONDITIONAL on whether the build-time fetch succeeds:
 *
 *   Backend reachable at build time (the normal production path):
 *     ├ ○ /sitemap.xml                       1h      1y
 *     prerender-manifest: renderingMode STATIC, initialRevalidateSeconds 3600,
 *     initialExpireSeconds 31536000, and a rendered `.next/server/app/
 *     sitemap.xml.body` on disk. A later backend outage is SURVIVED — the
 *     prerendered document keeps being served while revalidation fails.
 *
 *   Backend unreachable at build time (degraded):
 *     ├ ƒ /sitemap.xml                                   ← no window at all
 *     No prerendered body. The route re-renders per request, so a request while
 *     the backend is down returns 500. `next build` still EXITS 0 — the
 *     degradation is silent, and it persists until the next deploy.
 *
 * A route-level `export const revalidate` is NOT inert here — it binds as a
 * MINIMUM. Measured, with the per-fetch hint set to 600:
 *
 *   no export                       → ○  10m  1y   (initialRevalidateSeconds 600)
 *   export const revalidate = 60    → ○   1m  1y   (initialRevalidateSeconds  60)
 *   export const revalidate = 7200  → ○  10m  1y   (initialRevalidateSeconds 600)
 *
 * i.e. effective window = min(route export, per-fetch revalidate). There is no
 * export here because any value >= the fetch hint is a no-op — NOT because the
 * knob does nothing. An earlier version of this comment claimed it was "inert",
 * which was wrong: it had only been tried at 3600, equal to the fetch hint, so
 * it could not have shown an effect. If you add one for an unrelated reason, be
 * aware it silently CAPS sitemap freshness.
 *
 * On a build where the fetch succeeded, the window above comes from the
 * per-fetch `next: { revalidate }` below.
 *
 * The one-YEAR expire in the static case is worth knowing about: a prerendered
 * document can be served for that long if revalidations keep failing. That is a
 * candidate mechanism for the original unexplained staleness — see PSY-1644,
 * where it is recorded as a lead to measure, NOT as an established cause.
 *
 * NOTE: this file fixes the fail-open half of the incident in the backend's
 * contracts.SitemapEntry, not the staleness half.
 */
import { MetadataRoute } from 'next'
import { getBlogSlugs, getBlogPost, getMixSlugs, getMix } from '@/features/blog'
import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import type { components } from '@/types/api'
import {
  FAMILY_SHARD_IDS,
  PAGES_SHARD_ID,
  type Family,
} from './sitemap-shards'

const BASE_URL = 'https://psychichomily.com'

/**
 * How long a fetched entry set stays warm in Next's Data Cache. On a build
 * where the fetch succeeded, this value is also where the route's ISR
 * revalidate window comes from — see the module header.
 */
const ENTRY_REVALIDATE_SECONDS = 3600

type SitemapEntries = components['schemas']['SitemapEntries']
type SitemapEntry = components['schemas']['SitemapEntry']

export { FAMILY_SHARD_IDS, PAGES_SHARD_ID }
export type { Family }

/**
 * How each family maps onto a URL.
 *
 * Typed as a total `Record<Family, …>` on purpose: when the backend adds a
 * family to `SitemapEntries`, this object stops compiling until it is mapped
 * here. Verified by simulation — adding a `labels` field to the generated
 * schema makes tsc emit TS2741 on this declaration. Without it a new family
 * would be fetched, ignored, and silently absent from the XML.
 *
 * `scene_weeks` slugs are already composite (`phoenix-az/2026-W31`), so the
 * same `/scenes` prefix yields the archived canonical URL shape.
 *
 * Public collections are deliberately absent — see contracts.SitemapEntries.
 */
const FAMILY_ROUTES: Record<
  Family,
  {
    prefix: string
    changeFrequency: MetadataRoute.Sitemap[number]['changeFrequency']
    priority: number
  }
> = {
  shows: { prefix: '/shows', changeFrequency: 'weekly', priority: 0.8 },
  artists: { prefix: '/artists', changeFrequency: 'monthly', priority: 0.6 },
  venues: { prefix: '/venues', changeFrequency: 'monthly', priority: 0.6 },
  scenes: { prefix: '/scenes', changeFrequency: 'weekly', priority: 0.7 },
  scene_weeks: { prefix: '/scenes', changeFrequency: 'weekly', priority: 0.6 },
  labels: { prefix: '/labels', changeFrequency: 'monthly', priority: 0.5 },
  releases: { prefix: '/releases', changeFrequency: 'monthly', priority: 0.5 },
  festivals: { prefix: '/festivals', changeFrequency: 'monthly', priority: 0.6 },
  tags: { prefix: '/tags', changeFrequency: 'monthly', priority: 0.5 },
}

const FAMILY_IDS = FAMILY_SHARD_IDS

/**
 * Deliberately not the shared `createBuildTimeApiSignal()` 10s budget — that
 * budget is what the old generator silently blew. The projection feed answers
 * in well under a second, so this ceiling exists only to stop a wedged backend
 * hanging the render, and hitting it throws rather than yielding a partial
 * sitemap.
 */
const ENTRY_FETCH_TIMEOUT_MS = 30_000

/**
 * Fetch one family's indexable slug set, and reject anything that is not a
 * complete answer for that family.
 *
 * Both the transport failure and the shape check throw, by design. Publishing
 * a document that is missing an entity family drops thousands of URLs out of
 * the index with no failure signal anywhere — the exact signature of the
 * incident in the backend's contracts.SitemapEntry. A 200 whose family key is
 * null or absent is that same failure wearing a success code, so it is treated
 * as an error rather than coerced to an empty list.
 *
 * Sharded by `?family=` so each generateSitemaps() id gets its own Data Cache
 * entry and its own ~1.5 MB budget (PSY-1622).
 */
async function fetchSitemapFamily(family: Family): Promise<SitemapEntry[]> {
  try {
    const res = await fetch(
      `${API_BASE_URL}/sitemap/entries?family=${encodeURIComponent(family)}`,
      {
        next: { revalidate: ENTRY_REVALIDATE_SECONDS },
        signal: AbortSignal.timeout(ENTRY_FETCH_TIMEOUT_MS),
      }
    )
    if (!res.ok) {
      throw new Error(`sitemap entries fetch returned ${res.status}`)
    }

    const entries: SitemapEntries = await res.json()
    const rows = entries?.[family]
    if (!Array.isArray(rows)) {
      throw new Error(`sitemap entries response is missing the "${family}" family`)
    }
    // Elements are validated here, inside the try, rather than filtered away
    // at the mapping site. Dropping a malformed row silently would publish a
    // sitemap short a URL with no failure signal — the original incident in
    // miniature, and the exact thing this module exists to prevent.
    if (rows.some(row => typeof row?.slug !== 'string')) {
      throw new Error(`sitemap entries response has a malformed row in the "${family}" family`)
    }
    return rows
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'sitemap', family },
    })
    throw error
  }
}

/**
 * An unparseable timestamp would emit an invalid `<lastmod>`, so omit it rather
 * than poison the entry — a missing `<lastmod>` is a weaker signal, not a
 * broken document.
 */
function lastModified(updatedAt: string | undefined): Date | undefined {
  if (!updatedAt) return undefined
  const parsed = new Date(updatedAt)
  return Number.isNaN(parsed.getTime()) ? undefined : parsed
}

function mapFamilyEntries(
  family: Family,
  rows: SitemapEntry[]
): MetadataRoute.Sitemap {
  const { prefix, changeFrequency, priority } = FAMILY_ROUTES[family]
  // Rows are known well-formed: fetchSitemapFamily rejected the response
  // otherwise. The remaining filter is the empty-slug case — a real row whose
  // name produced no slug — which is legitimately skipped, not an error.
  return rows
    .filter(entry => entry.slug)
    .map(entry => ({
      url: `${BASE_URL}${prefix}/${entry.slug}`,
      lastModified: lastModified(entry.updated_at),
      changeFrequency,
      priority,
    }))
}

function pagesShard(): MetadataRoute.Sitemap {
  // Static pages carry no `lastModified`: stamping them with `new Date()` on
  // every render claimed the whole site changed every time, which devalues the
  // signal for the entries that genuinely did change.
  const staticPages: MetadataRoute.Sitemap = [
    { url: BASE_URL, changeFrequency: 'daily', priority: 1 },
    { url: `${BASE_URL}/shows`, changeFrequency: 'daily', priority: 0.9 },
    { url: `${BASE_URL}/venues`, changeFrequency: 'weekly', priority: 0.8 },
    { url: `${BASE_URL}/scenes`, changeFrequency: 'weekly', priority: 0.8 },
    { url: `${BASE_URL}/labels`, changeFrequency: 'weekly', priority: 0.7 },
    { url: `${BASE_URL}/releases`, changeFrequency: 'weekly', priority: 0.7 },
    { url: `${BASE_URL}/festivals`, changeFrequency: 'weekly', priority: 0.7 },
    { url: `${BASE_URL}/tags`, changeFrequency: 'weekly', priority: 0.6 },
    { url: `${BASE_URL}/blog`, changeFrequency: 'weekly', priority: 0.8 },
    { url: `${BASE_URL}/dj-sets`, changeFrequency: 'weekly', priority: 0.7 },
    { url: `${BASE_URL}/privacy`, changeFrequency: 'monthly', priority: 0.3 },
    { url: `${BASE_URL}/terms`, changeFrequency: 'monthly', priority: 0.3 },
  ]

  // Blog and DJ sets are local MDX — no network, nothing to fail closed on.
  const blogPages: MetadataRoute.Sitemap = getBlogSlugs().map(slug => {
    const post = getBlogPost(slug)
    return {
      url: `${BASE_URL}/blog/${slug}`,
      lastModified: lastModified(post?.frontmatter.date),
      changeFrequency: 'monthly' as const,
      priority: 0.6,
    }
  })

  const mixPages: MetadataRoute.Sitemap = getMixSlugs().map(slug => {
    const mix = getMix(slug)
    return {
      url: `${BASE_URL}/dj-sets/${slug}`,
      lastModified: lastModified(mix?.frontmatter.date),
      changeFrequency: 'monthly' as const,
      priority: 0.5,
    }
  })

  return [...staticPages, ...blogPages, ...mixPages]
}

/**
 * One shard per entity family plus a pages shard. String ids become
 * `/sitemap/{id}.xml` under the `/sitemap.xml` index Next emits.
 */
export async function generateSitemaps() {
  return [{ id: PAGES_SHARD_ID }, ...FAMILY_IDS.map(id => ({ id }))]
}

export default async function sitemap(props: {
  id: Promise<string>
}): Promise<MetadataRoute.Sitemap> {
  const id = await props.id

  if (id === PAGES_SHARD_ID) {
    return pagesShard()
  }

  if (!(id in FAMILY_ROUTES)) {
    throw new Error(`unknown sitemap shard id "${id}"`)
  }

  const family = id as Family
  const rows = await fetchSitemapFamily(family)
  return mapFamilyEntries(family, rows)
}
