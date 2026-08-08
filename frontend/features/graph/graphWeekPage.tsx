import { cache } from 'react'
import type { Metadata } from 'next'
import Link from 'next/link'
import { ArrowRight } from 'lucide-react'

import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { SITE_URL } from '@/lib/seo/siteMetadata'

import { fetchGraphOverview } from './graphOverviewApi'
import { buildSceneMap, type SceneMap } from './sceneMap'
import {
  GRAPH_WEEK_PATH,
  formatGraphWeekCounts,
  formatGraphWeekRange,
  graphWeekKey,
  graphWeekSummary,
  resolveGraphWeek,
  type GraphWeek,
} from './graphWeek'
import { buildGraphWeekMotif } from './graphWeekOgLayout'

/**
 * `/graph/this-week` — the share surface for the weekly growth card (PSY-1738).
 *
 * A SHARE URL, not a content page. Its whole job is to carry the card as its
 * `og:image` and to hand whoever follows the link the same two numbers and a way
 * onto the map. `/graph`'s own OG is deliberately untouched (locked decision):
 * a weekly-changing share image on the map itself would change under everyone
 * who ever posted it, with no explicit share moment to justify it.
 */

/** The teaser motif's canvas, in its own coordinate space. */
const TEASER_BOX = { x: 0, y: 0, width: 900, height: 380 } as const

/** What the page is about, resolved once per request. */
interface GraphWeekView {
  map: SceneMap
  week: GraphWeek
}

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
export const getGraphWeek = cache(async (): Promise<GraphWeekView | null> => {
  const overview = await fetchGraphOverview('graph-week-page')
  if (!overview) return null
  const map = buildSceneMap(overview)
  if (!map) return null
  const week = resolveGraphWeek(map)
  if (!week) return null
  return { map, week }
})

export async function buildGraphWeekMetadata(): Promise<Metadata> {
  const view = await getGraphWeek()
  const canonical = `${SITE_URL}${GRAPH_WEEK_PATH}`

  if (!view) {
    return { title: 'This week in the graph', robots: { index: false, follow: false } }
  }

  const { week } = view
  const range = formatGraphWeekRange(week.start, week.end)
  const title = `This week in the graph — ${range}`
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

/**
 * The page body: the two numbers, the week they cover, a teaser of the map with
 * the week's arrivals lit up, and the way onto the real thing.
 *
 * Deliberately thin. Someone arrives here from a link, reads one fact and
 * leaves for `/graph` — so this adds no navigation, no controls and no second
 * call to action (density philosophy). The card is what does the work; this is
 * the page that has to exist for the card to have a URL.
 */
export function GraphWeekContent({ view }: { view: GraphWeekView }) {
  const { map, week } = view
  const motif = buildGraphWeekMotif(map, week, TEASER_BOX)
  const range = formatGraphWeekRange(week.start, week.end)

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-10 sm:px-6 sm:py-14">
      <p className="font-mono text-[10px] uppercase tracking-[0.2em] text-primary">
        The Map of the Scene
      </p>
      <h1 className="mt-2 font-display text-4xl font-medium sm:text-5xl">
        This week in the graph
      </h1>
      <p className="mt-4 font-mono text-sm uppercase tracking-wider text-primary">
        {formatGraphWeekCounts(week)}
      </p>
      <p className="mt-1 font-mono text-xs uppercase tracking-wider text-muted-foreground">
        {range}
      </p>

      <TeaserMotif motif={motif} label={graphWeekSummary(week)} />

      <Link
        href="/graph"
        className="mt-6 inline-flex items-center gap-1.5 text-sm font-medium text-primary underline-offset-4 hover:underline"
      >
        Open the map of the scene
        <ArrowRight className="size-3.5" aria-hidden="true" />
      </Link>
    </div>
  )
}

/**
 * The same projection the card paints, drawn with theme tokens.
 *
 * A static `<svg>`, not the map canvas: this is a picture of a snapshot, and
 * mounting the interactive canvas here would ship the graph renderer to a page
 * whose only job is to be a link preview with a body. `role="img"` plus the
 * summary is what makes it mean anything without sight of it.
 */
function TeaserMotif({
  motif,
  label,
}: {
  motif: ReturnType<typeof buildGraphWeekMotif>
  label: string
}) {
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${TEASER_BOX.width} ${TEASER_BOX.height}`}
      className="mt-8 w-full rounded-xl border border-border/60 bg-card"
    >
      <g className="stroke-primary/50" strokeWidth={1.6}>
        {motif.connectors.map((line, index) => (
          <line key={index} x1={line.x1} y1={line.y1} x2={line.x2} y2={line.y2} />
        ))}
      </g>
      <g className="fill-muted-foreground/35">
        {motif.dots.map((dot, index) => (
          <circle key={index} cx={dot.x} cy={dot.y} r={2.6} />
        ))}
      </g>
      <g className="fill-primary">
        {motif.newDots.map((dot, index) => (
          <circle key={index} cx={dot.x} cy={dot.y} r={5} />
        ))}
      </g>
    </svg>
  )
}
