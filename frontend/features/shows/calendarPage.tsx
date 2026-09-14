/**
 * `/shows/{yyyy}/{mm}` and `/shows/{yyyy}/{mm}/{dd}` — the upcoming list scoped
 * to one venue-local calendar month or day, served as a crawlable document.
 *
 * WHY A PATH SEGMENT rather than `?month=` on `/shows`: a month is the identity
 * of a result set, not a slice of one. It has to be bookmarkable, linkable from
 * the strip, and announceable in the sitemap, and a query facet is none of
 * those under the site's canonicalize-to-root pagination policy — every
 * `?month=` would collapse onto `/shows` and no month would be indexable.
 *
 * `?page=` inside a window stays a QUERY and stays client-side, and
 * `buildShowsCalendarMetadata` is reached from `params` alone — which is what
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
import { Suspense } from 'react'
import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
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

/** The window's own URL, absolute. Its canonical, and the crumb it links. */
function windowUrl(window: ShowsCalendarWindow): string {
  return listRootCanonical(showsWindowPath(window))
}

/**
 * Metadata for one window.
 *
 * The canonical is SELF-referencing and always the window root, so a `?page=2`
 * of this month canonicalizes here rather than declaring itself — the site's
 * pagination policy (`listRootCanonical`), and it holds because nothing in this
 * function can see the page number.
 *
 * It reads NOTHING. A month with no shows is a not-found page, and the route
 * body is where that is decided; emitting `noindex` here would mean a second
 * read of the same histogram in the head, on every request, to restate a
 * verdict the body already reaches. Next stamps the not-found response
 * `noindex` on its own.
 */
export function buildShowsCalendarMetadata(
  window: ShowsCalendarWindow
): Metadata {
  const label = calendarWindowLabel(window)
  const title = showsCalendarWindowTitle(window)
  const description =
    window.day === undefined
      ? `Every upcoming show we have on record in ${label}.`
      : `Every upcoming show we have on record on ${label}.`
  const canonical = windowUrl(window)

  return {
    title,
    description,
    alternates: { canonical },
    openGraph: { title, description, url: canonical, type: 'website' },
  }
}

/**
 * Page 1's rows, or null when this URL is not asking for page 1.
 *
 * `ShowList` reads `?page=` for itself, and a seed attaches to whatever key is
 * current — so seeding page 2 with page 1's slice would look like a cache hit
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
  return fetchListPayload<ShowsCalendarResponse>({
    url: showsCalendarWindowFirstScreenUrl(window),
    collection: 'shows',
    service: 'shows-calendar-window-first-screen',
  })
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
 * A `notFound()` here renders the not-found BODY. Under `cacheComponents` the
 * shell has already streamed by the time these reads resolve, so the status on
 * that response is 200 with the `noindex` Next injects, not 404 — the same
 * soft-404 the venue year archive's in-page `notFound()` paths carry, and the
 * reason `proxy.ts` decides the SHAPE of these URLs before the render starts.
 * Shape is all the proxy can decide without a backend probe; membership is a
 * data question and it is answered here.
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

  // WHICH MONTHS ARE DOCUMENTS is asked of the histogram, which is the same
  // source the strip links from and the `shows_months` sitemap family is
  // projected from — so the set announced, the set that renders and the set the
  // strip offers cannot drift apart. It is page-INDEPENDENT, which is what
  // makes `?page=2` of a dead month a not-found too, and it is the whole of the
  // past-month rule: the histogram covers the UPCOMING partition, so a month
  // that has ended is simply not in it. No redirect, by decision.
  //
  // 404 only on a POSITIVE absence. A read that FAILED is not an answer, and
  // treating it as one turns a backend blip into a not-found body for every
  // month on the site.
  //
  // Both reads are UNFILTERED. A month that exists but holds nothing for the
  // reader's own city filter renders the list's zero-result state, with its
  // filter suggestions; a 404 there would be a claim about the catalogue rather
  // than about the filter.
  const monthIsAddressable =
    months === null ||
    months.months.some(
      bucket => bucket.year === window.year && bucket.month === window.month
    )
  if (!monthIsAddressable) {
    notFound()
  }

  // A WINDOW the read answered for, with nothing in it.
  //
  // For a DAY this is the only gate there is: a day inside a month that does
  // have shows still has to have its own, and the histogram buckets months.
  // For a MONTH it is a second opinion the histogram has already given, and it
  // fires only if the two disagree about what "upcoming" means — in which case
  // the month page renders no rows, so a not-found is the honest answer.
  //
  // It needs the window's own total, which is read on page 1 and skipped on
  // every other. A deep page of an empty day therefore renders the list's
  // past-the-end state instead; it carries this route's canonical back to the
  // day root, which is the URL that 404s.
  if (shows && shows.total === 0) {
    notFound()
  }

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
          { name: label, url: windowUrl(window) },
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
): Metadata {
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
