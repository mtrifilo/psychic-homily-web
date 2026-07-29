import { MetadataRoute } from 'next'
import { getBlogSlugs, getBlogPost, getMixSlugs, getMix } from '@/features/blog'
import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import type { components } from '@/types/api'

const BASE_URL = 'https://psychichomily.com'

/**
 * Re-render window for the sitemap itself.
 *
 * Set explicitly, and load-bearing: without it this route is generated once at
 * build and then frozen, which is half of why the served sitemap went stale
 * (the other half was a fetch that failed open — see the backend's
 * contracts.SitemapEntry). A per-fetch `revalidate` alone demonstrably did not
 * keep this route re-rendering, so the route-level window is what the freshness
 * guarantee rests on. Re-verify after any Next upgrade that the served counts
 * move without a redeploy.
 */
export const revalidate = 3600

type SitemapEntries = components['schemas']['SitemapEntries']

/** The entity families, minus the `$schema` key Huma adds to every response. */
type Family = Exclude<keyof SitemapEntries, '$schema'>

/**
 * How each family maps onto a URL.
 *
 * Typed as a total `Record<Family, …>` on purpose: when the backend adds a
 * family to `SitemapEntries`, this object stops compiling until it is mapped
 * here. Without that, a new family would be fetched, ignored, and silently
 * absent from the XML — with nothing to notice it. PSY-1622 adds six families
 * at once, which is exactly when a silent omission would slip through.
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
 * Generous, and deliberately not the shared `createBuildTimeApiSignal()` 10s
 * budget — that budget is what the old generator silently blew. The projection
 * feed answers in well under a second, so this ceiling exists only to stop a
 * wedged backend hanging the render forever, and hitting it throws rather than
 * yielding a partial sitemap.
 */
const ENTRY_FETCH_TIMEOUT_MS = 30_000

/**
 * Fetch the indexable slug set.
 *
 * Throws on any failure, by design: publishing a document that is missing an
 * entity family drops thousands of URLs out of the index with no failure signal
 * anywhere. Failing the render leaves the last good sitemap in place — stale
 * beats empty for a crawler.
 */
async function fetchSitemapEntries(): Promise<SitemapEntries> {
  try {
    const res = await fetch(`${API_BASE_URL}/sitemap/entries`, {
      next: { revalidate },
      signal: AbortSignal.timeout(ENTRY_FETCH_TIMEOUT_MS),
    })
    if (!res.ok) {
      throw new Error(`sitemap entries fetch returned ${res.status}`)
    }
    return await res.json()
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
    return (entries[family] ?? [])
      .filter(entry => entry.slug)
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
