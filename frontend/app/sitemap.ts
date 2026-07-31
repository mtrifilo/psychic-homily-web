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
 * under the ~1.5 MB effective budget (2 MB cap, body base64-encoded). Children
 * live at `/sitemap/{id}.xml`; the index is `/sitemap-index` (robots points
 * there). `/sitemap.xml` 308s to the index — do not add `app/sitemap.xml/route.ts`
 * (collides with the metadata `[__metadata_id__]` route).
 *
 * The route mode is CONDITIONAL on whether the build-time fetch succeeds:
 *
 *   Backend reachable at build time (the normal production path):
 *     ├ ● /sitemap/[__metadata_id__]         1h      1d
 *     prerender-manifest: renderingMode STATIC, initialRevalidateSeconds 3600,
 *     initialExpireSeconds 86400, and a rendered body on disk. A later backend
 *     outage is SURVIVED — the prerendered document keeps being served while
 *     revalidation fails. Verified at runtime: `next start`, then kill the
 *     backend; `/sitemap/shows.xml` still answers 200 from the prerender.
 *
 *   Backend unreachable at build time (degraded):
 *     `next build` FAILS — "Export encountered an error on
 *     /sitemap/[__metadata_id__]/route: /sitemap/shows.xml, exiting the build",
 *     exit 1. This is a consequence of the `"use cache"` scope below: a cache
 *     scope has no dynamic bail-out, so the throw is fatal rather than demoted
 *     to a per-request render.
 *
 *     Before that scope existed this case was SILENT — the shards fell back to
 *     `ƒ /sitemap/[__metadata_id__]` with no window and no body, `next build`
 *     exited 0 with not one line about it in the log, and every request 500ed
 *     until the next deploy. Losing that silence is the trade: a backend outage
 *     during a build now blocks the deploy instead of shipping broken shards.
 *
 * `/sitemap/pages.xml` is the exception in both cases: it fetches nothing (its
 * content is static pages plus local MDX), so it prerenders unconditionally
 * with `initialRevalidateSeconds: false` and no expire, and a deploy is the
 * only thing that changes it.
 *
 * WHERE THE WINDOW COMES FROM — both halves come from `cacheLife()` in
 * `fetchSitemapFamily`, and nothing else does. Measured on this route:
 *
 *   cacheLife revalidate 1800, per-fetch hint 3600  →  1800   (30m)
 *   cacheLife revalidate 7200, per-fetch hint 3600  →  7200   (2h)
 *
 * The second line is the decisive one: cacheLife wins even when it is LARGER,
 * so a per-fetch `next: { revalidate }` inside the cache scope does not bind
 * the route window at all. That is why the fetch is `cache: 'no-store'` — the
 * hint would only add a second cache layer with its own drifting TTL.
 *
 * That trade has a BUILD-TIME COST, and it is not redundancy being removed.
 * `no-store` opts out of the Data Cache, which is persisted in
 * `.next/cache/fetch-cache` and restored across builds; the `"use cache"` entry
 * that replaces it is a process-local in-memory LRU that dies with the build.
 * So every `next build` now issues all 9 `?family=` requests (~13.5 MB of slug
 * projections) where a rebuild inside the hour previously served them from
 * disk with zero backend hits. Deliberate — a build should not bake a
 * 59-minute-old sitemap — but weigh it if deploy cadence climbs.
 *
 * Do not tune these very low. Measured: `revalidate: 5, expire: 15` made the
 * family shards `ƒ` DYNAMIC with no prerendered body even against a HEALTHY
 * backend. 3600 / 86400 prerenders; anything much smaller needs re-measuring.
 *
 * A route-level `export const revalidate` is NOT inert here — it binds as a
 * MINIMUM, so adding one silently CAPS sitemap freshness. Measured before the
 * cache scope existed, against a 600s per-fetch hint: `60` → 60, `7200` → 600.
 * NOT re-measured against the cache scope; don't add one casually.
 *
 * WHY THE EXPIRE IS SET AT ALL. Leave `cacheControl.expire` unset and Next
 * fills it from config `expireTime`, whose default is `CACHE_ONE_YEAR`
 * (31536000) — see the `getCacheControl` helper in `next/dist/build/index.js`,
 * which substitutes `config.expireTime` whenever revalidate is set and expire
 * is not. Measured before the cache scope: `initialExpireSeconds: 31536000`.
 * There is no route-segment `expire` export to reach it with (`AppSegmentConfig`
 * has `revalidate` and no counterpart), so a `"use cache"` scope is the only
 * route-local way to bound it. The alternative, config `expireTime`, would move
 * every ISR route in the app at once — and the others do not share this failure
 * mode: `lib/seo/fetchSeoList.ts` fails OPEN, so a stale year there strands a
 * JSON-LD block on a page that otherwise works. Here the fetch IS the artifact.
 *
 * NOTHING AUTOMATED ENFORCES THE BOUND. `cacheLife` throws outside a Next
 * server context, so the unit tests stub it; the window exists only in the
 * `next build` output. A refactor that inlines the fetch, or adds a second
 * uncached data source to the default export, reverts this route to the
 * one-year expire with a green test suite. Re-read the route table.
 *
 * PSY-1644 (measured): that one-year Full Route Cache expire is what held the
 * stale production sitemap. On the healthy STATIC path, a prerendered body
 * keeps being served for up to the expire while revalidations fail (the old
 * `/shows` fetch aborted at 10s every time). One prerender ⇒ every family
 * stale together — including artists that answered in 0.2s. The CDN edge is
 * not the holder (`cache-control: max-age=0`; stage serves `PRERENDER`/`HIT`
 * of the Next document). Hourly revalidate still works when the feed is
 * healthy (stage probe: held inside the 1h window, appeared after ≥1h).
 */
import { MetadataRoute } from 'next'
import { cacheLife } from 'next/cache'
import { getBlogSlugs, getBlogPost, getMixSlugs, getMix } from '@/features/blog'
import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import type { components } from '@/types/api'
import {
  ENTRY_REVALIDATE_SECONDS,
  FAMILY_SHARD_IDS,
  PAGES_SHARD_ID,
  type Family,
} from './sitemap-shards'

const BASE_URL = 'https://psychichomily.com'

/**
 * How long a prerendered shard may keep being served while every revalidation
 * fails. Past this, Next stops serving the stale document.
 *
 * The number is a deliberate trade, not a tuning detail: it is how long we
 * prefer a stale-but-valid document over a loud failure. One day survives an
 * overnight backend outage without a crawler ever seeing a break, and caps the
 * damage from a permanently-wedged feed at a day instead of a year. See the
 * module header for why it has to be set at all.
 */
const ENTRY_EXPIRE_SECONDS = 86_400

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
 * Sharded by `?family=` so each generateSitemaps() id gets its own cache entry
 * and its own ~1.5 MB budget (PSY-1622).
 *
 * The `"use cache"` scope below is what sets the shard's ISR window — see the
 * module header before changing either number.
 */
async function fetchSitemapFamily(family: Family): Promise<SitemapEntry[]> {
  'use cache'
  // No `stale`: it is the client Router Cache window, and `getCacheControlHeader`
  // derives the emitted header from `revalidate` and `expire` alone. A metadata
  // route only crawlers fetch never enters the client router cache, so setting
  // it would add a third number that looks load-bearing and is not.
  cacheLife({
    revalidate: ENTRY_REVALIDATE_SECONDS,
    expire: ENTRY_EXPIRE_SECONDS,
  })
  try {
    const res = await fetch(
      `${API_BASE_URL}/sitemap/entries?family=${encodeURIComponent(family)}`,
      {
        // Not `next: { revalidate }`. Inside a `"use cache"` scope that hint no
        // longer binds the route window (measured — see the module header), so
        // leaving it in would stack a second, independent cache layer whose TTL
        // could drift from the one above and add its own staleness on top.
        //
        // This is not free — see the module header's note on build-time cost.
        cache: 'no-store',
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
  return [{ id: PAGES_SHARD_ID }, ...FAMILY_SHARD_IDS.map(id => ({ id }))]
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
