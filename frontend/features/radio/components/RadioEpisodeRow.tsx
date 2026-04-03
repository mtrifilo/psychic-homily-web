'use client'

import Link from 'next/link'
import { Calendar, Music, ExternalLink } from 'lucide-react'

import type { RadioEpisodeListItem } from '../types'

interface RadioEpisodeRowProps {
  episode: RadioEpisodeListItem
  stationSlug: string
  showSlug: string
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  return date.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

export function RadioEpisodeRow({ episode, stationSlug, showSlug }: RadioEpisodeRowProps) {
  const episodeUrl = `/radio/${stationSlug}/${showSlug}/${episode.air_date}`
  const title = episode.title || formatDate(episode.air_date)

  return (
    <Link
      href={episodeUrl}
      className="flex items-center gap-4 px-3 py-2.5 rounded-md hover:bg-muted/50 transition-colors group"
    >
      {/* Date */}
      <div className="shrink-0 text-center w-14">
        <Calendar className="h-4 w-4 text-muted-foreground mx-auto mb-0.5" />
        <span className="text-[10px] text-muted-foreground tabular-nums">
          {episode.air_date}
        </span>
      </div>

      {/* Title */}
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium group-hover:text-foreground truncate">
          {title}
        </p>
        {episode.duration_minutes && (
          <p className="text-xs text-muted-foreground">
            {episode.duration_minutes} min
          </p>
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
