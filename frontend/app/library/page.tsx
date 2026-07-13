'use client'

import { Suspense, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { redirect } from 'next/navigation'
import {
  Loader2,
  Clock,
  CheckCircle2,
  EyeOff,
  Pencil,
  X,
  Trash2,
  Globe,
  Ban,
  TicketX,
  MoreVertical,
} from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useInfiniteSavedShows,
  useMySubmissions,
  useUnsaveShow,
} from '@/features/shows'
import type { SavedShowResponse, ShowResponse } from '@/features/shows'
import {
  getReleaseTypeLabel,
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
import {
  formatShowDate,
  formatShowTime,
  formatPrice,
} from '@/lib/utils/formatters'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  BracketLink,
  ReleaseSaveButton,
  SaveButton,
  SubmissionSuccessDialog,
} from '@/components/shared'
import {
  DeleteShowDialog,
  UnpublishShowDialog,
  MakePrivateDialog,
  PublishShowDialog,
} from '@/features/shows'
import { VenueDeniedDialog } from '@/features/venues'
import { CalendarFeedSection } from '@/features/collections'
import {
  useSetShowSoldOut,
  useSetShowCancelled,
} from '@/lib/hooks/admin/useAdminShows'
import { ShowForm } from '@/features/shows'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { SHOW_LIST_FEATURE_POLICY } from '@/features/shows'

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
  'submissions',
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
  return (
    <article className="-mx-3 flex items-start justify-between gap-3 rounded-lg border-b border-border/50 px-3 py-4 transition-colors duration-200 hover:bg-muted/30">
      <div className="min-w-0 flex-1">
        <Link
          href={`/releases/${release.slug || release.id}`}
          className="font-semibold leading-tight transition-colors hover:text-primary"
        >
          {release.title}
        </Link>
        <p className="mt-1 text-sm text-muted-foreground">
          {release.artists.map((artist, index) => (
            <span key={artist.id}>
              {index > 0 ? ' · ' : null}
              <Link
                href={`/artists/${artist.slug || artist.id}`}
                className="transition-colors hover:text-primary"
              >
                {artist.name}
              </Link>
            </span>
          ))}
          {release.artists.length > 0 && release.label_name ? ' · ' : null}
          {release.label_name && release.label_slug ? (
            <Link
              href={`/labels/${release.label_slug}`}
              className="transition-colors hover:text-primary"
            >
              {release.label_name}
            </Link>
          ) : (
            release.label_name
          )}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-3">
        <span className="hidden text-xs uppercase tracking-wide text-muted-foreground sm:inline">
          {getReleaseTypeLabel(release.release_type)}
        </span>
        <ReleaseSaveButton
          releaseId={release.id}
          saveData={{ save_count: 0, is_saved: true }}
          variant="bracket"
        />
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
// Submissions tab — user-submitted shows with owner controls
// ---------------------------------------------------------------------------

interface SubmissionShowCardProps {
  show: SavedShowResponse | ShowResponse
  currentUserId?: number
  isAdmin?: boolean
}

function SubmissionShowCard({
  show,
  currentUserId,
  isAdmin,
}: SubmissionShowCardProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isUnpublishDialogOpen, setIsUnpublishDialogOpen] = useState(false)
  const [isMakePrivateDialogOpen, setIsMakePrivateDialogOpen] = useState(false)
  const [isPublishDialogOpen, setIsPublishDialogOpen] = useState(false)
  const [isVenueDeniedDialogOpen, setIsVenueDeniedDialogOpen] = useState(false)
  const venue = show.venues[0]
  const artists = show.artists

  // Status mutation hooks
  const setSoldOutMutation = useSetShowSoldOut()
  const setCancelledMutation = useSetShowCancelled()

  // Check if user owns this show
  const isOwner = currentUserId && show.submitted_by === currentUserId

  // Check if user can unpublish this show (approved -> private)
  const canUnpublish = show.status === 'approved' && (isAdmin || isOwner)

  // Check if user can make show private (pending -> private)
  // Note: New shows are never pending, but legacy data may have this status
  const canMakePrivate = show.status === 'pending' && (isAdmin || isOwner)

  // Check if user can publish show (private/rejected -> approved)
  const canPublish =
    (show.status === 'private' || show.status === 'rejected') &&
    (isAdmin || isOwner)

  // Check if user can edit: admin or show owner
  const canEdit = isAdmin || isOwner

  // Check if user can delete: admin or show owner
  const canDelete = isAdmin || isOwner

  // Check if user can toggle status (admin or owner)
  const canToggleStatus = isAdmin || isOwner

  const handleToggleSoldOut = () => {
    setSoldOutMutation.mutate({ showId: show.id, value: !show.is_sold_out })
  }

  const handleToggleCancelled = () => {
    setCancelledMutation.mutate({ showId: show.id, value: !show.is_cancelled })
  }

  const handleEditSuccess = () => {
    setIsEditing(false)
  }

  const handleEditCancel = () => {
    setIsEditing(false)
  }

  return (
    <article className="border-b border-border/50 py-5 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date, Location, and Status */}
        <div className="w-full md:w-1/5 md:pr-4 mb-2 md:mb-0">
          <h2 className="text-sm font-bold tracking-wide text-primary">
            {formatShowDate(
              show.event_date,
              show.state,
              false,
              show.venues?.[0]?.timezone
            )}
          </h2>
          <h3 className="text-xs text-muted-foreground mt-0.5">
            {show.city}, {show.state}
          </h3>

          {/* Status Badge - only show for owner's own shows or admins */}
          <div className="mt-2 flex flex-col gap-1">
            {(isAdmin || isOwner) &&
              (show.status === 'approved' ? (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 w-fit">
                  <CheckCircle2 className="h-3 w-3" />
                  Published
                </span>
              ) : show.status === 'pending' ? (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400 w-fit">
                  <Clock className="h-3 w-3" />
                  Pending
                </span>
              ) : show.status === 'private' || show.status === 'rejected' ? (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-slate-500/10 text-slate-600 dark:text-slate-400 w-fit">
                  <EyeOff className="h-3 w-3" />
                  Private
                </span>
              ) : null)}

            {/* Sold Out Badge */}
            {show.is_sold_out && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-rose-500/10 text-rose-600 dark:text-rose-400 w-fit">
                <TicketX className="h-3 w-3" />
                Sold Out
              </span>
            )}

            {/* Cancelled Badge */}
            {show.is_cancelled && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-slate-500/10 text-slate-600 dark:text-slate-400 w-fit">
                <Ban className="h-3 w-3" />
                Cancelled
              </span>
            )}
          </div>
        </div>

        {/* Right column: Artists, Venue, Details */}
        <div className="w-full md:w-4/5 md:pl-4">
          <div className="flex items-start justify-between gap-2">
            {/* Artists */}
            <h1 className="text-lg font-semibold leading-tight tracking-tight flex-1">
              {artists.map((artist, index) => (
                <span key={artist.id}>
                  {index > 0 && (
                    <span className="text-muted-foreground/60 font-normal">
                      &nbsp;•&nbsp;
                    </span>
                  )}
                  {artist.slug ? (
                    <Link
                      href={`/artists/${artist.slug}`}
                      className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
                    >
                      {artist.name}
                    </Link>
                  ) : artist.socials?.instagram ? (
                    <a
                      href={`https://instagram.com/${artist.socials.instagram}`}
                      className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {artist.name}
                    </a>
                  ) : (
                    <span>{artist.name}</span>
                  )}
                </span>
              ))}
            </h1>

            {/* Action Buttons */}
            <div className="flex items-center gap-1 shrink-0">
              {/* Save Button - always visible for quick access */}
              {SHOW_LIST_FEATURE_POLICY.ownership.showSaveButton && (
                <SaveButton showId={show.id} variant="ghost" size="sm" />
              )}

              {/* Cancel Edit Button - shown when editing */}
              {isEditing && (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setIsEditing(false)}
                  className="h-7 w-7 p-0"
                  aria-label="Cancel editing"
                >
                  <X className="h-4 w-4" />
                </Button>
              )}

              {/* Overflow Menu - secondary actions */}
              {SHOW_LIST_FEATURE_POLICY.ownership.showOwnerActions &&
                canEdit &&
                !isEditing && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground"
                      >
                        <MoreVertical className="h-4 w-4" />
                        <span className="sr-only">Show actions</span>
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {/* Edit */}
                      <DropdownMenuItem onClick={() => setIsEditing(true)}>
                        <Pencil className="h-4 w-4 mr-2" />
                        Edit show
                      </DropdownMenuItem>

                      {/* Visibility controls */}
                      {canUnpublish && (
                        <DropdownMenuItem
                          onClick={() => setIsUnpublishDialogOpen(true)}
                        >
                          <EyeOff className="h-4 w-4 mr-2" />
                          Make private
                        </DropdownMenuItem>
                      )}
                      {canMakePrivate && (
                        <DropdownMenuItem
                          onClick={() => setIsMakePrivateDialogOpen(true)}
                        >
                          <EyeOff className="h-4 w-4 mr-2" />
                          Make private
                        </DropdownMenuItem>
                      )}
                      {canPublish && (
                        <DropdownMenuItem
                          onClick={() => {
                            if (show.status === 'rejected') {
                              setIsVenueDeniedDialogOpen(true)
                            } else {
                              setIsPublishDialogOpen(true)
                            }
                          }}
                        >
                          <Globe className="h-4 w-4 mr-2" />
                          Publish show
                        </DropdownMenuItem>
                      )}

                      <DropdownMenuSeparator />

                      {/* Status toggles */}
                      {canToggleStatus && (
                        <DropdownMenuItem
                          onClick={handleToggleSoldOut}
                          disabled={setSoldOutMutation.isPending}
                        >
                          <TicketX className="h-4 w-4 mr-2" />
                          {show.is_sold_out ? 'Undo sold out' : 'Mark sold out'}
                        </DropdownMenuItem>
                      )}
                      {canToggleStatus && (
                        <DropdownMenuItem
                          onClick={handleToggleCancelled}
                          disabled={setCancelledMutation.isPending}
                        >
                          <Ban className="h-4 w-4 mr-2" />
                          {show.is_cancelled
                            ? 'Undo cancelled'
                            : 'Mark cancelled'}
                        </DropdownMenuItem>
                      )}

                      {/* Delete - destructive, always last */}
                      {canDelete && (
                        <>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => setIsDeleteDialogOpen(true)}
                          >
                            <Trash2 className="h-4 w-4 mr-2" />
                            Delete show
                          </DropdownMenuItem>
                        </>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
            </div>
          </div>

          {/* Venue and Details */}
          <div className="text-sm mt-1.5 text-muted-foreground">
            {venue &&
              (venue.slug ? (
                <Link
                  href={`/venues/${venue.slug}`}
                  className="text-primary/80 hover:text-primary font-medium transition-colors"
                >
                  {venue.name}
                </Link>
              ) : (
                <span className="text-primary/80 font-medium">
                  {venue.name}
                </span>
              ))}
            {show.price != null && (
              <span>&nbsp;•&nbsp;{formatPrice(show.price)}</span>
            )}
            {show.age_requirement && (
              <span>&nbsp;•&nbsp;{show.age_requirement}</span>
            )}
            <span>
              &nbsp;•&nbsp;
              {formatShowTime(
                show.event_date,
                show.state,
                show.venues?.[0]?.timezone
              )}
            </span>
            {SHOW_LIST_FEATURE_POLICY.ownership.showDetailsLink && (
              <>
                <span>&nbsp;•&nbsp;</span>
                <Link
                  href={`/shows/${show.slug || show.id}`}
                  className="text-primary/80 hover:text-primary underline underline-offset-2 transition-colors"
                >
                  Details
                </Link>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Inline Edit Form */}
      {isEditing && (
        <div className="mt-4 pt-4 border-t border-border/50">
          <ShowForm
            mode="edit"
            initialData={show}
            onSuccess={handleEditSuccess}
            onCancel={handleEditCancel}
          />
        </div>
      )}

      {/* Unpublish Confirmation Dialog */}
      <UnpublishShowDialog
        show={show}
        open={isUnpublishDialogOpen}
        onOpenChange={setIsUnpublishDialogOpen}
      />

      {/* Make Private Dialog */}
      <MakePrivateDialog
        show={show}
        open={isMakePrivateDialogOpen}
        onOpenChange={setIsMakePrivateDialogOpen}
      />

      {/* Publish Show Dialog */}
      <PublishShowDialog
        show={show}
        open={isPublishDialogOpen}
        onOpenChange={setIsPublishDialogOpen}
      />

      {/* Venue Denied Dialog (for rejected shows) */}
      <VenueDeniedDialog
        show={show}
        open={isVenueDeniedDialogOpen}
        onOpenChange={setIsVenueDeniedDialogOpen}
      />

      {/* Delete Confirmation Dialog */}
      <DeleteShowDialog
        show={show}
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
      />
    </article>
  )
}

function SubmissionsTab({
  currentUserId,
  isAdmin,
}: {
  currentUserId?: number
  isAdmin?: boolean
}) {
  const { isAuthenticated } = useAuthContext()
  const { data, isLoading, error } = useMySubmissions({
    enabled: isAuthenticated,
  })

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center text-destructive py-12">
        <p>Failed to load your submissions. Please try again later.</p>
      </div>
    )
  }

  const shows = data?.shows || []

  if (shows.length === 0) {
    return (
      <EmptyState
        title="No submissions yet."
        description="Shows you submit will appear here."
        browseHref="/shows/submit"
        browseLabel="Submit a show"
      />
    )
  }

  return (
    <section className="w-full">
      {shows.map(show => (
        <SubmissionShowCard
          key={show.id}
          show={show as SavedShowResponse}
          currentUserId={currentUserId}
          isAdmin={isAdmin}
        />
      ))}
    </section>
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
      entityId:
        entity.entity_type === 'scene' ? entity.slug : entity.entity_id,
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
  submissions: 'Submissions',
}

const FOLLOWING_TAB_TYPES = {
  artists: 'artist',
  venues: 'venue',
  scenes: 'scene',
  labels: 'label',
  festivals: 'festival',
} as const satisfies Partial<Record<LibraryTab, string>>

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
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading: authLoading, user } = useAuthContext()
  const followingTabCounts = useFollowingTabCounts()

  const rawTab = searchParams.get('tab')
  const currentTab: LibraryTab = isLibraryTab(rawTab) ? rawTab : 'shows'
  const tabListRef = useRef<HTMLDivElement | null>(null)
  const activeTabTriggerRef = useRef<HTMLButtonElement | null>(null)

  // Private-submission success dialog (preserved from old /collection page)
  const isPrivateSubmission = searchParams.get('submitted') === 'private'
  const [dialogDismissed, setDialogDismissed] = useState(false)
  const showSuccessDialog = !dialogDismissed && isPrivateSubmission

  // Normalize invalid tab query values
  useEffect(() => {
    if (rawTab && !isLibraryTab(rawTab)) {
      const newParams = new URLSearchParams(searchParams.toString())
      newParams.delete('tab')
      const qs = newParams.toString()
      router.replace(qs ? `/library?${qs}` : '/library', { scroll: false })
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
  }, [currentTab])

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

  const handleDialogClose = (open: boolean) => {
    if (!open) {
      setDialogDismissed(true)
      const newParams = new URLSearchParams(searchParams.toString())
      newParams.delete('submitted')
      const qs = newParams.toString()
      router.replace(qs ? `/library?${qs}` : '/library', { scroll: false })
    }
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

  const currentUserId = user?.id ? Number(user.id) : undefined

  return (
    <div className="container mx-auto max-w-6xl px-4 py-5 md:py-10">
      {/* Private submission success dialog */}
      <SubmissionSuccessDialog
        open={showSuccessDialog}
        onOpenChange={handleDialogClose}
      />

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
                followingTabCounts[tab] === undefined
                  ? TAB_LABELS[tab]
                  : `${TAB_LABELS[tab]}, ${followingTabCounts[tab]} followed`
              }
              className="flex-none rounded-none border-0 border-b-2 border-b-transparent bg-transparent px-3 py-2 text-muted-foreground shadow-none data-[state=active]:border-b-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none dark:data-[state=active]:border-b-primary dark:data-[state=active]:bg-transparent"
            >
              {TAB_LABELS[tab]}
              {tab in FOLLOWING_TAB_TYPES &&
                followingTabCounts[tab] !== undefined && (
                  <span aria-hidden> · {followingTabCounts[tab]}</span>
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

        <TabsContent value="submissions">
          <SubmissionsTab
            currentUserId={currentUserId}
            isAdmin={user?.is_admin}
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
