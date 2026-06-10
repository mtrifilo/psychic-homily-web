'use client'

import Link from 'next/link'
import { ArrowLeft, ArrowUpRight, Loader2, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  useRadioEpisode,
  useEpisodeNeighbors,
  computeArtistMatchStats,
  formatDurationMinutes,
  formatTimeOfDay,
} from '@/features/radio'
import { EpisodeNav } from './EpisodeNav'
import { PlaylistTable } from './PlaylistTable'

interface EpisodeDateDetailProps {
  stationSlug: string
  showSlug: string
  date: string
}

function formatLongDate(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  return date.toLocaleDateString('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

function formatWeekday(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  if (isNaN(date.getTime())) return ''
  return date.toLocaleDateString('en-US', { weekday: 'short' })
}

export default function EpisodeDateDetail({ stationSlug, showSlug, date }: EpisodeDateDetailProps) {
  const { data: episode, isLoading, error } = useRadioEpisode(showSlug, date)
  const { data: neighbors } = useEpisodeNeighbors(showSlug, date)

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

  const plays = episode.plays ?? []
  const showUrl = `/radio/${stationSlug}/${showSlug}`

  const matchStats = computeArtistMatchStats(plays)
  const duration = formatDurationMinutes(episode.duration_minutes)
  const airTime = formatTimeOfDay(episode.air_time)
  const airedLine = `aired ${formatWeekday(episode.air_date)}${airTime ? ` ${airTime}` : ''}`

  const metaParts = [
    `${episode.play_count} ${episode.play_count === 1 ? 'track' : 'tracks'}`,
    duration,
    airedLine,
    matchStats.total > 0
      ? `${matchStats.matched} of ${matchStats.total} artists matched to the graph`
      : null,
  ].filter(Boolean) as string[]

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        {/* Breadcrumb */}
        <div className="mb-6 font-mono text-xs text-muted-foreground">
          <Link href={showUrl} className="hover:text-foreground transition-colors">
            ← {episode.show_name}
          </Link>
        </div>

        {/* Episode header */}
        <header className="mb-6">
          <div className="flex items-start justify-between gap-4 flex-wrap">
            <h1 className="text-2xl font-bold min-w-0">
              {formatLongDate(episode.air_date)}
              {episode.title && (
                <span className="ml-2 text-base font-normal text-muted-foreground">
                  — {episode.title}
                </span>
              )}
            </h1>
            <div className="flex items-center gap-2 shrink-0">
              {episode.archive_url && (
                <Button asChild size="sm">
                  <a
                    href={episode.archive_url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Play className="h-3.5 w-3.5 mr-1.5" />
                    Play archive
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
                    Mixcloud
                    <ArrowUpRight className="h-3.5 w-3.5 ml-1" />
                  </a>
                </Button>
              )}
            </div>
          </div>

          <p className="mt-1 font-mono text-xs text-muted-foreground">
            {metaParts.join(' · ')}
          </p>

          <div className="mt-3">
            <EpisodeNav neighbors={neighbors} showUrl={showUrl} />
          </div>

          {episode.description && (
            <p className="mt-3 text-sm text-muted-foreground leading-relaxed max-w-3xl">
              {episode.description}
            </p>
          )}
        </header>

        {/* Track table */}
        {plays.length > 0 ? (
          <PlaylistTable plays={plays} />
        ) : (
          <div className="py-8 text-center text-sm text-muted-foreground">
            No playlist data available for this episode
          </div>
        )}
      </main>
    </div>
  )
}
