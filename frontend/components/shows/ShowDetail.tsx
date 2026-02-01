'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Loader2, MapPin, Pencil, X, Trash2 } from 'lucide-react'
import { useShow } from '@/lib/hooks/useShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { ArtistResponse } from '@/lib/types/show'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import { Button } from '@/components/ui/button'
import { SocialLinks, MusicEmbed, SaveButton } from '@/components/shared'
import { ShowForm } from '@/components/forms'
import { DeleteShowDialog } from './DeleteShowDialog'

interface ShowDetailProps {
  showId: string | number
}

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

function artistHasMusic(artist: ArtistResponse): boolean {
  return !!(
    artist.bandcamp_embed_url ||
    artist.socials?.spotify ||
    artist.socials?.bandcamp
  )
}

export function ShowDetail({ showId }: ShowDetailProps) {
  const { data: show, isLoading, error } = useShow(showId)
  const { isAuthenticated, user } = useAuthContext()
  const isAdmin = isAuthenticated && user?.is_admin

  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)

  // Check if user can delete: admin or show owner
  const canDelete =
    isAdmin ||
    (user?.id && show?.submitted_by && String(show.submitted_by) === user.id)

  const handleEditSuccess = () => {
    setIsEditing(false)
  }

  const handleEditCancel = () => {
    setIsEditing(false)
  }

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load show'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Show Not Found' : 'Error Loading Show'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The show you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/shows">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Shows
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!show) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Show Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The show you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/shows">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Shows
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const venue = show.venues[0]
  const artists = show.artists
  const artistsWithMusic = artists.filter(artistHasMusic)

  return (
    <div className="container max-w-4xl mx-auto px-4 py-6">
      {/* Back Navigation */}
      <div className="mb-6">
        <Link
          href="/shows"
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4 mr-1" />
          Back to Shows
        </Link>
      </div>

      {/* Header */}
      <header className="mb-8">
        <div className="flex items-start justify-between gap-4">
          <div className="flex-1">
            {/* Date */}
            <div className="text-lg font-bold text-primary mb-2">
              {formatDate(show.event_date, show.state)}
            </div>

            {/* Artists */}
            <h1 className="text-2xl md:text-3xl font-bold">
              {artists.map((artist, index) => (
                <span key={artist.id}>
                  {index > 0 && (
                    <span className="text-muted-foreground/60 font-normal">
                      {' '}&bull;{' '}
                    </span>
                  )}
                  {artist.slug ? (
                    <Link
                      href={`/artists/${artist.slug}`}
                      className="hover:text-primary transition-colors"
                    >
                      {artist.name}
                    </Link>
                  ) : (
                    <span>{artist.name}</span>
                  )}
                </span>
              ))}
            </h1>

            {/* Venue and Location */}
            {venue && (
              <div className="mt-2">
                {venue.slug ? (
                  <Link
                    href={`/venues/${venue.slug}`}
                    className="text-lg text-primary/80 hover:text-primary font-medium transition-colors"
                  >
                    {venue.name}
                  </Link>
                ) : (
                  <span className="text-lg text-primary/80 font-medium">
                    {venue.name}
                  </span>
                )}
                <div className="flex items-center gap-1 text-muted-foreground mt-1">
                  <MapPin className="h-4 w-4" />
                  <span>
                    {venue.city}, {venue.state}
                  </span>
                </div>
              </div>
            )}

            {/* Show Details */}
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground mt-3">
              <span>{formatTime(show.event_date, show.state)}</span>
              {show.price != null && <span>{formatPrice(show.price)}</span>}
              {show.age_requirement && <span>{show.age_requirement}</span>}
            </div>

            {/* Description */}
            {show.description && (
              <p className="mt-4 text-muted-foreground">{show.description}</p>
            )}
          </div>

          {/* Action Buttons */}
          <div className="flex items-center gap-2 shrink-0">
            <SaveButton showId={show.id} variant="outline" size="sm" />

            {isAdmin && (
              <Button
                variant={isEditing ? 'secondary' : 'outline'}
                size="sm"
                onClick={() => setIsEditing(!isEditing)}
              >
                {isEditing ? (
                  <>
                    <X className="h-4 w-4 mr-2" />
                    Cancel
                  </>
                ) : (
                  <>
                    <Pencil className="h-4 w-4 mr-2" />
                    Edit
                  </>
                )}
              </Button>
            )}

            {canDelete && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setIsDeleteDialogOpen(true)}
                className="text-destructive hover:text-destructive hover:bg-destructive/10"
              >
                <Trash2 className="h-4 w-4 mr-2" />
                Delete
              </Button>
            )}
          </div>
        </div>
      </header>

      {/* Edit Form */}
      {isEditing && (
        <div className="mb-8 p-4 rounded-lg border border-border bg-muted/30">
          <ShowForm
            mode="edit"
            initialData={show}
            onSuccess={handleEditSuccess}
            onCancel={handleEditCancel}
          />
        </div>
      )}

      {/* Artist Music Section */}
      {artistsWithMusic.length > 0 && (
        <section className="mb-8">
          <h2 className="text-lg font-semibold mb-4">Listen to the Artists</h2>
          <div className="space-y-6">
            {artistsWithMusic.map(artist => (
              <div key={artist.id} className="space-y-2">
                <div className="flex items-start justify-between gap-2">
                  <div>
                    {artist.slug ? (
                      <Link
                        href={`/artists/${artist.slug}`}
                        className="font-medium hover:text-primary transition-colors"
                      >
                        {artist.name}
                      </Link>
                    ) : (
                      <span className="font-medium">{artist.name}</span>
                    )}
                    {(artist.city || artist.state) && (
                      <div className="flex items-center gap-1 text-xs text-muted-foreground mt-0.5">
                        <MapPin className="h-3 w-3" />
                        <span>
                          {[artist.city, artist.state].filter(Boolean).join(', ')}
                        </span>
                      </div>
                    )}
                  </div>
                  <SocialLinks social={artist.socials} className="shrink-0" />
                </div>
                <MusicEmbed
                  bandcampAlbumUrl={artist.bandcamp_embed_url}
                  bandcampProfileUrl={artist.socials?.bandcamp}
                  spotifyUrl={artist.socials?.spotify}
                  artistName={artist.name}
                />
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Delete Confirmation Dialog */}
      <DeleteShowDialog
        show={show}
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
      />
    </div>
  )
}
