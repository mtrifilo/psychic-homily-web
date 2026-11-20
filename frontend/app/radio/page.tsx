import { Suspense } from 'react'
import { Metadata } from 'next'
import { HydrationBoundary } from '@tanstack/react-query'
import RadioHub, { RadioDial, RadioDialLoader } from './_components/RadioHub'
import { CachedListFallback } from '@/components/shared/CachedListFallback'
import {
  RADIO_DIAL_FIRST_SCREEN_KEY,
  RADIO_DIAL_FIRST_SCREEN_URL,
} from '@/features/radio/dialFirstScreen'
import {
  dehydrateFirstScreen,
  type FirstScreenRead,
} from '@/lib/ssr/firstScreenSeed'

export const metadata: Metadata = {
  title: 'Radio',
  description: 'Explore radio stations, shows, and playlists tracked on Psychic Homily',
  openGraph: {
    title: 'Radio',
    description: 'Explore radio stations, shows, and playlists tracked on Psychic Homily',
    type: 'website',
    url: '/radio',
  },
}

/** The read that seeds the dial. */
const FIRST_SCREEN_READS: readonly FirstScreenRead[] = [
  {
    url: RADIO_DIAL_FIRST_SCREEN_URL,
    queryKey: RADIO_DIAL_FIRST_SCREEN_KEY,
    collection: 'stations',
    service: 'radio-stations-first-screen',
  },
]

/** The keys that read seeds: what the dial's fallback gates on. */
const FIRST_SCREEN_KEYS = FIRST_SCREEN_READS.map(read => read.queryKey)

/**
 * `RadioDial` under a boundary seeded with the station list, so every dial
 * strip and its station link are in the server HTML.
 *
 * Only the stations are seeded. The strips' detail and now-playing reads and
 * the guide fill in after hydration, as they do on an unseeded dial.
 *
 * The hub reads no URL params, so every request is the first screen. A failed
 * read seeds nothing, and the dial fetches for itself and owns the loading and
 * error states. The element is a `HydrationBoundary` on every path.
 */
export async function HydratedRadioDial() {
  const dehydratedState = await dehydrateFirstScreen(FIRST_SCREEN_READS)

  return (
    <HydrationBoundary state={dehydratedState}>
      <RadioDial />
    </HydrationBoundary>
  )
}

/**
 * Only the dial waits on the server read: the page head, the latest playlists
 * and the sidebar sit outside the boundary and start their own reads at
 * hydration. The fallback is the dial's loader on the server and on a cold
 * cache, and the dial itself over the cached stations on a navigation back
 * (see `CachedListFallback`).
 */
export default function RadioPage() {
  return (
    <RadioHub
      dial={
        <Suspense
          fallback={
            <CachedListFallback
              seededKeys={FIRST_SCREEN_KEYS}
              loader={<RadioDialLoader />}
            >
              <RadioDial />
            </CachedListFallback>
          }
        >
          <HydratedRadioDial />
        </Suspense>
      }
    />
  )
}
