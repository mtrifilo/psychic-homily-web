import { Suspense } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
// Deep imports, not the `@/features/home` barrel: this file pulls in BOTH
// variants, and a `'use client'` barrel is not tree-shaken per export here.
// See features/sharedChunkBarrelGuard.test.ts.
import { AnonymousHome } from '@/features/home/components/AnonymousHome'
import { HomeVariantSwitch } from '@/features/home/components/HomeVariantSwitch'
import { SignedInHome } from '@/features/home/components/SignedInHome'
import { resolveHomeLayout } from '@/features/home/sections'
import {
  HOME_UPCOMING_FIRST_SCREEN_KEY,
  HOME_UPCOMING_FIRST_SCREEN_URL,
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOW_CITIES_FIRST_SCREEN_URL,
} from '@/features/shows/api'
import { HomeShowList } from '@/features/shows/components/HomeShowList'
import type {
  ShowCitiesResponse,
  UpcomingShowsResponse,
} from '@/features/shows/types'
import {
  getAuthenticatedHomeLayout,
  prefetchHomeSavedShows,
  resolveHomeViewer,
} from '@/lib/auth-hydration'
import { seedFirstScreen, type QuerySeed } from '@/lib/query-hydration'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'
import { withoutPassedShows } from '@/lib/ssr/withoutPassedShows'

/**
 * Picks the homepage's viewer variant on the SERVER (PSY-2103), so a signed-in
 * viewer's first paint is already their page and nothing re-branches after
 * hydration.
 *
 * `app/page.tsx` renders this inside its own `<Suspense>`. That boundary is a
 * STREAMING boundary, not what makes the route dynamic: `app/layout.tsx`
 * already wraps every page's children in a Suspense around `<AuthHydrator>`,
 * which awaits `prefetchAuthProfile()` and therefore `cookies()`, so the page
 * body has always rendered inside that postponed region. What the inner
 * boundary buys is that the variant swap suspends on its own instead of the
 * page's markup.
 *
 * The profile read shares `AppShell`'s `React.cache()`, so the variant costs
 * no extra backend fetch. A signed-in viewer's saved rows are prefetched here
 * too, so the module's first paint is its rows rather than a skeleton, and the
 * same cached read supplies their section layout, so a custom order is in the
 * first server HTML rather than reflowing after hydration.
 *
 * Three answers, three renders: a named viewer gets their page; an answered
 * "nobody" gets the anonymous page, with its upcoming-shows first screen
 * seeded (see {@link seedAnonymousHomeShows}); a read the backend could not
 * answer gets the anonymous page with a client-side switch, so the viewer
 * whose cookie is valid still reaches their page once their own profile query
 * settles. See {@link resolveHomeViewer}.
 */
export async function HomeContentSlot() {
  const viewer = await resolveHomeViewer()
  if (viewer === 'authenticated') {
    const layout = await getAuthenticatedHomeLayout()
    // The rows are prefetched only for a viewer whose saved-shows section is
    // actually on their page. Hiding it should take the list out of the page
    // SOURCE too, not just out of the render, and it saves the backend read.
    // Both reads share the same `React.cache()`d profile, so asking for the
    // layout first costs nothing.
    const wantsSavedShows = resolveHomeLayout(layout).some(
      section => section.id === 'saved_shows' && section.visible
    )
    return (
      <HydrationBoundary
        state={wantsSavedShows ? await prefetchHomeSavedShows() : undefined}
      >
        <SignedInHome layout={layout} />
      </HydrationBoundary>
    )
  }
  if (viewer === 'indeterminate') return <HomeVariantSwitch />
  // Only the list waits on its seed. The fallback must not be the list itself:
  // rendering it registers the query in the request's client first, and the
  // seed's HydrationBoundary then defers that entry to an effect, which never
  // runs on the server, so the resolved list would render its loader. `null`
  // also keeps loader markup out of the served HTML.
  return (
    <AnonymousHome
      upcomingShows={
        <Suspense fallback={null}>
          <SeededHomeShowList />
        </Suspense>
      }
    />
  )
}

/** The anonymous home's upcoming-shows list, under its server seed. */
async function SeededHomeShowList() {
  return (
    <HydrationBoundary state={await seedAnonymousHomeShows()}>
      <HomeShowList />
    </HydrationBoundary>
  )
}

/**
 * The anonymous home's upcoming-shows first screen: the unfiltered first page
 * of rows and the city facet the filter chips above them read, seeded under the
 * keys `HomeShowList` asks for on its first render, so both are in the served
 * HTML.
 *
 * Each read seeds independently. A `null` (failed, slow or malformed read)
 * leaves that query to its own client fetch: no rows means the list renders
 * its loader, as it does unseeded; no cities means no chips until the browser
 * has them. Rows that have provably left the upcoming window since the cached
 * read are dropped (`withoutPassedShows`).
 *
 * `seedFirstScreen`, not `prefetchEntities`, although only an anonymous viewer
 * reaches this: the payloads come out of the Data Cache and can be older than
 * the latest show change, and the stale stamp is what makes the browser re-ask
 * on mount instead of trusting them for a full `staleTime`.
 */
async function seedAnonymousHomeShows() {
  const [shows, cities] = await Promise.all([
    fetchListPayload<UpcomingShowsResponse>({
      url: HOME_UPCOMING_FIRST_SCREEN_URL,
      collection: 'shows',
      service: 'home-upcoming-first-screen',
    }),
    fetchListPayload<ShowCitiesResponse>({
      url: SHOW_CITIES_FIRST_SCREEN_URL,
      collection: 'cities',
      service: 'home-show-cities-first-screen',
    }),
  ])
  const seeds: QuerySeed[] = [
    {
      queryKey: HOME_UPCOMING_FIRST_SCREEN_KEY,
      data: shows && withoutPassedShows(shows),
    },
    { queryKey: SHOW_CITIES_FIRST_SCREEN_KEY, data: cities },
  ].filter(seed => seed.data !== null)
  return seeds.length > 0 ? seedFirstScreen(seeds) : undefined
}
