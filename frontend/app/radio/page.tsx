import { Suspense } from 'react'
import { Metadata } from 'next'
import { HydrationBoundary } from '@tanstack/react-query'
import RadioHub, { RadioHubLoading } from './_components/RadioHub'
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
 * `RadioHub` under a boundary seeded with the station list, so every dial
 * strip and its station link are in the server HTML.
 *
 * Only the stations are seeded. The strips' detail and now-playing reads, the
 * guide, the latest playlists and the stats all fill in after hydration, as
 * they do on an unseeded hub.
 *
 * The hub reads no URL params, so every request is the first screen. A failed
 * read seeds nothing, and the hub fetches for itself and owns the loading and
 * error states. The element is a `HydrationBoundary` on every path.
 */
export async function HydratedRadioHub() {
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
      <RadioHub />
    </HydrationBoundary>
  )
}

/**
 * The fallback is the hub as it renders before any read has answered (heading,
 * dial loader and all), drawn without the hub's queries: a fallback that ran
 * them would pre-empt the seed (see `seedFirstScreen`).
 */
export default function RadioPage() {
  return (
    <Suspense fallback={<RadioHubLoading />}>
      <HydratedRadioHub />
    </Suspense>
  )
}
