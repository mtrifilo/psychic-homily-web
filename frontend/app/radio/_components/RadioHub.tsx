'use client'

import { Radio, Loader2 } from 'lucide-react'
import { useRadioStations, useRadioStats, RadioStationCard } from '@/features/radio'

export default function RadioHub() {
  const { data: stationsData, isLoading, error } = useRadioStations()
  const { data: stats } = useRadioStats()

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
