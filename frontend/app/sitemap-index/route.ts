/**
 * Sitemap index for the generateSitemaps() shards in app/sitemap.ts.
 *
 * With generateSitemaps(), Next serves children at /sitemap/{id}.xml but does
 * NOT emit an index document (measured on Next 16.1.4 — /sitemap.xml 404s, and
 * a route under app/sitemap.xml/ collides with the metadata [__metadata_id__]
 * matcher). This dedicated path is what robots.txt points crawlers at.
 *
 * The id list and the shard path shape both come from sitemap-shards.ts, so
 * this route cannot drift from generateSitemaps().
 */
import { ALL_SHARD_IDS, shardRoutePath } from '../sitemap-shards'

const BASE_URL = 'https://psychichomily.com'

export async function GET() {
  const body = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${ALL_SHARD_IDS.map(
  id => `  <sitemap>
    <loc>${BASE_URL}${shardRoutePath(id)}</loc>
  </sitemap>`
).join('\n')}
</sitemapindex>
`

  return new Response(body, {
    headers: {
      'Content-Type': 'application/xml; charset=utf-8',
      // Match the per-fetch revalidate window on the child shards so the index
      // does not outlive a family rename / addition by a week.
      'Cache-Control': 'public, max-age=0, s-maxage=3600',
    },
  })
}
