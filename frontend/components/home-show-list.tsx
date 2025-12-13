'use client'

import { useState } from 'react'
import { useUpcomingShows } from '@/lib/hooks/useShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { ShowResponse } from '@/lib/types/show'
import { Pencil, X } from 'lucide-react'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms'

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

interface ShowCardProps {
  show: ShowResponse
  isAdmin: boolean
}

function ShowCard({ show, isAdmin }: ShowCardProps) {
  const [isEditing, setIsEditing] = useState(false)
  const venue = show.venues[0]
  const artists = show.artists

  const handleEditSuccess = () => {
    setIsEditing(false)
  }

  const handleEditCancel = () => {
    setIsEditing(false)
  }

  return (
    <article className="border-b border-border/50 py-4 -mx-2 px-2 rounded-md hover:bg-muted/30 transition-colors duration-200">
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date and Location */}
        <div className="w-full md:w-1/5 md:pr-4 mb-1 md:mb-0">
          <h3 className="text-sm font-bold tracking-wide text-primary">
            {formatDate(show.event_date, show.state)}
          </h3>
          <p className="text-xs text-muted-foreground">
            {show.city}, {show.state}
          </p>
        </div>

        {/* Right column: Artists, Venue, Details */}
        <div className="w-full md:w-4/5 md:pl-4">
          <div className="flex items-start justify-between gap-2">
            {/* Artists */}
            <h2 className="text-base font-semibold leading-tight tracking-tight flex-1">
              {artists.map((artist, index) => (
                <span key={artist.id}>
                  {index > 0 && (
                    <span className="text-muted-foreground/60 font-normal">
                      &nbsp;•&nbsp;
                    </span>
                  )}
                  <span>{artist.name}</span>
                </span>
              ))}
            </h2>

            {/* Admin Edit Button */}
            {isAdmin && (
              <Button
                variant={isEditing ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => setIsEditing(!isEditing)}
                className="shrink-0 h-7 w-7 p-0"
                title={isEditing ? 'Cancel editing' : 'Edit show'}
              >
                {isEditing ? (
                  <X className="h-4 w-4" />
                ) : (
                  <Pencil className="h-3.5 w-3.5" />
                )}
              </Button>
            )}
          </div>

          {/* Venue and Details */}
          <div className="text-sm mt-1 text-muted-foreground">
            {venue && (
              <span className="text-primary/80 font-medium">{venue.name}</span>
            )}
            {show.price != null && (
              <span>&nbsp;•&nbsp;{formatPrice(show.price)}</span>
            )}
            {show.age_requirement && (
              <span>&nbsp;•&nbsp;{show.age_requirement}</span>
            )}
            <span>&nbsp;•&nbsp;{formatTime(show.event_date, show.state)}</span>
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
    </article>
  )
}

export function HomeShowList() {
  const { user } = useAuthContext()
  const isAdmin = user?.is_admin ?? false

  const { data, isLoading, error } = useUpcomingShows({
    timezone:
      typeof window !== 'undefined'
        ? Intl.DateTimeFormat().resolvedOptions().timeZone
        : 'America/Phoenix',
    limit: 10,
  })

  if (isLoading) {
    return (
      <div className="flex justify-center items-center py-8">
        <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-foreground"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>Unable to load shows.</p>
      </div>
    )
  }

  if (!data?.shows || data.shows.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>No upcoming shows at this time.</p>
      </div>
    )
  }

  return (
    <div className="w-full">
      {data.shows.map(show => (
        <ShowCard key={show.id} show={show} isAdmin={isAdmin} />
      ))}
    </div>
  )
}
