import { Suspense, cache } from 'react'
import { connection } from 'next/server'
import { HydrationBoundary } from '@tanstack/react-query'
// Imported by path rather than through the `@/features/scenes` barrel, which
// is a surface of client components (`SceneList`, `ScenePulseCard`, the Atlas
// globe). This is a server component that needs none of them, and the sibling
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
  UPCOMING_SHOWS_FIRST_SCREEN_KEY,
  UPCOMING_SHOWS_FIRST_SCREEN_URL,
} from '@/features/shows/api'
import type { ShowCitiesResponse, UpcomingShowsResponse } from '@/features/shows/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { API_ENDPOINTS } from '@/lib/api'
import { BUILD_TIME_API_FETCH_TIMEOUT_MS } from '@/lib/build-time-api'
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
 * what `ShowList` itself requests, which is the endpoint's own default, and it
 * is fetched separately (see `HydratedShowList`) precisely so the two are not
 * coupled. Raising this changes how many entries a crawler is offered and
 * nothing about what a reader sees.
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
 *   /shows/upcoming?limit=50    80,327  107,104    5.1%  this file's limit
 *   /shows/upcoming             80,327  107,104    5.1%  backend default:"50"
 *   /shows/cities                8,948   11,932    0.6%  one row per city
 *   /scenes                      7,256    9,676    0.5%  UNBOUNDED
 *
 * The first three URLs have since lost an inert `timezone`. That re-keyed their
 * Data Cache entries but cannot have moved the sizes, so the measurement stands.
 *
 * So "the limit protects it" is true of the ItemList fetch only. The seed URL
 * deliberately omits `limit` (see the note above) and is held at 50 by the
 * backend's `default:"50"` — a bound that lives in another repo layer and could
 * be raised without anyone reading this file. `/scenes` has no bound at all; it
 * is the same unbounded-list shape that blew up `GET /artists`, and is only
 * small because scenes are few. It also runs behind `await connection()`, so it
 * is request-time only: NEITHER half of lib/data-cache-budget can fail a build
 * on it, and a Sentry report after the fact is the whole signal.
 *
 * Re-measure if any of those bounds move, or if a field is added to the show
 * response — the row count is not what blew the budget on /artists, the fields
 * were.
 */

/**
 * The `ItemList` read of `/shows/upcoming`.
 *
 * Separate from the first-screen seed below, and the split is deliberate after
 * getting it wrong in both directions.
 *
 * They cannot share one call, because they need different abort budgets. This
 * one runs in the PRERENDERED SHELL, where the only cost of waiting is a slower
 * build and giving up early bakes a schema-less page in for a whole revalidate
 * window. The seed runs at REQUEST time behind `await connection()`, where the
 * same ten seconds is a visitor watching a skeleton. `React.cache` does not
 * bridge them either: under `cacheComponents` the shell and the postponed
 * resume are different render passes, so a `cache()` entry made in one is not
 * visible in the other. What actually dedupes a repeated URL is Next's Data
 * Cache, and only when the URLs match.
 *
 * They also carry different bounds, which is why the URLs do NOT match. This
 * one sends the explicit `limit` argued at `UPCOMING_SHOWS_LIMIT`; the seed
 * sends exactly what the client hook sends, so that what it caches is what the
 * hook will later ask for. Two Data Cache entries, invalidated together by
 * `lib/proxy-revalidation.ts`.
 *
 * Since PSY-1678 that `limit` is the ONLY difference between the two URLs, and
 * it happens to equal the endpoint's own `default:"50"` — so dropping it would
 * collapse these into one cached backend read. That is deliberately not done
 * here: `UPCOMING_SHOWS_LIMIT` argues at length for keeping the bound visible
 * rather than inherited, and trading it for a cache hit is a product call about
 * SEO coverage, not a cleanup.
 */
const getUpcomingShowsPayload = cache(() =>
  fetchListPayload<UpcomingShowsResponse>({
    // `?`, not `&`: the first-screen URL is the bare endpoint, so this is the
    // only parameter. The endpoint decides "upcoming" against each show's own
    // venue zone (PSY-1678), so this block advertises exactly the rows the page
    // renders.
    url: `${UPCOMING_SHOWS_FIRST_SCREEN_URL}?limit=${UPCOMING_SHOWS_LIMIT}`,
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
 * Seed the two cache entries `ShowList` blocks its first paint on — the first
 * page of upcoming shows and the city facet counts — so dates, artists, venues
 * and cities reach the server HTML (PSY-1624).
 *
 * BOTH are required: `ShowList` returns its skeleton while EITHER query is
 * still loading, so seeding the rows alone server-renders the skeleton.
 *
 * The rows are a SEPARATE fetch from the `ItemList`'s `getUpcomingShowsPayload`
 * above, against the bare first-screen URL rather than that one's explicit
 * `limit`. That is deliberate on both counts and the reasoning is on
 * `getUpcomingShowsPayload`: they need different abort budgets, and this one has
 * to request exactly what the client hook will request or the seed is not the
 * entry the hook reads. Two Data Cache entries, invalidated together. Do not
 * "dedupe" them onto one call without reading that block first.
 *
 * A failed fetch renders `<ShowList />` unseeded rather than throwing; the
 * component fetches for itself and owns the error state (see
 * `fetchListPayload`).
 */
async function HydratedShowList() {
  const [shows, cities] = await Promise.all([
    fetchListPayload<UpcomingShowsResponse>({
      url: UPCOMING_SHOWS_FIRST_SCREEN_URL,
      collection: 'shows',
      service: 'shows-first-screen',
    }),
    fetchListPayload<ShowCitiesResponse>({
      url: SHOW_CITIES_FIRST_SCREEN_URL,
      collection: 'cities',
      service: 'show-cities-first-screen',
    }),
  ])

  if (!shows || !cities) {
    return <ShowList />
  }

  const dehydratedState = await seedFirstScreen([
    { queryKey: UPCOMING_SHOWS_FIRST_SCREEN_KEY, data: shows },
    { queryKey: SHOW_CITIES_FIRST_SCREEN_KEY, data: cities },
  ])

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
