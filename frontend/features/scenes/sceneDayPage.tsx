import { cache } from 'react'
import type { Metadata } from 'next'
import { JsonLd } from '@/components/seo/JsonLd'
import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { SceneDayView } from './components/SceneDayView'
import { fetchSceneDay } from './sceneDayApi'
import { dayShows, formatDayFull, type SceneDayResponse } from './sceneDay'
import { looksLikeISOWeek } from './sceneWeek'
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
    // Names the count and the city, like the populated form — but never "no
    // shows in {city}". We know our own calendar, not the city's, and "listed"
    // is what keeps the difference visible in a snippet read out of context.
    return `0 shows listed for the ${day.city} rooms we track ${when}. A room may have a show we haven't listed.`
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

  // A payload that cannot name its own page. `asPayload` in scenePeriodApi.ts
  // only checks that each required field is a STRING, and `prev_date`/`next_date`
  // in that same list are empty BY DESIGN at the window edges, so an empty
  // string is a shape this guard demonstrably lets through. With an empty slug
  // or date the URLs below would collapse to `/scenes//` shapes and be offered
  // to crawlers as this page's identity, which is worse than offering nothing.
  //
  // (The narrow fix would be to reject empty strings at the boundary itself,
  // for the fields that are never legitimately empty. That validator is shared
  // with the week and OG-image surfaces, so it is left for its own change.)
  if (!day.slug || !day.date) {
    return { title, description, robots: { index: false, follow: false } }
  }

  const dayPermalink = `${SITE_URL}/scenes/${day.slug}/${day.date}`

  // The rolling /tonight URL canonicalizes to the scene's WEEK permalink; a
  // DATED permalink stays its own canonical.
  //
  // /tonight cannot be its own canonical, because its content changes every
  // night and an indexed snippet would describe a night that has passed. It
  // names the week rather than the dated permalink because of what the sitemap
  // carries: SITEMAP_FAMILIES in app/sitemap-shards.ts has scene_weeks and no
  // scene_days. Neither URL space is infinite — days are bounded by
  // `dateIsServable` (2015..next year, ~4.7k per scene) and week keys are not
  // bounded at all by the week route — but the scene_weeks family announces a
  // rolling 8-week window, and there is no day family at any size. The week is
  // simply the only one of the two the site tells crawlers about.
  //
  // Day permalinks stay reachable regardless: the prev/next chips on every day
  // page are real anchors. The JSON-LD breadcrumb also names the day, but that
  // is structured data, not a crawl edge, and the VISIBLE breadcrumb stops at
  // the scene. If those chips ever become non-anchors, day permalinks are
  // orphaned and this decision needs revisiting.
  //
  // `day.iso_week` comes from the PAYLOAD and is never derived here. "Tonight"
  // is resolved by the backend in the scene's timezone against a 6am night
  // boundary, so at 01:30 on a Monday tonight is still Sunday, the last day of
  // the PREVIOUS ISO week. A week computed from a clock on this side would skip
  // the scene forward a week on exactly that boundary.
  //
  // The discriminator is the ABSENT `date` argument, not `day.is_tonight`: that
  // flag is also true for a dated permalink naming today, and that permalink
  // must keep pointing at itself rather than be folded into the week.
  // (SceneDayView's "full week" chip deliberately still keys off `is_tonight`.
  // It is answering "which week link helps a reader here", not "which URL is
  // this page", so the two are allowed to differ.)
  const isRollingRoute = date === undefined
  const canonical =
    isRollingRoute && day.iso_week
      ? `${SITE_URL}/scenes/${day.slug}/${day.iso_week}`
      : dayPermalink

  // A night with nothing on it is thin content — real, worth serving, worth
  // linking out of, not worth an index entry. `follow` stays on precisely
  // because the page's job in that state is to point at the week and the rooms.
  //
  // Only on the DATED route, which is its own canonical. A noindex emitted
  // beside a canonical naming a DIFFERENT URL is a contradiction search engines
  // are documented to resolve by consolidating the suppression onto the target.
  // /tonight needs no noindex anyway: declaring another URL as its canonical is
  // already what keeps it from being indexed as itself.
  //
  // KNOWN GAP, accepted here rather than papered over. The sitemap emits a week
  // only when that week has at least one approved show, so on a scene whose
  // whole current week is quiet, /tonight consolidates onto a week page that is
  // thin, announced nowhere, and carries no noindex of its own (see
  // buildSceneWeekMetadata, which never sets robots in any state). Closing it
  // means either a per-week show count on the day payload or dropping the
  // zero-show exclusion for the current week — both backend changes, which this
  // change is scoped out of.
  const robots =
    !isRollingRoute && dayShows(day).length === 0
      ? { index: false, follow: true }
      : undefined

  // The rolling /tonight URL is a constant (hash of the route source), so
  // pointing scrapers at `tonight/opengraph-image` would pin whichever week
  // they first saw. The family has one renderer, and the dated permalink's
  // file-convention image 404s a date key, so the URL that actually varies
  // AND serves a card is the archived week image, keyed by the night's
  // `iso_week` from the payload — the same week this page canonicalizes to,
  // including Monday before 6am. Setting `images` explicitly also suppresses
  // the `[period]` convention on the dated permalink, which would otherwise
  // advertise that 404. The site-wide jpg stays on dated nights and on a
  // payload that cannot name its week.
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
      // The DAY permalink, deliberately NOT the canonical. og:url is a share
      // object's identity, not a duplicate-consolidation hint: Facebook and
      // Discord cache one unfurled object per og:url. Pointing /tonight's at
      // the week permalink would make this night's title, description and card
      // collide with the week page's, which declares that same og:url with its
      // own generated card, and whichever a scraper saw first would win for
      // both. The dated permalink names exactly the night these tags describe.
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
