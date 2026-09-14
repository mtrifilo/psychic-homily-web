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
import {
  SHOWS_MONTHS_FIRST_SCREEN_URL,
  SHOW_CITIES_FIRST_SCREEN_URL,
  showsCalendarWindowFirstScreenKey,
  showsCalendarWindowFirstScreenUrl,
} from './api'
import {
  SHOWS_ROOT,
  calendarWindowLabel,
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
  // A month is a period one is IN and a day is one one is ON. The two windows
  // share every other part of this head, and the preposition is the only place
  // that difference has to show.
  const title = window.day === undefined ? `Shows in ${label}` : `Shows on ${label}`
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

  // A DAY inside a month that does have shows still has to have its own. The
  // histogram cannot answer that, so the window's own total does — which is
  // available on page 1, the only page of a day this route server-reads. A deep
  // page of an empty day renders the list's past-the-end state instead; it
  // carries this route's canonical back to the day root, which 404s.
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
  const heading =
    window.day === undefined ? `Shows in ${label}` : `Shows on ${label}`
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
      <h1 className="mb-6 text-2xl font-semibold tracking-tight">{heading}</h1>
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
