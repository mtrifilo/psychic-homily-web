'use client'

import { Suspense, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { redirect } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useInfiniteSavedShows, useUnsaveShow } from '@/features/shows'
import type { SavedShowResponse } from '@/features/shows'
import {
  useSavedReleases,
  type SavedReleaseResponse,
} from '@/features/releases'
import { getSavedReleasePageBounds } from '@/features/releases/savedReleasePagination'
import {
  useAllMyFollowing,
  useMyFollowing,
  useUnfollow,
} from '@/lib/hooks/common/useFollow'
import type { FollowingEntity } from '@/lib/types/follow'
import { formatShowTime } from '@/lib/utils/formatters'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { BracketLink, ReleaseSaveButton } from '@/components/shared'
import { CalendarFeedSection } from '@/features/collections'

// ---------------------------------------------------------------------------
// Tab definitions
// ---------------------------------------------------------------------------

const LIBRARY_TABS = [
  'shows',
  'artists',
  'venues',
  'scenes',
  'labels',
  'festivals',
  'releases',
] as const
type LibraryTab = (typeof LIBRARY_TABS)[number]

function isLibraryTab(value: string | null): value is LibraryTab {
  return value !== null && LIBRARY_TABS.includes(value as LibraryTab)
}

// ---------------------------------------------------------------------------
// Shared empty-state component
// ---------------------------------------------------------------------------

// Dense editorial empty state (Library board D): left-aligned title + one-line
// muted hint + a small outline browse CTA, with optional bracket-style mono
// discovery links. No giant centered icon.
function EmptyState({
  title,
  description,
  browseHref,
  browseLabel,
  discoveryLinks,
}: {
  title: string
  description: string
  browseHref: string
  browseLabel: string
  discoveryLinks?: { label: string; href: string }[]
}) {
  return (
    <div className="pb-6 pt-12">
      <p className="font-medium text-foreground">{title}</p>
      <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      <div className="mt-5 flex flex-wrap items-center gap-x-5 gap-y-2">
        <Button asChild variant="outline" size="sm">
          <Link href={browseHref}>{browseLabel}</Link>
        </Button>
        {discoveryLinks?.map(link => (
          <BracketLink
            key={link.href}
            label={link.label}
            href={link.href}
            className="font-mono text-[11px]"
          />
        ))}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Shows tab — the user's saved shows
// ---------------------------------------------------------------------------

const COLLAPSED_SHOW_COUNT = 4

function SavedShowCard({
  show,
  isPast,
  onRemove,
  isRemoving,
  isRemovalPending,
}: {
  show: SavedShowResponse
  isPast: boolean
  onRemove: (showId: number) => void
  isRemoving: boolean
  isRemovalPending: boolean
}) {
  const venue = show.venues[0]
  const artists = show.artists
  const dateBadge = formatShowDateBadge(
    show.event_date,
    show.state,
    show.venues?.[0]?.timezone
  )

  return (
    <article
      aria-label={show.title}
      className="grid grid-cols-[74px_minmax(0,1fr)] gap-x-3 border-b border-border py-2.5 md:grid-cols-[104px_minmax(0,1fr)_auto] md:gap-x-5 md:py-3"
    >
      <div
        className={`row-span-2 font-mono text-[11px] font-bold uppercase md:row-span-1 md:text-xs ${
          isPast ? 'text-muted-foreground' : 'text-primary'
        }`}
      >
        <span className="md:hidden">{dateBadge.monthDay}</span>
        <span className="hidden md:inline">
          {dateBadge.dayOfWeek} {dateBadge.monthDay}
        </span>
        <div className="mt-0.5 hidden text-[11px] font-normal normal-case text-muted-foreground md:block">
          {formatShowTime(
            show.event_date,
            show.state,
            show.venues?.[0]?.timezone
          )}
        </div>
      </div>

      <div className="min-w-0 self-center">
        <Link
          href={`/shows/${show.slug || show.id}`}
          className="block truncate text-sm font-medium leading-tight transition-colors hover:text-primary md:text-[15px]"
        >
          {artists.map(a => a.name).join(' \u00B7 ')}
        </Link>

        <div className="mt-0.5 truncate text-xs text-muted-foreground md:text-[13px]">
          {venue && (
            <>
              {venue.slug ? (
                <Link
                  href={`/venues/${venue.slug}`}
                  className={`transition-colors hover:text-primary ${
                    isPast ? '' : 'md:text-primary/80'
                  }`}
                >
                  {venue.name}
                </Link>
              ) : (
                <span className={isPast ? undefined : 'md:text-primary/80'}>
                  {venue.name}
                </span>
              )}
              {(venue.city || venue.state) && (
                <span>
                  {' '}
                  &middot;{' '}
                  {[venue.city, venue.state].filter(Boolean).join(', ')}
                </span>
              )}
            </>
          )}
        </div>
      </div>

      <div className="col-start-2 mt-1 flex items-center justify-between gap-3 font-mono text-[11px] text-muted-foreground md:col-start-3 md:row-start-1 md:mt-0 md:flex-col md:items-end md:self-center">
        <span className="whitespace-nowrap">
          saved {formatRelativeTime(show.saved_at, { short: true })}
        </span>
        <button
          type="button"
          onClick={() => onRemove(show.id)}
          disabled={isRemovalPending}
          className="whitespace-nowrap transition-colors hover:text-destructive disabled:cursor-wait disabled:opacity-60"
          aria-label={`Remove ${show.title} from saved shows`}
        >
          {isRemoving ? 'removing…' : '✕ remove'}
        </button>
      </div>
    </article>
  )
}

function SavedShowsSection({
  title,
  shows,
  total,
  isPast,
  hasNextPage,
  isFetchingNextPage,
  onExpand,
  onRemove,
  removingShowId,
  isRemovalPending,
}: {
  title: 'Upcoming' | 'Past'
  shows: SavedShowResponse[]
  total: number
  isPast: boolean
  hasNextPage: boolean
  isFetchingNextPage: boolean
  onExpand: () => Promise<void>
  onRemove: (showId: number) => void
  removingShowId?: number
  isRemovalPending: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const visibleShows = expanded ? shows : shows.slice(0, COLLAPSED_SHOW_COUNT)
  const hasExpandableRows = shows.length > COLLAPSED_SHOW_COUNT
  const countLabel = `${total} ${total === 1 ? 'show' : 'shows'}`
  const orderLabel = isPast ? 'most recent first' : 'soonest first'
  const headingId = `saved-shows-${title.toLowerCase()}`

  return (
    <section aria-labelledby={headingId}>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 border-b border-border pb-2">
        <h2 id={headingId} className="text-base font-semibold">
          {title}
        </h2>
        <p className="font-mono text-[11px] text-muted-foreground md:text-xs">
          {countLabel} · {orderLabel}
          {isPast && (
            <span className="hidden md:inline">
              {' '}
              · saved shows move here automatically when the date passes
            </span>
          )}
        </p>
        {isPast && (
          <p className="w-full font-mono text-[11px] text-muted-foreground md:hidden">
            Saved shows move here automatically when the date passes.
          </p>
        )}
      </div>

      {shows.length === 0 ? (
        <p className="py-4 text-sm text-muted-foreground">
          No {title.toLowerCase()} saved shows.
        </p>
      ) : (
        <div>
          {visibleShows.map(show => (
            <SavedShowCard
              key={show.id}
              show={show}
              isPast={isPast}
              onRemove={onRemove}
              isRemoving={removingShowId === show.id}
              isRemovalPending={isRemovalPending}
            />
          ))}
        </div>
      )}

      {(hasExpandableRows || hasNextPage) && (
        <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1">
          <BracketLink
            label={
              isFetchingNextPage
                ? 'Loading…'
                : expanded && hasNextPage
                  ? 'Retry loading'
                  : expanded
                    ? 'Show fewer'
                    : `View all ${total}`
            }
            disabled={isFetchingNextPage || isRemovalPending}
            onClick={async () => {
              if (expanded && !hasNextPage) {
                setExpanded(false)
                return
              }
              setExpanded(true)
              if (hasNextPage) await onExpand()
            }}
          />
          {expanded && hasNextPage && !isFetchingNextPage && (
            <span className="font-mono text-[11px] text-muted-foreground">
              Could not load every saved show. Retry when ready.
            </span>
          )}
        </div>
      )}
    </section>
  )
}

function ShowsTab({ currentUserId }: { currentUserId?: number }) {
  const upcoming = useInfiniteSavedShows(
    'upcoming',
    currentUserId,
    currentUserId !== undefined
  )
  const past = useInfiniteSavedShows(
    'past',
    currentUserId,
    currentUserId !== undefined
  )
  const unsaveShow = useUnsaveShow({
    syncMode: 'patch-infinite',
    userId: currentUserId,
  })

  const upcomingShows = upcoming.data?.pages.flatMap(page => page.shows) ?? []
  const pastShows = past.data?.pages.flatMap(page => page.shows) ?? []
  const upcomingTotal = upcoming.data?.pages[0]?.total ?? 0
  const pastTotal = past.data?.pages[0]?.total ?? 0
  const isInitialLoading =
    (upcoming.isLoading && !upcoming.data) || (past.isLoading && !past.data)
  const error = upcoming.error ?? past.error
  const isEmpty = upcomingTotal + pastTotal === 0
  const isSavedShowActionPending =
    unsaveShow.isPending ||
    upcoming.isFetchingNextPage ||
    past.isFetchingNextPage

  const fetchAllPages = async (
    query: typeof upcoming | typeof past
  ): Promise<void> => {
    let result = await query.fetchNextPage()
    while (result.hasNextPage && !result.isFetchNextPageError) {
      result = await query.fetchNextPage()
    }
  }

  return (
    <div className="space-y-7">
      <CalendarFeedSection />

      {isInitialLoading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </div>
      ) : error ? (
        <div className="py-12 text-center text-destructive">
          <p>Failed to load your shows. Please try again later.</p>
        </div>
      ) : isEmpty ? (
        <EmptyState
          title="Nothing saved yet."
          description="Save a show and it lands here — upcoming shows first, past ones kept as your record."
          browseHref="/shows"
          browseLabel="Browse shows"
          discoveryLinks={[
            { label: 'explore the graph', href: '/explore' },
            { label: 'the atlas', href: '/atlas' },
            { label: 'show submissions', href: '/contribute/submissions' },
          ]}
        />
      ) : (
        <div className="space-y-8">
          <SavedShowsSection
            title="Upcoming"
            shows={upcomingShows}
            total={upcomingTotal}
            isPast={false}
            hasNextPage={upcoming.hasNextPage}
            isFetchingNextPage={upcoming.isFetchingNextPage}
            onExpand={() => fetchAllPages(upcoming)}
            onRemove={unsaveShow.mutate}
            removingShowId={
              unsaveShow.isPending ? unsaveShow.variables : undefined
            }
            isRemovalPending={isSavedShowActionPending}
          />
          <SavedShowsSection
            title="Past"
            shows={pastShows}
            total={pastTotal}
            isPast
            hasNextPage={past.hasNextPage}
            isFetchingNextPage={past.isFetchingNextPage}
            onExpand={() => fetchAllPages(past)}
            onRemove={unsaveShow.mutate}
            removingShowId={
              unsaveShow.isPending ? unsaveShow.variables : undefined
            }
            isRemovalPending={isSavedShowActionPending}
          />
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Releases tab — the user's saved releases
// ---------------------------------------------------------------------------

function SavedReleaseCard({ release }: { release: SavedReleaseResponse }) {
  const hasArtists = release.artists.length > 0

  return (
    <article className="border-b border-border py-3 first:border-t">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
        <div className="min-w-0 flex-1">
          <Link
            href={`/releases/${release.slug || release.id}`}
            className="block truncate text-[15px] font-medium leading-tight transition-colors hover:text-primary"
          >
            {release.title}
          </Link>
          <div className="mt-1 flex min-w-0 flex-wrap items-baseline text-[13px] text-muted-foreground">
            {release.artists.map((artist, index) => (
              <span key={artist.id}>
                {index > 0 ? ' · ' : null}
                <Link
                  href={`/artists/${artist.slug || artist.id}`}
                  className="text-primary transition-colors hover:text-primary/80"
                >
                  {artist.name}
                </Link>
              </span>
            ))}
            {release.release_year != null ? (
              <span>
                {hasArtists ? ' · ' : null}
                {release.release_year}
              </span>
            ) : null}
            {release.label_name ? (
              <span>
                {hasArtists || release.release_year != null ? ' · ' : null}
                {release.label_slug ? (
                  <Link
                    href={`/labels/${release.label_slug}`}
                    className="transition-colors hover:text-foreground"
                  >
                    {release.label_name}
                  </Link>
                ) : (
                  release.label_name
                )}
              </span>
            ) : null}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2 font-mono text-[11px] text-muted-foreground">
          <span className="whitespace-nowrap">
            saved {formatRelativeTime(release.saved_at, { short: true })}
          </span>
          <ReleaseSaveButton
            releaseId={release.id}
            saveData={{ save_count: 0, is_saved: true }}
            variant="text"
            actionLabel="✕ remove"
            actionAriaLabel={`Remove ${release.title} from saved releases`}
          />
        </div>
      </div>
    </article>
  )
}

const SAVED_RELEASES_PAGE_SIZE = 50

function ReleasesTab({ userId }: { userId?: number }) {
  const router = useRouter()
  const searchParams = useSearchParams()
  const rawPage = Number.parseInt(searchParams.get('release_page') ?? '1', 10)
  const { page } = getSavedReleasePageBounds(
    rawPage,
    0,
    SAVED_RELEASES_PAGE_SIZE
  )
  const offset = (page - 1) * SAVED_RELEASES_PAGE_SIZE
  const { data, isLoading, error } = useSavedReleases(
    SAVED_RELEASES_PAGE_SIZE,
    offset,
    userId
  )
  const savedReleases = data?.releases ?? []
  const { totalPages, targetPage } = getSavedReleasePageBounds(
    page,
    data?.total ?? 0,
    SAVED_RELEASES_PAGE_SIZE
  )

  useEffect(() => {
    if (!data || page === targetPage) return
    const params = new URLSearchParams(searchParams.toString())
    if (targetPage <= 1) params.delete('release_page')
    else params.set('release_page', String(targetPage))
    const query = params.toString()
    router.replace(query ? `/library?${query}` : '/library', { scroll: false })
  }, [data, page, router, searchParams, targetPage])

  const changePage = (nextPage: number) => {
    const params = new URLSearchParams(searchParams.toString())
    if (nextPage <= 1) params.delete('release_page')
    else params.set('release_page', String(nextPage))
    const query = params.toString()
    router.replace(query ? `/library?${query}` : '/library', { scroll: false })
  }

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error && !data) {
    return (
      <div className="py-12 text-center text-destructive">
        <p>Failed to load your releases. Please try again later.</p>
      </div>
    )
  }

  if (data && page > totalPages) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (savedReleases.length === 0) {
    return (
      <EmptyState
        title="No releases saved yet."
        description="Save releases to see them here."
        browseHref="/releases"
        browseLabel="Browse releases"
      />
    )
  }

  return (
    <div>
      <section className="w-full">
        {savedReleases.map(release => (
          <SavedReleaseCard key={release.id} release={release} />
        ))}
      </section>
      {totalPages > 1 ? (
        <div className="mt-6 flex items-center justify-center gap-3">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => changePage(page - 1)}
          >
            Previous
          </Button>
          <span className="text-xs text-muted-foreground">
            Page {page} of {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => changePage(page + 1)}
          >
            Next
          </Button>
        </div>
      ) : null}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Following entity card (reused for Artists, Labels, Festivals tabs)
// ---------------------------------------------------------------------------

const entityTypeInfo: Record<
  string,
  { plural: string; label: string; href: (slug: string) => string }
> = {
  artist: {
    plural: 'artists',
    label: 'Artist',
    href: slug => `/artists/${slug}`,
  },
  venue: { plural: 'venues', label: 'Venue', href: slug => `/venues/${slug}` },
  scene: { plural: 'scenes', label: 'Scene', href: slug => `/scenes/${slug}` },
  label: { plural: 'labels', label: 'Label', href: slug => `/labels/${slug}` },
  festival: {
    plural: 'festivals',
    label: 'Festival',
    href: slug => `/festivals/${slug}`,
  },
}

function FollowingEntityCard({ entity }: { entity: FollowingEntity }) {
  const unfollow = useUnfollow()
  const info = entityTypeInfo[entity.entity_type]

  const handleUnfollow = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!info || unfollow.isPending) return
    unfollow.mutate({
      entityType: info.plural,
      entityId: entity.entity_type === 'scene' ? entity.slug : entity.entity_id,
    })
  }

  const href = info?.href(entity.slug) ?? '#'
  const followedDate = new Date(entity.followed_at)
  const formattedDate = followedDate.toLocaleDateString(undefined, {
    month: 'short',
    year: 'numeric',
  })

  return (
    <article className="border-b border-border py-3 first:border-t">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
        <Link
          href={href}
          className="min-w-0 truncate text-[15px] font-medium leading-tight transition-colors hover:text-primary"
        >
          {entity.name}
        </Link>

        <div className="flex shrink-0 items-center gap-2 font-mono text-[11px] text-muted-foreground">
          <span className="whitespace-nowrap">followed {formattedDate}</span>
          <BracketLink
            label={unfollow.isPending ? 'unfollowing…' : 'unfollow'}
            ariaLabel={`${unfollow.isPending ? 'Unfollowing' : 'Unfollow'} ${entity.name}`}
            onClick={handleUnfollow}
            disabled={unfollow.isPending || !info}
          />
        </div>
      </div>
      {unfollow.isError && (
        <p className="mt-2 text-xs text-destructive" role="alert">
          Couldn&apos;t unfollow {entity.name}. Try again.
        </p>
      )}
    </article>
  )
}

// ---------------------------------------------------------------------------
// Generic following list for a single entity type
// ---------------------------------------------------------------------------

function FollowingList({
  type,
  emptyTitle,
  emptyDescription,
  browseHref,
  browseLabel,
}: {
  type: string
  emptyTitle: string
  emptyDescription: string
  browseHref: string
  browseLabel: string
}) {
  const { data, isLoading, error, isFetching } = useAllMyFollowing(type)

  const following = [...(data?.following ?? [])].sort((a, b) =>
    a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
  )

  if (isLoading && !data) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center text-destructive py-12">
        <p>Failed to load. Please try again later.</p>
      </div>
    )
  }

  if (following.length === 0) {
    return (
      <EmptyState
        title={emptyTitle}
        description={emptyDescription}
        browseHref={browseHref}
        browseLabel={browseLabel}
      />
    )
  }

  return (
    <div
      className={
        isFetching
          ? 'opacity-60 transition-opacity duration-75'
          : 'transition-opacity duration-75'
      }
    >
      <section className="w-full">
        {following.map(entity => (
          <FollowingEntityCard
            key={`${entity.entity_type}-${entity.entity_id}`}
            entity={entity}
          />
        ))}
      </section>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main page content
// ---------------------------------------------------------------------------

const TAB_LABELS: Record<LibraryTab, string> = {
  shows: 'Shows',
  artists: 'Artists',
  venues: 'Venues',
  scenes: 'Scenes',
  releases: 'Releases',
  labels: 'Labels',
  festivals: 'Festivals',
}

function useFollowingTabCounts(): Partial<Record<LibraryTab, number>> {
  const artists = useMyFollowing({ type: 'artist', limit: 1 })
  const venues = useMyFollowing({ type: 'venue', limit: 1 })
  const scenes = useMyFollowing({ type: 'scene', limit: 1 })
  const labels = useMyFollowing({ type: 'label', limit: 1 })
  const festivals = useMyFollowing({ type: 'festival', limit: 1 })

  return {
    artists: artists.data?.total,
    venues: venues.data?.total,
    scenes: scenes.data?.total,
    labels: labels.data?.total,
    festivals: festivals.data?.total,
  }
}

function LibraryContent() {
  const searchParams = useSearchParams()
  const rawTab = searchParams.get('tab')

  if (rawTab === 'submissions') {
    const nextParams = new URLSearchParams(searchParams.toString())
    nextParams.delete('tab')
    const queryString = nextParams.toString()
    redirect(
      queryString
        ? `/contribute/submissions?${queryString}`
        : '/contribute/submissions'
    )
    return null
  }

  return <ActiveLibraryContent searchParams={searchParams} />
}

function ActiveLibraryContent({
  searchParams,
}: {
  searchParams: ReturnType<typeof useSearchParams>
}) {
  const router = useRouter()
  const { isAuthenticated, isLoading: authLoading, user } = useAuthContext()
  const followingTabCounts = useFollowingTabCounts()
  const currentUserId = user?.id ? Number(user.id) : undefined
  const savedReleaseCount = useSavedReleases(1, 0, currentUserId)
  const tabCounts: Partial<Record<LibraryTab, number>> = {
    ...followingTabCounts,
    releases: savedReleaseCount.data?.total,
  }
  const tabCountSignature = LIBRARY_TABS.map(tab => tabCounts[tab] ?? '').join(
    ':'
  )

  const rawTab = searchParams.get('tab')
  const currentTab: LibraryTab = isLibraryTab(rawTab) ? rawTab : 'shows'
  const tabListRef = useRef<HTMLDivElement | null>(null)
  const activeTabTriggerRef = useRef<HTMLButtonElement | null>(null)

  // Keep unrelated invalid tab values on Library's existing normalization path.
  useEffect(() => {
    if (rawTab && !isLibraryTab(rawTab)) {
      const nextParams = new URLSearchParams(searchParams.toString())
      nextParams.delete('tab')
      const queryString = nextParams.toString()
      router.replace(queryString ? `/library?${queryString}` : '/library', {
        scroll: false,
      })
    }
  }, [rawTab, router, searchParams])

  // A deep-linked trailing tab can begin outside the mobile scroll viewport.
  // Move only the horizontal tab scroller so page scroll restoration is intact.
  useEffect(() => {
    const tabList = tabListRef.current
    const activeTrigger = activeTabTriggerRef.current
    if (!tabList || !activeTrigger) return

    const listBounds = tabList.getBoundingClientRect()
    const triggerBounds = activeTrigger.getBoundingClientRect()
    let left = tabList.scrollLeft

    if (triggerBounds.left < listBounds.left) {
      left -= listBounds.left - triggerBounds.left
    } else if (triggerBounds.right > listBounds.right) {
      left += triggerBounds.right - listBounds.right
    } else {
      return
    }

    tabList.scrollTo({ left, behavior: 'instant' })
  }, [currentTab, tabCountSignature])

  const handleTabChange = (tab: string) => {
    if (!isLibraryTab(tab)) return
    const params = new URLSearchParams()
    if (tab !== 'shows') {
      params.set('tab', tab)
    }
    const queryString = params.toString()
    router.replace(queryString ? `/library?${queryString}` : '/library', {
      scroll: false,
    })
  }

  if (!authLoading && !isAuthenticated) {
    redirect('/auth')
  }

  if (authLoading) {
    return (
      <div className="flex justify-center items-center min-h-screen">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  return (
    <div className="container mx-auto max-w-6xl px-4 py-5 md:py-10">
      {/* Header (Library board A): plain editorial title, no icon */}
      <header className="mb-4 md:mb-7">
        <h1 className="text-2xl font-semibold tracking-tight md:text-[28px]">
          Library
        </h1>
        <p className="mt-1.5 hidden text-sm text-muted-foreground md:block">
          Your saved shows, and the artists, venues, scenes and labels you
          follow.
        </p>
      </header>

      {/* Tabs — underline style per the Library design direction (board A),
          horizontally scrollable on small screens instead of wrapping (board F) */}
      <Tabs
        value={currentTab}
        onValueChange={handleTabChange}
        className="w-full"
      >
        <TabsList
          ref={tabListRef}
          className="mb-6 h-auto w-full flex-nowrap justify-start gap-1 overflow-x-auto rounded-none border-b border-border bg-transparent p-0"
        >
          {LIBRARY_TABS.map(tab => (
            <TabsTrigger
              key={tab}
              ref={tab === currentTab ? activeTabTriggerRef : undefined}
              value={tab}
              aria-label={
                tabCounts[tab] === undefined
                  ? TAB_LABELS[tab]
                  : tab === 'releases'
                    ? `${TAB_LABELS[tab]}, ${tabCounts[tab]} saved`
                    : `${TAB_LABELS[tab]}, ${tabCounts[tab]} followed`
              }
              className="flex-none rounded-none border-0 border-b-2 border-b-transparent bg-transparent px-3 py-2 text-muted-foreground shadow-none data-[state=active]:border-b-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none dark:data-[state=active]:border-b-primary dark:data-[state=active]:bg-transparent"
            >
              {TAB_LABELS[tab]}
              {tabCounts[tab] !== undefined && (
                <span aria-hidden> · {tabCounts[tab]}</span>
              )}
            </TabsTrigger>
          ))}
        </TabsList>

        <TabsContent value="shows">
          <ShowsTab currentUserId={currentUserId} />
        </TabsContent>

        <TabsContent value="artists">
          <FollowingList
            type="artist"
            emptyTitle="No artists followed."
            emptyDescription="Follow artists to keep up with their shows and releases."
            browseHref="/artists"
            browseLabel="Browse artists"
          />
        </TabsContent>

        <TabsContent value="venues">
          <FollowingList
            type="venue"
            emptyTitle="No venues followed."
            emptyDescription="Follow venues to keep up with their upcoming shows."
            browseHref="/venues"
            browseLabel="Browse venues"
          />
        </TabsContent>

        <TabsContent value="scenes">
          <FollowingList
            type="scene"
            emptyTitle="No scenes followed."
            emptyDescription="Follow scenes to keep up with the places you care about."
            browseHref="/atlas"
            browseLabel="Explore scenes"
          />
        </TabsContent>

        <TabsContent value="releases">
          <ReleasesTab userId={currentUserId} />
        </TabsContent>

        <TabsContent value="labels">
          <FollowingList
            type="label"
            emptyTitle="No labels followed."
            emptyDescription="Follow labels to discover new releases and roster updates."
            browseHref="/labels"
            browseLabel="Browse labels"
          />
        </TabsContent>

        <TabsContent value="festivals">
          <FollowingList
            type="festival"
            emptyTitle="No festivals followed."
            emptyDescription="Follow festivals to get lineup and schedule updates."
            browseHref="/festivals"
            browseLabel="Browse festivals"
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function LibraryLoading() {
  return (
    <div className="flex justify-center items-center min-h-screen">
      <Loader2 className="h-8 w-8 animate-spin text-primary" />
    </div>
  )
}

export default function LibraryPage() {
  return (
    <Suspense fallback={<LibraryLoading />}>
      <LibraryContent />
    </Suspense>
  )
}
