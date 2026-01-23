'use client'

import { useState } from 'react'
import { useUpcomingShows } from '@/lib/hooks/useShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { ShowResponse, ArtistResponse } from '@/lib/types/show'
import Link from 'next/link'
import { Pencil, Trash2, X, ChevronDown, ChevronUp, MapPin } from 'lucide-react'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms'
import { SaveButton } from '@/components/SaveButton'
import { DeleteShowDialog } from '@/components/DeleteShowDialog'
import { SocialLinks } from '@/components/SocialLinks'
import { MusicEmbed } from '@/components/MusicEmbed'

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

/**
 * Check if an artist has any music available (Bandcamp embed, Spotify, or Bandcamp profile)
 */
function artistHasMusic(artist: ArtistResponse): boolean {
  return !!(
    artist.bandcamp_embed_url ||
    artist.socials?.spotify ||
    artist.socials?.bandcamp
  )
}

/**
 * Check if any artist in the list has music
 */
function showHasArtistMusic(artists: ArtistResponse[]): boolean {
  return artists.some(artistHasMusic)
}

interface ShowCardProps {
  show: ShowResponse
  isAdmin: boolean
}

function ShowCard({ show, isAdmin }: ShowCardProps) {
  const { user } = useAuthContext()
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isExpanded, setIsExpanded] = useState(false)
  const venue = show.venues[0]
  const artists = show.artists

  // Check if any artist has music to show the expand button
  const hasArtistMusic = showHasArtistMusic(artists)

  // Check if user can delete: admin or show owner
  const canDelete = isAdmin || (user?.id === String(show.submitted_by))

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
                  <Link
                    href={`/artists/${artist.id}`}
                    className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
                  >
                    {artist.name}
                  </Link>
                </span>
              ))}
            </h2>

            {/* Action Buttons */}
            <div className="flex items-center gap-1 shrink-0">
              {/* Expand Button - only show if artists have music */}
              {hasArtistMusic && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setIsExpanded(!isExpanded)}
                  className="h-7 w-7 p-0"
                  title={isExpanded ? 'Hide artist music' : 'Discover artist music'}
                >
                  {isExpanded ? (
                    <ChevronUp className="h-4 w-4" />
                  ) : (
                    <ChevronDown className="h-4 w-4" />
                  )}
                </Button>
              )}

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

              {/* Delete Button */}
              {canDelete && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setIsDeleteDialogOpen(true)}
                  className="h-7 w-7 p-0 text-destructive hover:bg-destructive/10"
                  title="Delete show"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              )}
            </div>
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

      {/* Expanded Artist Music Section */}
      {isExpanded && hasArtistMusic && (
        <div className="mt-4 pt-4 border-t border-border/50">
          <div className="space-y-6">
            {artists.filter(artistHasMusic).map(artist => (
              <div key={artist.id} className="space-y-2">
                <div className="flex items-start justify-between gap-2">
                  <div>
                    <Link
                      href={`/artists/${artist.id}`}
                      className="font-medium hover:text-primary transition-colors"
                    >
                      {artist.name}
                    </Link>
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
                  compact
                />
              </div>
            ))}
          </div>
        </div>
      )}

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
