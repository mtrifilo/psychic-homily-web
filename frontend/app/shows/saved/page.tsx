'use client'

import { useState } from 'react'
import { useSavedShows } from '@/lib/hooks/useSavedShows'
import { useShowUnpublish } from '@/lib/hooks/useShowUnpublish'
import { useAuthContext } from '@/lib/context/AuthContext'
import { redirect } from 'next/navigation'
import Link from 'next/link'
import { Heart, Loader2, Clock, CheckCircle2, EyeOff, Pencil, X, Trash2 } from 'lucide-react'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import type { SavedShowResponse } from '@/lib/types/show'
import { SaveButton } from '@/components/SaveButton'
import { DeleteShowDialog } from '@/components/DeleteShowDialog'
import { ShowForm } from '@/components/forms'
import { Button } from '@/components/ui/button'

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

interface SavedShowCardProps {
  show: SavedShowResponse
  currentUserId?: number
  isAdmin?: boolean
}

function SavedShowCard({
  show,
  currentUserId,
  isAdmin,
}: SavedShowCardProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const venue = show.venues[0]
  const artists = show.artists
  const unpublishMutation = useShowUnpublish()

  // Check if user can unpublish this show
  const canUnpublish =
    show.status === 'approved' &&
    (isAdmin || (currentUserId && show.submitted_by === currentUserId))

  // Check if user can delete: admin or show owner
  const canDelete =
    isAdmin || (currentUserId && show.submitted_by === currentUserId)

  const handleUnpublish = () => {
    if (confirm('Are you sure you want to unpublish this show? It will be set to pending and removed from public view.')) {
      unpublishMutation.mutate(show.id)
    }
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
            {formatDate(show.event_date, show.state)}
          </h2>
          <h3 className="text-xs text-muted-foreground mt-0.5">
            {show.city}, {show.state}
          </h3>

          {/* Status Badge */}
          <div className="mt-2">
            {show.status === 'approved' ? (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                <CheckCircle2 className="h-3 w-3" />
                Published
              </span>
            ) : show.status === 'pending' ? (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400">
                <Clock className="h-3 w-3" />
                Pending
              </span>
            ) : show.status === 'rejected' ? (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-red-500/10 text-red-600 dark:text-red-400">
                Rejected
              </span>
            ) : null}
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
                  {artist.socials?.instagram ? (
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
              {/* Save Button */}
              <SaveButton showId={show.id} variant="ghost" size="sm" />

              {/* Unpublish Button */}
              {canUnpublish && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleUnpublish}
                  disabled={unpublishMutation.isPending}
                  className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground"
                  title="Unpublish show"
                >
                  {unpublishMutation.isPending ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <EyeOff className="h-3.5 w-3.5" />
                  )}
                </Button>
              )}

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

export default function SavedShowsPage() {
  const { isAuthenticated, isLoading: authLoading, user } = useAuthContext()
  const { data, isLoading, error } = useSavedShows()

  // Redirect if not authenticated
  if (!authLoading && !isAuthenticated) {
    redirect('/auth')
  }

  if (authLoading || isLoading) {
    return (
      <div className="flex justify-center items-center min-h-screen">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="container max-w-4xl mx-auto px-4 py-12">
        <div className="text-center text-destructive">
          <p>Failed to load your saved shows. Please try again later.</p>
        </div>
      </div>
    )
  }

  const shows = data?.shows || []
  const total = data?.total || 0

  return (
    <div className="container max-w-4xl mx-auto px-4 py-12">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <Heart className="h-8 w-8 fill-red-500 text-red-500" />
          <h1 className="text-3xl font-bold tracking-tight">My List</h1>
        </div>
        <p className="text-muted-foreground">
          {total === 0
            ? 'No saved shows yet'
            : `${total} saved ${total === 1 ? 'show' : 'shows'}`}
        </p>
      </div>

      {/* Shows List */}
      {shows.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          <Heart className="h-16 w-16 mx-auto mb-4 text-muted-foreground/30" />
          <p className="text-lg mb-2">Your list is empty</p>
          <p className="text-sm">
            Save shows by clicking the heart icon on any show
          </p>
          <Link
            href="/shows"
            className="inline-block mt-6 px-6 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 transition-colors"
          >
            Browse Shows
          </Link>
        </div>
      ) : (
        <section className="w-full">
          {shows.map(show => (
            <SavedShowCard
              key={show.id}
              show={show}
              currentUserId={user?.id}
              isAdmin={user?.is_admin}
            />
          ))}
        </section>
      )}
    </div>
  )
}
