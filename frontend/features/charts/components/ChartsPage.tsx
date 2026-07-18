'use client'

import { useEffect, useMemo, useTransition, type ReactNode } from 'react'
import Link from 'next/link'
import { ChevronDown } from 'lucide-react'
import { parseAsString, parseAsStringLiteral, useQueryState } from 'nuqs'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  FollowButton,
  ReleaseSaveButton,
  SaveButton,
} from '@/components/shared'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useBatchFollowStatus } from '@/lib/hooks/common/useFollow'
import { useShowSaveCountBatch } from '@/features/shows'
import {
  getReleaseTypeLabel,
  useReleaseSaveCountBatch,
} from '@/features/releases'
import { showDisplayTitle } from '@/lib/utils/showDisplayTitle'
import { cn } from '@/lib/utils'
import {
  useBusiestVenues,
  useChartScenes,
  useChartsSummary,
  useFreshlyAdded,
  useMostActiveArtists,
  useMostAnticipated,
  useNewReleases,
  useOnTheRadio,
  useOpenersToWatch,
} from '../hooks'
import {
  CHART_WINDOWS,
  type ChartEntityReference,
  type ChartScene,
  type ChartWindow,
  type FreshlyAddedItem,
} from '../types'
import { ChartModule, ChartRow } from './ChartModule'
import { PersonalStatsStrip } from './PersonalStatsStrip'

const chartWindowParser =
  parseAsStringLiteral(CHART_WINDOWS).withDefault('quarter')
const WINDOW_LABELS: Record<ChartWindow, string> = {
  month: 'This Month',
  quarter: 'Quarter',
  all_time: 'All Time',
}
const WINDOW_CONTEXT: Record<ChartWindow, string> = {
  month: 'last 30 days',
  quarter: 'last 90 days',
  all_time: 'all time',
}
const WINDOW_SUMMARY: Record<ChartWindow, string> = {
  month: 'THIS MONTH',
  quarter: 'THIS QUARTER',
  all_time: 'ALL TIME',
}

function metroDisplayName(name: string): string {
  return name.replace(/,\s*[A-Z-]+$/, '')
}

function countLabel(count: number, singular: string, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`
}

const linkClass =
  'hover:text-primary focus-visible:text-primary focus-visible:outline-none'

function location(city: string, state: string): string {
  return [city, state].filter(Boolean).join(', ')
}

function formatDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

function formatDateOnly(value: string | null): string {
  if (!value) return 'graph new'
  const date = new Date(`${value}T00:00:00Z`)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })
}

function entityHref(route: string, slug: string, id: number): string {
  return `/${route}/${slug || id}`
}

function fullListHref(module: string, window: ChartWindow, scene: string) {
  const search = new URLSearchParams()
  if (window !== 'quarter') search.set('window', window)
  if (scene) search.set('scene', scene)
  const query = search.toString()
  return `/charts/${module}${query ? `?${query}` : ''}`
}

function EntityReferenceList({
  references,
  route,
}: {
  references: ChartEntityReference[]
  route: 'artists' | 'labels'
}) {
  return references.map((reference, index) => (
    <span key={`${route}-${reference.id}`}>
      {index > 0 ? ', ' : null}
      <Link
        href={entityHref(route, reference.slug, reference.id)}
        className={linkClass}
      >
        {reference.name}
      </Link>
    </span>
  ))
}

function SceneSwitcher({
  scenes,
  selectedScene,
  isLoading,
  isError,
  onChange,
  onRetry,
  treatment = 'chip',
}: {
  scenes: ChartScene[]
  selectedScene: ChartScene | undefined
  isLoading: boolean
  isError: boolean
  onChange: (scene: string | null) => void
  onRetry: () => void
  treatment?: 'chip' | 'masthead'
}) {
  const triggerLabel = selectedScene?.city ?? 'All scenes'
  const displayedLabel = isLoading
    ? 'Loading scenes'
    : isError
      ? 'Scenes unavailable'
      : triggerLabel
  const triggerClassName = cn(
    'inline-flex items-center transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60',
    treatment === 'chip'
      ? 'gap-1 rounded-sm border border-border px-2 py-1 font-mono text-[11px] hover:border-foreground/50'
      : 'gap-2 text-left font-display text-3xl font-bold leading-none text-primary hover:text-primary/80'
  )

  if (isError) {
    return (
      <button
        type="button"
        onClick={onRetry}
        className={triggerClassName}
        aria-label="Retry chart scenes"
      >
        Scenes unavailable · Retry
      </button>
    )
  }

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        disabled={isLoading}
        className={triggerClassName}
        aria-label={`Chart scene: ${displayedLabel}`}
      >
        {displayedLabel}
        <ChevronDown
          className={treatment === 'chip' ? 'size-3' : 'size-5'}
          aria-hidden
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align={treatment === 'chip' ? 'end' : 'start'}
        className="min-w-64 font-mono"
      >
        <DropdownMenuRadioGroup
          value={selectedScene?.metro ?? 'all'}
          onValueChange={value => onChange(value === 'all' ? null : value)}
        >
          <DropdownMenuRadioItem value="all">All scenes</DropdownMenuRadioItem>
          {scenes.map(scene => (
            <DropdownMenuRadioItem key={scene.metro} value={scene.metro}>
              <span className="min-w-0 flex-1 truncate">
                {location(scene.city, scene.state)}
              </span>
              <span className="text-[10px] text-muted-foreground">
                {scene.show_count} shows
              </span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function StatStrip({
  window,
  summary,
  isLoading,
}: {
  window: ChartWindow
  summary:
    | {
        shows_added: number
        new_artists: number
        new_releases: number
        radio_plays: number
        active_scenes: number
      }
    | undefined
  isLoading: boolean
}) {
  return (
    <div className="flex min-h-[38px] items-center gap-4 border-y border-border py-2 font-mono">
      {isLoading ? (
        <span className="h-3 w-3/4 animate-pulse rounded-sm bg-muted" />
      ) : summary ? (
        <p className="min-w-0 flex-1 text-[11px] leading-4 sm:text-xs">
          <span>{WINDOW_SUMMARY[window]}:</span>{' '}
          <span>{summary.shows_added} shows added</span>
          <span className="text-muted-foreground"> · </span>
          <span>{summary.new_artists} new artists</span>
          <span className="text-muted-foreground"> · </span>
          <span>{summary.new_releases} releases</span>
          <span className="text-muted-foreground"> · </span>
          <span>{summary.radio_plays} radio plays</span>
          <span className="text-muted-foreground"> · </span>
          <span>{summary.active_scenes} active scenes</span>
        </p>
      ) : (
        <p className="flex-1 text-xs text-destructive">Summary unavailable.</p>
      )}
      <span className="hidden shrink-0 text-[11px] text-muted-foreground sm:inline">
        updated hourly
      </span>
    </div>
  )
}

function tickerHref(item: FreshlyAddedItem): string {
  switch (item.entity_type) {
    case 'artist':
      return entityHref('artists', item.slug, item.entity_id)
    case 'venue':
      return entityHref('venues', item.slug, item.entity_id)
    case 'release':
      return entityHref('releases', item.slug, item.entity_id)
    case 'station':
      return entityHref('radio', item.slug, item.entity_id)
  }
}

function FreshlyAddedTicker({ items }: { items: FreshlyAddedItem[] }) {
  if (items.length === 0) return null
  return (
    <section className="flex flex-col gap-2 border-t-2 border-foreground py-2.5 sm:flex-row sm:items-baseline sm:gap-4">
      <h2 className="shrink-0 font-mono text-[11px] font-bold uppercase tracking-[0.06em]">
        Freshly Added
      </h2>
      <p className="min-w-0 flex-1 text-xs leading-5 text-muted-foreground">
        {items.map((item, index) => (
          <span key={`${item.entity_type}-${item.entity_id}`}>
            {index > 0 ? ' · ' : null}
            <Link href={tickerHref(item)} className={linkClass}>
              {item.name}
            </Link>{' '}
            <span className="text-[10px]">({item.entity_type})</span>
          </span>
        ))}
      </p>
      <span aria-disabled="true" className="shrink-0 text-xs text-primary">
        View the firehose →
      </span>
    </section>
  )
}

export function ChartsPage() {
  const [isPending, startTransition] = useTransition()
  const [window, setWindow] = useQueryState(
    'window',
    chartWindowParser.withOptions({ history: 'push', startTransition })
  )
  const [scene, setScene] = useQueryState(
    'scene',
    parseAsString.withOptions({ history: 'push', startTransition })
  )
  const { isAuthenticated, user } = useAuthContext()

  const sceneList = useChartScenes(window)
  const sceneListMatchesWindow = sceneList.data?.window === window
  const selectedScene = sceneListMatchesWindow
    ? sceneList.data?.scenes.find(option => option.metro === scene)
    : undefined
  const sceneHasValidShape = !scene || /^[0-9]{1,10}$/.test(scene)
  const sceneValidationComplete =
    sceneList.isSuccess && sceneListMatchesWindow && !sceneList.isFetching
  const sceneResolved =
    !scene || Boolean(selectedScene) || sceneValidationComplete
  const sceneValidationFailed =
    Boolean(scene) && sceneHasValidShape && sceneList.isError && !selectedScene
  const effectiveScene = selectedScene?.metro ?? ''
  const chartQueryOptions = {
    scene: effectiveScene,
    enabled: sceneResolved,
  }

  useEffect(() => {
    if (
      scene &&
      (!sceneHasValidShape || (sceneValidationComplete && !selectedScene))
    ) {
      void setScene(null)
    }
  }, [
    scene,
    sceneHasValidShape,
    sceneValidationComplete,
    selectedScene,
    setScene,
  ])

  const active = useMostActiveArtists(window, 7, chartQueryOptions)
  const radio = useOnTheRadio(window, 7, chartQueryOptions)
  const anticipated = useMostAnticipated(window, 6, chartQueryOptions)
  const venues = useBusiestVenues(window, 7, chartQueryOptions)
  const releases = useNewReleases(window, 6, chartQueryOptions)
  const openers = useOpenersToWatch(window, 6, chartQueryOptions)
  const summary = useChartsSummary(window, chartQueryOptions)
  const freshlyAdded = useFreshlyAdded(6, chartQueryOptions)

  const artistIDs = useMemo(
    () =>
      Array.from(
        new Set([
          ...(active.data?.artists ?? []).map(artist => artist.artist_id),
          ...(radio.data?.artists ?? []).map(artist => artist.artist_id),
          ...(openers.data?.artists ?? []).map(artist => artist.artist_id),
        ])
      ).sort((a, b) => a - b),
    [active.data?.artists, radio.data?.artists, openers.data?.artists]
  )
  const venueIDs = useMemo(
    () =>
      (venues.data?.venues ?? [])
        .map(venue => venue.venue_id)
        .sort((a, b) => a - b),
    [venues.data?.venues]
  )
  const showIDs = useMemo(
    () =>
      (anticipated.data?.shows ?? [])
        .map(show => show.show_id)
        .sort((a, b) => a - b),
    [anticipated.data?.shows]
  )
  const releaseIDs = useMemo(
    () =>
      (releases.data?.releases ?? [])
        .map(release => release.release_id)
        .sort((a, b) => a - b),
    [releases.data?.releases]
  )

  const artistFollows = useBatchFollowStatus(
    'artists',
    isAuthenticated ? artistIDs : []
  )
  const venueFollows = useBatchFollowStatus(
    'venues',
    isAuthenticated ? venueIDs : []
  )
  const showSaves = useShowSaveCountBatch(
    isAuthenticated ? showIDs : [],
    isAuthenticated,
    user?.id
  )
  const releaseSaves = useReleaseSaveCountBatch(
    isAuthenticated ? releaseIDs : [],
    isAuthenticated,
    user?.id
  )
  const followFallback = { follower_count: 0, is_following: false }
  const saveFallback = { save_count: 0, is_saved: false }

  const changeWindow = (next: ChartWindow) => {
    void setWindow(next === 'quarter' ? null : next)
  }

  return (
    <div
      className={cn('space-y-6', isPending && 'opacity-75 transition-opacity')}
    >
      <header className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div className="min-w-0">
          <h1 className="font-display text-3xl font-bold leading-none">
            {selectedScene ? (
              <SceneSwitcher
                scenes={sceneList.data?.scenes ?? []}
                selectedScene={selectedScene}
                isLoading={sceneList.isLoading}
                isError={sceneList.isError && !selectedScene}
                onChange={nextScene => void setScene(nextScene)}
                onRetry={() => void sceneList.refetch()}
                treatment="masthead"
              />
            ) : (
              'Charts'
            )}
          </h1>
          <p className="mt-1 text-[13px] text-muted-foreground">
            {selectedScene ? (
              <>
                Scene charts · {metroDisplayName(selectedScene.name)} metro ·{' '}
                {countLabel(selectedScene.artist_count, 'artist')} based here ·{' '}
                {countLabel(selectedScene.venue_count, 'venue')} tracked
              </>
            ) : (
              <>
                The ledger of what’s moving — artists, shows, venues, releases,
                airwaves.
              </>
            )}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <div
            role="group"
            aria-label="Chart window"
            className="flex items-stretch gap-0.5"
          >
            {CHART_WINDOWS.map(value => (
              <button
                key={value}
                type="button"
                aria-pressed={window === value}
                onClick={() => changeWindow(value)}
                className={cn(
                  'border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  window === value && 'border-primary text-foreground'
                )}
              >
                {WINDOW_LABELS[value]}
              </button>
            ))}
          </div>
          {!selectedScene ? (
            <SceneSwitcher
              scenes={sceneList.data?.scenes ?? []}
              selectedScene={selectedScene}
              isLoading={sceneList.isLoading}
              isError={sceneList.isError && !selectedScene}
              onChange={nextScene => void setScene(nextScene)}
              onRetry={() => void sceneList.refetch()}
            />
          ) : null}
        </div>
      </header>

      {sceneValidationFailed ? (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 border-y border-destructive/40 py-3 text-sm"
        >
          <p>Unable to verify this scene. Your chart selection is preserved.</p>
          <button
            type="button"
            onClick={() => void sceneList.refetch()}
            className="font-mono text-xs text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Try again
          </button>
        </div>
      ) : (
        <StatStrip
          window={window}
          summary={summary.data}
          isLoading={summary.isLoading || !sceneResolved}
        />
      )}

      <PersonalStatsStrip />

      <div
        hidden={sceneValidationFailed}
        className="grid items-start gap-x-6 gap-y-6 md:grid-cols-2 xl:grid-cols-3"
      >
        <ChartModule
          title="Hardest-Working Artists"
          context={WINDOW_CONTEXT[window]}
          rowCount={active.data?.artists.length ?? 0}
          isLoading={active.isLoading || !sceneResolved}
          isError={active.isError}
          hasData={active.data !== undefined}
          testId="chart-most-active-artists"
          fullListHref={fullListHref(
            'most-active-artists',
            window,
            effectiveScene
          )}
        >
          {(active.data?.artists ?? []).map(artist => (
            <ChartRow
              key={artist.artist_id}
              rank={artist.rank}
              primary={
                <Link
                  href={entityHref('artists', artist.slug, artist.artist_id)}
                  className={linkClass}
                >
                  {artist.name}
                </Link>
              }
              meta={location(artist.city, artist.state)}
              stat={`${artist.show_count}${artist.rank === 1 ? ' shows' : ''}`}
              action={
                <FollowButton
                  entityType="artists"
                  entityId={artist.artist_id}
                  variant="bracket"
                  followData={
                    artistFollows.isError
                      ? undefined
                      : (artistFollows.data?.[String(artist.artist_id)] ??
                        followFallback)
                  }
                  disabled={artistFollows.isLoading}
                  className="font-mono text-[11px]"
                />
              }
            />
          ))}
        </ChartModule>

        <ChartModule
          title="On the Radio"
          context={WINDOW_CONTEXT[window]}
          rowCount={radio.data?.artists.length ?? 0}
          isLoading={radio.isLoading || !sceneResolved}
          isError={radio.isError}
          hasData={radio.data !== undefined}
          testId="chart-on-the-radio"
          fullListHref={fullListHref('on-the-radio', window, effectiveScene)}
        >
          {(radio.data?.artists ?? []).map(artist => (
            <ChartRow
              key={artist.artist_id}
              rank={artist.rank}
              primary={
                <span className="inline-flex max-w-full items-center gap-1.5">
                  <Link
                    href={entityHref('artists', artist.slug, artist.artist_id)}
                    className={cn(linkClass, 'truncate')}
                  >
                    {artist.name}
                  </Link>
                  {artist.is_new ? (
                    <Badge variant="accent" className="px-1 py-0 text-[9px]">
                      New
                    </Badge>
                  ) : null}
                </span>
              }
              meta={`${artist.play_count} ${artist.play_count === 1 ? 'play' : 'plays'} · ${artist.station_count} ${artist.station_count === 1 ? 'station' : 'stations'}`}
              stat={artist.play_count}
              action={
                <FollowButton
                  entityType="artists"
                  entityId={artist.artist_id}
                  variant="bracket"
                  followData={
                    artistFollows.isError
                      ? undefined
                      : (artistFollows.data?.[String(artist.artist_id)] ??
                        followFallback)
                  }
                  disabled={artistFollows.isLoading}
                  className="font-mono text-[11px]"
                />
              }
            />
          ))}
        </ChartModule>

        <ChartModule
          title="Most Anticipated Shows"
          context="upcoming"
          rowCount={anticipated.data?.shows.length ?? 0}
          isLoading={anticipated.isLoading || !sceneResolved}
          isError={anticipated.isError}
          hasData={anticipated.data !== undefined}
          testId="chart-most-anticipated"
          fullListHref={fullListHref(
            'most-anticipated',
            window,
            effectiveScene
          )}
        >
          {(anticipated.data?.shows ?? []).map((show, index) => (
            <ChartRow
              key={show.show_id}
              rank={
                show.rank ??
                (anticipated.data?.mode === 'ranked' ? index + 1 : null)
              }
              primary={
                <Link
                  href={entityHref('shows', show.slug, show.show_id)}
                  className={linkClass}
                >
                  {showDisplayTitle(show.title, show.artist_names)}
                </Link>
              }
              meta={
                <>
                  {formatDate(show.date)} ·{' '}
                  {show.venue_slug ? (
                    <Link
                      href={`/venues/${show.venue_slug}`}
                      className={linkClass}
                    >
                      {show.venue_name}
                    </Link>
                  ) : (
                    show.venue_name
                  )}
                  {show.city ? ` · ${show.city}` : ''}
                </>
              }
              stat={
                anticipated.data?.mode === 'ranked'
                  ? show.save_count
                  : undefined
              }
              action={
                <SaveButton
                  showId={show.show_id}
                  variant="bracket"
                  saveData={
                    showSaves.isError
                      ? undefined
                      : (showSaves.data?.[String(show.show_id)] ?? {
                          save_count: show.save_count ?? 0,
                          is_saved: false,
                        })
                  }
                  disabled={showSaves.isLoading}
                />
              }
            />
          ))}
        </ChartModule>

        <ChartModule
          title="Busiest Venues"
          context={WINDOW_CONTEXT[window]}
          rowCount={venues.data?.venues.length ?? 0}
          isLoading={venues.isLoading || !sceneResolved}
          isError={venues.isError}
          hasData={venues.data !== undefined}
          testId="chart-busiest-venues"
          fullListHref={fullListHref('busiest-venues', window, effectiveScene)}
        >
          {(venues.data?.venues ?? []).map(venue => (
            <ChartRow
              key={venue.venue_id}
              rank={venue.rank}
              primary={
                <Link
                  href={entityHref('venues', venue.slug, venue.venue_id)}
                  className={linkClass}
                >
                  {venue.name}
                </Link>
              }
              meta={location(venue.city, venue.state)}
              stat={`${venue.show_count}${venue.rank === 1 ? ' shows' : ''}`}
              action={
                <FollowButton
                  entityType="venues"
                  entityId={venue.venue_id}
                  variant="bracket"
                  followData={
                    venueFollows.isError
                      ? undefined
                      : (venueFollows.data?.[String(venue.venue_id)] ??
                        followFallback)
                  }
                  disabled={venueFollows.isLoading}
                  className="font-mono text-[11px]"
                />
              }
            />
          ))}
        </ChartModule>

        <ChartModule
          title="New Releases"
          context={
            window === 'all_time'
              ? 'all time'
              : window === 'month'
                ? 'this month'
                : 'this quarter'
          }
          rowCount={releases.data?.releases.length ?? 0}
          isLoading={releases.isLoading || !sceneResolved}
          isError={releases.isError}
          hasData={releases.data !== undefined}
          testId="chart-new-releases"
          fullListHref={fullListHref('new-releases', window, effectiveScene)}
        >
          {(releases.data?.releases ?? []).map(release => {
            const meta: ReactNode = (
              <>
                {getReleaseTypeLabel(release.release_type)}
                {release.artists.length > 0 ? (
                  <>
                    {' '}
                    ·{' '}
                    <EntityReferenceList
                      references={release.artists}
                      route="artists"
                    />
                  </>
                ) : null}
                {release.labels.length > 0 ? (
                  <>
                    {' '}
                    ·{' '}
                    <EntityReferenceList
                      references={release.labels}
                      route="labels"
                    />
                  </>
                ) : null}
              </>
            )
            return (
              <ChartRow
                key={release.release_id}
                rank={release.rank}
                primary={
                  <Link
                    href={entityHref(
                      'releases',
                      release.slug,
                      release.release_id
                    )}
                    className={linkClass}
                  >
                    {release.title}
                  </Link>
                }
                meta={meta}
                stat={formatDateOnly(release.release_date)}
                action={
                  <ReleaseSaveButton
                    releaseId={release.release_id}
                    variant="bracket"
                    saveData={
                      releaseSaves.isError
                        ? undefined
                        : (releaseSaves.data?.[String(release.release_id)] ??
                          saveFallback)
                    }
                    disabled={releaseSaves.isLoading}
                  />
                }
              />
            )
          })}
        </ChartModule>

        <ChartModule
          title="Openers to Watch"
          context={WINDOW_CONTEXT[window]}
          rowCount={openers.data?.artists.length ?? 0}
          isLoading={openers.isLoading || !sceneResolved}
          isError={openers.isError}
          hasData={openers.data !== undefined}
          testId="chart-openers-to-watch"
          fullListHref={fullListHref(
            'openers-to-watch',
            window,
            effectiveScene
          )}
        >
          {(openers.data?.artists ?? []).map(artist => (
            <ChartRow
              key={artist.artist_id}
              rank={artist.rank}
              primary={
                <Link
                  href={entityHref('artists', artist.slug, artist.artist_id)}
                  className={linkClass}
                >
                  {artist.name}
                </Link>
              }
              meta={location(artist.city, artist.state)}
              stat={`${artist.support_slot_count}${artist.rank === 1 ? ' slots' : ''}`}
              action={
                <FollowButton
                  entityType="artists"
                  entityId={artist.artist_id}
                  variant="bracket"
                  followData={
                    artistFollows.isError
                      ? undefined
                      : (artistFollows.data?.[String(artist.artist_id)] ??
                        followFallback)
                  }
                  disabled={artistFollows.isLoading}
                  className="font-mono text-[11px]"
                />
              }
            />
          ))}
        </ChartModule>
      </div>

      {!sceneValidationFailed && sceneResolved ? (
        <FreshlyAddedTicker items={freshlyAdded.data?.items ?? []} />
      ) : null}
    </div>
  )
}
