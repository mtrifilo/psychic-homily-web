import { Suspense, cache } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
import { ShowList, ShowListSkeleton } from '@/features/shows'
import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOW_CITIES_FIRST_SCREEN_URL,
  UPCOMING_SHOWS_FIRST_SCREEN_KEY,
} from '@/features/shows/api'
import type { ShowCitiesResponse, UpcomingShowsResponse } from '@/features/shows/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { API_BASE_URL } from '@/lib/api-base'
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
 */
export const UPCOMING_SHOWS_LIMIT = 50

/**
 * The one read of `/shows/upcoming` this page makes.
 *
 * It feeds two consumers with different needs — the JSON-LD `ItemList` wants
 * names and slugs, the hydration seed wants the whole response including
 * `pagination` and `total` — and `React.cache` is what lets the page body and
 * the Suspense child below share a single fetch rather than each starting one.
 *
 * They were briefly two fetches, on the theory that the seed had to reproduce
 * the client hook's request byte-for-byte. It does not: what the hook reads is
 * decided by the query KEY, which `seedFirstScreen` is handed separately, so
 * the URL only has to yield an equivalent payload. `?limit=50` and no `limit`
 * return the same 50 rows (the endpoint's own default — see
 * `UPCOMING_SHOWS_LIMIT`), so the second fetch bought nothing but a second
 * Data Cache entry and a way for the `ItemList` and the visible list to
 * disagree across cache windows.
 */
const getUpcomingShowsPayload = cache(() =>
  fetchListPayload<UpcomingShowsResponse>({
    url: `${API_BASE_URL}/shows/upcoming?limit=${UPCOMING_SHOWS_LIMIT}`,
    collection: 'shows',
    service: 'shows-listing',
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
    getUpcomingShowsPayload(),
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
