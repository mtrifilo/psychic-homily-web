/**
 * The sitemap generator.
 *
 * HOW THIS ROUTE CACHES — measured against `next build` in this repo on Next
 * 16.1.4 with `cacheComponents: true`. Do not reason about it from the Next
 * docs; three confident readings were wrong before this was measured, and the
 * first two measurements were themselves wrong because they only tested ONE of
 * the two cases below. Re-measure BOTH after any Next upgrade.
 *
 * With `generateSitemaps()` (PSY-1622) the route shards by family, and three
 * families are sharded further still: `shows`, `artists` and `releases` are
 * each served in eight buckets of their primary key (PSY-2019). Each shard fetches
 * `GET /sitemap/entries?family=…` with its OWN id as the value, so each Next
 * Data Cache entry stays under the ~1.50 MiB effective budget (2 MiB cap, body
 * base64-encoded). Children live at `/sitemap/{id}.xml`; the index is
 * `/sitemap-index` (robots points there). `/sitemap.xml` 308s to the index — do
 * not add `app/sitemap.xml/route.ts` (collides with the metadata
 * `[__metadata_id__]` route).
 *
 * SHARDING BY FAMILY ALONE IS NOT ENOUGH, and that is measured rather than
 * assumed. Against production on 2026-09-03 the whole `releases` family answers
 * 2,041,734 raw bytes (129.8% of the cap once encoded), `shows` 1,267,618
 * (80.6%, which is what failed a production build) and `artists` 918,744
 * (58.4%). `fetchShard` weighs every response, so a genuine breach fails the
 * build. The bucket scheme, the arithmetic behind the count, and what changing
 * it costs are documented on SHOW_SHARD_IDS in ./sitemap-shards.ts and on
 * `sitemapShard` in the backend.
 *
 * The route mode is CONDITIONAL on whether the build-time fetch succeeds, and
 * the build's fetch Data Cache is a second input. All four rows measured by
 * build → `next start` → kill the backend → curl. The first two were
 * RE-MEASURED on PSY-1763, against a database holding the production release
 * catalogue, once the shard count went from 10 to 14. Row 1 was re-measured
 * again on PSY-2019 at 32 shards, against a database holding the production
 * catalogue (`32 of 32 shards have a fallback document`); rows 2 to 4 were
 * NOT, so read their counts as the count they were taken at:
 *
 *   Backend reachable at build time (the normal production path):
 *     ├ ● /sitemap/[__metadata_id__]         1h      1y
 *     prerender-manifest: renderingMode STATIC, initialRevalidateSeconds 3600,
 *     initialExpireSeconds 31536000, and a rendered body on disk for EVERY
 *     shard. A later
 *     backend outage is SURVIVED — every shard served 200 with the previous
 *     document. It survives a COLD Data Cache too: deleting
 *     `.next/cache/fetch-cache` before starting still served 200 from the
 *     prerender. The prerendered body IS the stale-serving fallback, it ships
 *     inside the deployment, and it does not depend on the Data Cache.
 *
 *   Backend unreachable at build time, clean build cache (degraded):
 *     ├ ƒ /sitemap/[__metadata_id__]                     ← no window at all
 *     ├ ● /sitemap/[__metadata_id__] └ /sitemap/pages.xml
 *     Only the pages shard prerenders — it makes no network call. EVERY entity
 *     shard loses its body, re-renders per request, and returns 500 while the
 *     backend is down. `/sitemap-index` still 200s, advertising shards that all
 *     500. `next build` alone EXITS 0, so this used to ship silently and
 *     persist until some later deploy.
 *
 *   Backend unreachable at build time, WARM build cache:
 *     All shards prerender anyway, from in-window Data Cache entries —
 *     verified by the stamp in the served body. Treat this row as the weakest
 *     of the four: it was measured against a two-slug stub, so it assumes the
 *     family's response fits a Data Cache entry (~2 MiB cap), and it assumes
 *     Vercel restores `.next/cache` between builds, which is documented
 *     platform behaviour rather than something probed here. It says the
 *     degraded row should be RARE — needing a cache miss and an outage at the
 *     same moment — not that a warm cache will rescue a build.
 *
 * Rows 1 to 3 do not depend on payload size: they are about which artifacts a
 * build produces and what the server does with them.
 *
 * The degraded row is now unshippable rather than survivable: the `build` npm
 * script chains lib/sitemap-prerender/cli.ts, which exits 1 when any shard has
 * no prerendered body. That is also Vercel's buildCommand, so the deploy fails
 * and the PREVIOUS deployment keeps serving its own prerendered sitemap — which
 * is what "stale beats a 500" resolves to in practice. See
 * lib/sitemap-prerender/check.ts for the full matrix and the reasoning.
 *
 * A route-level `export const revalidate` is NOT inert here — it binds as a
 * MINIMUM. Measured, with the per-fetch hint set to 600:
 *
 *   no export                       → ●  10m  1y   (initialRevalidateSeconds 600)
 *   export const revalidate = 60    → ●   1m  1y   (initialRevalidateSeconds  60)
 *   export const revalidate = 7200  → ●  10m  1y   (initialRevalidateSeconds 600)
 *
 * i.e. effective window = min(route export, per-fetch revalidate). There is no
 * export here because any value >= the fetch hint is a no-op — NOT because the
 * knob does nothing. An earlier version of this comment claimed it was "inert",
 * which was wrong: it had only been tried at 3600, equal to the fetch hint, so
 * it could not have shown an effect. If you add one for an unrelated reason, be
 * aware it silently CAPS sitemap freshness.
 *
 * On a build where the fetch succeeded, the revalidate half of the window comes
 * from the per-fetch `next: { revalidate }` below. The one-YEAR expire is NOT
 * from that hint — Next fills `cacheControl.expire` from config `expireTime`
 * (default `CACHE_ONE_YEAR` = 31536000) whenever expire is unset
 * (`next/dist/build/index.js`). Measured: `initialExpireSeconds: 31536000`.
 *
 * PSY-1644 (measured): that one-year Full Route Cache expire is what held the
 * stale production sitemap. On the healthy STATIC path, a prerendered body
 * keeps being served for up to a year while revalidations fail (the old
 * `/shows` fetch aborted at 10s every time). One prerender ⇒ every family
 * stale together — including artists that answered in 0.2s. The CDN edge is
 * not the holder (`cache-control: max-age=0`; stage serves `PRERENDER`/`HIT`
 * of the Next document). Hourly revalidate still works when the feed is
 * healthy (stage probe: held inside the 1h window, appeared after ≥1h).
 *
 * DO NOT try to bound that expire. It cannot be done from the cache layer —
 * disproven on BOTH runtimes (PSY-1652, which shipped no code; PR #1759 was
 * closed unmerged). `"use cache"` + `cacheLife` moves the manifest number and
 * bounds nothing:
 *
 *   build   ● 1h 1y / initialExpireSeconds 31536000  →  ● 1h 1d / 86400
 *   runtime `next start`, expire 960: served the stale body at 1043s — 83s PAST
 *           expire — with `.body`/`.meta` RE-STAMPED mid-poll, so every failed
 *           revalidation resets the age clock. `calculateRevalidate` reads only
 *           `cacheControl.revalidate`; expire is absent from that decision.
 *   Vercel  PREVIEW probe, expire 400: 200 `x-vercel-cache: STALE` at age 1566s
 *           (3.9x expire), with a PASSING positive control proving the route
 *           does revalidate on that window. `@vercel/next` really does serialize
 *           `staleExpiration` into `.prerender-config.json` — the platform
 *           receives the number and ignores it. CAVEAT: preview only. Production
 *           ISR policy is ASSUMED identical, not probed. Both environments were
 *           measured serving shards via the same Prerender path and emit
 *           identical build artifacts, but that is inference, not measurement.
 *
 * Consequence: there is NO upper bound on how long a stale sitemap serves while
 * revalidation keeps failing. That is a deliberate, ACCEPTED posture rather
 * than an open defect — stale wins over a 500, because repeated 5xx is what
 * makes Search Console mark a sitemap "Couldn't fetch" whereas a stale
 * document keeps the known URLs discoverable. PSY-1629's
 * freshness monitor is not a compensating control — it is the only one. If a
 * bound is ever wanted it has to be something this route owns: an age check
 * against a feed timestamp, or on-demand revalidation driven by a health
 * signal. Not a cache-layer knob; that was tried and disproven above.
 *
 * Fail-closed is unchanged and orthogonal: `fetchShard` still throws on
 * a bad answer, so a WRONG document is never published. Fail-closed governs
 * what gets written; stale-wins governs what gets served when nothing new can
 * be written.
 *
 * Two traps if you tune these numbers anyway: `cacheLife` beats the per-fetch
 * hint even when LARGER (7200 vs 3600 → 7200; not a `min()`), and an expire
 * below 300 (`DYNAMIC_EXPIRE`) means the route is not prerendered AT ALL —
 * measured `ƒ` with no body against a healthy backend.
 *
 * NOTE: this file fixes the fail-open half of the incident via the projection
 * feed. The year-expire holder has no fix, and no longer wants one; see above.
 */
import { MetadataRoute } from 'next'
import { getBlogSlugs, getBlogPost, getMixSlugs, getMix } from '@/features/blog'
import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import { readJsonWithinDataCacheBudget } from '@/lib/data-cache-budget/assert'
import type { components } from '@/types/api'
import {
  ALL_SHARD_IDS,
  FAMILY_URL_PREFIXES,
  PAGES_SHARD_ID,
  shardFamily,
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

/**
 * The crawl hints each family carries.
 *
 * Typed as a total `Record<Family, …>` on purpose: when the backend adds a
 * family to `SitemapEntries`, this object stops compiling until it is mapped
 * here. Verified by simulation — adding a `labels` field to the generated
 * schema makes tsc emit TS2741 on this declaration. Without it a new family
 * would be fetched, ignored, and silently absent from the XML.
 *
 * The URL prefix is deliberately NOT here: it lives in FAMILY_URL_PREFIXES in
 * sitemap-shards.ts because lib/sitemap-monitor has to map served URLs back
 * onto families with the same table. `changeFrequency` and `priority` stay
 * local because nothing outside this generator has any use for them.
 *
 * Public collections are deliberately absent — see contracts.SitemapEntries.
 */
const FAMILY_ROUTES: Record<
  Family,
  {
    changeFrequency: MetadataRoute.Sitemap[number]['changeFrequency']
    priority: number
  }
> = {
  shows: { changeFrequency: 'weekly', priority: 0.8 },
  artists: { changeFrequency: 'monthly', priority: 0.6 },
  venues: { changeFrequency: 'monthly', priority: 0.6 },
  // A venue's year archive changes only while that year is the current one, and
  // never again afterwards; `yearly` is the honest signal for a family that is
  // overwhelmingly closed history. Priority sits below the venue page itself
  // because the venue page is the entry point and the archive is a facet of it.
  venue_years: { changeFrequency: 'yearly', priority: 0.4 },
  scenes: { changeFrequency: 'weekly', priority: 0.7 },
  scene_weeks: { changeFrequency: 'weekly', priority: 0.6 },
  labels: { changeFrequency: 'monthly', priority: 0.5 },
  releases: { changeFrequency: 'monthly', priority: 0.5 },
  festivals: { changeFrequency: 'monthly', priority: 0.6 },
  tags: { changeFrequency: 'monthly', priority: 0.5 },
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
 * The statuses that mean "this backend does not serve that family", as opposed
 * to "this backend could not answer" (PSY-1756).
 *
 * The distinction is the whole safety argument for the degraded path below, so
 * it is drawn as narrowly as the wire allows:
 *
 *   422 — huma rejected the `family` query value against its enum before the
 *         handler ran. Measured against the deployed stage backend:
 *         `{"status":422,"detail":"validation failed","errors":[{"message":
 *         "expected value to be one of \"shows, artists, …\"","location":
 *         "query.family","value":"venue_years"}]}`.
 *   400 — SitemapService's own `unknown sitemap family` guard, for a family
 *         that clears the enum but that the service does not implement.
 *
 * Deliberately NOT 404. A moved or misconfigured `/sitemap/entries` answers 404
 * for EVERY family, so degrading on it would publish an empty document for the
 * whole catalogue — which is the incident in contracts.SitemapEntry, not a
 * mitigation of it. Deliberately not 5xx, a timeout, or a network error either:
 * that is the outage row the prerender gate exists to refuse.
 */
const UNKNOWN_FAMILY_STATUSES = new Set([400, 422])

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
 * ONE case does not throw: a backend that says it does not serve this family at
 * all (see UNKNOWN_FAMILY_STATUSES). That is a definite answer, not a failure,
 * and it has exactly one cause in practice — the frontend deployed ahead of the
 * backend that adds the family. It is not hypothetical: PSY-1756 added
 * `venue_years` in a PR carrying both halves, and every preview build failed the
 * prerender gate on that one shard (1 of 11, with the other ten prerendered
 * from the same healthy backend) because Vercel builds against the DEPLOYED
 * stage API. The same race recurs at merge, between Vercel's production build
 * and Railway's backend deploy, and hand-sequencing two deploy pipelines is not
 * a fix anyone can be relied on to repeat.
 *
 * So the shard degrades to an EMPTY, valid document rather than throwing. That
 * keeps the gate's intent rather than bending it: the build ships instead of
 * failing on a race nobody can sequence away.
 *
 * For a new FAMILY the document is also TRUE — a backend without the family
 * genuinely has no such URLs to announce. For a new SUB-SHARD of a family the
 * backend already serves it is not: those rows exist and go unannounced until
 * the backend learns the id.
 *
 * What this does NOT buy is a prerendered body. Next only writes a Data Cache
 * entry for a 200, so a 422'd shard is uncached, renders per request, and ships
 * Dynamic — measured as `1 of 11 shards` on PSY-1756's preview builds. It
 * therefore has no ISR timer and no stale-serving fallback: it recovers when it
 * is next RENDERED after the backend ships, and fully on the next build. Do not
 * read an hourly self-heal into this path; that window belongs to the healthy
 * STATIC path above. `partitionShardFailures` in
 * lib/sitemap-prerender/check.ts owns this statement, including why the gate
 * cannot distinguish the two cases and what covers the gap instead.
 *
 * The residual risk is a family being RENAMED on the backend while the frontend
 * still asks for the old name: that would empty a real family quietly. Two
 * things already cover it, and neither is this function — `FAMILY_ROUTES` below
 * is a total `Record<Family, …>` over the generated schema, and
 * `SITEMAP_FAMILIES` carries a compile-time exhaustiveness guard, so a renamed
 * family fails `bun run api:types` and then `tsc` before it can reach a build.
 *
 * A SUB-SHARD id is not a schema key, so `FAMILY_ROUTES` cannot cover it — but
 * it IS in the generated `operations` type, because it is a value of the
 * `family` query enum. The bucket id lists are declared
 * `satisfies readonly WireFamily[]` against exactly that, so a renamed
 * sub-shard fails
 * `bun run api:types` and then `tsc`, the same two steps a renamed family fails
 * on. The backend's own enum is separately asserted against its shard table
 * (TestSitemapFamilyEnumMatchesTheService), so neither side can drift alone.
 *
 * `shardId` is what goes on the wire and `family` is which key of the response
 * carries the rows: they are the same string for the seven single-document
 * families, and differ for every bucket of the other three. Passing both rather
 * than deriving one
 * from the other keeps this function ignorant of which families are sharded.
 *
 * Sharded by `?family=` so each generateSitemaps() id gets its own Data Cache
 * entry and its own ~1.50 MiB budget (PSY-1622, PSY-2019).
 */
async function fetchShard(shardId: string, family: Family): Promise<SitemapEntry[]> {
  const url = `${API_BASE_URL}/sitemap/entries?family=${encodeURIComponent(shardId)}`
  try {
    const res = await fetch(url, {
      next: { revalidate: ENTRY_REVALIDATE_SECONDS },
      signal: AbortSignal.timeout(ENTRY_FETCH_TIMEOUT_MS),
    })
    if (UNKNOWN_FAMILY_STATUSES.has(res.status)) {
      // Loud, but not fatal. A warning rather than an error because this is an
      // expected state during one deploy window; if it is still firing after
      // the backend has shipped, the family name has drifted and the message
      // says which one to look at.
      const message =
        `sitemap: backend does not serve the "${shardId}" family ` +
        `(HTTP ${res.status}) — emitting an empty shard until it does`
      console.warn(message)
      Sentry.captureMessage(message, {
        level: 'warning',
        tags: { service: 'sitemap', family, shard: shardId },
        extra: { url, status: res.status },
      })
      return []
    }
    if (!res.ok) {
      throw new Error(`sitemap entries fetch returned ${res.status}`)
    }

    // Sharding is what keeps each entry under the cache-item cap, and this is
    // what checks that it still does: a family outgrows its entry silently
    // otherwise. Weighed on the way through, because an over-cap response is
    // never written to the Data Cache and so is observable nowhere else. The
    // absolute URL is passed deliberately — it is the same identity the
    // post-build scan reads out of the cache envelope, so both halves of the
    // gate name a breach the same way.
    const entries = await readJsonWithinDataCacheBudget<SitemapEntries>(url, res)
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
      tags: { service: 'sitemap', family, shard: shardId },
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
  const { changeFrequency, priority } = FAMILY_ROUTES[family]
  const prefix = FAMILY_URL_PREFIXES[family]
  // Rows are known well-formed: fetchShard rejected the response
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
 * One shard per entity family, or one per bucket for a family that does not fit
 * a single Data Cache entry, plus a pages shard. String ids become
 * `/sitemap/{id}.xml` under the `/sitemap-index` document.
 */
export async function generateSitemaps() {
  return ALL_SHARD_IDS.map(id => ({ id }))
}

export default async function sitemap(props: {
  id: Promise<string>
}): Promise<MetadataRoute.Sitemap> {
  const id = await props.id

  if (id === PAGES_SHARD_ID) {
    return pagesShard()
  }

  // Resolved through the shard table rather than by checking `id in
  // FAMILY_ROUTES`: a sub-shard id is not a family key, so the membership test
  // would reject exactly the ids this route now has to serve.
  const family = shardFamily(id)
  if (!family) {
    throw new Error(`unknown sitemap shard id "${id}"`)
  }

  const rows = await fetchShard(id, family)
  return mapFamilyEntries(family, rows)
}
