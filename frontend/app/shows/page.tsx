import { Suspense, cache } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
import { ShowList, ShowListSkeleton } from '@/features/shows'
import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOW_CITIES_FIRST_SCREEN_URL,
  UPCOMING_SHOWS_FIRST_SCREEN_KEY,
  UPCOMING_SHOWS_FIRST_SCREEN_URL,
} from '@/features/shows/api'
import type { ShowCitiesResponse, UpcomingShowsResponse } from '@/features/shows/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { API_BASE_URL } from '@/lib/api-base'
import { CANONICAL_FIRST_SCREEN_TIMEZONE } from '@/lib/canonicalTimezone'
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
 * They also carry different bounds, which is why the URLs deliberately do NOT
 * match. This one sends the explicit `limit` argued at `UPCOMING_SHOWS_LIMIT`.
 * The seed sends exactly what the client hook sends, so that what it caches is
 * what the hook will later ask for. Two Data Cache entries, invalidated
 * together by `lib/proxy-revalidation.ts`.
 */
const getUpcomingShowsPayload = cache(() =>
  fetchListPayload<UpcomingShowsResponse>({
    // The canonical zone matters even here: it decides where start-of-today
    // falls, so omitting it would advertise last night's shows to a crawler.
    url: `${API_BASE_URL}/shows/upcoming?${new URLSearchParams({
      limit: String(UPCOMING_SHOWS_LIMIT),
      timezone: CANONICAL_FIRST_SCREEN_TIMEZONE,
    })}`,
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
 * The rows come from `getUpcomingShowsPayload`, the same `React.cache`'d fetch
 * the `ItemList` above reads, so the crawler's list and the reader's list are
 * one response rather than two that can disagree.
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
      </div>
    </>
  )
}
