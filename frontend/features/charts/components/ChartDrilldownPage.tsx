'use client'

import { useEffect, useMemo, useTransition, type ReactNode } from 'react'
import Link from 'next/link'
import {
  parseAsInteger,
  parseAsString,
  parseAsStringLiteral,
  useQueryState,
} from 'nuqs'
import { Badge } from '@/components/ui/badge'
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
  useMostActiveArtists,
  useMostAnticipated,
  useNewReleases,
  useOnTheRadio,
  useOpenersToWatch,
} from '../hooks'
import {
  CHART_MODULE_CONFIG,
  type ChartColumnKey,
  type ChartModuleSlug,
} from '../moduleConfig'
import {
  CHART_WINDOWS,
  type ChartEntityReference,
  type ChartWindow,
} from '../types'

const PAGE_SIZE = 50
const MAX_PAGE = 201 // Backend offsets are capped at 10,000.
const chartWindowParser =
  parseAsStringLiteral(CHART_WINDOWS).withDefault('quarter')
const pageParser = parseAsInteger.withDefault(1)
const WINDOW_LABELS: Record<ChartWindow, string> = {
  month: 'This Month',
  quarter: 'Quarter',
  all_time: 'All Time',
}
const linkClass =
  'hover:text-primary focus-visible:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

interface DrilldownRow {
  key: string
  rank: number | null
  cells: Partial<Record<ChartColumnKey, ReactNode>>
  action: ReactNode
}

function entityHref(route: string, slug: string, id: number): string {
  return `/${route}/${slug || id}`
}

function location(city: string, state: string): string {
  return [city, state].filter(Boolean).join(', ') || '—'
}

function formatDate(value: string | null, includeYear = false): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    ...(includeYear ? { year: 'numeric' } : {}),
    timeZone: 'UTC',
  })
}

function ReferenceList({
  references,
  route,
}: {
  references: ChartEntityReference[]
  route: 'artists' | 'labels'
}) {
  if (references.length === 0) return '—'
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

function paginationItems(currentPage: number, totalPages: number) {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, index) => index + 1)
  }
  const pages = new Set([1, totalPages, currentPage])
  if (currentPage > 1) pages.add(currentPage - 1)
  if (currentPage < totalPages) pages.add(currentPage + 1)
  const sorted = [...pages].sort((a, b) => a - b)
  const items: Array<number | 'ellipsis'> = []
  sorted.forEach((page, index) => {
    if (index > 0 && page - sorted[index - 1] > 1) items.push('ellipsis')
    items.push(page)
  })
  return items
}

export function ChartDrilldownPage({ module }: { module: ChartModuleSlug }) {
  const config = CHART_MODULE_CONFIG[module]
  const [isPending, startTransition] = useTransition()
  const [window, setWindow] = useQueryState(
    'window',
    chartWindowParser.withOptions({ history: 'push', startTransition })
  )
  const [scene, setScene] = useQueryState(
    'scene',
    parseAsString.withOptions({ history: 'push', startTransition })
  )
  const [rawPage, setPage] = useQueryState(
    'page',
    pageParser.withOptions({ history: 'push', startTransition })
  )
  const page = Math.min(MAX_PAGE, Math.max(1, rawPage))
  const offset = (page - 1) * PAGE_SIZE
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
  const queryOptions = (selectedModule: ChartModuleSlug) => ({
    scene: effectiveScene,
    offset,
    enabled: sceneResolved && module === selectedModule,
  })

  useEffect(() => {
    if (
      rawPage < 1 ||
      rawPage > MAX_PAGE ||
      (scene &&
        (!sceneHasValidShape || (sceneValidationComplete && !selectedScene)))
    ) {
      if (rawPage < 1) void setPage(null)
      if (rawPage > MAX_PAGE) void setPage(MAX_PAGE)
      if (scene) void setScene(null)
    }
  }, [
    rawPage,
    scene,
    sceneHasValidShape,
    sceneValidationComplete,
    selectedScene,
    setPage,
    setScene,
  ])

  const active = useMostActiveArtists(
    window,
    PAGE_SIZE,
    queryOptions('most-active-artists')
  )
  const radio = useOnTheRadio(window, PAGE_SIZE, queryOptions('on-the-radio'))
  const anticipated = useMostAnticipated(
    window,
    PAGE_SIZE,
    queryOptions('most-anticipated')
  )
  const venues = useBusiestVenues(
    window,
    PAGE_SIZE,
    queryOptions('busiest-venues')
  )
  const releases = useNewReleases(
    window,
    PAGE_SIZE,
    queryOptions('new-releases')
  )
  const openers = useOpenersToWatch(
    window,
    PAGE_SIZE,
    queryOptions('openers-to-watch')
  )

  const artistIDs = useMemo(() => {
    if (module === 'most-active-artists')
      return (active.data?.artists ?? []).map(row => row.artist_id)
    if (module === 'on-the-radio')
      return (radio.data?.artists ?? []).map(row => row.artist_id)
    if (module === 'openers-to-watch')
      return (openers.data?.artists ?? []).map(row => row.artist_id)
    return []
  }, [active.data, module, openers.data, radio.data])
  const venueIDs = useMemo(
    () =>
      module === 'busiest-venues'
        ? (venues.data?.venues ?? []).map(row => row.venue_id)
        : [],
    [module, venues.data]
  )
  const showIDs = useMemo(
    () =>
      module === 'most-anticipated'
        ? (anticipated.data?.shows ?? []).map(row => row.show_id)
        : [],
    [anticipated.data, module]
  )
  const releaseIDs = useMemo(
    () =>
      module === 'new-releases'
        ? (releases.data?.releases ?? []).map(row => row.release_id)
        : [],
    [module, releases.data]
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

  let rows: DrilldownRow[] = []
  let total = 0
  let isLoading = !sceneResolved && !sceneValidationFailed
  let isError = false

  switch (module) {
    case 'most-active-artists':
      total = active.data?.total ?? 0
      isLoading ||= active.isLoading
      isError = active.isError && active.data === undefined
      rows = (active.data?.artists ?? []).map(artist => ({
        key: String(artist.artist_id),
        rank: artist.rank,
        cells: {
          artist: <span>
            <Link
              href={entityHref('artists', artist.slug, artist.artist_id)}
              className={linkClass}
            >
              {artist.name}
            </Link>
            <span className="block text-[11px] text-muted-foreground">
              {location(artist.city, artist.state)}
            </span>
          </span>,
          shows: artist.show_count,
          headline: `${artist.headline_pct}%`,
          'last-show': artist.last_show_slug ? (
            <Link
              href={`/shows/${artist.last_show_slug}`}
              className={linkClass}
            >
              {artist.last_show_venue || formatDate(artist.last_show_date)}
              <span className="block text-[11px] text-muted-foreground">
                {formatDate(artist.last_show_date)}
              </span>
            </Link>
          ) : (
            '—'
          ),
        },
        action: (
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
        ),
      }))
      break
    case 'on-the-radio':
      total = radio.data?.total ?? 0
      isLoading ||= radio.isLoading
      isError = radio.isError && radio.data === undefined
      rows = (radio.data?.artists ?? []).map(artist => ({
        key: String(artist.artist_id),
        rank: artist.rank,
        cells: {
          artist: <span>
            <Link
              href={entityHref('artists', artist.slug, artist.artist_id)}
              className={linkClass}
            >
              {artist.name}
            </Link>
            <span className="block text-[11px] text-muted-foreground">
              {location(artist.city, artist.state)}
            </span>
          </span>,
          plays: artist.play_count,
          stations: artist.station_count,
          rotation: artist.is_new ? <Badge>New</Badge> : 'Current',
        },
        action: (
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
        ),
      }))
      break
    case 'most-anticipated':
      total = anticipated.data?.total ?? 0
      isLoading ||= anticipated.isLoading
      isError = anticipated.isError && anticipated.data === undefined
      rows = (anticipated.data?.shows ?? []).map(show => ({
        key: String(show.show_id),
        rank: show.rank ?? null,
        cells: {
          show: <span>
            <Link
              href={entityHref('shows', show.slug, show.show_id)}
              className={linkClass}
            >
              {showDisplayTitle(show.title, show.artist_names)}
            </Link>
            {show.artist_names.length > 0 ? (
              <span className="block text-[11px] text-muted-foreground">
                {show.artist_names.join(', ')}
              </span>
            ) : null}
          </span>,
          date: formatDate(show.date, true),
          venue: show.venue_slug ? (
            <Link
              href={`/venues/${show.venue_slug}`}
              className={linkClass}
            >
              {show.venue_name}
              {show.city ? (
                <span className="block text-[11px] text-muted-foreground">
                  {show.city}
                </span>
              ) : null}
            </Link>
          ) : (
            show.venue_name || '—'
          ),
          saves: show.save_count ?? '—',
        },
        action: (
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
        ),
      }))
      break
    case 'busiest-venues':
      total = venues.data?.total ?? 0
      isLoading ||= venues.isLoading
      isError = venues.isError && venues.data === undefined
      rows = (venues.data?.venues ?? []).map(venue => ({
        key: String(venue.venue_id),
        rank: venue.rank,
        cells: {
          venue: (
            <Link
              href={entityHref('venues', venue.slug, venue.venue_id)}
              className={linkClass}
            >
              {venue.name}
            </Link>
          ),
          location: location(venue.city, venue.state),
          shows: venue.show_count,
        },
        action: (
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
        ),
      }))
      break
    case 'new-releases':
      total = releases.data?.total ?? 0
      isLoading ||= releases.isLoading
      isError = releases.isError && releases.data === undefined
      rows = (releases.data?.releases ?? []).map(release => ({
        key: String(release.release_id),
        rank: release.rank,
        cells: {
          release: <span>
            <Link
              href={entityHref('releases', release.slug, release.release_id)}
              className={linkClass}
            >
              {release.title}
            </Link>
            <span className="block text-[11px] text-muted-foreground">
              {getReleaseTypeLabel(release.release_type)}
            </span>
          </span>,
          artists: <ReferenceList
            references={release.artists}
            route="artists"
          />,
          labels: <ReferenceList
            references={release.labels}
            route="labels"
          />,
          released: formatDate(release.release_date, true),
          added: formatDate(release.added_at, true),
        },
        action: (
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
        ),
      }))
      break
    case 'openers-to-watch':
      total = openers.data?.total ?? 0
      isLoading ||= openers.isLoading
      isError = openers.isError && openers.data === undefined
      rows = (openers.data?.artists ?? []).map(artist => ({
        key: String(artist.artist_id),
        rank: artist.rank,
        cells: {
          artist: (
            <Link
              href={entityHref('artists', artist.slug, artist.artist_id)}
              className={linkClass}
            >
              {artist.name}
            </Link>
          ),
          location: location(artist.city, artist.state),
          slots: artist.support_slot_count,
        },
        action: (
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
        ),
      }))
      break
  }

  const reachableTotal = Math.min(total, MAX_PAGE * PAGE_SIZE)
  const totalPages = Math.max(1, Math.ceil(reachableTotal / PAGE_SIZE))
  useEffect(() => {
    if (!isLoading && total > 0 && page > totalPages) void setPage(totalPages)
  }, [isLoading, page, setPage, total, totalPages])

  const goToPage = (nextPage: number) =>
    void setPage(nextPage === 1 ? null : nextPage)
  const changeWindow = (nextWindow: ChartWindow) => {
    void setPage(null)
    void setWindow(nextWindow === 'quarter' ? null : nextWindow)
  }
  const changeScene = (nextScene: string) => {
    void setPage(null)
    void setScene(nextScene || null)
  }
  const showingStart = total === 0 ? 0 : offset + 1
  const showingEnd = Math.min(offset + rows.length, total)

  return (
    <div className={cn('space-y-6', isPending && 'opacity-75')}>
      <header className="space-y-4">
        <Link
          href="/charts"
          className="font-mono text-xs text-primary hover:underline"
        >
          ← Charts / {config.title}
        </Link>
        <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
          <div>
            <h1 className="font-display text-3xl font-bold leading-none">
              {config.title}
            </h1>
            <p className="mt-1 text-[13px] text-muted-foreground">
              {isLoading
                ? 'Counting qualifying rows…'
                : `${total.toLocaleString()} qualifying ${config.qualifyingNoun}`}
              {selectedScene ? ` · ${selectedScene.city} scene` : ''}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <div
              role="group"
              aria-label="Chart window"
              className="flex gap-0.5"
            >
              {CHART_WINDOWS.map(value => (
                <button
                  key={value}
                  type="button"
                  aria-pressed={window === value}
                  onClick={() => changeWindow(value)}
                  className={cn(
                    'border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                    window === value && 'border-primary text-foreground'
                  )}
                >
                  {WINDOW_LABELS[value]}
                </button>
              ))}
            </div>
            <label className="rounded-sm border border-border px-2 py-1 font-mono text-[11px]">
              <span className="sr-only">Chart scene</span>
              <select
                value={selectedScene?.metro ?? ''}
                disabled={sceneList.isLoading}
                onChange={event => changeScene(event.target.value)}
                className="bg-transparent focus:outline-none"
                aria-label="Chart scene"
              >
                <option value="">All scenes</option>
                {(sceneList.data?.scenes ?? []).map(option => (
                  <option key={option.metro} value={option.metro}>
                    {location(option.city, option.state)}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </div>
      </header>

      <div className="overflow-x-auto border-t-2 border-foreground">
        <table className="w-full min-w-[760px] border-collapse text-left text-xs">
          <thead className="font-mono text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
            <tr className="border-b border-border">
              <th className="w-12 px-2 py-2 font-bold">#</th>
              {config.columns.map(column => (
                <th
                  key={column.key}
                  className={cn('px-2 py-2 font-bold', column.className)}
                >
                  {column.label}
                </th>
              ))}
              <th className="w-[70px] min-w-[70px] max-w-[70px] px-2 py-2 text-right font-bold">
                {module === 'most-anticipated' || module === 'new-releases'
                  ? 'Save'
                  : 'Follow'}
              </th>
            </tr>
          </thead>
          <tbody>
            {sceneValidationFailed ? (
              <tr>
                <td
                  colSpan={config.columns.length + 2}
                  className="px-2 py-8 text-center text-destructive"
                >
                  <p>
                    Unable to verify this scene. Your selection is preserved.
                  </p>
                  <button
                    type="button"
                    onClick={() => void sceneList.refetch()}
                    className="mt-2 font-mono text-xs text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    Try again
                  </button>
                </td>
              </tr>
            ) : isLoading ? (
              Array.from({ length: 8 }, (_, index) => (
                <tr key={index} className="border-b border-border">
                  <td colSpan={config.columns.length + 2} className="px-2 py-3">
                    <span className="block h-4 animate-pulse rounded-sm bg-muted" />
                  </td>
                </tr>
              ))
            ) : isError ? (
              <tr>
                <td
                  colSpan={config.columns.length + 2}
                  className="px-2 py-8 text-center text-destructive"
                >
                  Unable to load this chart.
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td
                  colSpan={config.columns.length + 2}
                  className="px-2 py-8 text-center text-muted-foreground"
                >
                  No qualifying rows in this window and scene.
                </td>
              </tr>
            ) : (
              rows.map(row => (
                <tr key={row.key} className="border-b border-border align-top">
                  <td className="px-2 py-3 font-mono tabular-nums text-muted-foreground">
                    {row.rank ?? '—'}
                  </td>
                  {config.columns.map((column, index) => (
                    <td
                      key={column.key}
                      className={cn(
                        'px-2 py-3 leading-4',
                        column.className,
                        index > 0 && 'font-mono tabular-nums'
                      )}
                    >
                      {row.cells[column.key]}
                    </td>
                  ))}
                  <td className="w-[70px] min-w-[70px] max-w-[70px] px-2 py-3 text-right leading-none">
                    {row.action}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {!sceneValidationFailed && !isLoading && !isError ? (
        <nav
          aria-label="Chart pagination"
          className="flex flex-col gap-3 border-t border-border pt-4 font-mono text-xs sm:flex-row sm:items-center sm:justify-between"
        >
          <p className="text-muted-foreground">
            Showing {showingStart}–{showingEnd} of {total.toLocaleString()}
            {total > reachableTotal
              ? ` · first ${reachableTotal.toLocaleString()} accessible`
              : ''}
          </p>
          <div className="flex flex-wrap items-center gap-1">
            <button
              type="button"
              disabled={page <= 1}
              onClick={() => goToPage(page - 1)}
              className="px-2 py-1 text-primary disabled:text-muted-foreground"
            >
              Previous
            </button>
            {paginationItems(page, totalPages).map((item, index) =>
              item === 'ellipsis' ? (
                <span
                  key={`ellipsis-${index}`}
                  className="px-1 text-muted-foreground"
                >
                  …
                </span>
              ) : (
                <button
                  key={item}
                  type="button"
                  aria-current={page === item ? 'page' : undefined}
                  onClick={() => goToPage(item)}
                  className={cn(
                    'min-w-7 px-2 py-1 text-primary',
                    page === item && 'bg-primary text-primary-foreground'
                  )}
                >
                  {item}
                </button>
              )
            )}
            <button
              type="button"
              disabled={page >= totalPages}
              onClick={() => goToPage(page + 1)}
              className="px-2 py-1 text-primary disabled:text-muted-foreground"
            >
              Next
            </button>
          </div>
        </nav>
      ) : null}
    </div>
  )
}
