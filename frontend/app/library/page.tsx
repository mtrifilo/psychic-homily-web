'use client'

import { Suspense, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { redirect } from 'next/navigation'
import {
  BookOpen,
  CalendarCheck,
  Star,
  Mic2,
  MapPin,
  Tag,
  Tent,
  Disc3,
  Loader2,
  Calendar,
  Users,
  UserMinus,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useMyShows } from '@/features/shows'
import { useSavedShows } from '@/features/shows'
import type { AttendingShow, SavedShowResponse } from '@/features/shows'
import { useMyFollowing, useUnfollow } from '@/lib/hooks/common/useFollow'
import { useFavoriteVenues } from '@/features/auth'
import type { FollowingEntity } from '@/lib/types/follow'
import { formatShowDate, formatShowTime } from '@/lib/utils/formatters'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'

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
] as const
type LibraryTab = (typeof LIBRARY_TABS)[number]

function isLibraryTab(value: string | null): value is LibraryTab {
  return value !== null && LIBRARY_TABS.includes(value as LibraryTab)
}

// ---------------------------------------------------------------------------
// Shared empty-state component
// ---------------------------------------------------------------------------

function EmptyState({
  icon: Icon,
  title,
  description,
  browseHref,
  browseLabel,
}: {
  icon: LucideIcon
  title: string
  description: string
  browseHref: string
  browseLabel: string
}) {
  return (
    <div className="text-center py-12 text-muted-foreground">
      <Icon className="h-16 w-16 mx-auto mb-4 text-muted-foreground/30" />
      <p className="text-lg mb-2">{title}</p>
      <p className="text-sm">{description}</p>
      <Link
        href={browseHref}
        className="inline-block mt-6 px-6 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
      >
        {browseLabel}
      </Link>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Shows tab — attending (going/interested) shows
// ---------------------------------------------------------------------------

function AttendingShowCard({ show }: { show: AttendingShow }) {
  return (
    <article className="border-b border-border/50 py-4 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex items-start justify-between gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <Link
              href={`/shows/${show.slug || show.show_id}`}
              className="text-base font-semibold leading-tight hover:text-primary transition-colors truncate"
            >
              {show.title}
            </Link>
            <Badge
              variant={show.status === 'going' ? 'default' : 'secondary'}
              className="shrink-0 text-xs"
            >
              {show.status === 'going' ? (
                <CalendarCheck className="h-3 w-3 mr-1" />
              ) : (
                <Star className="h-3 w-3 mr-1" />
              )}
              {show.status === 'going' ? 'Going' : 'Interested'}
            </Badge>
          </div>

          <div className="text-sm text-muted-foreground">
            {show.venue_name && (
              <>
                {show.venue_slug ? (
                  <Link
                    href={`/venues/${show.venue_slug}`}
                    className="text-primary/80 hover:text-primary font-medium transition-colors"
                  >
                    {show.venue_name}
                  </Link>
                ) : (
                  <span className="text-primary/80 font-medium">{show.venue_name}</span>
                )}
                {(show.city || show.state) && (
                  <span className="text-muted-foreground/80">
                    {' '}&middot; {[show.city, show.state].filter(Boolean).join(', ')}
                  </span>
                )}
              </>
            )}
          </div>
        </div>

        <div className="text-right shrink-0">
          <div className="text-sm font-medium text-primary">
            {formatShowDate(show.event_date, show.state ?? undefined)}
          </div>
          <div className="text-xs text-muted-foreground">
            {formatShowTime(show.event_date, show.state ?? undefined)}
          </div>
        </div>
      </div>
    </article>
  )
}

function SavedShowCard({ show }: { show: SavedShowResponse }) {
  const venue = show.venues[0]
  const artists = show.artists

  return (
    <article className="border-b border-border/50 py-4 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex items-start justify-between gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <Link
              href={`/shows/${show.slug || show.id}`}
              className="text-base font-semibold leading-tight hover:text-primary transition-colors truncate"
            >
              {artists.map((a) => a.name).join(' \u00B7 ')}
            </Link>
          </div>

          <div className="text-sm text-muted-foreground">
            {venue && (
              <>
                {venue.slug ? (
                  <Link
                    href={`/venues/${venue.slug}`}
                    className="text-primary/80 hover:text-primary font-medium transition-colors"
                  >
                    {venue.name}
                  </Link>
                ) : (
                  <span className="text-primary/80 font-medium">{venue.name}</span>
                )}
                {(venue.city || venue.state) && (
                  <span className="text-muted-foreground/80">
                    {' '}&middot; {[venue.city, venue.state].filter(Boolean).join(', ')}
                  </span>
                )}
              </>
            )}
          </div>
        </div>

        <div className="text-right shrink-0">
          <div className="text-sm font-medium text-primary">
            {formatShowDate(show.event_date, show.state)}
          </div>
          <div className="text-xs text-muted-foreground">
            {formatShowTime(show.event_date, show.state)}
          </div>
        </div>
      </div>
    </article>
  )
}

function ShowsTab() {
  const [attendingOffset, setAttendingOffset] = useState(0)
  const limit = 20

  const { data: attendingData, isLoading: attendingLoading, error: attendingError, isFetching: attendingFetching } =
    useMyShows({ status: 'all', limit, offset: attendingOffset })

  const { data: savedData, isLoading: savedLoading, error: savedError } = useSavedShows()

  const attendingShows = attendingData?.shows ?? []
  const attendingTotal = attendingData?.total ?? 0
  const attendingHasMore = attendingOffset + limit < attendingTotal

  const savedShows = savedData?.shows ?? []

  const isLoading = attendingLoading || savedLoading
  const hasAnyContent = attendingShows.length > 0 || savedShows.length > 0

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (attendingError && savedError) {
    return (
      <div className="text-center text-destructive py-12">
        <p>Failed to load your shows. Please try again later.</p>
      </div>
    )
  }

  if (!hasAnyContent) {
    return (
      <EmptyState
        icon={Calendar}
        title="No shows saved yet"
        description="Mark shows as Going/Interested or save them to see them here."
        browseHref="/shows"
        browseLabel="Browse Shows"
      />
    )
  }

  return (
    <div className="space-y-8">
      {/* Attending shows section */}
      {attendingShows.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            Going / Interested
          </h3>
          <div className={attendingFetching ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
            <section className="w-full">
              {attendingShows.map((show) => (
                <AttendingShowCard key={`${show.show_id}-${show.status}`} show={show} />
              ))}
            </section>

            {attendingHasMore && (
              <div className="text-center py-4">
                <Button
                  variant="outline"
                  onClick={() => setAttendingOffset((prev) => prev + limit)}
                  disabled={attendingFetching}
                >
                  {attendingFetching ? 'Loading...' : 'Load More'}
                </Button>
              </div>
            )}

            {attendingTotal > 0 && (
              <p className="text-center text-xs text-muted-foreground mt-1">
                {Math.min(attendingOffset + limit, attendingTotal)} of {attendingTotal} attending
              </p>
            )}
          </div>
        </div>
      )}

      {/* Saved shows section */}
      {savedShows.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            Saved
          </h3>
          <section className="w-full">
            {savedShows.map((show) => (
              <SavedShowCard key={show.id} show={show} />
            ))}
          </section>
        </div>
      )}
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
  const Icon = getEntityIcon(entity.entity_type)
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
            <Icon className="h-4 w-4 text-muted-foreground" />
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
          title="Unfollow"
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
  emptyIcon: EmptyIcon,
  emptyTitle,
  emptyDescription,
  browseHref,
  browseLabel,
}: {
  type: string
  emptyIcon: LucideIcon
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
        icon={EmptyIcon}
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
// Venues tab — favorite venues + followed venues
// ---------------------------------------------------------------------------

function VenuesTab() {
  const { isAuthenticated } = useAuthContext()
  const { data: favData, isLoading: favLoading } = useFavoriteVenues({
    enabled: isAuthenticated,
  })

  const favoriteVenues = favData?.venues ?? []

  return (
    <div className="space-y-8">
      {/* Favorite venues section */}
      {favLoading && (
        <div className="flex justify-center py-6">
          <Loader2 className="h-6 w-6 animate-spin text-primary" />
        </div>
      )}
      {!favLoading && favoriteVenues.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            Favorite Venues
          </h3>
          <section className="w-full">
            {favoriteVenues.map((venue) => (
              <article
                key={venue.id}
                className="border-b border-border/50 py-4 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200"
              >
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-3 min-w-0 flex-1">
                    <div className="shrink-0 h-9 w-9 rounded-md bg-muted flex items-center justify-center">
                      <MapPin className="h-4 w-4 text-muted-foreground" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <Link
                        href={`/venues/${venue.slug}`}
                        className="text-base font-semibold leading-tight hover:text-primary transition-colors truncate block"
                      >
                        {venue.name}
                      </Link>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        {venue.city}, {venue.state}
                        {venue.upcoming_show_count > 0 && (
                          <span>
                            {' '}&middot; {venue.upcoming_show_count}{' '}
                            {venue.upcoming_show_count === 1 ? 'upcoming show' : 'upcoming shows'}
                          </span>
                        )}
                      </p>
                    </div>
                  </div>
                </div>
              </article>
            ))}
          </section>
        </div>
      )}

      {/* Followed venues */}
      <div>
        {favoriteVenues.length > 0 && (
          <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            Followed Venues
          </h3>
        )}
        <FollowingList
          type="venue"
          emptyIcon={MapPin}
          emptyTitle={favoriteVenues.length > 0 ? 'No followed venues' : 'No venues saved yet'}
          emptyDescription="Follow or favorite venues to see them here."
          browseHref="/venues"
          browseLabel="Browse Venues"
        />
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main page content
// ---------------------------------------------------------------------------

function LibraryContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated, isLoading: authLoading } = useAuthContext()

  const rawTab = searchParams.get('tab')
  const currentTab: LibraryTab = isLibraryTab(rawTab) ? rawTab : 'shows'

  const handleTabChange = (tab: string) => {
    if (!isLibraryTab(tab)) return
    const params = new URLSearchParams()
    if (tab !== 'shows') {
      params.set('tab', tab)
    }
    const queryString = params.toString()
    router.replace(queryString ? `/library?${queryString}` : '/library', { scroll: false })
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
    <div className="container max-w-6xl mx-auto px-4 py-12">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <BookOpen className="h-8 w-8 text-primary" />
          <h1 className="text-3xl font-bold tracking-tight">Library</h1>
        </div>
        <p className="text-muted-foreground">
          All of your saved shows, followed artists, venues, and more
        </p>
      </div>

      {/* Tabs */}
      <Tabs value={currentTab} onValueChange={handleTabChange} className="w-full">
        <TabsList className="mb-6 flex-wrap h-auto gap-1">
          <TabsTrigger value="shows" className="gap-1.5">
            <Calendar className="h-4 w-4" />
            Shows
          </TabsTrigger>
          <TabsTrigger value="artists" className="gap-1.5">
            <Mic2 className="h-4 w-4" />
            Artists
          </TabsTrigger>
          <TabsTrigger value="venues" className="gap-1.5">
            <MapPin className="h-4 w-4" />
            Venues
          </TabsTrigger>
          <TabsTrigger value="releases" className="gap-1.5">
            <Disc3 className="h-4 w-4" />
            Releases
          </TabsTrigger>
          <TabsTrigger value="labels" className="gap-1.5">
            <Tag className="h-4 w-4" />
            Labels
          </TabsTrigger>
          <TabsTrigger value="festivals" className="gap-1.5">
            <Tent className="h-4 w-4" />
            Festivals
          </TabsTrigger>
        </TabsList>

        <TabsContent value="shows">
          <ShowsTab />
        </TabsContent>

        <TabsContent value="artists">
          <FollowingList
            type="artist"
            emptyIcon={Mic2}
            emptyTitle="No artists followed"
            emptyDescription="Follow artists to keep up with their shows and releases."
            browseHref="/artists"
            browseLabel="Browse Artists"
          />
        </TabsContent>

        <TabsContent value="venues">
          <VenuesTab />
        </TabsContent>

        <TabsContent value="releases">
          <EmptyState
            icon={Disc3}
            title="No releases saved yet"
            description="Release bookmarks are coming soon. Browse releases in the meantime."
            browseHref="/releases"
            browseLabel="Browse Releases"
          />
        </TabsContent>

        <TabsContent value="labels">
          <FollowingList
            type="label"
            emptyIcon={Tag}
            emptyTitle="No labels followed"
            emptyDescription="Follow labels to discover new releases and roster updates."
            browseHref="/labels"
            browseLabel="Browse Labels"
          />
        </TabsContent>

        <TabsContent value="festivals">
          <FollowingList
            type="festival"
            emptyIcon={Tent}
            emptyTitle="No festivals followed"
            emptyDescription="Follow festivals to get lineup and schedule updates."
            browseHref="/festivals"
            browseLabel="Browse Festivals"
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
