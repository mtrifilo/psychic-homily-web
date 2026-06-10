'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, ArrowUpRight, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BracketLink } from '@/components/shared/BracketLink'
import { SectionHeader } from '@/components/shared/SectionHeader'
import {
  useRadioShow,
  useRadioEpisodes,
  useRadioTopArtists,
  useRadioTopLabels,
  isAirDateToday,
} from '@/features/radio'
import type { RadioTopArtist, RadioTopLabel } from '@/features/radio'
import { EpisodeArchiveTable } from './EpisodeArchiveTable'

interface RadioShowDetailProps {
  stationSlug: string
  showSlug: string
}

function pluralize(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`
}

function TopArtistsSidebar({ showSlug }: { showSlug: string }) {
  const { data, isLoading } = useRadioTopArtists({ showSlug, limit: 10 })

  if (isLoading || !data?.artists || data.artists.length === 0) return null

  return (
    <section className="rounded-md border border-border/60 px-3 py-2.5">
      <SectionHeader title="Top artists — last 90 days" />
      <div className="space-y-1">
        {data.artists.map((artist: RadioTopArtist) => (
          <div
            key={artist.artist_name}
            className="flex items-baseline justify-between gap-2 py-0.5"
          >
            {artist.artist_slug ? (
              <Link
                href={`/artists/${artist.artist_slug}`}
                className="text-sm text-primary hover:text-primary/80 transition-colors truncate"
              >
                {artist.artist_name}
              </Link>
            ) : (
              <span className="text-sm text-foreground truncate">
                {artist.artist_name}
              </span>
            )}
            <span className="text-xs text-muted-foreground tabular-nums whitespace-nowrap shrink-0">
              {pluralize(artist.play_count, 'play')} ·{' '}
              {pluralize(artist.episode_count, 'episode')}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}

function TopLabelsSidebar({ showSlug }: { showSlug: string }) {
  const { data, isLoading } = useRadioTopLabels({ showSlug, limit: 10 })

  if (isLoading || !data?.labels || data.labels.length === 0) return null

  return (
    <section className="rounded-md border border-border/60 px-3 py-2.5">
      <SectionHeader title="Top labels — last 90 days" />
      <div className="space-y-1">
        {data.labels.map((label: RadioTopLabel) => (
          <div
            key={label.label_name}
            className="flex items-baseline justify-between gap-2 py-0.5"
          >
            {label.label_slug ? (
              <Link
                href={`/labels/${label.label_slug}`}
                className="text-sm text-primary hover:text-primary/80 transition-colors truncate"
              >
                {label.label_name}
              </Link>
            ) : (
              <span className="text-sm text-foreground truncate">
                {label.label_name}
              </span>
            )}
            <span className="text-xs text-muted-foreground tabular-nums whitespace-nowrap shrink-0">
              {pluralize(label.play_count, 'play')}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}

const PAGE_SIZE = 25

export default function RadioShowDetail({ stationSlug, showSlug }: RadioShowDetailProps) {
  const { data: show, isLoading, error } = useRadioShow(showSlug)
  const [offset, setOffset] = useState(0)
  const { data: episodesData, isLoading: episodesLoading } = useRadioEpisodes({
    showSlug,
    limit: PAGE_SIZE,
    offset,
    enabled: !!show,
  })
  // Separate limit-1 query (cheap, cached) so the ON AIR strip always derives
  // from the LATEST episode regardless of which archive page is in view.
  const { data: latestData } = useRadioEpisodes({
    showSlug,
    limit: 1,
    enabled: !!show,
  })

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !show) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Show Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The radio show you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href={`/radio/${stationSlug}`}>
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Station
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const genreTags = show.genre_tags ?? []
  const total = episodesData?.total ?? 0
  const hasNextPage = offset + PAGE_SIZE < total
  const hasPrevPage = offset > 0
  const currentPage = Math.floor(offset / PAGE_SIZE) + 1
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  // v1 "on air" heuristic (PSY-1051): live when the latest episode aired
  // today. Swaps to PSY-1022's live now-playing endpoint without layout change.
  const latestEpisode = latestData?.episodes?.[0]
  const isOnAir = isAirDateToday(latestEpisode?.air_date)
  const livePlaylistUrl = latestEpisode
    ? `/radio/${stationSlug}/${showSlug}/${latestEpisode.air_date}`
    : null

  const metaParts = [
    show.schedule_display,
    show.station_name,
    ...genreTags,
  ].filter(Boolean) as string[]

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        {/* Breadcrumb */}
        <div className="mb-6 font-mono text-xs text-muted-foreground">
          <Link
            href={`/radio/${stationSlug}`}
            className="hover:text-foreground transition-colors"
          >
            ← {show.station_name}
          </Link>
        </div>

        {/* Show header */}
        <header className="mb-4">
          <div className="flex items-start justify-between gap-4">
            <h1 className="text-2xl font-bold min-w-0">
              {show.name}
              {show.host_name && (
                <span className="ml-2 text-base font-normal text-muted-foreground">
                  w/ {show.host_name}
                </span>
              )}
            </h1>
            {show.archive_url && (
              <Button asChild variant="outline" size="sm" className="shrink-0">
                <a
                  href={show.archive_url}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Archive
                  <ArrowUpRight className="h-3.5 w-3.5 ml-1" />
                </a>
              </Button>
            )}
          </div>

          {metaParts.length > 0 && (
            <p className="mt-1 font-mono text-xs text-muted-foreground">
              {metaParts.join(' · ')}
            </p>
          )}

          {show.description && (
            <p className="mt-2 text-sm text-muted-foreground leading-relaxed max-w-3xl">
              {show.description}
            </p>
          )}
        </header>

        {/* ON AIR strip (v1 heuristic until PSY-1022) */}
        {isOnAir && livePlaylistUrl && (
          <div className="mb-6 flex items-center justify-between gap-3 rounded-md border border-primary/40 bg-primary/5 px-4 py-2.5">
            <span className="font-mono text-xs text-foreground">
              <span className="text-primary" aria-hidden="true">
                ●
              </span>{' '}
              ON AIR NOW — tonight&apos;s playlist is updating live
            </span>
            <BracketLink
              label="open live playlist →"
              href={livePlaylistUrl}
              className="font-mono text-xs text-primary hover:text-primary/80"
            />
          </div>
        )}

        <div className="flex flex-col lg:flex-row gap-8">
          {/* Playlist archive */}
          <div className="flex-1 min-w-0">
            <SectionHeader
              as="h2"
              size="md"
              title={`Playlists — ${pluralize(total, 'episode')}`}
            />

            {episodesLoading && (
              <div className="flex justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            )}

            {!episodesLoading && episodesData?.episodes && episodesData.episodes.length > 0 ? (
              <>
                <EpisodeArchiveTable
                  episodes={episodesData.episodes}
                  stationSlug={stationSlug}
                  showSlug={showSlug}
                />

                {/* Pagination */}
                {(hasNextPage || hasPrevPage) && (
                  <div className="mt-3 flex items-center gap-3 font-mono text-xs text-muted-foreground">
                    <span>
                      page {currentPage} of {totalPages}
                    </span>
                    {hasPrevPage && (
                      <BracketLink
                        label="← newer"
                        onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                        className="text-xs"
                      />
                    )}
                    {hasNextPage && (
                      <BracketLink
                        label="older →"
                        onClick={() => setOffset(offset + PAGE_SIZE)}
                        className="text-xs"
                      />
                    )}
                  </div>
                )}
              </>
            ) : !episodesLoading ? (
              <div className="py-8 text-center text-sm text-muted-foreground">
                No episodes yet
              </div>
            ) : null}
          </div>

          {/* Sidebar */}
          <aside className="w-full lg:w-72 shrink-0 space-y-4">
            <TopArtistsSidebar showSlug={showSlug} />
            <TopLabelsSidebar showSlug={showSlug} />
          </aside>
        </div>
      </main>
    </div>
  )
}
