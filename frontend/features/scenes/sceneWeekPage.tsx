import { cache } from 'react'
import type { Metadata } from 'next'
import { JsonLd } from '@/components/seo/JsonLd'
import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { SceneWeekView } from './components/SceneWeekView'
import { fetchSceneWeek } from './sceneWeekApi'
import { countShows, formatWeekRange, type SceneWeekResponse } from './sceneWeek'
import { sceneWeekTitle } from './sceneWindow'
import { buildSceneWeekJsonLd } from './sceneWeekJsonLd'

/**
 * Fetch one scene-week for the page.
 *
 * Wrapped in `React.cache` so `generateMetadata` and the page body share one
 * trip to the API per request, matching the existing scene-page pattern
 * (PSY-906). The wrapper stays here rather than in `sceneWeekApi` because
 * `React.cache` is server-component-only — the share card, which renders on the
 * edge, calls the fetch directly.
 */
export const getSceneWeek = cache(
  (slug: string, week?: string): Promise<SceneWeekResponse | null> =>
    fetchSceneWeek(slug, week, 'scene-week')
)

export async function buildSceneWeekMetadata(
  slug: string,
  week?: string
): Promise<Metadata> {
  const data = await getSceneWeek(slug, week)
  if (!data) {
    return { title: 'Week not found', robots: { index: false, follow: false } }
  }

  // The ABSENT `week` argument is what names the rolling route, and it is the
  // discriminator for everything below that speaks about now.
  const isRollingRoute = week === undefined
  const total = countShows(data)
  const range = formatWeekRange(data.start_date, data.end_date)
  // The family's one title rule, so the tab and the H1 read alike. A dated
  // permalink names its own Monday rather than saying "this week".
  const title = sceneWeekTitle(data.start_date, data.city, isRollingRoute)
  const description =
    total > 0
      ? `${total} ${total === 1 ? 'show' : 'shows'} at the ${data.city} rooms we track, ${range}.`
      : `No shows at the ${data.city} rooms we track, ${range}.`

  // The archived week is the canonical URL even when reached via the rolling
  // /week route: the rolling URL's content changes weekly, so pointing search
  // engines at it would make every indexed snippet go stale.
  const canonical = `${SITE_URL}/scenes/${data.slug}/${data.iso_week}`

  // Both routes advertise the ARCHIVED card, and that is deliberate.
  //
  // Next would otherwise inject each route's own file-convention image, and the
  // rolling route's URL is a constant — it carries a hash of the route source,
  // not of the week. Our own `Cache-Control` cannot help: Facebook, Discord and
  // Slack cache an unfurled image against its URL for far longer than any header
  // we set, so the rolling URL — the one people actually post — would keep
  // showing whichever week that scraper happened to see first. The archived
  // URL carries the week, so a new week is a new image.
  //
  // Setting `images` explicitly suppresses the file convention, so the
  // dimensions and alt that convention would have supplied are given here.
  const ogImage = `${canonical}/opengraph-image`

  // The page description already names the city, the count and the week, which
  // is exactly what the card shows — and it beats the route-level `alt`, which
  // Next requires to be a constant and so reads identically on every card.
  const imageAlt = description

  // A week with nothing on it is thin content: real, worth serving, worth
  // linking out of, not worth an index entry. `follow` stays on precisely
  // because the page's job in that state is to point at the rooms and the
  // neighbouring weeks. Same rule and same shape as the empty night the day
  // builder suppresses.
  //
  // THREE conditions, and each one is load-bearing:
  //
  //  - the DATED route, because the rolling `/week` declares this dated
  //    permalink as its canonical, and a noindex beside a canonical naming a
  //    different URL is a contradiction search engines resolve by consolidating
  //    the suppression onto the target;
  //  - zero shows, not `is_past_week`: `scene_weeks` in the sitemap announces
  //    the last eight weeks per scene and excludes only the weeks with no
  //    approved show, so noindexing every past week would mark submitted URLs
  //    noindex. The pages suppressed here are the ones it never names;
  //  - NOT the current week, which is the URL both rolling routes canonicalise
  //    to: `/week` here and `/tonight` in the day builder. Suppressing it
  //    would consolidate onto those two, which are a quiet scene's whole
  //    discovery surface. A thin current week therefore stays indexable, and
  //    turns into an archived one that does not the following Monday.
  const robots =
    !isRollingRoute && total === 0 && !data.is_current_week
      ? { index: false, follow: true }
      : undefined

  return {
    title,
    description,
    alternates: { canonical },
    ...(robots ? { robots } : {}),
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
          alt: imageAlt,
        },
      ],
    },
    // `images` is deliberately absent: Next copies the openGraph descriptor
    // across when Twitter has none, so omitting it inherits the alt and
    // dimensions. Setting a bare URL string here would silently drop them.
    twitter: { card: 'summary_large_image', title, description },
  }
}

/**
 * Render a week that has ALREADY been fetched.
 *
 * Deliberately does not fetch or call `notFound()` itself: `notFound()` must be
 * invoked from the page component so Next.js sets a real 404 status. Calling it
 * from a helper in another module rendered the not-found BODY but left the
 * response at HTTP 200 — which search engines and link unfurlers read as a
 * valid page. Verified against the existing scene page, which 404s correctly
 * with the decision in the page body (PSY-906).
 */
export function SceneWeekContent({
  data,
  isRollingRoute,
}: {
  data: SceneWeekResponse
  /** True on `/scenes/{slug}/week`. See `SceneWeekView`. */
  isRollingRoute: boolean
}) {
  const { breadcrumb, itemList, events } = buildSceneWeekJsonLd(data)

  return (
    <>
      <JsonLd data={breadcrumb} />
      {itemList && <JsonLd data={itemList} />}
      {/* One array-valued script rather than a tag per show: a busy week is 50
          shows, and 50 extra <script> elements is markup weight for no gain —
          a top-level JSON-LD array carries the same graph. */}
      {events.length > 0 && <JsonLd data={events} />}
      <SceneWeekView week={data} isRollingRoute={isRollingRoute} />
    </>
  )
}
