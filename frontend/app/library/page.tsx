'use client'

import { Suspense, createElement, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { redirect } from 'next/navigation'
import {
  Mic2,
  MapPin,
  Tag,
  Tent,
  Loader2,
  Users,
  UserMinus,
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
import { useSavedShows, useMySubmissions } from '@/features/shows'
import type { SavedShowResponse, ShowResponse } from '@/features/shows'
import { useMyFollowing, useUnfollow } from '@/lib/hooks/common/useFollow'
import type { FollowingEntity } from '@/lib/types/follow'
import { formatShowDate, formatShowTime, formatPrice } from '@/lib/utils/formatters'
import { formatShowMonthDay } from '@/lib/utils/showDateBadge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { BracketLink, SaveButton, SubmissionSuccessDialog } from '@/components/shared'
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
  'releases',
  'labels',
  'festivals',
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
        {discoveryLinks?.map((link) => (
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

function SavedShowCard({ show }: { show: SavedShowResponse }) {
  const venue = show.venues[0]
  const artists = show.artists
  const mobileDate = formatShowMonthDay(
    show.event_date,
    show.state,
    show.venues?.[0]?.timezone
  )

  return (
    <article
      aria-label={show.title}
      className="grid grid-cols-[74px_minmax(0,1fr)] gap-3 border-b border-border py-2.5 md:grid-cols-[104px_minmax(0,1fr)] md:gap-5 md:py-3"
    >
      <div className="font-mono text-[11px] font-bold uppercase text-primary md:text-xs">
        <span className="md:hidden">{mobileDate}</span>
        <span className="hidden md:inline">
          {formatShowDate(show.event_date, show.state, false, show.venues?.[0]?.timezone)}
        </span>
        <div className="mt-0.5 hidden text-[11px] font-normal normal-case text-muted-foreground md:block">
          {formatShowTime(show.event_date, show.state, show.venues?.[0]?.timezone)}
        </div>
      </div>

      <div className="min-w-0">
        <Link
          href={`/shows/${show.slug || show.id}`}
          className="block truncate text-sm font-medium leading-tight transition-colors hover:text-primary md:text-[15px]"
        >
          {artists.map((a) => a.name).join(' \u00B7 ')}
        </Link>

        <div className="mt-0.5 truncate text-xs text-muted-foreground md:text-[13px]">
          {venue && (
            <>
              {venue.slug ? (
                <Link
                  href={`/venues/${venue.slug}`}
                  className="transition-colors hover:text-primary md:text-primary/80"
                >
                  {venue.name}
                </Link>
              ) : (
                <span className="md:text-primary/80">{venue.name}</span>
              )}
              {(venue.city || venue.state) && (
                <span>
                  {' '}&middot; {[venue.city, venue.state].filter(Boolean).join(', ')}
                </span>
              )}
            </>
          )}
        </div>
      </div>
    </article>
  )
}

function ShowsTab() {
  const { data: savedData, isLoading, error } = useSavedShows()

  const savedShows = savedData?.shows ?? []

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
        <p>Failed to load your shows. Please try again later.</p>
      </div>
    )
  }

  return (
    <div className="space-y-8">
      {/* Calendar feed subscription */}
      <CalendarFeedSection />

      {savedShows.length === 0 ? (
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
        <section className="w-full">
          {savedShows.map((show) => (
            <SavedShowCard key={show.id} show={show} />
          ))}
        </section>
      )}
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
            {formatShowDate(show.event_date, show.state, false, show.venues?.[0]?.timezone)}
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
              &nbsp;•&nbsp;{formatShowTime(show.event_date, show.state, show.venues?.[0]?.timezone)}
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
      {shows.map((show) => (
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
  artist: { plural: 'artists', label: 'Artist', href: (slug) => `/artists/${slug}` },
  venue: { plural: 'venues', label: 'Venue', href: (slug) => `/venues/${slug}` },
  label: { plural: 'labels', label: 'Label', href: (slug) => `/labels/${slug}` },
  festival: { plural: 'festivals', label: 'Festival', href: (slug) => `/festivals/${slug}` },
}

function getEntityIcon(entityType: string) {
  switch (entityType) {
    case 'artist':
      return Mic2
    case 'venue':
      return MapPin
    case 'label':
      return Tag
    case 'festival':
      return Tent
    default:
      return Users
  }
}

function FollowingEntityCard({ entity }: { entity: FollowingEntity }) {
  const unfollow = useUnfollow()
  // `getEntityIcon` selects a stable, module-scope lucide component; render via
  // `createElement` to satisfy react-hooks/static-components (a function-call
  // result rendered as <Icon /> is misread as a component created per render).
  const icon = getEntityIcon(entity.entity_type)
  const info = entityTypeInfo[entity.entity_type]

  const handleUnfollow = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!info || unfollow.isPending) return
    unfollow.mutate({
      entityType: info.plural,
      entityId: entity.entity_id,
    })
  }

  const href = info?.href(entity.slug) ?? '#'
  const followedDate = new Date(entity.followed_at)
  const formattedDate = followedDate.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })

  return (
    <article className="border-b border-border/50 py-4 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div className="shrink-0 h-9 w-9 rounded-md bg-muted flex items-center justify-center">
            {createElement(icon, { className: 'h-4 w-4 text-muted-foreground' })}
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <Link
                href={href}
                className="text-base font-semibold leading-tight hover:text-primary transition-colors truncate"
              >
                {entity.name}
              </Link>
            </div>
            <p className="text-xs text-muted-foreground mt-0.5">
              Followed {formattedDate}
            </p>
          </div>
        </div>

        <Button
          variant="ghost"
          size="sm"
          onClick={handleUnfollow}
          disabled={unfollow.isPending}
          className="text-muted-foreground hover:text-destructive shrink-0"
          aria-label={`Unfollow ${entity.name}`}
        >
          {unfollow.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <UserMinus className="h-4 w-4" />
          )}
        </Button>
      </div>
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
  const [offset, setOffset] = useState(0)
  const limit = 20

  const { data, isLoading, error, isFetching } = useMyFollowing({
    type,
    limit,
    offset,
  })

  const following = data?.following ?? []
  const total = data?.total ?? 0
  const hasMore = offset + limit < total

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
    <div className={isFetching ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
      <section className="w-full">
        {following.map((entity) => (
          <FollowingEntityCard
            key={`${entity.entity_type}-${entity.entity_id}`}
            entity={entity}
          />
        ))}
      </section>

      {hasMore && (
        <div className="text-center py-6">
          <Button
            variant="outline"
            onClick={() => setOffset((prev) => prev + limit)}
            disabled={isFetching}
          >
            {isFetching ? 'Loading...' : 'Load More'}
          </Button>
        </div>
      )}

      {total > 0 && (
        <p className="text-center text-xs text-muted-foreground mt-2">
          Showing {Math.min(offset + limit, total)} of {total}
        </p>
      )}
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
  releases: 'Releases',
  labels: 'Labels',
  festivals: 'Festivals',
  submissions: 'Submissions',
}

function LibraryContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading: authLoading, user } = useAuthContext()

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
    router.replace(queryString ? `/library?${queryString}` : '/library', { scroll: false })
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
        <h1 className="text-2xl font-semibold tracking-tight md:text-[28px]">Library</h1>
        <p className="mt-1.5 hidden text-sm text-muted-foreground md:block">
          Your saved shows, and the artists, venues, scenes and labels you
          follow.
        </p>
      </header>

      {/* Tabs — underline style per the Library design direction (board A),
          horizontally scrollable on small screens instead of wrapping (board F) */}
      <Tabs value={currentTab} onValueChange={handleTabChange} className="w-full">
        <TabsList
          ref={tabListRef}
          className="mb-6 h-auto w-full flex-nowrap justify-start gap-1 overflow-x-auto rounded-none border-b border-border bg-transparent p-0"
        >
          {LIBRARY_TABS.map((tab) => (
            <TabsTrigger
              key={tab}
              ref={tab === currentTab ? activeTabTriggerRef : undefined}
              value={tab}
              className="flex-none rounded-none border-0 border-b-2 border-b-transparent bg-transparent px-3 py-2 text-muted-foreground shadow-none data-[state=active]:border-b-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none dark:data-[state=active]:border-b-primary dark:data-[state=active]:bg-transparent"
            >
              {TAB_LABELS[tab]}
            </TabsTrigger>
          ))}
        </TabsList>

        <TabsContent value="shows">
          <ShowsTab />
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

        <TabsContent value="releases">
          <EmptyState
            title="No releases saved yet."
            description="Release bookmarks are coming soon. Browse releases in the meantime."
            browseHref="/releases"
            browseLabel="Browse releases"
          />
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
