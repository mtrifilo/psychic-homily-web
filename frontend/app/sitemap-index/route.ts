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
import {
  ENTRY_REVALIDATE_SECONDS,
  FAMILY_SHARD_IDS,
  PAGES_SHARD_ID,
} from '../sitemap-shards'

const BASE_URL = 'https://psychichomily.com'

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
      // Match the revalidate window on the child shards so the index does not
      // outlive a family rename / addition by a week. Shared constant rather
      // than a literal: split across the two files, retuning one would silently
      // desync the index from the children it indexes.
      'Cache-Control': `public, max-age=0, s-maxage=${ENTRY_REVALIDATE_SECONDS}`,
    },
  })
}
