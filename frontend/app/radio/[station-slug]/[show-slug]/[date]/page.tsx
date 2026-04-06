'use client'

import { use } from 'react'
import Link from 'next/link'
import {
  ArrowLeft,
  Loader2,
  Calendar,
  Clock,
  Music,
  ExternalLink,
  Headphones,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useRadioEpisode, RadioPlayRow } from '@/features/radio'

interface EpisodeDatePageProps {
  params: Promise<{
    'station-slug': string
    'show-slug': string
    date: string
  }>
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

/**
 * Format a time string like "06:00:00" or "21:30:00" into "6:00 AM" or "9:30 PM".
 */
function formatAirTime(timeStr: string): string {
  const [hoursStr, minutesStr] = timeStr.split(':')
  const hours = parseInt(hoursStr, 10)
  const minutes = parseInt(minutesStr, 10)
  if (isNaN(hours) || isNaN(minutes)) return timeStr
  const period = hours >= 12 ? 'PM' : 'AM'
  const displayHours = hours === 0 ? 12 : hours > 12 ? hours - 12 : hours
  const displayMinutes = minutes.toString().padStart(2, '0')
  return `${displayHours}:${displayMinutes} ${period}`
}

export default function EpisodeDatePage({ params }: EpisodeDatePageProps) {
  const {
    'station-slug': stationSlug,
    'show-slug': showSlug,
    date,
  } = use(params)
  const { data: episode, isLoading, error } = useRadioEpisode(showSlug, date)

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !episode) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Episode Not Found</h1>
          <p className="text-muted-foreground mb-4">
            No episode found for {date}.
          </p>
          <Button asChild variant="outline">
            <Link href={`/radio/${stationSlug}/${showSlug}`}>
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Show
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const title = episode.title || formatDate(episode.air_date)
  const plays = episode.plays ?? []
  const genreTags = episode.genre_tags ?? []
  const moodTags = episode.mood_tags ?? []

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-4xl px-4 py-8 md:px-8">
        {/* Breadcrumb */}
        <div className="mb-6 flex items-center gap-1.5 text-sm text-muted-foreground">
          <Link
            href="/radio"
            className="hover:text-foreground transition-colors"
          >
            Radio
          </Link>
          <span>/</span>
          <Link
            href={`/radio/${stationSlug}`}
            className="hover:text-foreground transition-colors"
          >
            {episode.station_name}
          </Link>
          <span>/</span>
          <Link
            href={`/radio/${stationSlug}/${showSlug}`}
            className="hover:text-foreground transition-colors"
          >
            {episode.show_name}
          </Link>
        </div>

        {/* Episode header */}
        <div className="mb-8">
          <h1 className="text-2xl font-bold">{title}</h1>
          <p className="text-sm text-muted-foreground mt-1">
            <Link
              href={`/radio/${stationSlug}/${showSlug}`}
              className="hover:text-foreground transition-colors"
            >
              {episode.show_name}
            </Link>
            {' on '}
            <Link
              href={`/radio/${stationSlug}`}
              className="hover:text-foreground transition-colors"
            >
              {episode.station_name}
            </Link>
          </p>

          <div className="flex items-center gap-3 flex-wrap mt-3">
            <span className="flex items-center gap-1 text-sm text-muted-foreground">
              <Calendar className="h-3.5 w-3.5" />
              {formatDate(episode.air_date)}
            </span>
            {episode.air_time && (
              <span className="flex items-center gap-1 text-sm text-muted-foreground">
                <Clock className="h-3.5 w-3.5" />
                {formatAirTime(episode.air_time)}
              </span>
            )}
            {episode.duration_minutes && (
              <span className="text-sm text-muted-foreground">
                {episode.duration_minutes} min
              </span>
            )}
            <span className="flex items-center gap-1 text-sm text-muted-foreground">
              <Music className="h-3.5 w-3.5" />
              {episode.play_count} tracks
            </span>
          </div>

          {/* Tags */}
          {(genreTags.length > 0 || moodTags.length > 0) && (
            <div className="flex items-center gap-1.5 flex-wrap mt-3">
              {genreTags.map(tag => (
                <Badge key={tag} variant="secondary" className="text-[10px] px-1.5 py-0">
                  {tag}
                </Badge>
              ))}
              {moodTags.map(tag => (
                <Badge key={tag} variant="outline" className="text-[10px] px-1.5 py-0">
                  {tag}
                </Badge>
              ))}
            </div>
          )}

          {/* External links */}
          <div className="flex items-center gap-2 mt-4">
            {episode.archive_url && (
              <Button asChild variant="outline" size="sm">
                <a
                  href={episode.archive_url}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <Headphones className="h-4 w-4 mr-2" />
                  Listen to Archive
                  <ExternalLink className="h-3 w-3 ml-1" />
                </a>
              </Button>
            )}
            {episode.mixcloud_url && (
              <Button asChild variant="outline" size="sm">
                <a
                  href={episode.mixcloud_url}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <Headphones className="h-4 w-4 mr-2" />
                  Mixcloud
                  <ExternalLink className="h-3 w-3 ml-1" />
                </a>
              </Button>
            )}
          </div>

          {episode.description && (
            <p className="text-sm text-muted-foreground leading-relaxed mt-4">
              {episode.description}
            </p>
          )}
        </div>

        {/* Playlist */}
        <section>
          <h2 className="text-lg font-bold mb-3 flex items-center gap-2">
            <Music className="h-5 w-5" />
            Playlist
            <span className="text-sm font-normal text-muted-foreground">
              ({plays.length} tracks)
            </span>
          </h2>

          {plays.length > 0 ? (
            <div className="space-y-0.5 border border-border/50 rounded-lg p-2">
              {plays.map(play => (
                <RadioPlayRow key={play.id} play={play} />
              ))}
            </div>
          ) : (
            <div className="py-8 text-center text-sm text-muted-foreground">
              No playlist data available for this episode
            </div>
          )}
        </section>
      </main>
    </div>
  )
}
