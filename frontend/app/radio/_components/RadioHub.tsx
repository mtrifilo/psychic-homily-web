'use client'

import type { ReactNode } from 'react'
import { Loader2, Radio } from 'lucide-react'
import { BracketLink } from '@/components/shared/BracketLink'
import {
  useRadioStats,
  useRadioStations,
  useNewReleaseRadar,
  useRecentRadioEpisodes,
  useRadioGuide,
  isStationVisibleOnIndex,
  sortDialStations,
} from '@/features/radio'
import type { RadioStationListItem } from '@/features/radio/types'
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
 *
 * `dial` is the slot between the page head and the latest playlists: the
 * part of the hub that reads the station list. The route passes a seeded
 * `RadioDial` there; left out, the hub renders its own.
 */
export default function RadioHub({ dial = <RadioDial /> }: { dial?: ReactNode }) {
  const { data: stats } = useRadioStats()
  const { data: recentData, isLoading: recentLoading, error: recentError } =
    useRecentRadioEpisodes({ limit: 12 })
  const { data: radarData, isLoading: radarLoading } = useNewReleaseRadar({
    limit: 5,
  })

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        {/* Page head */}
        <header className="mb-8">
          <h1 className="text-3xl font-bold">Radio</h1>
          <p className="mt-1.5 text-muted-foreground">
            Freeform radio stations, shows, and playlists updated in real-time.
          </p>
          {stats && (
            <p className="mt-2 font-data text-xs text-muted-foreground">
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

        {dial}

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
                  className="font-data text-xs"
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
 * The dial strips and the program guide below them: the hub's only readers of
 * the station list (the guide maps each scheduled slug to its network).
 */
export function RadioDial() {
  const stationsQuery = useRadioStations()
  const guideQuery = useRadioGuide()
  return <DialView stationsQuery={stationsQuery} guideQuery={guideQuery} />
}

/**
 * The dial as it renders before its station read answers: the dial loader,
 * and no guide (the guide renders nothing while loading). It is the `loader`
 * of the dial's `CachedListFallback`, which renders it wherever the cache
 * cannot answer the dial's first screen, so it must create no query.
 */
export function RadioDialLoader() {
  return (
    <DialView
      stationsQuery={{ data: undefined, isLoading: true, error: null }}
      guideQuery={{ data: undefined, isError: false }}
    />
  )
}

function DialView({
  stationsQuery,
  // On a refetch error React Query RETAINS the previous data (the PSY-1136
  // stale-retained-data class) while the hook's interval stops — so an
  // errored guide must render nothing, not yesterday's ON NOW forever.
  guideQuery: { data: guideData, isError: guideError },
}: {
  stationsQuery: Pick<
    ReturnType<typeof useRadioStations>,
    'data' | 'isLoading' | 'error'
  >
  guideQuery: Pick<ReturnType<typeof useRadioGuide>, 'data' | 'isError'>
}) {
  const allStations = stationsQuery.data?.stations ?? []
  const stations = sortDialStations(allStations.filter(isStationVisibleOnIndex))
  // Every station, sub-streams included: the guide schedules sub-streams the
  // dial hides.
  const stationNetworks = new Map(
    allStations.map(station => [station.slug, station.network])
  )

  return (
    <>
      <DialSection
        isLoading={stationsQuery.isLoading}
        notice={dialNoticeFor(stationsQuery, stations.length)}
        stations={stations}
      />

      {/* ON NOW / UP NEXT guide (PSY-1053) — self-hides when the guide is
          empty, loading, or errored. */}
      <RadioGuide
        onNow={guideError ? null : guideData?.on_now}
        upNext={guideError ? null : guideData?.up_next}
        stationNetworks={stationNetworks}
      />
    </>
  )
}

/**
 * The notice the dial shows in place of strips, or null when it shows strips
 * or its loader. The error is for a dial with nothing to show: a failed
 * refetch of stations already on screen keeps the strips and says nothing,
 * since a server-seeded dial refetches on its first client commit.
 */
function dialNoticeFor(
  stationsQuery: Parameters<typeof DialView>[0]['stationsQuery'],
  visibleStationCount: number
): string | null {
  if (stationsQuery.isLoading) return null
  if (stationsQuery.error && !stationsQuery.data) {
    return "Couldn't load radio stations."
  }
  if (visibleStationCount === 0) return 'No radio stations yet.'
  return null
}

function DialSection({
  isLoading,
  notice,
  stations,
}: {
  isLoading: boolean
  notice: string | null
  stations: RadioStationListItem[]
}) {
  return (
    // This `aria-label` is a TEST LOCATOR, not just an a11y name:
    // e2e/pages/radio.spec.ts scopes its station-link queries to
    // `region "The dial"` to separate these strips from the program
    // guide, which links the same stations by the same bare names.
    // Renaming it (for instance to match the visible heading below)
    // breaks that spec. Rename the constant in the spec too.
    <section className="mb-10" aria-label="The dial">
      <DialSectionHeading title="The dial — live now" />
      {isLoading && (
        <div className="flex justify-center py-12">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
          <span className="sr-only">Loading stations</span>
        </div>
      )}
      {notice && (
        <div className="flex flex-col items-center gap-1 py-12 text-center">
          <Radio className="size-7 text-muted-foreground/30" />
          <p className="text-sm text-muted-foreground">{notice}</p>
        </div>
      )}
      {stations.map(station => (
        <DialStationStrip key={station.id} station={station} />
      ))}
    </section>
  )
}

/**
 * Data-face section heading in the radio register (matches PSY-1016's panel
 * headers; the shared SectionHeader primitive uses the sans entity-page
 * register, which isn't this surface's idiom).
 */
function DialSectionHeading({ title }: { title: string }) {
  return (
    <h2 className="mb-1 border-b border-border pb-1.5 text-xs text-muted-foreground">
      {title}
    </h2>
  )
}
