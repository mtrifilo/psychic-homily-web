import { Suspense } from 'react'
import { Metadata } from 'next'
import { HydrationBoundary } from '@tanstack/react-query'
import RadioHub, { RadioDial, RadioDialLoading } from './_components/RadioHub'
import {
  RADIO_DIAL_FIRST_SCREEN_KEY,
  RADIO_DIAL_FIRST_SCREEN_URL,
} from '@/features/radio/dialFirstScreen'
import { dehydrateFirstScreen } from '@/lib/ssr/firstScreenSeed'

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
  const dehydratedState = await dehydrateFirstScreen([
    {
      url: RADIO_DIAL_FIRST_SCREEN_URL,
      queryKey: RADIO_DIAL_FIRST_SCREEN_KEY,
      collection: 'stations',
      service: 'radio-stations-first-screen',
    },
  ])

  return (
    <HydrationBoundary state={dehydratedState}>
      <RadioDial />
    </HydrationBoundary>
  )
}

/**
 * Only the dial waits on the server read: the page head, the latest playlists
 * and the sidebar sit outside the boundary and start their own reads at
 * hydration. The fallback is the dial as it renders before its reads answer,
 * drawn without the dial's queries, which a fallback must not run (see
 * `seedFirstScreen`).
 */
export default function RadioPage() {
  return (
    <RadioHub
      dial={
        <Suspense fallback={<RadioDialLoading />}>
          <HydratedRadioDial />
        </Suspense>
      }
    />
  )
}
