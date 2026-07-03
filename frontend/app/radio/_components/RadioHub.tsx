'use client'

import { Loader2, Radio } from 'lucide-react'
import { BracketLink } from '@/components/shared/BracketLink'
import {
  useRadioStats,
  useRadioStations,
  useNewReleaseRadar,
  useRecentRadioEpisodes,
  useRadioGuide,
  isStationVisibleOnIndex,
} from '@/features/radio'
import { DialStationStrip } from './DialStationStrip'
import { LatestPlaylistsTable } from './LatestPlaylistsTable'
import { NewReleaseRadarBox, DialStatsBox } from './DialSidebarBoxes'
import { RadioGuide } from './RadioGuide'

/**
 * The Dial — /radio hub (PSY-1049, Option A, locked 2026-06-09).
 *
 * Every station and channel is visible as a full-width strip with on-air info
 * inline (zero clicks to see the whole dial), followed by the dial-wide
 * latest-playlists feed (PSY-1048) with New Release Radar + lifetime stats in
 * the sidebar. The top-bar Radio item links straight here (PSY-1057 retired
 * the D2 popover once this page became the dial).
 */
export default function RadioHub() {
  const { data: stats } = useRadioStats()
  const stationsQuery = useRadioStations()
  const { data: recentData, isLoading: recentLoading, error: recentError } =
    useRecentRadioEpisodes({ limit: 12 })
  const { data: radarData, isLoading: radarLoading } = useNewReleaseRadar({
    limit: 5,
  })
  // On a refetch error React Query RETAINS the previous data (the PSY-1136
  // stale-retained-data class) while the hook's interval stops — so an
  // errored guide must render nothing, not yesterday's ON NOW forever.
  const { data: guideData, isError: guideError } = useRadioGuide()

  const stations = (stationsQuery.data?.stations ?? []).filter(
    isStationVisibleOnIndex
  )

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        {/* Page head */}
        <header className="mb-8">
          <h1 className="text-3xl font-bold">Radio</h1>
          <p className="mt-1.5 text-muted-foreground">
            Independent radio, wired into the knowledge graph — every playlist
            links into artists, releases, and labels.
          </p>
          {stats && (
            <p className="mt-2 font-mono text-xs text-muted-foreground">
              {stats.total_stations.toLocaleString()}{' '}
              {stats.total_stations === 1 ? 'station' : 'stations'}
              {' · '}
              {stats.total_shows.toLocaleString()}{' '}
              {stats.total_shows === 1 ? 'show' : 'shows'}
              {' · '}
              {stats.total_episodes.toLocaleString()}{' '}
              {stats.total_episodes === 1 ? 'playlist' : 'playlists'}
              {' · '}
              {stats.total_plays.toLocaleString()}{' '}
              {stats.total_plays === 1 ? 'play' : 'plays'} tracked
            </p>
          )}
        </header>

        {/* THE DIAL */}
        <section className="mb-10" aria-label="The dial">
          <DialSectionHeading title="The dial — live now" />
          {stationsQuery.isLoading && (
            <div className="flex justify-center py-12">
              <Loader2 className="size-5 animate-spin text-muted-foreground" />
              <span className="sr-only">Loading stations</span>
            </div>
          )}
          {!stationsQuery.isLoading &&
            (stationsQuery.error || stations.length === 0) && (
              <div className="flex flex-col items-center gap-1 py-12 text-center">
                <Radio className="size-7 text-muted-foreground/30" />
                <p className="text-sm text-muted-foreground">
                  {stationsQuery.error
                    ? "Couldn't load radio stations."
                    : 'No radio stations yet.'}
                </p>
              </div>
            )}
          {stations.map(station => (
            <DialStationStrip key={station.id} station={station} />
          ))}
        </section>

        {/* ON NOW / UP NEXT guide (PSY-1053) — self-hides when the guide is
            empty, loading, or errored. */}
        <RadioGuide
          onNow={guideError ? null : guideData?.on_now}
          upNext={guideError ? null : guideData?.up_next}
        />

        {/* Latest playlists + sidebar */}
        <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_280px]">
          <section aria-label="Latest playlists">
            <DialSectionHeading title="Latest playlists — across the dial" />
            <LatestPlaylistsTable
              rows={recentData?.episodes}
              isLoading={recentLoading}
              error={recentError}
            />
            {/* PSY-1076: the hub table is a capped teaser — link the full,
                paginated dial-wide feed. */}
            {!recentError && (recentData?.episodes?.length ?? 0) > 0 && (
              <div className="mt-2">
                <BracketLink
                  label="all playlists →"
                  href="/radio/playlists"
                  className="font-mono text-xs"
                />
              </div>
            )}
          </section>

          <aside className="flex flex-col gap-5">
            <NewReleaseRadarBox
              releases={radarData?.releases}
              isLoading={radarLoading}
            />
            <DialStatsBox stats={stats} />
          </aside>
        </div>
      </main>
    </div>
  )
}

/**
 * Mono section heading in the radio register (matches PSY-1016's panel
 * headers; the shared SectionHeader primitive uses the sans entity-page
 * register, which isn't this surface's idiom).
 */
function DialSectionHeading({ title }: { title: string }) {
  return (
    <h2 className="mb-1 border-b border-border pb-1.5 font-mono text-[11px] uppercase tracking-[1.2px] text-muted-foreground">
      {title}
    </h2>
  )
}
