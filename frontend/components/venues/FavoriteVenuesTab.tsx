'use client'

import { useState } from 'react'
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
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { CompactShowRow } from '@/components/shows/CompactShowRow'
import { SHOW_LIST_FEATURE_POLICY } from '@/components/shows/showListFeaturePolicy'
import { FavoriteVenueButton } from './FavoriteVenueButton'
import type { FavoriteVenueResponse } from '@/lib/types/venue'

type ViewMode = 'chronological' | 'byVenue'

const VIEW_MODE_STORAGE_KEY = 'favoriteVenuesViewMode'

interface VenueCardExpandableProps {
  venue: FavoriteVenueResponse
}

function VenueCardExpandable({ venue }: VenueCardExpandableProps) {
  const [isExpanded, setIsExpanded] = useState(true)
  const hasShows = venue.upcoming_show_count > 0

  const { data, isLoading, error } = useVenueShows({
    venueId: venue.id,
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    enabled: hasShows, // Always fetch if venue has shows since expanded by default
  })

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
                <CompactShowRow
                  key={show.id}
                  show={show}
                  state={venue.state}
                  showDetailsLink={
                    SHOW_LIST_FEATURE_POLICY.context.showDetailsLink
                  }
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
  const { isAuthenticated } = useAuthContext()
  const { data, isLoading, error } = useFavoriteVenueShows({
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
        <p>
          Failed to load shows from favorite venues. Please try again later.
        </p>
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
        <CompactShowRow
          key={show.id}
          show={show}
          state={show.state}
          showVenueLine
          venue={{
            name: show.venue_name,
            slug: show.venue_slug,
          }}
          showDetailsLink={SHOW_LIST_FEATURE_POLICY.context.showDetailsLink}
        />
      ))}
    </section>
  )
}

function ByVenueView() {
  const { isAuthenticated } = useAuthContext()
  const { data, isLoading, error } = useFavoriteVenues({
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
  const { isAuthenticated } = useAuthContext()
  const [viewMode, setViewMode] = useState<ViewMode>(() => {
    if (typeof window === 'undefined') {
      return 'byVenue'
    }
    const stored = window.localStorage.getItem(VIEW_MODE_STORAGE_KEY)
    return stored === 'chronological' || stored === 'byVenue'
      ? stored
      : 'byVenue'
  })
  const { data: venuesData, isLoading: venuesLoading } = useFavoriteVenues({
    enabled: isAuthenticated,
  })

  // Save view mode to localStorage when it changes
  const handleViewModeChange = (mode: ViewMode) => {
    setViewMode(mode)
    window.localStorage.setItem(VIEW_MODE_STORAGE_KEY, mode)
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
        <p className="text-sm">Star venues to see their upcoming shows here</p>
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
