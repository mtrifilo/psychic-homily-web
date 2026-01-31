'use client'

import { useState } from 'react'
import {
  ChevronDown,
  ChevronUp,
  BadgeCheck,
  MapPin,
  Plus,
  Pencil,
  Trash2,
} from 'lucide-react'
import Link from 'next/link'
import { useVenueShows } from '@/lib/hooks/useVenues'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useQueryClient } from '@tanstack/react-query'
import { createInvalidateQueries } from '@/lib/queryClient'
import type { VenueWithShowCount, VenueShow } from '@/lib/types/venue'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import { ShowForm } from '@/components/forms/ShowForm'
import { VenueEditForm } from '@/components/forms/VenueEditForm'
import { DeleteVenueDialog } from '@/components/DeleteVenueDialog'
import { FavoriteVenueButton } from '@/components/FavoriteVenueButton'
import { Button } from '@/components/ui/button'

interface VenueCardProps {
  venue: VenueWithShowCount
}

/**
 * Format a date string to "Mon, Dec 1" format in venue timezone
 */
function formatDate(dateString: string, state?: string | null): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return formatDateInTimezone(dateString, timezone)
}

/**
 * Format a date string to "7:30 PM" format in venue timezone
 */
function formatTime(dateString: string, state?: string | null): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return formatTimeInTimezone(dateString, timezone)
}

/**
 * Format price as $XX.XX
 */
function formatPrice(price: number): string {
  return `$${price.toFixed(2)}`
}

interface ShowItemProps {
  show: VenueShow
  state: string
}

function ShowItem({ show, state }: ShowItemProps) {
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
                {artist.slug ? (
                  <Link
                    href={`/artists/${artist.slug}`}
                    className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
                    onClick={e => e.stopPropagation()}
                  >
                    {artist.name}
                  </Link>
                ) : (
                  <span>{artist.name}</span>
                )}
              </span>
            ))}
            {show.artists.length === 0 && 'TBA'}
          </div>
        </div>
        <div className="text-right text-sm text-muted-foreground shrink-0">
          <div>{formatTime(show.event_date, state)}</div>
          {show.price != null && <div>{formatPrice(show.price)}</div>}
        </div>
      </div>
    </div>
  )
}

export function VenueCard({ venue }: VenueCardProps) {
  const [isExpanded, setIsExpanded] = useState(false)
  const [isAddingShow, setIsAddingShow] = useState(false)
  const [isEditingVenue, setIsEditingVenue] = useState(false)
  const [isDeleteVenueOpen, setIsDeleteVenueOpen] = useState(false)
  const { isAuthenticated, user } = useAuthContext()
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  // User can edit if they're an admin OR if they submitted the venue
  const canEdit =
    isAuthenticated &&
    (user?.is_admin ||
      (venue.submitted_by != null &&
        venue.submitted_by === Number(user?.id)))

  const { data, isLoading, error, refetch } = useVenueShows({
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
                {venue.slug ? (
                  <Link
                    href={`/venues/${venue.slug}`}
                    className="hover:text-primary transition-colors"
                    onClick={e => e.stopPropagation()}
                  >
                    {venue.name}
                  </Link>
                ) : (
                  <span>{venue.name}</span>
                )}
              </h2>
              {venue.verified && (
                <BadgeCheck className="h-4 w-4 text-primary shrink-0" />
              )}
              <FavoriteVenueButton venueId={venue.id} size="sm" />
              {canEdit && (
                <>
                  <button
                    onClick={e => {
                      e.stopPropagation()
                      setIsEditingVenue(true)
                    }}
                    className="p-1 rounded-md hover:bg-muted transition-colors"
                    title="Edit venue"
                  >
                    <Pencil className="h-3.5 w-3.5 text-muted-foreground hover:text-foreground" />
                  </button>
                  <button
                    onClick={e => {
                      e.stopPropagation()
                      setIsDeleteVenueOpen(true)
                    }}
                    className="p-1 rounded-md hover:bg-destructive/10 transition-colors"
                    title="Delete venue"
                  >
                    <Trash2 className="h-3.5 w-3.5 text-muted-foreground hover:text-destructive" />
                  </button>
                </>
              )}
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
              <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-foreground"></div>
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
                <ShowItem key={show.id} show={show} state={venue.state} />
              ))}
              {data.total > data.shows.length && venue.slug && (
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

          {/* Add Show Form/Button */}
          {isAddingShow ? (
            <div className="mt-4 pt-4 border-t border-border/50">
              <ShowForm
                mode="create"
                prefilledVenue={{
                  id: venue.id,
                  slug: venue.slug,
                  name: venue.name,
                  city: venue.city,
                  state: venue.state,
                  address: venue.address,
                  verified: venue.verified,
                }}
                onSuccess={() => {
                  setIsAddingShow(false)
                  refetch()
                }}
                onCancel={() => setIsAddingShow(false)}
                redirectOnCreate={false}
              />
            </div>
          ) : (
            isAuthenticated && (
              <div className="mt-4 pt-4 border-t border-border/50">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setIsAddingShow(true)}
                  className="w-full"
                >
                  <Plus className="h-4 w-4 mr-2" />
                  Add a show at {venue.name}
                </Button>
              </div>
            )
          )}
        </div>
      )}

      {/* Venue Edit Form Dialog */}
      <VenueEditForm
        venue={venue}
        open={isEditingVenue}
        onOpenChange={setIsEditingVenue}
        onSuccess={() => refetch()}
      />

      {/* Delete Venue Dialog */}
      <DeleteVenueDialog
        venue={venue}
        open={isDeleteVenueOpen}
        onOpenChange={setIsDeleteVenueOpen}
        onSuccess={() => invalidateQueries.venues()}
      />
    </article>
  )
}
