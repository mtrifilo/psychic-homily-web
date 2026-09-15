/**
 * `/shows/{yyyy}/{mm}` and `/shows/{yyyy}/{mm}/{dd}`, the upcoming list scoped
 * to one venue-local calendar month or day, served as a crawlable document.
 *
 * WHY A PATH SEGMENT rather than `?month=` on `/shows`: a month is the identity
 * of a result set, not a slice of one. It has to be bookmarkable, linkable from
 * the strip, and announceable in the sitemap, and a query facet is none of
 * those under the site's canonicalize-to-root pagination policy, every
 * `?month=` would collapse onto `/shows` and no month would be indexable.
 *
 * `?page=` inside a window stays a QUERY and stays client-side, and
 * `buildShowsCalendarMetadata` is reached from `params` alone, which is what
 * makes every page of a window canonicalize to the window root structurally
 * rather than by remembering to. The venue year archive settled this shape; see
 * `features/venues/yearArchivePage.tsx`.
 *
 * KNOWN LIMIT of the params-only head, stated because it is load-bearing and
 * invisible: the city and the page count cannot appear in `<title>`. The city a
 * reader sees is derived per-viewer inside `ShowList` (favourites, then IP geo,
 * then the `?cities=` param) and the page count needs the filtered total, so
 * naming either in the head means reading `searchParams` there, which costs the
 * route its prerendered shell AND gives each `?page=` its own canonical. The
 * document names the window; the page names the city, beside the `<h1>`.
 */
import { cache, Suspense } from 'react'
import type { Metadata } from 'next'
import { HydrationBoundary } from '@tanstack/react-query'
import { JsonLd } from '@/components/seo/JsonLd'
import { Breadcrumb } from '@/components/shared'
import { SITE_URL, listRootCanonical } from '@/lib/seo/siteMetadata'
import { generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { seedFirstScreen } from '@/lib/query-hydration'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'
import { archiveIsFirstPage } from './showArchive.server'
import { showsFirstScreenSeeds } from './firstScreen'
import { ShowList } from './components/ShowList'
import { ShowListSkeleton } from './components/ShowListSkeleton'
import {
  SHOWS_MONTHS_FIRST_SCREEN_URL,
  SHOW_CITIES_FIRST_SCREEN_URL,
  showsCalendarWindowFirstScreenKey,
  showsCalendarWindowFirstScreenUrl,
} from './api'
import {
  SHOWS_ROOT,
  calendarWindowLabel,
  showsCalendarWindowTitle,
  showsWindowHref,
  showsWindowPath,
  type ShowsCalendarWindow,
} from './showsCalendarRoute'
import type {
  ShowCitiesResponse,
  ShowMonthsResponse,
  ShowsCalendarResponse,
} from './types'

/** The route's search params, un-awaited. Awaited in exactly one place below. */
export type ShowsCalendarSearchParams = Promise<
  Record<string, string | string[] | undefined>
>

/**
 * The window's CANONICAL, absolute. A run canonicalizes to its anchor day, which
 * is what `showsWindowPath` returns for one.
 */
function windowUrl(window: ShowsCalendarWindow): string {
  return listRootCanonical(showsWindowPath(window))
}

/**
 * The window's OWN address, absolute, which for a run carries its `?days=`.
 *
 * The crumb links this rather than the canonical: a breadcrumb names where the
 * reader is, and for a run that is the span, not the day it opens on.
 */
function windowSelfUrl(window: ShowsCalendarWindow): string {
  return `${SITE_URL}${showsWindowHref(window)}`
}

/**
 * Page 1 of a window, read at most once per request.
 *
 * `React.cache` so the head and the body share ONE trip to the API, the same
 * wrapper the scene pages use for the same reason. The key is the URL rather
 * than the window, so the two callers cannot miss each other by building the
 * same request two ways.
 */
const readWindowFirstScreen = cache(
  (url: string): Promise<ShowsCalendarResponse | null> =>
    fetchListPayload<ShowsCalendarResponse>({
      url,
      collection: 'shows',
      service: 'shows-calendar-window-first-screen',
    })
)

function readWindowPage(
  window: ShowsCalendarWindow
): Promise<ShowsCalendarResponse | null> {
  return readWindowFirstScreen(showsCalendarWindowFirstScreenUrl(window))
}

/**
 * Metadata for one window.
 *
 * The canonical is SELF-referencing and always the window root, so a `?page=2`
 * of this month canonicalizes here rather than declaring itself, the site's
 * pagination policy (`listRootCanonical`), and it holds because nothing in this
 * function can see the page number.
 *
 * It reads the window's own total, and that is the ONE thing it reads. The read
 * is shared with the body, so a page-1 request pays for it once; a deep page
 * pays for it alone, which is the cost of a suppression verdict that is the
 * same on every page of a window. A window's total is page-independent, so
 * nothing here needs the page number to reach the verdict.
 */
export async function buildShowsCalendarMetadata(
  window: ShowsCalendarWindow
): Promise<Metadata> {
  const label = calendarWindowLabel(window)
  const title = showsCalendarWindowTitle(window)
  const description = `Every upcoming show we have on record ${windowPreposition(window)} ${label}.`
  const canonical = windowUrl(window)

  // A RUN is a relative window resolved to an absolute anchor: "this weekend"
  // means a different three days every week, so the URL a reader shares is
  // worth keeping and the page behind it is not worth an index entry. It is
  // already canonical to the day root, which is the identity that IS indexed;
  // `follow` because every row on it links somewhere that should be crawled.
  //
  // A QUIET window is suppressed on the same terms and for the same reason the
  // scene pages suppress an empty night: real, worth serving, worth linking out
  // of, not worth an index entry. Outside the addressable span there is no page
  // at all, and `proxy.ts` answers those with a status before this runs.
  //
  // Only a POSITIVE zero suppresses. A read that FAILED is not an answer, and
  // treating it as one would noindex every window on the site during a backend
  // blip.
  const page = await readWindowPage(window)
  const suppress = window.days !== undefined || page?.total === 0

  return {
    title,
    description,
    alternates: { canonical },
    ...(suppress ? { robots: { index: false, follow: true } } : {}),
    openGraph: { title, description, url: canonical, type: 'website' },
  }
}

/**
 * The preposition a window takes in a sentence: `in` a month, `on` a day, `from`
 * one date to another. The same three the title rule uses, so the description
 * and the title cannot read a window two ways.
 */
function windowPreposition(window: ShowsCalendarWindow): string {
  if (window.day === undefined) return 'in'
  return window.days === undefined ? 'on' : 'from'
}

/**
 * Page 1's rows, or null when this URL is not asking for page 1.
 *
 * `ShowList` reads `?page=` for itself, and a seed attaches to whatever key is
 * current, so seeding page 2 with page 1's slice would look like a cache hit
 * and never correct itself. An async function rather than an inline branch so
 * the caller can hand it straight to `Promise.all`: awaiting `searchParams`
 * first and only then deciding would put the row read BEHIND the other two
 * rather than beside them.
 *
 * `archiveIsFirstPage` is the shared derivation, built on the same nuqs parser
 * the list reads the URL with, so the two cannot disagree about which URLs are
 * page 1.
 */
async function readSeedableWindowPage(
  window: ShowsCalendarWindow,
  searchParams: ShowsCalendarSearchParams
): Promise<ShowsCalendarResponse | null> {
  if (!archiveIsFirstPage(await searchParams)) return null
  return readWindowPage(window)
}

/**
 * The window body, and the only component on these routes that reads anything.
 * The route file owns the page container; this owns the document inside it.
 *
 * ONE async component under ONE Suspense boundary, rather than the page body
 * doing the reads. That is what lets `?page=` be read at all: `searchParams` in
 * the page body would make the whole route dynamic and cost it its prerendered
 * shell. The route files keep only the path-segment validation, which needs no
 * network.
 *
 * All three reads start TOGETHER; none takes an input from another.
 *
 * NOTHING here produces a not-found. Under `cacheComponents` the shell has
 * already streamed by the time these reads resolve, so a `notFound()` would
 * commit a 404 BODY at HTTP 200; whether these URLs exist at all is decided in
 * `proxy.ts`, which runs before the render and can still set a status. What is
 * left here is a window that EXISTS, so every state it can be in is a page: a
 * month or day inside the addressable span with nothing on renders the list's
 * own quiet state, keeping the chips, the month axis and the filters that are
 * the way out of it.
 */
export async function ShowsCalendarContent({
  window,
  searchParams,
}: {
  window: ShowsCalendarWindow
  searchParams: ShowsCalendarSearchParams
}) {
  const [shows, cities, months] = await Promise.all([
    readSeedableWindowPage(window, searchParams),
    fetchListPayload<ShowCitiesResponse>({
      url: SHOW_CITIES_FIRST_SCREEN_URL,
      collection: 'cities',
      service: 'show-cities-first-screen',
    }),
    fetchListPayload<ShowMonthsResponse>({
      url: SHOWS_MONTHS_FIRST_SCREEN_URL,
      collection: 'months',
      service: 'shows-months-first-screen',
    }),
  ])

  const seeds = showsFirstScreenSeeds({
    shows,
    cities,
    months,
    calendarKey: showsCalendarWindowFirstScreenKey(window),
  })

  const label = calendarWindowLabel(window)
  const list = <ShowList window={window} />

  return (
    <>
      <JsonLd
        data={generateBreadcrumbSchema([
          { name: 'Home', url: SITE_URL },
          { name: 'Shows', url: `${SITE_URL}${SHOWS_ROOT}` },
          { name: label, url: windowSelfUrl(window) },
        ])}
      />
      <Breadcrumb
        fallback={{ href: SHOWS_ROOT, label: 'Shows' }}
        currentPage={label}
      />
      {/* The same string the `<title>` carries, from the one function that
          builds it, so the document cannot name itself two ways. */}
      <h1 className="mb-6 text-2xl font-semibold tracking-tight">
        {showsCalendarWindowTitle(window)}
      </h1>
      {seeds ? (
        <HydrationBoundary state={await seedFirstScreen(seeds)}>
          {list}
        </HydrationBoundary>
      ) : (
        list
      )}
    </>
  )
}

/**
 * The head for a window route, or the not-found head when the segments could
 * not be one.
 *
 * Both route files reach it, so the `noindex` a malformed address carries is
 * stated once. `window` is already parsed when it arrives: parsing is the
 * route file's own job, and the only one it has.
 */
export function showsCalendarRouteMetadata(
  window: ShowsCalendarWindow | null
): Promise<Metadata> | Metadata {
  if (window === null) {
    return { title: 'Shows not found', robots: { index: false, follow: false } }
  }
  return buildShowsCalendarMetadata(window)
}

/**
 * The page shell both window routes render.
 *
 * ONE Suspense boundary over the only component that reads anything, and the
 * page container outside it so the skeleton sits where the rows will. The
 * fallback is the list's own skeleton rather than `null`: the route body
 * returns as soon as `params` resolves, so this boundary owns the whole wait.
 */
export function ShowsCalendarRoute({
  window,
  searchParams,
}: {
  window: ShowsCalendarWindow
  searchParams: ShowsCalendarSearchParams
}) {
  return (
    <div className="w-full max-w-6xl mx-auto px-4 py-8 md:px-8">
      <Suspense fallback={<ShowListSkeleton />}>
        <ShowsCalendarContent window={window} searchParams={searchParams} />
      </Suspense>
    </div>
  )
}
