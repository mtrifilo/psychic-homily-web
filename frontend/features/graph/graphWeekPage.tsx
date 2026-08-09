import { cache } from 'react'
import type { Metadata } from 'next'

import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { SITE_URL } from '@/lib/seo/siteMetadata'

import { loadResolvedGraphWeek, type ResolvedGraphWeek } from './graphOverviewApi'
import {
  GRAPH_WEEK_PATH,
  formatGraphWeekRange,
  graphWeekKey,
  graphWeekSummary,
} from './graphWeek'

/**
 * `/graph/this-week` — the share surface for the weekly growth card (PSY-1738).
 *
 * A SHARE URL, not a content page. Its whole job is to carry the card as its
 * `og:image` and to hand whoever follows the link the same two numbers and a way
 * onto the map. `/graph`'s own OG is deliberately untouched (locked decision):
 * a weekly-changing share image on the map itself would change under everyone
 * who ever posted it, with no explicit share moment to justify it.
 *
 * This module is the SEO surface — the resolver and the metadata. The bodies it
 * describes live in `components/GraphWeekView`.
 */

/**
 * Resolve the week, memoised for the request.
 *
 * `cache` matters more here than on the scene pages it copies: `generateMetadata`
 * and the page body both need this, and without memoisation the snapshot would
 * be DECODED twice — a few thousand nodes and tens of thousands of CSR slots —
 * to render one page. The `fetch` itself is deduplicated by Next either way;
 * the decode is not.
 *
 * It memoises within a REQUEST SCOPE, which is why the unit test beside this
 * file deliberately does not assert the dedup: vitest has no such scope, so both
 * calls miss there and the number proves nothing about production.
 */
export const getGraphWeek = cache(
  (): Promise<ResolvedGraphWeek | null> => loadResolvedGraphWeek('graph-week-page')
)

export async function buildGraphWeekMetadata(): Promise<Metadata> {
  const view = await getGraphWeek()
  const canonical = `${SITE_URL}${GRAPH_WEEK_PATH}`

  if (!view) {
    return { title: 'This week in the graph', robots: { index: false, follow: false } }
  }

  const { week } = view
  const range = formatGraphWeekRange(week.start, week.end)
  // A middle dot rather than a dash: no em dashes in UI copy (project rule), and
  // it is the separator the card itself sets between its two counts.
  const title = `This week in the graph · ${range}`
  const description = graphWeekSummary(week)

  // The week rides in the QUERY STRING, and it is the only thing making this URL
  // vary. A file-convention OG route's URL carries a hash of the route source,
  // so it is a constant — while third-party unfurl caches (Facebook, Discord,
  // Slack) key on the image URL and hold it far longer than any `Cache-Control`
  // we set. Without this, the first scraper to see the card would pin that
  // week's image against this URL indefinitely. The scene cards solved the same
  // problem by advertising an archived permalink that carries the week; this
  // route has no archive, so the key goes here.
  //
  // Setting `images` explicitly suppresses the file convention, so the
  // dimensions and alt that convention would have supplied are given here.
  const ogImage = `${canonical}/opengraph-image?w=${graphWeekKey(week)}`

  return {
    title,
    description,
    alternates: { canonical },
    // NOINDEX, FOLLOW — the disclosed indexing posture for this route.
    //
    // The content changes every night, so an indexed snippet is stale by
    // definition; `/graph` is the surface that should rank, and it is reachable
    // from here, which is what `follow: true` preserves. A canonical pointing at
    // `/graph` was rejected instead: canonicalisation asserts DUPLICATE content,
    // and the interactive map is a different page, not another copy of this one.
    // The route is also absent from the sitemap for the same reason.
    //
    // This costs sharing nothing. Unfurlers read the OG tags directly and do not
    // consult `robots`, which is exactly the split this route wants.
    robots: { index: false, follow: true },
    openGraph: {
      title,
      description,
      url: canonical,
      type: 'website',
      images: [
        {
          url: ogImage,
          width: OG_SIZE.width,
          height: OG_SIZE.height,
          type: OG_CONTENT_TYPE,
          // The real numbers, which the route-level `alt` cannot carry — Next
          // requires that to be a constant, so it reads identically every night.
          alt: description,
        },
      ],
    },
    // `images` is deliberately absent: Next copies the openGraph descriptor
    // across when Twitter has none, so omitting it inherits the alt and the
    // dimensions. A bare URL string here would silently drop them.
    twitter: { card: 'summary_large_image', title, description },
  }
}
