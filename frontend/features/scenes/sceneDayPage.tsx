import { cache } from 'react'
import type { Metadata } from 'next'
import { JsonLd } from '@/components/seo/JsonLd'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { SceneDayView } from './components/SceneDayView'
import { fetchSceneDay } from './sceneDayApi'
import { dayShows, formatDayFull, type SceneDayResponse } from './sceneDay'
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
 * The page title, in the words someone would actually type.
 *
 * "Phoenix Shows Tonight" is the query; the date follows it so a result that
 * outlives the night still says which night it was. A date that is NOT the
 * scene's current night drops the word — the dated permalink is a permanent URL
 * and calling an archived Tuesday "tonight" would be false the day after it was
 * written.
 */
function dayTitle(day: SceneDayResponse): string {
  const date = formatDayFull(day.date)
  return day.is_tonight
    ? `${day.city} Shows Tonight — ${date}`
    : `${day.city} Shows — ${date}`
}

function dayDescription(day: SceneDayResponse): string {
  const total = dayShows(day).length
  const date = formatDayFull(day.date)
  const when = day.is_tonight ? `tonight, ${date}` : date
  if (total === 0) {
    // Never "no shows in {city}" — we know our own calendar, not the city's.
    return `Nothing on our calendar for the ${day.city} rooms we track ${when}.`
  }
  return `${total} ${total === 1 ? 'show' : 'shows'} at the ${day.city} rooms we track ${when}.`
}

export async function buildSceneDayMetadata(slug: string, date?: string): Promise<Metadata> {
  const day = await getSceneDay(slug, date)
  if (!day) {
    return { title: 'Day not found', robots: { index: false, follow: false } }
  }

  const title = dayTitle(day)
  const description = dayDescription(day)

  // The DATED permalink is the canonical URL even when this renders at the
  // rolling /tonight URL: that URL's content changes every night, so pointing
  // search engines at it would leave every indexed snippet describing a night
  // that has passed.
  const canonical = `${SITE_URL}/scenes/${day.slug}/${day.date}`

  // A night with nothing on it is thin content — real, worth serving, worth
  // linking out of, not worth an index entry. `follow` stays on precisely
  // because the page's job in that state is to point at the week and the rooms.
  const robots =
    dayShows(day).length === 0 ? { index: false, follow: true } : undefined

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
      // Set explicitly to suppress the file-convention image the sibling
      // `[key]` route contributes: that route's `opengraph-image` renders the
      // WEEK card and answers 404 for a date key, so inheriting it would
      // advertise a card that does not exist. The site-wide card is a plain
      // truth about the site rather than a wrong claim about this night.
      images: [{ url: '/og-image.jpg', width: 1200, height: 630, alt: 'Psychic Homily' }],
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
export function SceneDayContent({ data }: { data: SceneDayResponse }) {
  const { breadcrumb, itemList, events } = buildSceneDayJsonLd(data)

  return (
    <>
      <JsonLd data={breadcrumb} />
      {itemList && <JsonLd data={itemList} />}
      {/* One array-valued script rather than a tag per show: a top-level
          JSON-LD array carries the same graph without N extra elements. */}
      {events.length > 0 && <JsonLd data={events} />}
      <SceneDayView day={data} />
    </>
  )
}
