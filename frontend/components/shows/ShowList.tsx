'use client'

import { useState, useCallback, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useUpcomingShows, useShowCities } from '@/lib/hooks/useShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { ShowResponse, ArtistResponse } from '@/lib/types/show'
import Link from 'next/link'
import { Pencil, X, Trash2, ChevronDown, ChevronUp, MapPin } from 'lucide-react'
import {
  formatDateInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms'
import { SaveButton, SocialLinks, MusicEmbed } from '@/components/shared'
import { DeleteShowDialog } from './DeleteShowDialog'
import { ExportShowButton } from './ExportShowButton'
import { ShowStatusBadge } from './ShowStatusBadge'
import { ShowCardSkeleton } from './ShowCardSkeleton'
import { CityFilters, type CityWithCount } from '@/components/filters'

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
  userId?: string
}

function ShowCard({ show, isAdmin, userId }: ShowCardProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isExpanded, setIsExpanded] = useState(false)
  const venue = show.venues[0] // Primary venue
  const artists = show.artists

  // Check if any artist has music to show the expand button
  const hasArtistMusic = showHasArtistMusic(artists)

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
    <article className={`border-b border-border/50 py-5 -mx-3 px-3 rounded-lg hover:bg-muted/30 transition-colors duration-75 ${show.is_cancelled ? 'opacity-60' : ''}`}>
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date and Location */}
        <div className="w-full md:w-1/5 md:pr-4 mb-2 md:mb-0">
          <Link
            href={`/shows/${show.slug || show.id}`}
            className="block group"
          >
            <h2 className="text-sm font-bold tracking-wide text-primary group-hover:underline underline-offset-2">
              {formatDate(show.event_date, show.state)}
            </h2>
            <h3 className="text-xs text-muted-foreground mt-0.5">
              {show.city}, {show.state}
            </h3>
          </Link>
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
                  ) : (
                    <span>{artist.name}</span>
                  )}
                </span>
              ))}
              {/* Status badges */}
              <ShowStatusBadge show={show} className="ml-2 inline-flex gap-1" />
            </h1>

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

              {/* Export Button (admin, dev only) */}
              {isAdmin && (
                <ExportShowButton
                  showId={show.id}
                  showTitle={show.title}
                  variant="ghost"
                  size="sm"
                  className="h-7 w-7 p-0"
                  iconOnly
                />
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
              venue.slug ? (
                <Link
                  href={`/venues/${venue.slug}`}
                  className="text-primary/80 hover:text-primary font-medium transition-colors"
                >
                  {venue.name}
                </Link>
              ) : (
                <span className="text-primary/80 font-medium">{venue.name}</span>
              )
            )}
            {show.price != null && (
              <span>&nbsp;•&nbsp;{formatPrice(show.price)}</span>
            )}
            {show.age_requirement && (
              <span>&nbsp;•&nbsp;{show.age_requirement}</span>
            )}
            <span>&nbsp;•&nbsp;{formatTime(show.event_date, show.state)}</span>
            <span>&nbsp;•&nbsp;</span>
            <Link
              href={`/shows/${show.slug || show.id}`}
              className="text-primary/80 hover:text-primary underline underline-offset-2 transition-colors"
            >
              Details
            </Link>
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

export function ShowList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { user } = useAuthContext()
  const isAdmin = user?.is_admin ?? false
  const [isPending, startTransition] = useTransition()

  const selectedCity = searchParams.get('city')
  const selectedState = searchParams.get('state')

  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const [cursor, setCursor] = useState<string | undefined>(undefined)
  const [accumulatedShows, setAccumulatedShows] = useState<ShowResponse[]>([])

  const { data: citiesData, isLoading: citiesLoading, isFetching: citiesFetching } = useShowCities({
    timezone,
  })

  const { data, isLoading, isFetching, error, refetch } = useUpcomingShows({
    timezone,
    cursor,
    city: selectedCity || undefined,
    state: selectedState || undefined,
  })

  const handleLoadMore = useCallback(() => {
    if (data?.pagination.next_cursor) {
      // Accumulate current shows before loading next page
      const currentShows = data.shows || []
      setAccumulatedShows(prev => [...prev, ...currentShows])
      setCursor(data.pagination.next_cursor!)
    }
  }, [data])

  const handleFilterChange = (city: string | null, state: string | null) => {
    // Reset pagination on filter change
    setCursor(undefined)
    setAccumulatedShows([])
    const params = new URLSearchParams()
    if (city) params.set('city', city)
    if (state) params.set('state', state)

    const queryString = params.toString()
    startTransition(() => {
      router.push(queryString ? `/shows?${queryString}` : '/shows')
    })
  }

  // Only show skeleton on FIRST load (no data yet)
  if ((isLoading && !data) || (citiesLoading && !citiesData)) {
    return (
      <section className="w-full max-w-4xl">
        {Array.from({ length: 5 }).map((_, i) => (
          <ShowCardSkeleton key={i} />
        ))}
      </section>
    )
  }

  // Track if we're updating (fetching but already have data)
  const isUpdating = isFetching || citiesFetching || isPending

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load shows. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  // Map ShowCity to CityWithCount
  const cities: CityWithCount[] = citiesData?.cities?.map(c => ({
    city: c.city,
    state: c.state,
    count: c.show_count,
  })) ?? []

  return (
    <section className="w-full max-w-4xl">
      {cities.length > 1 && (
        <CityFilters
          cities={cities}
          selectedCity={selectedCity}
          selectedState={selectedState}
          onFilterChange={handleFilterChange}
        />
      )}

      {/* Dim content while fetching, don't hide it */}
      <div className={isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
        {(() => {
          const allShows = [...accumulatedShows, ...(data?.shows || [])]
          if (allShows.length === 0) {
            return (
              <div className="text-center py-12 text-muted-foreground">
                <p>
                  {selectedCity
                    ? `No upcoming shows in ${selectedCity}.`
                    : 'No upcoming shows at this time.'}
                </p>
                {selectedCity && (
                  <button
                    onClick={() => handleFilterChange(null, null)}
                    className="mt-4 text-primary hover:underline"
                  >
                    View all shows
                  </button>
                )}
              </div>
            )
          }
          return (
            <>
              {allShows.map(show => (
                <ShowCard
                  key={show.id}
                  show={show}
                  isAdmin={isAdmin}
                  userId={user?.id}
                />
              ))}

              {data?.pagination.has_more && (
                <div className="text-center py-6">
                  <Button
                    variant="outline"
                    onClick={handleLoadMore}
                    disabled={isFetching}
                  >
                    {isFetching ? 'Loading...' : 'Load More'}
                  </Button>
                </div>
              )}
            </>
          )
        })()}
      </div>
    </section>
  )
}
