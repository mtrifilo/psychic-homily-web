'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import {
  Star,
  Loader2,
  Calendar,
  List,
  ChevronDown,
  ChevronUp,
  MapPin,
  BadgeCheck,
} from 'lucide-react'
import {
  useFavoriteVenues,
  useFavoriteVenueShows,
} from '@/lib/hooks/useFavoriteVenues'
import { useVenueShows } from '@/lib/hooks/useVenues'
import { Button } from '@/components/ui/button'
import { FavoriteVenueButton } from '@/components/FavoriteVenueButton'
import type {
  FavoriteVenueResponse,
  FavoriteVenueShow,
  VenueShow,
} from '@/lib/types/venue'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'

type ViewMode = 'chronological' | 'byVenue'

const VIEW_MODE_STORAGE_KEY = 'favoriteVenuesViewMode'

function formatDate(dateString: string, state?: string | null): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return formatDateInTimezone(dateString, timezone)
}

function formatTime(dateString: string, state?: string | null): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return formatTimeInTimezone(dateString, timezone)
}

function formatPrice(price: number): string {
  return `$${price.toFixed(2)}`
}

interface ShowItemProps {
  show: FavoriteVenueShow | VenueShow
  state: string | null
  showVenue?: boolean
  venueSlug?: string
  venueName?: string
}

function ShowItem({
  show,
  state,
  showVenue = false,
  venueSlug,
  venueName,
}: ShowItemProps) {
  // Handle both FavoriteVenueShow and VenueShow types
  const displayVenueSlug =
    'venue_slug' in show ? show.venue_slug : venueSlug
  const displayVenueName =
    'venue_name' in show ? show.venue_name : venueName

  return (
    <div className="py-3 border-b border-border/30 last:border-b-0">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium text-primary">
            {formatDate(show.event_date, state)}
          </div>
          <div className="text-base font-semibold">
            {show.artists.map((artist, index) => (
              <span key={artist.id}>
                {index > 0 && (
                  <span className="text-muted-foreground/60 font-normal">
                    {' '}
                    &bull;{' '}
                  </span>
                )}
                <Link
                  href={`/artists/${artist.slug}`}
                  className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
                >
                  {artist.name}
                </Link>
              </span>
            ))}
            {show.artists.length === 0 && 'TBA'}
          </div>
          {showVenue && displayVenueSlug && displayVenueName && (
            <div className="text-sm text-muted-foreground mt-1">
              <Link
                href={`/venues/${displayVenueSlug}`}
                className="text-primary/80 hover:text-primary font-medium transition-colors"
              >
                {displayVenueName}
              </Link>
            </div>
          )}
        </div>
        <div className="text-right text-sm text-muted-foreground shrink-0">
          <div>{formatTime(show.event_date, state)}</div>
          {show.price != null && <div>{formatPrice(show.price)}</div>}
        </div>
      </div>
    </div>
  )
}

interface VenueCardExpandableProps {
  venue: FavoriteVenueResponse
}

function VenueCardExpandable({ venue }: VenueCardExpandableProps) {
  const [isExpanded, setIsExpanded] = useState(false)

  const { data, isLoading, error } = useVenueShows({
    venueId: venue.id,
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    enabled: isExpanded,
  })

  const hasShows = venue.upcoming_show_count > 0

  return (
    <article className="border border-border/50 rounded-lg mb-4 overflow-hidden bg-card">
      {/* Header - always visible */}
      <div
        onClick={() => hasShows && setIsExpanded(!isExpanded)}
        role={hasShows ? 'button' : undefined}
        tabIndex={hasShows ? 0 : undefined}
        onKeyDown={e => {
          if (hasShows && (e.key === 'Enter' || e.key === ' ')) {
            e.preventDefault()
            setIsExpanded(!isExpanded)
          }
        }}
        className={`w-full px-4 py-4 text-left transition-colors ${
          hasShows
            ? 'hover:bg-muted/30 cursor-pointer'
            : 'cursor-default opacity-80'
        }`}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="text-lg font-semibold truncate">
                <Link
                  href={`/venues/${venue.slug}`}
                  className="hover:text-primary transition-colors"
                  onClick={e => e.stopPropagation()}
                >
                  {venue.name}
                </Link>
              </h2>
              {venue.verified && (
                <BadgeCheck className="h-4 w-4 text-primary shrink-0" />
              )}
              <FavoriteVenueButton venueId={venue.id} size="sm" />
            </div>
            <div className="flex items-center gap-1 text-sm text-muted-foreground mt-1">
              <MapPin className="h-3.5 w-3.5" />
              <span>
                {venue.city}, {venue.state}
              </span>
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <span
              className={`text-sm font-medium px-2 py-1 rounded-full ${
                hasShows
                  ? 'bg-primary/10 text-primary'
                  : 'bg-muted text-muted-foreground'
              }`}
            >
              {venue.upcoming_show_count}{' '}
              {venue.upcoming_show_count === 1 ? 'show' : 'shows'}
            </span>
            {hasShows &&
              (isExpanded ? (
                <ChevronUp className="h-5 w-5 text-muted-foreground" />
              ) : (
                <ChevronDown className="h-5 w-5 text-muted-foreground" />
              ))}
          </div>
        </div>
      </div>

      {/* Expandable shows section */}
      {isExpanded && hasShows && (
        <div className="px-4 pb-4 border-t border-border/50">
          {isLoading && (
            <div className="flex justify-center py-6">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          )}

          {error && (
            <div className="py-4 text-center text-sm text-destructive">
              Failed to load shows
            </div>
          )}

          {data?.shows && data.shows.length > 0 && (
            <div className="pt-2">
              {data.shows.map(show => (
                <ShowItem
                  key={show.id}
                  show={show}
                  state={venue.state}
                  venueSlug={venue.slug}
                  venueName={venue.name}
                />
              ))}
              {data.total > data.shows.length && (
                <div className="text-center pt-3">
                  <Link
                    href={`/venues/${venue.slug}`}
                    className="text-sm text-primary hover:underline"
                  >
                    View all {data.total} shows
                  </Link>
                </div>
              )}
            </div>
          )}

          {data?.shows && data.shows.length === 0 && (
            <div className="py-4 text-center text-sm text-muted-foreground">
              No upcoming shows
            </div>
          )}
        </div>
      )}
    </article>
  )
}

function ChronologicalView() {
  const { data, isLoading, error } = useFavoriteVenueShows()

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
        <p>Failed to load shows from favorite venues. Please try again later.</p>
      </div>
    )
  }

  const shows = data?.shows || []

  if (shows.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <Calendar className="h-12 w-12 mx-auto mb-3 text-muted-foreground/30" />
        <p className="text-base">No upcoming shows from your favorite venues</p>
      </div>
    )
  }

  return (
    <section className="w-full">
      {shows.map(show => (
        <ShowItem key={show.id} show={show} state={show.state} showVenue />
      ))}
    </section>
  )
}

function ByVenueView() {
  const { data, isLoading, error } = useFavoriteVenues()

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
        <p>Failed to load favorite venues. Please try again later.</p>
      </div>
    )
  }

  const venues = data?.venues || []

  if (venues.length === 0) {
    return null // Empty state handled by parent
  }

  return (
    <section className="w-full">
      {venues.map(venue => (
        <VenueCardExpandable key={venue.id} venue={venue} />
      ))}
    </section>
  )
}

export function FavoriteVenuesTab() {
  const [viewMode, setViewMode] = useState<ViewMode>('chronological')
  const { data: venuesData, isLoading: venuesLoading } = useFavoriteVenues()

  // Load view mode from localStorage on mount
  useEffect(() => {
    const stored = localStorage.getItem(VIEW_MODE_STORAGE_KEY)
    if (stored === 'chronological' || stored === 'byVenue') {
      setViewMode(stored)
    }
  }, [])

  // Save view mode to localStorage when it changes
  const handleViewModeChange = (mode: ViewMode) => {
    setViewMode(mode)
    localStorage.setItem(VIEW_MODE_STORAGE_KEY, mode)
  }

  // Show empty state if no favorites
  const hasFavorites = (venuesData?.venues?.length || 0) > 0

  if (venuesLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (!hasFavorites) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Star className="h-16 w-16 mx-auto mb-4 text-muted-foreground/30" />
        <p className="text-lg mb-2">No favorite venues yet</p>
        <p className="text-sm">
          Star venues to see their upcoming shows here
        </p>
        <Link
          href="/venues"
          className="inline-block mt-6 px-6 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
        >
          Browse Venues
        </Link>
      </div>
    )
  }

  return (
    <div className="w-full">
      {/* View Toggle */}
      <div className="flex justify-end mb-4">
        <div className="inline-flex rounded-md border border-border p-1 bg-muted/30">
          <Button
            variant={viewMode === 'chronological' ? 'secondary' : 'ghost'}
            size="sm"
            onClick={() => handleViewModeChange('chronological')}
            className="gap-1.5 px-3"
          >
            <Calendar className="h-4 w-4" />
            <span className="hidden sm:inline">By Date</span>
          </Button>
          <Button
            variant={viewMode === 'byVenue' ? 'secondary' : 'ghost'}
            size="sm"
            onClick={() => handleViewModeChange('byVenue')}
            className="gap-1.5 px-3"
          >
            <List className="h-4 w-4" />
            <span className="hidden sm:inline">By Venue</span>
          </Button>
        </div>
      </div>

      {/* Content */}
      {viewMode === 'chronological' ? <ChronologicalView /> : <ByVenueView />}
    </div>
  )
}
