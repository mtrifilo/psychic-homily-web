import { Suspense, cache } from 'react'
import { connection } from 'next/server'
import { HydrationBoundary } from '@tanstack/react-query'
// Imported by path rather than through the `@/features/scenes` barrel, which
// is a surface of client components (`SceneList`, `ScenePreviewPanel`, the
// Atlas globe). This is a server component that needs none of them, and the sibling
// page-level scene components — `SceneWeekView`, `SceneDayView` — are imported
// by path for the same reason. NOTE: maplibre is safe either way — AtlasGlobe
// dynamic-imports its canvas. The force graph is NOT: `ForceGraphView` was
// reachable through this barrel via SceneGraph until PSY-1772 removed that
// export. See features/scenes/components/index.ts before adding anything back.
import { ThisWeekByCity } from '@/features/scenes/components/ThisWeekByCity'
import {
  currentWeekBounds,
  SCENE_WEEK_INDEX_TIMEZONE,
} from '@/features/scenes/sceneWeek'
import type { SceneListResponse } from '@/features/scenes/types'
import { ShowList, ShowListSkeleton } from '@/features/shows'
import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOW_CITIES_FIRST_SCREEN_URL,
  SHOWS_CALENDAR_FIRST_SCREEN_KEY,
  SHOWS_CALENDAR_FIRST_SCREEN_URL,
  SHOWS_MONTHS_FIRST_SCREEN_KEY,
  SHOWS_MONTHS_FIRST_SCREEN_URL,
  showEndpoints,
} from '@/features/shows/api'
import type {
  ShowCitiesResponse,
  ShowMonthsResponse,
  ShowsCalendarResponse,
  UpcomingShowsResponse,
} from '@/features/shows/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { API_ENDPOINTS } from '@/lib/api'
import { BUILD_TIME_API_FETCH_TIMEOUT_MS } from '@/lib/build-time-api'
import { showsFirstScreenSeeds } from '@/features/shows/firstScreen'
import { seedFirstScreen } from '@/lib/query-hydration'
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'

export const metadata = {
  title: 'Upcoming Shows',
  description: 'Discover upcoming live music shows in Phoenix and beyond.',
  alternates: {
    canonical: 'https://psychichomily.com/shows',
  },
  openGraph: {
    title: 'Upcoming Shows | Psychic Homily',
    description: 'Discover upcoming live music shows in Phoenix and beyond.',
    url: '/shows',
    type: 'website',
  },
}

interface ShowListItem {
  slug?: string
  title: string
  artists: Array<{ name: string; is_headliner?: boolean | null }>
  venues: Array<{ name: string }>
}

/**
 * Explicit because the implicit value was a surprise. Sending no `limit` let
 * `GET /shows/upcoming` apply its `default:"50"` — the tightest bound of the
 * three SEO lists, arrived at by accident and written down nowhere.
 *
 * 50 is what this call has always effectively sent, so stating it changes no
 * output. It is NOT an argued number, and it is measurably short: production
 * still reports `has_more: true` at the endpoint's `maximum` of 200
 * (2026-07-29), so even the largest single request would not cover the
 * catalogue — full coverage needs the cursor. How many entries an SEO
 * `ItemList` should carry is a product question, and it is deliberately not
 * being settled here by nudging a number. It is left visible instead of
 * invisible.
 *
 * It bounds the `ItemList` ONLY. The server-rendered first screen is sized by
 * `SHOWS_PAGE_SIZE`, which `ShowList` and its seed both state explicitly, and
 * it is fetched separately (see `HydratedShowList`) precisely so the two are
 * not coupled. The two numbers are equal today and answer different questions:
 * how many entries a crawler is offered, and how many rows a reader gets per
 * page. Raising this changes the first and nothing about what a reader sees.
 */
export const UPCOMING_SHOWS_LIMIT = 50

/**
 * Data Cache exposure, measured against production 2026-08-08 (PSY-1674), when
 * `/artists` was found 206% over the 2 MB cache-item cap and silently uncached.
 *
 * ALL FOUR cached fetches this page makes, not just the ItemList one — they are
 * four separate Data Cache entries with four separate budgets, and what bounds
 * each differs:
 *
 *   fetch                      raw      base64   % cap   bounded by
 *   -------------------------  -------  -------  ------  --------------------
 *   /shows/upcoming?limit=50    80,327  107,104    5.1%  UPCOMING_SHOWS_LIMIT
 *   /shows/calendar?limit=50   INFERRED           ~5%    SHOWS_PAGE_SIZE
 *   /shows/months                    -       -      -    one row per month
 *   /shows/cities                8,948   11,932    0.6%  one row per city
 *   /scenes                      7,256    9,676    0.5%  UNBOUNDED
 *
 * The calendar row is INFERRED, not measured: it is the same 50 rows of the
 * same venue-local partition as the cursor read above it (the backend states
 * that an unwindowed offset page matches row for row), but its envelope drops
 * `pagination` and adds six scalars, so the byte count differs by a little.
 * `/shows/months` is one small object per month and was never near the cap.
 *
 * WHICH ROWS THE BUILD CAN FAIL ON is narrower than this table: the check only
 * judges what a build actually writes, and the three fetches in
 * `HydratedShowList` reach `await connection()` inside `seedFirstScreen`, the
 * same reason `/scenes` is called out below. Re-measure a row rather than
 * quoting this table if a field is added to the show response. (The `/shows/upcoming` response echoes a `timezone` parameter the
 * backend ignores. `/shows/cities` has no such field. Nothing consumes it
 * either way; show times render from each venue's own zone.)
 *
 * Both show fetches state their bound in THIS repo, in the constants named
 * above. `/scenes` has no bound at all; it is the same unbounded-list shape that
 * blew up `GET /artists`, and is only small because scenes are few. It also runs
 * behind `await connection()`, so it is request-time only: NEITHER half of
 * lib/data-cache-budget can fail a build on it, and a Sentry report after the
 * fact is the whole signal.
 *
 * Re-measure if any of those bounds move, or if a field is added to the show
 * response — the row count is not what blew the budget on /artists, the fields
 * were.
 */

/**
 * The `ItemList` read of the upcoming list.
 *
 * A SEPARATE call from the first-screen seed below, and it keeps the longer
 * `BUILD_TIME_API_FETCH_TIMEOUT_MS` budget: giving up early on this one costs
 * the page its structured data, while the seed only costs a visitor the
 * server-rendered rows the component would fetch for itself anyway.
 *
 * OPEN QUESTION, do not build on either answer without re-measuring. That split
 * was argued on this call running in the prerendered shell and the seed running
 * at request time. A `next build` against a reachable backend (2026-09-13) puts
 * `/shows` at Partial Prerender, and its prerendered `shows.html` is 9.9 KB
 * carrying NEITHER this `ItemList` nor the list's rows. Both therefore appear
 * to stream, and the render-pass half of that argument is unverified. The
 * budgets themselves are still the right way round on the costs above.
 *
 * `React.cache` does not bridge the two: under `cacheComponents` a shell and a
 * postponed resume are different render passes, so a `cache()` entry made in
 * one is not visible in the other. What dedupes a repeated URL is Next's Data
 * Cache, and only when the URLs match. Keeping the two calls on DIFFERENT URLs
 * is therefore what keeps their budgets separate.
 *
 * It reads the CURSOR endpoint while the list below reads the offset one. The
 * two share one predicate set and one ordering, and the backend pins their
 * agreement with an integration test,
 * `TestGetUpcomingShowsPage_NoWindowMatchesTheCursorList`, so this block
 * advertises the rows the page renders.
 * Both are 50 rows; the bounds are separate constants because they answer
 * separate questions: how many entries a crawler is offered, and how many rows
 * a reader gets per page. Two Data Cache entries, invalidated together by
 * `lib/proxy-revalidation.ts`.
 */
const getUpcomingShowsPayload = cache(() =>
  fetchListPayload<UpcomingShowsResponse>({
    url: `${showEndpoints.UPCOMING}?limit=${UPCOMING_SHOWS_LIMIT}`,
    collection: 'shows',
    service: 'shows-listing',
    timeoutMs: BUILD_TIME_API_FETCH_TIMEOUT_MS,
  })
)

/** The `ItemList` rows. `null` (a failed fetch) yields no block, as before. */
export async function getUpcomingShows(): Promise<ShowListItem[]> {
  const payload = await getUpcomingShowsPayload()
  return (payload?.shows as ShowListItem[] | undefined) ?? []
}

function getShowName(show: ShowListItem): string {
  const headliner = show.artists?.find(a => a.is_headliner)?.name
    || show.artists?.[0]?.name
    || 'Live Music'
  return show.title || `${headliner} at ${show.venues?.[0]?.name || 'TBA'}`
}

/**
 * The pager's month histogram, seeded so it is not a third client call.
 *
 * TAKES THE DEFAULT HOUR, and the shorter window its payload argues for was
 * measured and rejected rather than overlooked. It is scoped to CALENDAR
 * MONTHS, the shape `fetchListPayload` says should shorten its revalidate and
 * the scene-week block below does shorten. Setting 60s here moved the ROUTE's
 * own revalidate from 1h to 1m, measured on a `next build` 2026-09-13, which is
 * sixty times the regeneration on the site's busiest page. That is a cost
 * decision rather than a cleanup. (The scene-week block sets 60s without that
 * effect; why the two differ is the same render-pass question the OPEN QUESTION
 * above leaves open, so it is not restated here as a mechanism.)
 *
 * What the hour exposes is bounded: the seeded labels can name a month that has
 * ended, in the SERVER-RENDERED paint only. `seedFirstScreen` stamps every seed
 * `updatedAt: 0`, so this query is stale on mount and refetches immediately;
 * the 60s `staleTime` on `useShowMonths` bounds later refetches, not that one.
 */
export function getShowsMonthsPayload(): Promise<ShowMonthsResponse | null> {
  return fetchListPayload<ShowMonthsResponse>({
    url: SHOWS_MONTHS_FIRST_SCREEN_URL,
    collection: 'months',
    service: 'shows-months-first-screen',
  })
}

/**
 * Seed the two cache entries `ShowList` blocks its first paint on, the first
 * page of upcoming shows and the city facet counts, so dates, artists, venues
 * and cities reach the server HTML (PSY-1624).
 *
 * BOTH are required: `ShowList` returns its skeleton while EITHER query is
 * still loading, so seeding the rows alone server-renders the skeleton.
 *
 * The rows are a SEPARATE fetch from the `ItemList`'s `getUpcomingShowsPayload`
 * above, and against a different endpoint: the OFFSET reader the list pages
 * with, rather than the cursor one the `ItemList` reads. That is deliberate on
 * both counts and the reasoning is on `getUpcomingShowsPayload`: they need
 * different abort budgets, and this one requests exactly what the client hook
 * requests. The seed lands by KEY either way; matching the URL is what keeps
 * `SHOWS_CALENDAR_FIRST_SCREEN_URL` an honest description of the hook's
 * request. Two Data Cache entries, invalidated together. Do not "dedupe" them
 * onto one call without reading that block first.
 *
 * PAGE 1 only. `?page=2` and beyond are client-fetched: a long tail of
 * addresses, none of which a cold visitor or a crawler lands on.
 *
 * A failed fetch renders `<ShowList />` unseeded rather than throwing; the
 * component fetches for itself and owns the error state (see
 * `fetchListPayload`).
 */
async function HydratedShowList() {
  const [shows, cities, months] = await Promise.all([
    fetchListPayload<ShowsCalendarResponse>({
      url: SHOWS_CALENDAR_FIRST_SCREEN_URL,
      collection: 'shows',
      service: 'shows-first-screen',
    }),
    fetchListPayload<ShowCitiesResponse>({
      url: SHOW_CITIES_FIRST_SCREEN_URL,
      collection: 'cities',
      service: 'show-cities-first-screen',
    }),
    // The pager's page labels. Fetched here because it is otherwise a THIRD
    // client call on this route, against a per-IP budget everyone behind one
    // address shares, on a response no shared cache absorbs.
    //
    // Not a gate on the first paint: a `null` leaves the list rendering bare
    // numerals, and the seed below is skipped rather than suppressed. The one
    // failure it does NOT absorb is a Data Cache budget overrun, which
    // `fetchListPayload` rethrows on purpose. That rejection escapes this
    // `Promise.all` and takes the whole subtree, which is what a build-time
    // budget gate is for.
    //
    // The response is `private, max-age=60` because its handler consults
    // `upcomingListIncludesNonApproved`, and this seeds it into a SHARED Data
    // Cache and a shared render. That is sound only while all three hold: the
    // route is registered off the optional-auth group so the admin branch is
    // unreachable, this fetch forwards no credentials, and the key below
    // carries no viewer segment. A change to any one of them has to move the
    // other two.
    getShowsMonthsPayload(),
  ])

  const seeds = showsFirstScreenSeeds({
    shows,
    cities,
    months,
    calendarKey: SHOWS_CALENDAR_FIRST_SCREEN_KEY,
  })

  if (!seeds) {
    return <ShowList />
  }

  const dehydratedState = await seedFirstScreen(seeds)

  return (
    <HydrationBoundary state={dehydratedState}>
      <ShowList />
    </HydrationBoundary>
  )
}

/**
 * How long the by-city block's scene payload stays warm.
 *
 * A minute, against the hour every other first-screen fetch takes, because this
 * payload is scoped to a CALENDAR WEEK rather than to "recent". At any moment
 * except the Monday rollover the two behave the same; at the rollover a stale
 * entry stops being slightly old and starts describing the WRONG WEEK — every
 * row reporting last week's total beside a link that now serves this week's,
 * under a heading naming this week. That is the same lie this ticket removed
 * from the counts themselves, so it is not one to reintroduce through the cache.
 *
 * A minute is a bound, not a fix: the block can still be one minute wrong at the
 * boundary. Fixing it outright means serving the bounds alongside the counts and
 * rendering the heading from the payload, which is a design change (per-row
 * ranges) rather than a data one. `/scenes` is a small, cheap, unpaginated
 * response, so 60 refreshes an hour on one route is not a load concern.
 */
const SCENE_WEEK_INDEX_REVALIDATE_SECONDS = 60

/**
 * The scene-week index under the show list (PSY-1623).
 *
 * `/scenes/{slug}/week` is the best server-rendered answer this site has to
 * "what is on in my city this week", and before this block nothing linked to
 * it — not `/shows`, not `/scenes`, not even a scene's own page. `/shows` is
 * where a crawler following the top nav lands, so it is the edge that matters.
 *
 * Request-time rather than prerendered, and not by choice: naming the current
 * week means reading the clock, which `cacheComponents` refuses in a
 * prerenderable scope for the same reason it refuses a baked timestamp — a
 * cached shell would go on advertising last week's dates. `connection()` moves
 * this subtree into the dynamic resume, where the read is legitimate. It is
 * still one HTTP response, so the links are in the HTML a `curl` receives. The
 * payload underneath is Data-Cached, so this is a request-time render rather
 * than a request-time fetch, and the page already streams a subtree anyway.
 *
 * A failed fetch renders nothing. The block is supplementary to the list above
 * it, and `fetchListPayload` returns `null` precisely so a caller can drop a
 * section instead of turning an API blip into an error page.
 */
export function getScenesForWeekIndex(): Promise<SceneListResponse | null> {
  return fetchListPayload<SceneListResponse>({
    url: API_ENDPOINTS.SCENES.LIST,
    collection: 'scenes',
    service: 'shows-this-week-by-city',
    revalidateSeconds: SCENE_WEEK_INDEX_REVALIDATE_SECONDS,
  })
}

async function HydratedThisWeekByCity() {
  await connection()

  const payload = await getScenesForWeekIndex()

  if (!payload?.scenes?.length) {
    return null
  }

  const { start, end } = currentWeekBounds(new Date(), SCENE_WEEK_INDEX_TIMEZONE)

  return (
    <ThisWeekByCity scenes={payload.scenes} weekStart={start} weekEnd={end} />
  )
}

export default async function ShowsPage() {
  const shows = await getUpcomingShows()

  const showsWithSlugs = shows.filter(
    (s): s is ShowListItem & { slug: string } => !!s.slug
  )

  return (
    <>
      {showsWithSlugs.length > 0 && (
        <JsonLd data={generateItemListSchema({
          name: 'Upcoming Shows',
          description: 'Upcoming live music shows in Phoenix and beyond.',
          listItems: showsWithSlugs.map(show => ({
            url: `https://psychichomily.com/shows/${show.slug}`,
            name: getShowName(show),
          })),
        })} />
      )}
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Upcoming Shows', url: 'https://psychichomily.com/shows' },
      ])} />
      <div className="w-full max-w-6xl mx-auto px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8 leading-9">Upcoming Shows</h1>
        <Suspense fallback={<ShowListSkeleton />}>
          <HydratedShowList />
        </Suspense>
        {/* No fallback: the block carries no reader-facing state worth
            reserving space for, and a placeholder that resolves to `null` on a
            failed fetch would be a shape that promises content it may not
            have. */}
        <Suspense fallback={null}>
          <HydratedThisWeekByCity />
        </Suspense>
      </div>
    </>
  )
}
