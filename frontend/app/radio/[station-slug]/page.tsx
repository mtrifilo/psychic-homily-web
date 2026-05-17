import { Metadata } from 'next'
import { redirect } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import StationDetail from './_components/StationDetail'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface StationPageProps {
  params: Promise<{ 'station-slug': string }>
}

interface StationNetwork {
  slug: string
  is_flagship: boolean
}

interface StationData {
  name: string
  slug?: string
  description?: string | null
  city?: string | null
  state?: string | null
  network?: StationNetwork | null
}

async function getStation(slug: string): Promise<StationData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/radio-stations/${slug}`, {
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
}

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
  // PSY-674: sub-streams (non-flagship stations in a network) live under
  // /radio/{network-slug}/channel/{local-slug} now. Old direct URLs like
  // /radio/wfmu-drummer 308-redirect to /radio/wfmu/channel/drummer so
  // shared links keep working. The literal "channel" segment disambiguates
  // sub-streams from show pages (/radio/{station-slug}/{show-slug}).
  // Flagship + network-less stations stay on the 1-segment URL.
  const station = await getStation(stationSlug)
  if (station?.network && !station.network.is_flagship) {
    const prefix = `${station.network.slug}-`
    const localSlug = stationSlug.startsWith(prefix)
      ? stationSlug.slice(prefix.length)
      : stationSlug
    redirect(`/radio/${station.network.slug}/channel/${localSlug}`)
  }
  return <StationDetail stationSlug={stationSlug} />
}
