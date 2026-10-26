import { cache } from 'react'
import type { Metadata } from 'next'
import { JsonLd } from '@/components/seo/JsonLd'
import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { SceneDayView } from './components/SceneDayView'
import { plural } from './sceneCalendar'
import { fetchSceneDay } from './sceneDayApi'
import { dayShows, formatDayFull, type SceneDayResponse } from './sceneDay'
import { looksLikeISOWeek } from './sceneWeek'
import { sceneDayTitle, sceneWindowHref } from './sceneWindow'
import { buildSceneDayJsonLd } from './sceneDayJsonLd'

/**
 * Fetch one scene-day for the page.
 *
 * Wrapped in `React.cache` so `generateMetadata` and the page body share one
 * trip to the API per request, matching the scene-week and scene-detail pattern.
 */
export const getSceneDay = cache(
  (slug: string, date?: string): Promise<SceneDayResponse | null> => fetchSceneDay(slug, date)
)

/**
 * The meta description. On the rolling route it opens with the page's title
 * phrase (`Live shows in Chicago tonight: ...`), the words the reader searched
 * for, and the count and the date follow it. A dated permalink states the count
 * and the date, prefixed with "tonight" when the payload says that date is the
 * live night.
 */
function dayDescription(day: SceneDayResponse, title: string, isRollingRoute: boolean): string {
  const total = dayShows(day).length
  const date = formatDayFull(day.date)
  // The rolling title already says "tonight", so its tail names the date only.
  const lead = isRollingRoute ? `${title}: ` : ''
  const when = isRollingRoute ? `, ${date}` : day.is_tonight ? ` tonight, ${date}` : ` ${date}`
  if (total === 0) {
    // Never "no shows in {city}": we know our own calendar, not the city's,
    // and "listed" is what keeps the difference visible in a snippet read out
    // of context.
    return `${lead}0 shows listed for the ${day.city} rooms we track${when}. A room may have a show we haven't listed.`
  }
  return `${lead}${plural(total, 'show')} at the ${day.city} rooms we track${when}.`
}

export async function buildSceneDayMetadata(slug: string, date?: string): Promise<Metadata> {
  const day = await getSceneDay(slug, date)
  if (!day) {
    return { title: 'Day not found', robots: { index: false, follow: false } }
  }

  // The ABSENT `date` argument is what names the rolling route, and it is the
  // discriminator everything on this page keys off. Not `day.is_tonight`: that
  // flag is also true for a dated permalink naming today, and that permalink
  // is a different URL with its own canonical, title and robots rule.
  const isRollingRoute = date === undefined
  // The same rule the H1 renders, from the same function, so the tab and the
  // top of the page cannot drift. The full date, and the year the `{MON D}`
  // form drops, stay in the description.
  const title = sceneDayTitle(day.date, day.city, isRollingRoute)
  const description = dayDescription(day, title, isRollingRoute)

  // A day that cannot name its own page. With either field blank the URLs below
  // collapse to `/scenes//` shapes and are offered to crawlers as this page's
  // identity, which is worse than offering nothing. Checked here, on the values
  // this function actually interpolates, rather than trusted from upstream.
  if (!day.slug || !day.date) {
    return { title, description, robots: { index: false, follow: false } }
  }

  const dayPermalink = `${SITE_URL}/scenes/${day.slug}/${day.date}`

  // Every day route is its own canonical. /tonight is the scene's standing
  // answer to "what is on tonight", the URL search results send that query to,
  // and the scene_periods sitemap family announces it with a daily lastmod so
  // its snippet is refreshed as the night changes. A dated permalink names one
  // night forever.
  //
  // Day permalinks are announced in no sitemap (see SITEMAP_FAMILIES in
  // app/sitemap-shards.ts) and stay reachable through the prev/next chips on
  // every day page, which are real anchors. The JSON-LD breadcrumb also names
  // the day, but that is structured data, not a crawl edge, and the VISIBLE
  // breadcrumb stops at the scene.
  const canonical = isRollingRoute
    ? `${SITE_URL}${sceneWindowHref(day.slug, 'tonight')}`
    : dayPermalink

  // A dated night with nothing on it is thin content: real, worth serving,
  // worth linking out of, not worth an index entry. `follow` stays on precisely
  // because the page's job in that state is to point at the week and the rooms.
  //
  // /tonight stays indexable whatever its count. It is the URL the scene's
  // tonight searches land on, the sitemap announces it every day, and a quiet
  // night's noindex would drop it from the index until a crawler next saw a
  // busy one.
  const robots =
    !isRollingRoute && dayShows(day).length === 0
      ? { index: false, follow: true }
      : undefined

  // The rolling /tonight URL's file-convention image is a constant (hash of the
  // route source), so pointing scrapers at `tonight/opengraph-image` would pin
  // whichever week they first saw. The family has one renderer, and the dated
  // permalink's file-convention image 404s a date key, so the URL that actually
  // varies AND serves a card is the archived week image, keyed by the night's
  // `iso_week`.
  //
  // `day.iso_week` comes from the PAYLOAD and is never derived here. "Tonight"
  // is resolved by the backend in the scene's timezone against a 6am night
  // boundary, so at 01:30 on a Monday tonight is still Sunday, the last day of
  // the PREVIOUS ISO week. A week computed from a clock on this side would put
  // the card a week ahead on exactly that boundary.
  //
  // Setting `images` explicitly also suppresses the `[period]` convention on
  // the dated permalink, which would otherwise advertise that 404. The
  // site-wide jpg stays on dated nights and on a payload that cannot name its
  // week.
  const ogImage =
    isRollingRoute && looksLikeISOWeek(day.iso_week)
      ? {
          url: `${SITE_URL}/scenes/${day.slug}/${day.iso_week}/opengraph-image`,
          width: OG_SIZE.width,
          height: OG_SIZE.height,
          type: OG_CONTENT_TYPE,
          alt: description,
        }
      : { url: '/og-image.jpg', width: OG_SIZE.width, height: OG_SIZE.height, alt: 'Psychic Homily' }

  return {
    title,
    description,
    alternates: { canonical },
    ...(robots ? { robots } : {}),
    openGraph: {
      title,
      description,
      // The DAY permalink on both routes, so on /tonight deliberately NOT the
      // canonical. og:url is a share object's identity, not a
      // duplicate-consolidation hint: Facebook and Discord cache one unfurled
      // object per og:url. /tonight's own URL would make every night's title,
      // description and card one cached object, and whichever night a scraper
      // saw first would win for all of them. The dated permalink names exactly
      // the night these tags describe, and it is the URL the share control
      // hands out.
      url: dayPermalink,
      type: 'website',
      images: [ogImage],
    },
    // `images` is deliberately absent: Next copies the openGraph descriptor
    // across when Twitter has none, so omitting it inherits the alt and
    // dimensions. Setting a bare URL string here would silently drop them.
    twitter: { card: 'summary_large_image', title, description },
  }
}

/**
 * Render a day that has ALREADY been fetched.
 *
 * Deliberately does not fetch or call `notFound()` itself: `notFound()` must be
 * invoked from the page component so Next.js sets a real 404 status. Calling it
 * from a helper in another module renders the not-found BODY but leaves the
 * response at HTTP 200 — which search engines and link unfurlers read as a
 * valid page.
 */
export function SceneDayContent({
  data,
  isRollingRoute,
}: {
  data: SceneDayResponse
  /** True on `/scenes/{slug}/tonight`. See `SceneDayView`. */
  isRollingRoute: boolean
}) {
  const { breadcrumb, itemList, events } = buildSceneDayJsonLd(data)

  return (
    <>
      <JsonLd data={breadcrumb} />
      {itemList && <JsonLd data={itemList} />}
      {/* One array-valued script rather than a tag per show: a top-level
          JSON-LD array carries the same graph without N extra elements. */}
      {events.length > 0 && <JsonLd data={events} />}
      <SceneDayView day={data} isRollingRoute={isRollingRoute} />
    </>
  )
}
