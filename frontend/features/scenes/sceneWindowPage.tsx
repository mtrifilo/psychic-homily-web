import { cache } from 'react'
import type { Metadata } from 'next'
import { JsonLd } from '@/components/seo/JsonLd'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { SceneWindowView } from './components/SceneWindowView'
import { resolveShowTimezone } from '@/lib/utils/formatters'
import { calendarDateInZone, sceneTonightDate } from './sceneCalendar'
import { fetchSceneWeekChain } from './sceneWindowApi'
import { buildSceneWindowJsonLd } from './sceneWindowJsonLd'
import {
  NEXT_4_WEEKS_DAYS,
  NEXT_4_WEEKS_FETCH_WEEKS,
  SCENE_WINDOW_LABEL,
  SCENE_WINDOW_ROW_CAP,
  capWindowRows,
  flattenWeekDays,
  formatWindowRange,
  rollingDays,
  sceneWindowHref,
  weekendDays,
  type SceneWindowData,
  type SceneWindowKey,
} from './sceneWindow'

/** The two windows this module serves. `/tonight` and `/week` have their own. */
export type MultiDayWindow = Extract<SceneWindowKey, 'this-weekend' | 'next-4-weeks'>

/** How many week payloads each window needs to cover its span. */
const WEEKS_TO_FETCH: Record<MultiDayWindow, number> = {
  // The locked weekend is Friday, Saturday and Sunday — the last three days of
  // the Monday-anchored current week, so one payload holds all of it.
  'this-weekend': 1,
  'next-4-weeks': NEXT_4_WEEKS_FETCH_WEEKS,
}

/**
 * ONE clock read per request, shared by `generateMetadata` and the page body.
 *
 * Load-bearing for more than tidiness. `getSceneWindow` is `React.cache`d on its
 * arguments, and two `new Date()` calls are two distinct objects — so reading
 * the clock at each call site would miss the cache and fetch every week payload
 * twice per request. It would also let the two disagree across a midnight or
 * 6am boundary, publishing a title for one window above the rows of another.
 *
 * Callers must `await connection()` first: under `cacheComponents` reading the
 * clock is only legal at request time.
 */
export const getRequestNow = cache(() => new Date())

/** The window one step wider, offered when this one is empty. */
const WIDER_WINDOW: Record<MultiDayWindow, SceneWindowKey | null> = {
  'this-weekend': 'next-4-weeks',
  // Nothing in the family is wider than four weeks.
  'next-4-weeks': null,
}

/**
 * Resolve one window into everything its page renders.
 *
 * Wrapped in `React.cache` so `generateMetadata` and the page body share one
 * set of trips to the API per request, matching the rest of this feature.
 *
 * `now` is a parameter rather than a `new Date()` inside so the window bounds
 * can be tested against a pinned clock. Callers read the clock at REQUEST time,
 * behind `await connection()` — under `cacheComponents` that is the only legal
 * place to read it, and a window derived from build time would freeze whichever
 * weekend happened to be current when the route was compiled.
 */
export const getSceneWindow = cache(
  async (
    slug: string,
    window: MultiDayWindow,
    now: Date
  ): Promise<SceneWindowData | null> => {
    const weeks = await fetchSceneWeekChain(slug, WEEKS_TO_FETCH[window])
    if (!weeks || weeks.length === 0) return null

    const first = weeks[0]
    // The zone the backend resolved the week in — never the reader's. A viewer
    // in Berlin and one in Chicago must be shown the same Chicago weekend.
    //
    // NULL when the backend could not name it. Carried nullable rather than
    // filled in, because the one consumer of this field is the JSON-LD, where a
    // fallback zone would compose a UTC offset out of a guess.
    const timezone = first.timezone
    // The 6am night boundary, mirrored from the backend so this page and
    // `/tonight` cannot disagree about which date "tonight" names.
    //
    // Bucketing is a DATE question, so it reads on `resolveShowTimezone`'s
    // answer rather than on the nullable published zone: a fallback day is the
    // best available answer, and it is the same day every other surface files
    // this scene's nights under. Only a CLOCK is refused on the fallback.
    //
    // The trailing UTC date is defence only: `resolveShowTimezone` answers with
    // a zone `Intl` accepts in every branch. It is UTC's date rather than the
    // week's Monday because anchoring on the Monday would put up to six
    // already-finished nights at the top of a window whose label promises the
    // ones ahead.
    const bucketZone = resolveShowTimezone(first.state, timezone)
    const tonight = sceneTonightDate(now, bucketZone) ?? calendarDateInZone(now, 'UTC')

    const all = flattenWeekDays(weeks)
    const scoped =
      window === 'this-weekend'
        ? // Days already past are dropped for the same reason the four-week
          // window drops them: a weekend viewed on Sunday has two nights behind
          // it, and listing them under a header promising "this weekend" would
          // describe a backward stretch of time with a forward label.
          //
          // Between midnight and 6am on Monday the two authorities disagree by
          // design and the result is the one a reader wants: the backend has
          // already rolled to the new week, while `tonight` still names Sunday
          // under the night boundary. Every day of the NEW weekend is later than
          // that Sunday, so all three survive and the page shows the weekend
          // ahead rather than a weekend that has entirely finished.
          rollingDays(weekendDays(all), tonight, 3)
        : rollingDays(all, tonight, NEXT_4_WEEKS_DAYS)

    // A weekend shows all three of its nights, empty ones included — three bare
    // rules is what tells a reader we checked, and it is the convention `/week`
    // already sets for a seven-day window. Four weeks does the opposite: 28
    // headings mostly reading `0` buries the nights that have something, so that
    // window renders only the dates it can answer for (the scene page's own
    // four-week calendar made the same call).
    const listed =
      window === 'this-weekend' ? scoped : scoped.filter(d => (d.shows ?? []).length > 0)

    const { days, rendered, truncated } = capWindowRows(listed, SCENE_WINDOW_ROW_CAP)

    return {
      window,
      slug: first.slug,
      sceneName: first.scene_name,
      city: first.city,
      state: first.state,
      timezone,
      days,
      rendered,
      truncated,
      trackedVenues: (first.tracked_venues ?? []).map(v => ({
        name: v.name,
        slug: v.slug,
      })),
      widerWindow: WIDER_WINDOW[window],
    }
  }
)

export async function buildSceneWindowMetadata(
  slug: string,
  window: MultiDayWindow,
  now: Date
): Promise<Metadata> {
  const data = await getSceneWindow(slug, window, now)
  const label = SCENE_WINDOW_LABEL[window]
  if (!data) {
    return { title: `${label} not found`, robots: { index: false, follow: false } }
  }

  const range = formatWindowRange(data.days)
  const title = range
    ? `${data.sceneName} shows — ${label.toLowerCase()}, ${range}`
    : `${data.sceneName} shows — ${label.toLowerCase()}`
  const description =
    data.rendered > 0
      ? `${data.rendered} ${data.rendered === 1 ? 'show' : 'shows'} at the ${data.city} rooms we track, ${label.toLowerCase()}.`
      : `No shows at the ${data.city} rooms we track ${label.toLowerCase()}.`

  // These windows are ROLLING and, unlike `/week`, have no dated permalink to
  // defer to — so each is its own canonical. Deliberately NOT canonicalised to
  // the week: they bound different stretches of time, and pointing two distinct
  // windows at one URL is what made "this weekend" and "this week" the same
  // page in the first place (PSY-1849).
  const canonical = `${SITE_URL}${sceneWindowHref(data.slug, window)}`

  return {
    title,
    description,
    alternates: { canonical },
    // No `images`: the OG card family is keyed on a DATED permalink so a new
    // period is a new image URL (unfurl caches key on the URL forever). A
    // rolling window has no such key, so advertising a card here would pin
    // whichever weekend a scraper saw first. Adding one means giving these
    // windows dated permalinks, which is a separate decision.
    openGraph: { title, description, url: canonical, type: 'website' },
    twitter: { card: 'summary_large_image', title, description },
  }
}

/**
 * Render a window that has ALREADY been fetched.
 *
 * Deliberately does not fetch or call `notFound()` itself: `notFound()` must be
 * invoked from the page component so Next.js sets a real 404 status. Calling it
 * from a helper in another module rendered the not-found BODY but left the
 * response at HTTP 200 — which search engines and link unfurlers read as a
 * valid page (PSY-906).
 */
export function SceneWindowContent({ data }: { data: SceneWindowData }) {
  const { breadcrumb, itemList, events } = buildSceneWindowJsonLd(data)

  return (
    <>
      <JsonLd data={breadcrumb} />
      {itemList && <JsonLd data={itemList} />}
      {/* One array-valued script rather than a tag per show: a busy four weeks
          is dozens of shows, and that many extra <script> elements is markup
          weight for no gain — a top-level JSON-LD array carries the same graph. */}
      {events.length > 0 && <JsonLd data={events} />}
      <SceneWindowView data={data} />
    </>
  )
}
