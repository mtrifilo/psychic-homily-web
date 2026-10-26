import { Metadata } from 'next'
import * as Sentry from '@sentry/nextjs'
import EpisodeDateDetail from './_components/EpisodeDateDetail'
import { API_BASE_URL } from '@/lib/api-base'
import { reportServerFetchFailure } from '@/lib/ssr/reportServerFetchFailure'
import { serverReadHeaders } from '@/lib/ssr/serverReadHeaders'

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

/**
 * Whether `segment` is a real `YYYY-MM-DD` date. The backend answers 400 for
 * anything else, and a 400 is reported, so a URL that cannot name an episode
 * is settled here without asking it.
 */
function isRealCalendarDate(segment: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(segment)
  if (!match) return false
  const [year, month, day] = match.slice(1).map(Number)
  const parsed = new Date(Date.UTC(year, month - 1, day))
  return (
    parsed.getUTCFullYear() === year &&
    parsed.getUTCMonth() === month - 1 &&
    parsed.getUTCDate() === day
  )
}

async function getEpisode(showSlug: string, date: string): Promise<EpisodeData | null> {
  if (!isRealCalendarDate(date)) return null
  try {
    const res = await fetch(
      `${API_BASE_URL}/radio-shows/${encodeURIComponent(showSlug)}/episodes/${encodeURIComponent(date)}`,
      {
        headers: serverReadHeaders(),
        next: { revalidate: 3600 },
      }
    )
    if (res.ok) {
      return res.json()
    }
    reportServerFetchFailure({
      response: res,
      service: 'radio-episode-page',
      pathFamily: '/radio-shows/{slug}/episodes/{date}',
      notFoundIsAnswer: true,
      context: { showSlug, date },
    })
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
