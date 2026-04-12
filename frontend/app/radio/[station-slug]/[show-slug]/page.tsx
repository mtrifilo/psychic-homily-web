import { Metadata } from 'next'
import * as Sentry from '@sentry/nextjs'
import RadioShowDetail from './_components/RadioShowDetail'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface ShowPageProps {
  params: Promise<{ 'station-slug': string; 'show-slug': string }>
}

interface RadioShowData {
  name: string
  slug?: string
  station_name: string
  station_slug?: string
  host_name?: string | null
  description?: string | null
}

async function getRadioShow(slug: string): Promise<RadioShowData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/radio-shows/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Radio show page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'radio-show-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'radio-show-page' },
      extra: { slug },
    })
  }
  return null
}

export async function generateMetadata({ params }: ShowPageProps): Promise<Metadata> {
  const { 'station-slug': stationSlug, 'show-slug': showSlug } = await params
  const show = await getRadioShow(showSlug)

  if (show) {
    const description = show.description
      ? show.description.slice(0, 155) + (show.description.length > 155 ? '...' : '')
      : `${show.name} on ${show.station_name}${show.host_name ? `, hosted by ${show.host_name}` : ''}`
    const title = `${show.name} — ${show.station_name}`

    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/radio/${stationSlug}/${showSlug}`,
      },
      openGraph: {
        title,
        description,
        type: 'website',
        url: `/radio/${stationSlug}/${showSlug}`,
      },
    }
  }

  return {
    title: 'Radio Show',
    description: 'View radio show details, episodes, and playlists',
  }
}

export default async function ShowPage({ params }: ShowPageProps) {
  const { 'station-slug': stationSlug, 'show-slug': showSlug } = await params
  return <RadioShowDetail stationSlug={stationSlug} showSlug={showSlug} />
}
