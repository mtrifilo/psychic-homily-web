'use client'

import Link from 'next/link'
import { Clock, Music, ExternalLink } from 'lucide-react'

import type { RadioEpisodeListItem } from '../types'

interface RadioEpisodeRowProps {
  episode: RadioEpisodeListItem
  stationSlug: string
  showSlug: string
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  return date.toLocaleDateString('en-US', {
    month: 'short',
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

export function RadioEpisodeRow({ episode, stationSlug, showSlug }: RadioEpisodeRowProps) {
  const episodeUrl = `/radio/${stationSlug}/${showSlug}/${episode.air_date}`
  const hasTitle = !!episode.title
  const formattedDate = formatDate(episode.air_date)

  return (
    <Link
      href={episodeUrl}
      className="flex items-center gap-4 px-3 py-2.5 rounded-md hover:bg-muted/50 transition-colors group"
    >
      {/* Primary info */}
      <div className="flex-1 min-w-0">
        {hasTitle ? (
          <>
            <p className="text-sm font-medium group-hover:text-foreground truncate">
              {episode.title}
            </p>
            <p className="text-xs text-muted-foreground">
              {formattedDate}
              {episode.air_time && (
                <span className="inline-flex items-center gap-0.5 ml-1.5">
                  <Clock className="h-2.5 w-2.5 inline" />
                  {formatAirTime(episode.air_time)}
                </span>
              )}
              {episode.duration_minutes && (
                <span className="ml-1.5">&middot; {episode.duration_minutes} min</span>
              )}
            </p>
          </>
        ) : (
          <>
            <p className="text-sm font-medium group-hover:text-foreground">
              {formattedDate}
            </p>
            <p className="text-xs text-muted-foreground">
              {episode.air_time && (
                <span className="inline-flex items-center gap-0.5">
                  <Clock className="h-2.5 w-2.5 inline" />
                  {formatAirTime(episode.air_time)}
                </span>
              )}
              {episode.air_time && episode.duration_minutes && (
                <span className="ml-1.5">&middot; </span>
              )}
              {episode.duration_minutes && (
                <span>{episode.duration_minutes} min</span>
              )}
            </p>
          </>
        )}
      </div>

      {/* Play count */}
      <div className="shrink-0 flex items-center gap-1 text-xs text-muted-foreground">
        <Music className="h-3 w-3" />
        {episode.play_count} tracks
      </div>

      {/* Archive link */}
      {episode.archive_url && (
        <ExternalLink className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
      )}
    </Link>
  )
}
