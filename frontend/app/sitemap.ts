/**
 * The sitemap generator.
 *
 * HOW THIS ROUTE CACHES — measured against `next build` in this repo on Next
 * 16.1.4 with `cacheComponents: true`. Do not reason about it from the Next
 * docs; four confident readings have been wrong here, and two of the earlier
 * measurements were wrong because they tested only ONE of the two build cases
 * below. Re-measure BOTH after any Next upgrade, and state which case you
 * measured.
 *
 * With `generateSitemaps()` the route shards by family. Each shard fetches
 * `GET /sitemap/entries?family=…` so no single cache entry approaches the ~1.5
 * MB effective budget (2 MB cap, body base64-encoded). Children live at
 * `/sitemap/{id}.xml`; the index is `/sitemap-index` (robots points there).
 * `/sitemap.xml` 308s to the index — do not add `app/sitemap.xml/route.ts`
 * (collides with the metadata `[__metadata_id__]` route).
 *
 * The route mode is CONDITIONAL on whether the build-time fetch succeeds.
 * Both cases measured, before and after the cache scope was added:
 *
 *   Backend reachable (the normal production path):
 *     ├ ● /sitemap/[__metadata_id__]         1h      1d
 *     STATIC, initialRevalidateSeconds 3600, initialExpireSeconds 86400, body
 *     on disk. Verified at runtime: `next start`, kill the backend, and
 *     `/sitemap/shows.xml` still answers 200 from the prerender.
 *
 *   Backend unreachable:
 *     `next build` FAILS, exit 1 — "Export encountered an error on
 *     /sitemap/[__metadata_id__]/route: /sitemap/artists.xml".
 *     Before the cache scope it EXITED 0, silently: the shards became
 *     `ƒ /sitemap/[__metadata_id__]` with no window and no body, the log said
 *     nothing at all, and every request 500ed until the next deploy.
 *
 *     Both outcomes are measured. The MECHANISM is NOT established — do not
 *     repeat the plausible story that "a cache scope has no dynamic bail-out."
 *     The export catch in `next/dist/export/routes/app-route.js` demotes a
 *     route to `revalidate: 0` only for `isDynamicUsageError` (DynamicServer /
 *     BailoutToCSR / NextRouter / DynamicPostpone); a rejected `fetch` is none
 *     of those and rethrows either way, so that catch alone does not explain
 *     the old exit-0. Whatever demoted it pre-change is unidentified.
 *
 * `/sitemap/pages.xml` is the exception in both cases: it fetches nothing, so
 * it prerenders unconditionally with `initialRevalidateSeconds: false` and no
 * expire, and only a deploy changes it.
 *
 * WHERE THE WINDOW COMES FROM — both halves come from `cacheLife()` in
 * `fetchSitemapFamily`. Measured on this route, per-fetch hint pinned at 3600:
 *
 *   cacheLife revalidate 1800  →  1800   (30m)
 *   cacheLife revalidate 7200  →  7200   (2h)
 *
 * The second line is decisive: cacheLife wins even when LARGER, so it is not a
 * `min()` — a per-fetch `next: { revalidate }` inside the scope does not bind
 * the route window at all. Hence `cache: 'no-store'` on the fetch.
 *
 * That costs something, and it is NOT redundancy being removed: `no-store`
 * opts out of the Data Cache, which persists in `.next/cache/fetch-cache`
 * across builds, whereas the `"use cache"` entry replacing it is a
 * process-local LRU that dies with the build. Every build now refetches all 9
 * families (up to ~13.5 MB — that is 9 × the 1.5 MB per-family ceiling, NOT a
 * measured payload size) where a rebuild inside the window previously hit disk.
 *
 * EXPIRE MUST BE >= 300s OR THE ROUTE IS NOT PRERENDERED AT ALL. Measured:
 * expire 15 and expire 120 both produced `ƒ` DYNAMIC with no body against a
 * HEALTHY backend; expire 86400 prerenders. The rule is in the source —
 * `use-cache-wrapper.js` treats an entry as dynamic when
 * `entry.expire < DYNAMIC_EXPIRE`, and `DYNAMIC_EXPIRE` is 300 (5 minutes) in
 * `next/dist/server/use-cache/constants.js`. `revalidate` is not the trigger:
 * revalidate 900 with expire 86400 prerenders fine (15m / 1d).
 *
 * WHY THE EXPIRE IS SET AT ALL. Leave `cacheControl.expire` unset and Next
 * fills it from config `expireTime`, default `CACHE_ONE_YEAR` (31536000) — see
 * `getCacheControl` in `next/dist/build/index.js`. Measured before the cache
 * scope: `initialExpireSeconds: 31536000`. There is no route-segment `expire`
 * export to reach it with (`AppSegmentConfig` has `revalidate` and no
 * counterpart), so a `"use cache"` scope is the only route-local way to set it.
 * Config `expireTime` would move every ISR route at once, and the others do not
 * share this failure mode: `lib/seo/fetchSeoList.ts` fails OPEN, so a stale
 * year there strands a JSON-LD block on a page that otherwise works. Here the
 * fetch IS the artifact.
 *
 * !! THE BOUND DOES NOT HOLD UNDER `next start`. MEASURED, NOT THEORISED. !!
 * `initialExpireSeconds` demonstrably changes — and nothing observable acts on
 * it. Built a shard with `revalidate: 900, expire: 960` (route table confirmed
 * `15m / 16m`), served it with `next start`, killed the backend immediately,
 * and polled: still HTTP 200, still the stale body, 1043s after the entry was
 * created — 83s PAST its expire. The `.body`/`.meta` files were re-stamped
 * mid-poll, so every failed revalidation resets the entry's age and an
 * age-vs-expire check can never fire.
 *
 * That matches the source:
 *   - The origin ISR path ignores expire. `IncrementalCache.get` derives
 *     `isStale` from `revalidateAfter`, and `calculateRevalidate` reads only
 *     `cacheControl.revalidate`. `expire` appears nowhere in that decision.
 *   - It cannot reach a CDN as `stale-while-revalidate` either: metadata routes
 *     hardcode `Cache-Control: public, max-age=0, must-revalidate`
 *     (`CACHE_HEADERS.REVALIDATE` in `next-metadata-route-loader.js`), and the
 *     app-route template only adds an SWR header when none is already set.
 *
 * So this constant does NOT, on its own, stop a stale sitemap being served.
 * Whether Vercel's platform ISR reads `initialExpireSeconds` from the manifest
 * is the entire remaining hope and is UNTESTED. Do not describe this route as
 * "bounded to a day" until a stage probe says so. If the probe comes back
 * negative, the fix has to be something the app controls — an age check
 * against a feed timestamp inside the route, or on-demand revalidation — not
 * a cache-layer knob.
 *
 * NOTHING AUTOMATED ENFORCES IT EITHER. The unit tests pin the `cacheLife`
 * arguments (that much is testable), but the framework's translation of those
 * arguments into a served window only exists in `next build` output. A CI
 * assertion over `prerender-manifest.json` is the missing fence.
 *
 * Background: an unbounded expire is the measured holder of a stale sitemap.
 * On the healthy STATIC path a prerendered body keeps being served while every
 * revalidation fails, and because ONE prerender covers every shard, a single
 * slow family takes all of them stale together — including families that were
 * answering fine. The CDN edge is not the holder (`max-age=0`; stage serves
 * `PRERENDER`/`HIT` of the Next document). Hourly revalidate does work when the
 * feed is healthy — but that probe predates the cache scope and has NOT been
 * re-run against `cacheLife`.
 */
import { MetadataRoute } from 'next'
import { cacheLife } from 'next/cache'
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
 * How long a shard stays fresh before Next tries to re-render it. Both halves
 * of the window live here, together, because they are one decision — see the
 * module header for where they take effect and what does not honour them.
 */
const ENTRY_REVALIDATE_SECONDS = 3600

/**
 * How long a prerendered shard is DECLARED servable while every revalidation
 * fails. It sets `initialExpireSeconds` and nothing more: measured, `next
 * start` goes on serving the stale document past this value. Read the
 * enforcement section in the module header before treating it as a guarantee.
 *
 * PROPOSED, NOT CONFIRMED. The number is a product threshold — how long we
 * prefer a stale-but-valid document over a loud failure — and is awaiting
 * sign-off, not settled. One day survives an overnight backend outage without
 * a crawler seeing a break, and caps a permanently-wedged feed at a day rather
 * than the one year Next defaults to. Do not treat it as decided.
 *
 * Must stay >= 300s regardless of what is chosen, or the route stops
 * prerendering entirely — see the module header.
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
 * sitemap. At build time that throw is now FATAL to the whole build, and there
 * is no retry — nine families each get one attempt at this budget.
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
 * and its own ~1.5 MB budget.
 *
 * The `"use cache"` scope below is what sets the shard's ISR window — see the
 * module header before changing either number.
 */
async function fetchSitemapFamily(family: Family): Promise<SitemapEntry[]> {
  'use cache'
  // `stale` is left at the default cacheLife profile's value rather than being
  // absent — the entry always carries one. Not set here because it is the
  // client Router Cache window, `getCacheControlHeader` ignores it (it reads
  // `revalidate` and `expire` only), and a metadata route only crawlers fetch
  // never enters the client router cache. Setting it would add a third number
  // that looks load-bearing here and is not.
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
