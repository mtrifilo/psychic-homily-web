import { cache } from 'react'
import type { Metadata } from 'next'
import { JsonLd } from '@/components/seo/JsonLd'
import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { SceneWeekView } from './components/SceneWeekView'
import { fetchSceneWeek } from './sceneWeekApi'
import { countShows, formatWeekRange, type SceneWeekResponse } from './sceneWeek'
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

  const total = countShows(data)
  const range = formatWeekRange(data.start_date, data.end_date)
  const title = `${data.scene_name} shows — ${range}`
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

  return {
    title,
    description,
    alternates: { canonical },
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
export function SceneWeekContent({ data }: { data: SceneWeekResponse }) {
  const { breadcrumb, itemList, events } = buildSceneWeekJsonLd(data)

  return (
    <>
      <JsonLd data={breadcrumb} />
      {itemList && <JsonLd data={itemList} />}
      {/* One array-valued script rather than a tag per show: a busy week is 50
          shows, and 50 extra <script> elements is markup weight for no gain —
          a top-level JSON-LD array carries the same graph. */}
      {events.length > 0 && <JsonLd data={events} />}
      <SceneWeekView week={data} />
    </>
  )
}
