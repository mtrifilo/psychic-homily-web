'use client'

import Link from 'next/link'
import { Radio, Loader2, Disc3 } from 'lucide-react'
import {
  useRadioStations,
  useRadioStats,
  useNewReleaseRadar,
  RadioStationCard,
} from '@/features/radio'
import type { RadioNewReleaseRadarEntry } from '@/features/radio'

export default function RadioHub() {
  const { data: stationsData, isLoading, error } = useRadioStations()
  const { data: stats } = useRadioStats()
  const { data: radarData, isLoading: radarLoading } = useNewReleaseRadar({ limit: 10 })

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold flex items-center justify-center gap-3">
            <Radio className="h-8 w-8" />
            Radio
          </h1>
          <p className="text-muted-foreground mt-2">
            Explore radio stations, shows, and playlists
          </p>

          {stats && (
            <div className="flex items-center justify-center gap-6 mt-4 text-sm text-muted-foreground">
              <span>{stats.total_stations} {stats.total_stations === 1 ? 'station' : 'stations'}</span>
              <span>{stats.total_shows} {stats.total_shows === 1 ? 'show' : 'shows'}</span>
              <span>{stats.total_episodes.toLocaleString()} {stats.total_episodes === 1 ? 'episode' : 'episodes'}</span>
              <span>{stats.total_plays.toLocaleString()} {stats.total_plays === 1 ? 'play' : 'plays'} tracked</span>
            </div>
          )}
        </div>

        {/* New Release Radar */}
        <NewReleaseRadarSection releases={radarData?.releases} isLoading={radarLoading} />

        {isLoading && (
          <div className="flex justify-center items-center py-12">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        )}

        {error && (
          <div className="py-12 text-center text-sm text-destructive">
            Failed to load radio stations
          </div>
        )}

        {!isLoading && !error && stationsData && (
          <>
            {stationsData.stations && stationsData.stations.length > 0 ? (
              <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
                {stationsData.stations.map(station => (
                  <RadioStationCard key={station.id} station={station} />
                ))}
              </div>
            ) : (
              <div className="py-12 text-center">
                <Radio className="h-12 w-12 text-muted-foreground/30 mx-auto mb-3" />
                <p className="text-muted-foreground">No radio stations yet</p>
                <p className="text-sm text-muted-foreground/60 mt-1">
                  Radio stations will appear here once they are added.
                </p>
              </div>
            )}
          </>
        )}
      </main>
    </div>
  )
}

// ---------------------------------------------------------------------------
// New Release Radar sub-component
// ---------------------------------------------------------------------------

function NewReleaseRadarSection({
  releases,
  isLoading,
}: {
  releases: RadioNewReleaseRadarEntry[] | undefined
  isLoading: boolean
}) {
  // Don't render the section at all while loading or if there's no data
  if (isLoading) {
    return (
      <section className="mb-10">
        <h2 className="text-lg font-semibold flex items-center gap-2 mb-4">
          <Disc3 className="h-5 w-5" />
          New Release Radar
        </h2>
        <div className="rounded-lg border border-border/50 bg-card p-6">
          <div className="flex justify-center py-4">
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        </div>
      </section>
    )
  }

  if (!releases || releases.length === 0) {
    return null
  }

  return (
    <section className="mb-10">
      <h2 className="text-lg font-semibold flex items-center gap-2 mb-4">
        <Disc3 className="h-5 w-5" />
        New Release Radar
      </h2>

      <div className="rounded-lg border border-border/50 bg-card overflow-hidden">
        {/* Header row */}
        <div className="hidden sm:grid sm:grid-cols-[1fr_1fr_1fr_4.5rem_4.5rem] gap-3 px-4 py-2 border-b border-border/30 text-xs font-medium text-muted-foreground uppercase tracking-wider">
          <span>Artist</span>
          <span>Album</span>
          <span>Label</span>
          <span className="text-right">Plays</span>
          <span className="text-right">Stations</span>
        </div>

        {/* Rows */}
        {releases.map((entry, idx) => (
          <div
            key={`${entry.artist_name}-${entry.album_title}-${idx}`}
            className="sm:grid sm:grid-cols-[1fr_1fr_1fr_4.5rem_4.5rem] gap-3 px-4 py-2.5 hover:bg-muted/30 transition-colors border-b border-border/10 last:border-b-0"
          >
            {/* Artist */}
            <div className="truncate">
              {entry.artist_slug ? (
                <Link
                  href={`/artists/${entry.artist_slug}`}
                  className="text-sm font-medium hover:text-primary transition-colors"
                >
                  {entry.artist_name}
                </Link>
              ) : (
                <span className="text-sm font-medium">{entry.artist_name}</span>
              )}
            </div>

            {/* Album */}
            <div className="truncate">
              {entry.album_title ? (
                entry.release_slug ? (
                  <Link
                    href={`/releases/${entry.release_slug}`}
                    className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {entry.album_title}
                  </Link>
                ) : (
                  <span className="text-sm text-muted-foreground">{entry.album_title}</span>
                )
              ) : (
                <span className="text-sm text-muted-foreground/40">--</span>
              )}
            </div>

            {/* Label */}
            <div className="truncate">
              {entry.label_name ? (
                entry.label_slug ? (
                  <Link
                    href={`/labels/${entry.label_slug}`}
                    className="text-sm text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {entry.label_name}
                  </Link>
                ) : (
                  <span className="text-sm text-muted-foreground">{entry.label_name}</span>
                )
              ) : (
                <span className="text-sm text-muted-foreground/40">--</span>
              )}
            </div>

            {/* Play count */}
            <div className="text-sm text-muted-foreground tabular-nums text-right sm:block inline">
              <span className="sm:hidden text-xs text-muted-foreground/50 mr-1">plays:</span>
              {entry.play_count}
            </div>

            {/* Station count */}
            <div className="text-sm text-muted-foreground tabular-nums text-right sm:block inline ml-3 sm:ml-0">
              <span className="sm:hidden text-xs text-muted-foreground/50 mr-1">stations:</span>
              {entry.station_count}
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}
