'use client'

import { useState } from 'react'
import { useUpcomingShows } from '@/lib/hooks/useShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { ShowResponse } from '@/lib/types/show'
import Link from 'next/link'
import { Pencil, X, Trash2 } from 'lucide-react'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms'
import { SaveButton } from '@/components/SaveButton'
import { DeleteShowDialog } from '@/components/DeleteShowDialog'

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
  userId?: string
}

function ShowCard({ show, isAdmin, userId }: ShowCardProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const venue = show.venues[0] // Primary venue
  const artists = show.artists

  // Check if user can delete: admin or show owner
  const canDelete =
    isAdmin ||
    (userId && show.submitted_by && String(show.submitted_by) === userId)

  const handleEditSuccess = () => {
    setIsEditing(false)
  }

  const handleEditCancel = () => {
    setIsEditing(false)
  }

  return (
    <article className="border-b border-border/50 py-5 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-200">
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date and Location */}
        <div className="w-full md:w-1/5 md:pr-4 mb-2 md:mb-0">
          <h2 className="text-sm font-bold tracking-wide text-primary">
            {formatDate(show.event_date, show.state)}
          </h2>
          <h3 className="text-xs text-muted-foreground mt-0.5">
            {show.city}, {show.state}
          </h3>
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
                  <Link
                    href={`/artists/${artist.id}`}
                    className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
                  >
                    {artist.name}
                  </Link>
                </span>
              ))}
            </h1>

            {/* Action Buttons */}
            <div className="flex items-center gap-1 shrink-0">
              {/* Save Button */}
              <SaveButton showId={show.id} variant="ghost" size="sm" />

              {/* Admin Edit Button */}
              {isAdmin && (
                <Button
                  variant={isEditing ? 'secondary' : 'ghost'}
                  size="sm"
                  onClick={() => setIsEditing(!isEditing)}
                  className="h-7 w-7 p-0"
                  title={isEditing ? 'Cancel editing' : 'Edit show'}
                >
                  {isEditing ? (
                    <X className="h-4 w-4" />
                  ) : (
                    <Pencil className="h-3.5 w-3.5" />
                  )}
                </Button>
              )}

              {/* Delete Button (admin or owner) */}
              {canDelete && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setIsDeleteDialogOpen(true)}
                  className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                  title="Delete show"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              )}
            </div>
          </div>

          {/* Venue and Details */}
          <div className="text-sm mt-1.5 text-muted-foreground">
            {venue && (
              <Link
                href={`/venues/${venue.id}`}
                className="text-primary/80 hover:text-primary font-medium transition-colors"
              >
                {venue.name}
              </Link>
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

      {/* Delete Confirmation Dialog */}
      <DeleteShowDialog
        show={show}
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
      />
    </article>
  )
}

export function ShowList() {
  const { user } = useAuthContext()
  const isAdmin = user?.is_admin ?? false

  const { data, isLoading, error } = useUpcomingShows({
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  })

  if (isLoading) {
    return (
      <div className="flex justify-center items-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load shows. Please try again later.</p>
      </div>
    )
  }

  if (!data?.shows || data.shows.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <p>No upcoming shows at this time.</p>
      </div>
    )
  }

  return (
    <section className="w-full max-w-4xl">
      {data.shows.map(show => (
        <ShowCard
          key={show.id}
          show={show}
          isAdmin={isAdmin}
          userId={user?.id}
        />
      ))}

      {data.pagination.has_more && (
        <div className="text-center py-6">
          <p className="text-muted-foreground text-sm">
            More shows available...
          </p>
        </div>
      )}
    </section>
  )
}
