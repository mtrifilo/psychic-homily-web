'use client'

import { useState, useMemo, useCallback, useRef, useEffect } from 'react'
import { useUpcomingShows, useShowCities } from '@/lib/hooks/useShows'
import { useSavedShowBatch } from '@/lib/hooks/useSavedShows'
import { usePrefetchRoutes } from '@/lib/hooks/usePrefetchRoutes'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/lib/hooks/useAuth'
import { useSetFavoriteCities } from '@/lib/hooks/useFavoriteCities'
import type { ShowResponse, ArtistResponse } from '@/lib/types/show'
import type { CityState } from '@/components/filters'
import Link from 'next/link'
import { Pencil, Trash2, X, ChevronDown, ChevronUp, MapPin } from 'lucide-react'
import {
  formatShowDate,
  formatShowTime,
  formatPrice,
} from '@/lib/utils/formatters'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms'
import { SaveButton, SocialLinks, MusicEmbed } from '@/components/shared'
import { DeleteShowDialog } from './DeleteShowDialog'
import { ShowStatusBadge } from './ShowStatusBadge'
import { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'
import { CityFilters, type CityWithCount } from '@/components/filters'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'

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
  isSaved?: boolean
}

function ShowCard({ show, isAdmin, isSaved }: ShowCardProps) {
  const { user } = useAuthContext()
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isExpanded, setIsExpanded] = useState(false)
  const venue = show.venues[0]
  const artists = show.artists

  // Check if any artist has music to show the expand button
  const hasArtistMusic = showHasArtistMusic(artists)

  // Check if user can delete: admin or show owner
  const canDelete = isAdmin || user?.id === String(show.submitted_by)

  const handleEditSuccess = () => {
    setIsEditing(false)
  }

  const handleEditCancel = () => {
    setIsEditing(false)
  }

  return (
    <article
      className={`border-b border-border/50 py-4 -mx-2 px-2 rounded-md hover:bg-muted/30 transition-colors duration-75 ${show.is_cancelled ? 'opacity-60' : ''}`}
    >
      <div className="flex flex-col md:flex-row">
        {/* Left column: Date and Location */}
        <div className="w-full md:w-1/5 md:pr-4 mb-1 md:mb-0">
          <Link href={`/shows/${show.slug || show.id}`} className="block group">
            <h3 className="text-sm font-bold tracking-wide text-primary group-hover:underline underline-offset-2">
              {formatShowDate(show.event_date, show.state)}
            </h3>
            <p className="text-xs text-muted-foreground">
              {show.city}, {show.state}
            </p>
          </Link>
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
            </h2>

            {/* Action Buttons */}
            <div className="flex items-center gap-1 shrink-0">
              {/* Expand Button - only show if artists have music */}
              {SHOW_LIST_FEATURE_POLICY.discovery.showExpandMusic &&
                hasArtistMusic && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setIsExpanded(!isExpanded)}
                    className="h-7 w-7 p-0"
                    title={
                      isExpanded ? 'Hide artist music' : 'Discover artist music'
                    }
                  >
                    {isExpanded ? (
                      <ChevronUp className="h-4 w-4" />
                    ) : (
                      <ChevronDown className="h-4 w-4" />
                    )}
                  </Button>
                )}

              {/* Save Button */}
              {SHOW_LIST_FEATURE_POLICY.discovery.showSaveButton && (
                <SaveButton
                  showId={show.id}
                  variant="ghost"
                  size="sm"
                  isSaved={isSaved}
                />
              )}

              {/* Admin Edit Button */}
              {SHOW_LIST_FEATURE_POLICY.discovery.showAdminActions &&
                isAdmin && (
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
              {SHOW_LIST_FEATURE_POLICY.discovery.showOwnerActions &&
                canDelete && (
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
            {venue &&
              (venue.slug ? (
                <Link
                  href={`/venues/${venue.slug}`}
                  className="text-primary/80 hover:text-primary font-medium transition-colors"
                >
                  {venue.name}
                </Link>
              ) : (
                <span className="text-primary/80 font-medium">
                  {venue.name}
                </span>
              ))}
            {show.price != null && (
              <span>&nbsp;•&nbsp;{formatPrice(show.price)}</span>
            )}
            {show.age_requirement && (
              <span>&nbsp;•&nbsp;{show.age_requirement}</span>
            )}
            <span>
              &nbsp;•&nbsp;{formatShowTime(show.event_date, show.state)}
            </span>
            {SHOW_LIST_FEATURE_POLICY.discovery.showDetailsLink && (
              <>
                <span>&nbsp;•&nbsp;</span>
                <Link
                  href={`/shows/${show.slug || show.id}`}
                  className="text-primary/80 hover:text-primary underline underline-offset-2 transition-colors"
                >
                  Details
                </Link>
              </>
            )}
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
                          {[artist.city, artist.state]
                            .filter(Boolean)
                            .join(', ')}
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

/** Compare two city arrays for equality (order-insensitive) */
function citiesEqual(a: CityState[], b: CityState[]): boolean {
  if (a.length !== b.length) return false
  const setA = new Set(a.map(c => `${c.city}|${c.state}`))
  return b.every(c => setA.has(`${c.city}|${c.state}`))
}

export function HomeShowList() {
  const { user, isAuthenticated } = useAuthContext()
  const isAdmin = user?.is_admin ?? false
  const { data: profileData } = useProfile()
  const [selectedCities, setSelectedCities] = useState<CityState[]>([])
  const hasManuallyInteracted = useRef(false)

  // Read favorites from profile
  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  // Apply favorites as defaults when profile loads (only if user hasn't manually changed)
  useEffect(() => {
    if (!hasManuallyInteracted.current && favoriteCities.length > 0) {
      setSelectedCities(favoriteCities)
    }
  }, [favoriteCities])

  const handleFilterChange = useCallback((cities: CityState[]) => {
    hasManuallyInteracted.current = true
    setSelectedCities(cities)
  }, [])

  const timezone =
    typeof window !== 'undefined'
      ? Intl.DateTimeFormat().resolvedOptions().timeZone
      : 'America/Phoenix'

  const { data: citiesData } = useShowCities({ timezone })

  const { data, isLoading, isFetching, error } = useUpcomingShows({
    timezone,
    limit: 5,
    cities: selectedCities.length > 0 ? selectedCities : undefined,
  })

  // Prefetch /shows and /venues data during idle time
  usePrefetchRoutes(timezone)

  const showIds = useMemo(
    () => data?.shows?.map(s => s.id) ?? [],
    [data?.shows]
  )
  const { data: savedShowIds } = useSavedShowBatch(showIds, isAuthenticated)

  const cities: CityWithCount[] = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.show_count,
      })) ?? [],
    [citiesData?.cities]
  )

  // Determine if "Save as default" / "Clear defaults" should show
  const selectionDiffersFromFavorites = !citiesEqual(selectedCities, favoriteCities)

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

  return (
    <div className="w-full">
      {cities.length > 1 && (
        <CityFilters
          cities={cities}
          selectedCities={selectedCities}
          onFilterChange={handleFilterChange}
        >
          {isAuthenticated && selectionDiffersFromFavorites && (
            <SaveDefaultsButton
              selectedCities={selectedCities}
              favoriteCities={favoriteCities}
            />
          )}
        </CityFilters>
      )}

      <div className={isFetching ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
        {!data?.shows || data.shows.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <p>
              {selectedCities.length > 0
                ? `No upcoming shows in ${selectedCities.map(c => c.city).join(', ')}.`
                : 'No upcoming shows at this time.'}
            </p>
          </div>
        ) : (
          data.shows.map(show => (
            <ShowCard
              key={show.id}
              show={show}
              isAdmin={isAdmin}
              isSaved={savedShowIds?.has(show.id)}
            />
          ))
        )}
      </div>
    </div>
  )
}
