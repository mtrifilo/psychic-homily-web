import { cache } from 'react'
import { Metadata } from 'next'
import { redirect } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import StationDetail from './_components/StationDetail'
import {
  RADIO_STATION_REVALIDATE_SECONDS,
  radioStationApiUrl,
  radioSubStreamChannelPath,
  type RadioStationNetwork,
} from '@/lib/seo/radioStationCanonical'

interface StationPageProps {
  params: Promise<{ 'station-slug': string }>
}

interface StationData {
  name: string
  slug?: string
  description?: string | null
  city?: string | null
  state?: string | null
  network?: RadioStationNetwork | null
}

// Wrapped with `React.cache()` so `generateMetadata` and the page body
// share ONE backend fetch per request instead of two. The body needs the
// result for the sub-stream `redirect()` below. Matches the entity-page
// pattern in `app/artists/[slug]/page.tsx`.
const getStation = cache(async (slug: string): Promise<StationData | null> => {
  try {
    const res = await fetch(radioStationApiUrl(slug), {
      next: { revalidate: RADIO_STATION_REVALIDATE_SECONDS },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Radio station page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'radio-station-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'radio-station-page' },
      extra: { slug },
    })
  }
  return null
})

export async function generateMetadata({ params }: StationPageProps): Promise<Metadata> {
  const { 'station-slug': stationSlug } = await params
  const station = await getStation(stationSlug)

  if (station) {
    const location = [station.city, station.state].filter(Boolean).join(', ')
    const description = station.description
      ? station.description.slice(0, 155) + (station.description.length > 155 ? '...' : '')
      : `${station.name} radio station${location ? ` from ${location}` : ''} on Psychic Homily`
    const title = `${station.name} — Radio`

    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/radio/${stationSlug}`,
      },
      openGraph: {
        title,
        description,
        type: 'website',
        url: `/radio/${stationSlug}`,
      },
    }
  }

  return {
    title: 'Radio Station',
    description: 'View radio station details, shows, and playlists',
  }
}

export default async function StationPage({ params }: StationPageProps) {
  const { 'station-slug': stationSlug } = await params
  // Fallback for a sub-stream request proxy.ts let through (see
  // radioSubStreamRedirect there): it arrives inside the streamed document.
  const station = await getStation(stationSlug)
  const channelPath = radioSubStreamChannelPath(stationSlug, station?.network)
  if (channelPath) {
    redirect(channelPath)
  }
  return <StationDetail stationSlug={stationSlug} />
}
