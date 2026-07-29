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
 * build and then frozen. That is not hypothetical — the served sitemap sat at
 * 2,520 artists while the API held 3,591, and at 114 shows while the API held
 * 3,498, because nothing ever re-rendered it. A per-fetch `revalidate` alone
 * did not save us, so the route-level window is what the freshness guarantee
 * actually rests on. Verify after any Next upgrade that the served counts still
 * move without a redeploy.
 */
export const revalidate = 3600

type SitemapEntries = components['schemas']['SitemapEntries']
type SitemapEntry = components['schemas']['SitemapEntry']

/**
 * Generous, and deliberately not the shared `createBuildTimeApiSignal()` 10s
 * budget.
 *
 * That helper is what broke the sitemap: `/shows` grew past 10s, every fetch
 * aborted, the abort was swallowed, and an empty document shipped. The feed
 * this route now calls is a projection that answers in well under a second, so
 * the ceiling exists only to stop a wedged backend hanging the render forever —
 * and unlike before, hitting it throws rather than yielding a partial sitemap.
 */
const ENTRY_FETCH_TIMEOUT_MS = 30_000

/**
 * Fetch the indexable slug set.
 *
 * Throws on any failure, by design. A sitemap missing an entity family is
 * indistinguishable from that family being legitimately empty, so publishing a
 * partial document silently drops thousands of URLs out of the index with no
 * failure signal anywhere. Throwing fails the render and leaves the last good
 * sitemap in place: stale beats empty for a crawler, every time.
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
 * `lastModified` drives `<lastmod>`. An unparseable timestamp would emit an
 * invalid date into the XML, so omit it rather than poison the entry — a
 * missing `<lastmod>` is a weaker signal, not a broken document.
 */
function lastModified(updatedAt: string | undefined): Date | undefined {
  if (!updatedAt) return undefined
  const parsed = new Date(updatedAt)
  return Number.isNaN(parsed.getTime()) ? undefined : parsed
}

function entriesToSitemap(
  entries: SitemapEntry[] | null | undefined,
  pathPrefix: string,
  changeFrequency: MetadataRoute.Sitemap[number]['changeFrequency'],
  priority: number
): MetadataRoute.Sitemap {
  return (entries ?? [])
    .filter(entry => entry.slug)
    .map(entry => ({
      url: `${BASE_URL}${pathPrefix}/${entry.slug}`,
      lastModified: lastModified(entry.updated_at),
      changeFrequency,
      priority,
    }))
}

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const entries = await fetchSitemapEntries()

  // Static pages. `lastModified` is deliberately absent: stamping these with
  // `new Date()` on every render told crawlers the whole site changed every
  // time, which devalues the signal for the entries that genuinely did change.
  const staticPages: MetadataRoute.Sitemap = [
    { url: BASE_URL, changeFrequency: 'daily', priority: 1 },
    { url: `${BASE_URL}/shows`, changeFrequency: 'daily', priority: 0.9 },
    { url: `${BASE_URL}/venues`, changeFrequency: 'weekly', priority: 0.8 },
    { url: `${BASE_URL}/blog`, changeFrequency: 'weekly', priority: 0.8 },
    { url: `${BASE_URL}/dj-sets`, changeFrequency: 'weekly', priority: 0.7 },
    { url: `${BASE_URL}/privacy`, changeFrequency: 'monthly', priority: 0.3 },
    { url: `${BASE_URL}/terms`, changeFrequency: 'monthly', priority: 0.3 },
  ]

  const showPages = entriesToSitemap(entries.shows, '/shows', 'weekly', 0.8)
  const venuePages = entriesToSitemap(entries.venues, '/venues', 'monthly', 0.6)
  const artistPages = entriesToSitemap(entries.artists, '/artists', 'monthly', 0.6)

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

  return [
    ...staticPages,
    ...showPages,
    ...venuePages,
    ...artistPages,
    ...blogPages,
    ...mixPages,
  ]
}
