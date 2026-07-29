/**
 * The sitemap generator.
 *
 * HOW THIS ROUTE CACHES — measured against `next build` in this repo on Next
 * 16.1.4 with `cacheComponents: true`. Do not reason about it from the Next
 * docs; three confident readings were wrong before this was measured, and the
 * first two measurements were themselves wrong because they only tested ONE of
 * the two cases below. Re-measure BOTH after any Next upgrade.
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
 * An `export const revalidate` here is INERT: it was tried and the route mode
 * did not change in either case, so it was removed rather than left in as a
 * placebo. In the static case the window above comes from the per-fetch
 * `next: { revalidate }` below.
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

const BASE_URL = 'https://psychichomily.com'

/**
 * How long a fetched entry set stays warm in Next's Data Cache. The ONLY
 * freshness mechanism on this route — see the module header for why.
 */
const ENTRY_REVALIDATE_SECONDS = 3600

type SitemapEntries = components['schemas']['SitemapEntries']
type SitemapEntry = components['schemas']['SitemapEntry']

/** The entity families, minus the `$schema` key Huma adds to every response. */
type Family = Exclude<keyof SitemapEntries, '$schema'>

/**
 * How each family maps onto a URL.
 *
 * Typed as a total `Record<Family, …>` on purpose: when the backend adds a
 * family to `SitemapEntries`, this object stops compiling until it is mapped
 * here. Verified by simulation — adding a `labels` field to the generated
 * schema makes tsc emit TS2741 on this declaration. Without it a new family
 * would be fetched, ignored, and silently absent from the XML.
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
 * Fetch the indexable slug set, and reject anything that is not a complete
 * answer.
 *
 * Both the transport failure and the shape check throw, by design. Publishing
 * a document that is missing an entity family drops thousands of URLs out of
 * the index with no failure signal anywhere — the exact signature of the
 * incident in the backend's contracts.SitemapEntry. A 200 whose `shows` key is
 * null or absent is that same failure wearing a success code, so it is treated
 * as an error rather than coerced to an empty list.
 */
async function fetchSitemapEntries(): Promise<SitemapEntries> {
  try {
    const res = await fetch(`${API_BASE_URL}/sitemap/entries`, {
      next: { revalidate: ENTRY_REVALIDATE_SECONDS },
      signal: AbortSignal.timeout(ENTRY_FETCH_TIMEOUT_MS),
    })
    if (!res.ok) {
      throw new Error(`sitemap entries fetch returned ${res.status}`)
    }

    const entries: SitemapEntries = await res.json()
    for (const family of Object.keys(FAMILY_ROUTES) as Family[]) {
      if (!Array.isArray(entries?.[family])) {
        throw new Error(`sitemap entries response is missing the "${family}" family`)
      }
    }
    return entries
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'sitemap' },
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

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const entries = await fetchSitemapEntries()

  // Static pages carry no `lastModified`: stamping them with `new Date()` on
  // every render claimed the whole site changed every time, which devalues the
  // signal for the entries that genuinely did change.
  const staticPages: MetadataRoute.Sitemap = [
    { url: BASE_URL, changeFrequency: 'daily', priority: 1 },
    { url: `${BASE_URL}/shows`, changeFrequency: 'daily', priority: 0.9 },
    { url: `${BASE_URL}/venues`, changeFrequency: 'weekly', priority: 0.8 },
    { url: `${BASE_URL}/blog`, changeFrequency: 'weekly', priority: 0.8 },
    { url: `${BASE_URL}/dj-sets`, changeFrequency: 'weekly', priority: 0.7 },
    { url: `${BASE_URL}/privacy`, changeFrequency: 'monthly', priority: 0.3 },
    { url: `${BASE_URL}/terms`, changeFrequency: 'monthly', priority: 0.3 },
  ]

  const entityPages: MetadataRoute.Sitemap = (
    Object.keys(FAMILY_ROUTES) as Family[]
  ).flatMap(family => {
    const { prefix, changeFrequency, priority } = FAMILY_ROUTES[family]
    // No `?? []` here on purpose: fetchSitemapEntries has already rejected any
    // family that is not an array, and a coercion at this site would read as
    // the exact fail-open this module exists to remove.
    return (entries[family] as SitemapEntry[])
      // `entry?.slug`, not `entry.slug`: Array.isArray admits `[null]`, and a
      // deref here would throw OUTSIDE fetchSitemapEntries' try — still fail-
      // closed, but with no Sentry event and no diagnostic message.
      .filter(entry => entry?.slug)
      .map(entry => ({
        url: `${BASE_URL}${prefix}/${entry.slug}`,
        lastModified: lastModified(entry.updated_at),
        changeFrequency,
        priority,
      }))
  })

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

  return [...staticPages, ...entityPages, ...blogPages, ...mixPages]
}
