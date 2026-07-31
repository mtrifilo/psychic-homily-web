/**
 * Sitemap index for the generateSitemaps() shards in app/sitemap.ts.
 *
 * With generateSitemaps(), Next serves children at /sitemap/{id}.xml but does
 * NOT emit an index document (measured on Next 16.1.4 — /sitemap.xml 404s, and
 * a route under app/sitemap.xml/ collides with the metadata [__metadata_id__]
 * matcher). This dedicated path is what robots.txt points crawlers at.
 *
 * Keep the id list in lockstep with generateSitemaps() via sitemap-shards.ts.
 */
import { FAMILY_SHARD_IDS, PAGES_SHARD_ID } from '../sitemap-shards'

const BASE_URL = 'https://psychichomily.com'

/**
 * How long a CDN may serve a cached copy of this index.
 *
 * Deliberately its OWN number, not shared with the shards' revalidate window.
 * This body is derived entirely from compile-time constants, so the only thing
 * that changes it is a deploy — what this value controls is how long a crawler
 * may keep missing a newly added shard, which has nothing to do with how fresh
 * the shard DATA is. Tying it to the shards' window would mean that raising
 * theirs to cut backend load silently delayed discovery of a new family here.
 */
const INDEX_CACHE_SECONDS = 3600

export async function GET() {
  const ids = [PAGES_SHARD_ID, ...FAMILY_SHARD_IDS]
  const body = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${ids
  .map(
    id => `  <sitemap>
    <loc>${BASE_URL}/sitemap/${id}.xml</loc>
  </sitemap>`
  )
  .join('\n')}
</sitemapindex>
`

  return new Response(body, {
    headers: {
      'Content-Type': 'application/xml; charset=utf-8',
      // Bounds how long the index may advertise a shard list older than the
      // deploy that produced it. Note the children do NOT share this header —
      // metadata routes emit their own `public, max-age=0, must-revalidate`,
      // so the index and its shards cache by different mechanisms.
      'Cache-Control': `public, max-age=0, s-maxage=${INDEX_CACHE_SECONDS}`,
    },
  })
}
