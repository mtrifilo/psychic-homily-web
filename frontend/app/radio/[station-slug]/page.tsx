import { cache } from 'react'
import { Metadata } from 'next'
import { notFound, redirect } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import StationDetail from './_components/StationDetail'
import { API_BASE_URL } from '@/lib/api-base'
import { serverReadHeaders } from '@/lib/ssr/serverReadHeaders'
import { getStationDetailUrl } from '@/features/radio/types'

interface StationPageProps {
  params: Promise<{ 'station-slug': string }>
}

interface StationNetwork {
  slug: string
  is_flagship: boolean
}

interface StationData {
  name: string
  slug: string
  description?: string | null
  city?: string | null
  state?: string | null
  network: StationNetwork | null
}

// Wrapped with `React.cache()` so `generateMetadata` and the page body
// share ONE backend fetch per request instead of two. The body needs the
// result for the `notFound()` and canonical `redirect()` below. Matches the
// entity-page pattern in `app/artists/[slug]/page.tsx`.
const getStation = cache(async (slug: string): Promise<StationData | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/radio-stations/${slug}`, {
      headers: serverReadHeaders(),
      next: { revalidate: 3600 },
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
    // The page redirects any other address to this one, so the metadata
    // names it too.
    const canonicalPath = getStationDetailUrl(station.slug, station.network)

    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com${canonicalPath}`,
      },
      openGraph: {
        title,
        description,
        type: 'website',
        url: canonicalPath,
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
  // Sub-streams (non-flagship stations in a network) are served at
  // /radio/{network-slug}/channel/{local-slug}; flagship and network-less
  // stations at /radio/{slug}. The backend resolves a numeric id as well as a
  // slug, so the canonical URL is built from the slug the read returned, not
  // the requested segment: a sub-stream's one-segment URL and a numeric id
  // both redirect to it.
  const station = await getStation(stationSlug)
  if (!station) {
    notFound()
  }
  const canonicalPath = getStationDetailUrl(station.slug, station.network)
  if (canonicalPath !== `/radio/${stationSlug}`) {
    redirect(canonicalPath)
  }
  return <StationDetail stationSlug={stationSlug} />
}
