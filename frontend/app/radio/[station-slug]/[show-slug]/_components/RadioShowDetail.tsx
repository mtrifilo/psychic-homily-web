'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  ArrowLeft,
  Loader2,
  Mic2,
  Calendar,
  Tag,
  Music,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  useRadioShow,
  useRadioEpisodes,
  useRadioTopArtists,
  useRadioTopLabels,
  RadioEpisodeRow,
} from '@/features/radio'
import type { RadioTopArtist, RadioTopLabel } from '@/features/radio'

interface RadioShowDetailProps {
  stationSlug: string
  showSlug: string
}

function TopArtistsSidebar({ showSlug }: { showSlug: string }) {
  const { data, isLoading } = useRadioTopArtists({ showSlug, limit: 10 })

  if (isLoading || !data?.artists || data.artists.length === 0) return null

  return (
    <div>
      <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
        Top Artists (90d)
      </h3>
      <div className="space-y-1">
        {data.artists.map((artist: RadioTopArtist) => (
          <div
            key={artist.artist_name}
            className="flex items-center justify-between py-0.5"
          >
            {artist.artist_slug ? (
              <Link
                href={`/artists/${artist.artist_slug}`}
                className="text-sm text-muted-foreground hover:text-foreground transition-colors truncate mr-2"
              >
                {artist.artist_name}
              </Link>
            ) : (
              <span className="text-sm text-muted-foreground truncate mr-2">
                {artist.artist_name}
              </span>
            )}
            <span className="text-xs text-muted-foreground/60 tabular-nums shrink-0">
              {artist.play_count}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

function TopLabelsSidebar({ showSlug }: { showSlug: string }) {
  const { data, isLoading } = useRadioTopLabels({ showSlug, limit: 10 })

  if (isLoading || !data?.labels || data.labels.length === 0) return null

  return (
    <div>
      <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
        Top Labels (90d)
      </h3>
      <div className="space-y-1">
        {data.labels.map((label: RadioTopLabel) => (
          <div
            key={label.label_name}
            className="flex items-center justify-between py-0.5"
          >
            {label.label_slug ? (
              <Link
                href={`/labels/${label.label_slug}`}
                className="text-sm text-muted-foreground hover:text-foreground transition-colors truncate mr-2"
              >
                {label.label_name}
              </Link>
            ) : (
              <span className="text-sm text-muted-foreground truncate mr-2">
                {label.label_name}
              </span>
            )}
            <span className="text-xs text-muted-foreground/60 tabular-nums shrink-0">
              {label.play_count}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

const PAGE_SIZE = 20

export default function RadioShowDetail({ stationSlug, showSlug }: RadioShowDetailProps) {
  const { data: show, isLoading, error } = useRadioShow(showSlug)
  const [offset, setOffset] = useState(0)
  const { data: episodesData, isLoading: episodesLoading } = useRadioEpisodes({
    showSlug,
    limit: PAGE_SIZE,
    offset,
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

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
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
            {show.station_name}
          </Link>
        </div>

        <div className="flex flex-col lg:flex-row gap-8">
          {/* Main content */}
          <div className="flex-1 min-w-0">
            {/* Show header */}
            <div className="flex items-start gap-4 mb-6">
              <div className="shrink-0 rounded-xl bg-muted/50 flex items-center justify-center overflow-hidden h-20 w-20">
                {show.image_url ? (
                  <img
                    src={show.image_url}
                    alt={show.name}
                    className="h-full w-full object-cover"
                  />
                ) : (
                  <Mic2 className="h-10 w-10 text-muted-foreground/40" />
                )}
              </div>

              <div className="flex-1 min-w-0">
                <h1 className="text-2xl font-bold">{show.name}</h1>
                <p className="text-sm text-muted-foreground mt-0.5">
                  on{' '}
                  <Link
                    href={`/radio/${stationSlug}`}
                    className="hover:text-foreground transition-colors"
                  >
                    {show.station_name}
                  </Link>
                </p>

                {show.host_name && (
                  <p className="text-sm text-muted-foreground mt-1">
                    Hosted by {show.host_name}
                  </p>
                )}

                <div className="flex items-center gap-2 flex-wrap mt-2">
                  {show.schedule_display && (
                    <span className="flex items-center gap-1 text-sm text-muted-foreground">
                      <Calendar className="h-3.5 w-3.5" />
                      {show.schedule_display}
                    </span>
                  )}
                  {genreTags.length > 0 && (
                    <div className="flex items-center gap-1">
                      <Tag className="h-3.5 w-3.5 text-muted-foreground" />
                      {genreTags.map(tag => (
                        <Badge
                          key={tag}
                          variant="secondary"
                          className="text-[10px] px-1.5 py-0"
                        >
                          {tag}
                        </Badge>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </div>

            {show.description && (
              <p className="text-sm text-muted-foreground leading-relaxed mb-6">
                {show.description}
              </p>
            )}

            {/* Episodes */}
            <section>
              <h2 className="text-lg font-bold mb-3 flex items-center gap-2">
                <Music className="h-5 w-5" />
                Recent Episodes
                {total > 0 && (
                  <span className="text-sm font-normal text-muted-foreground">
                    ({total})
                  </span>
                )}
              </h2>

              {episodesLoading && (
                <div className="flex justify-center py-8">
                  <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
              )}

              {!episodesLoading && episodesData?.episodes && episodesData.episodes.length > 0 ? (
                <>
                  <div className="space-y-0.5">
                    {episodesData.episodes.map(episode => (
                      <RadioEpisodeRow
                        key={episode.id}
                        episode={episode}
                        stationSlug={stationSlug}
                        showSlug={showSlug}
                      />
                    ))}
                  </div>

                  {/* Pagination */}
                  {(hasNextPage || hasPrevPage) && (
                    <div className="flex items-center justify-between mt-4 pt-4 border-t border-border/50">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                        disabled={!hasPrevPage}
                      >
                        <ChevronLeft className="h-4 w-4 mr-1" />
                        Newer
                      </Button>
                      <span className="text-xs text-muted-foreground">
                        {offset + 1}-{Math.min(offset + PAGE_SIZE, total)} of {total}
                      </span>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setOffset(offset + PAGE_SIZE)}
                        disabled={!hasNextPage}
                      >
                        Older
                        <ChevronRight className="h-4 w-4 ml-1" />
                      </Button>
                    </div>
                  )}
                </>
              ) : !episodesLoading ? (
                <div className="py-8 text-center text-sm text-muted-foreground">
                  No episodes yet
                </div>
              ) : null}
            </section>
          </div>

          {/* Sidebar */}
          <aside className="w-full lg:w-64 shrink-0 space-y-6">
            <TopArtistsSidebar showSlug={showSlug} />
            <TopLabelsSidebar showSlug={showSlug} />
          </aside>
        </div>
      </main>
    </div>
  )
}
