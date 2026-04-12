import { Metadata } from 'next'
import * as Sentry from '@sentry/nextjs'
import EpisodeDateDetail from './_components/EpisodeDateDetail'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface EpisodeDatePageProps {
  params: Promise<{
    'station-slug': string
    'show-slug': string
    date: string
  }>
}

interface EpisodeData {
  show_name: string
  show_slug?: string
  station_name: string
  station_slug?: string
  title?: string | null
  air_date: string
  description?: string | null
  play_count: number
}

function formatShortDate(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

async function getEpisode(showSlug: string, date: string): Promise<EpisodeData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/radio-shows/${showSlug}/episodes/${date}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Radio episode page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'radio-episode-page' },
        extra: { showSlug, date, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'radio-episode-page' },
      extra: { showSlug, date },
    })
  }
  return null
}

export async function generateMetadata({ params }: EpisodeDatePageProps): Promise<Metadata> {
  const { 'station-slug': stationSlug, 'show-slug': showSlug, date } = await params
  const episode = await getEpisode(showSlug, date)

  if (episode) {
    const formattedDate = formatShortDate(episode.air_date)
    const title = `${episode.show_name} — ${formattedDate}`
    const description = episode.description
      ? episode.description.slice(0, 155) + (episode.description.length > 155 ? '...' : '')
      : `${episode.show_name} episode from ${formattedDate} on ${episode.station_name} — ${episode.play_count} tracks`

    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/radio/${stationSlug}/${showSlug}/${date}`,
      },
      openGraph: {
        title,
        description,
        type: 'website',
        url: `/radio/${stationSlug}/${showSlug}/${date}`,
      },
    }
  }

  return {
    title: 'Radio Episode',
    description: 'View radio episode playlist and details',
  }
}

export default async function EpisodeDatePage({ params }: EpisodeDatePageProps) {
  const { 'station-slug': stationSlug, 'show-slug': showSlug, date } = await params
  return <EpisodeDateDetail stationSlug={stationSlug} showSlug={showSlug} date={date} />
}
