'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { useRouter, usePathname } from 'next/navigation'
import { ArrowLeft, BadgeCheck, Pencil, Trash2, Loader2, ExternalLink } from 'lucide-react'
import { useVenue, useVenueGenres } from '../hooks/useVenues'
import type { ApiError } from '@/lib/api'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useNavigationBreadcrumbs } from '@/lib/context/NavigationBreadcrumbContext'
import { useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/lib/queryClient'
import { SocialLinks, RevisionHistory, FollowButton, Breadcrumb, TagPill } from '@/components/shared'
import { NotifyMeButton } from '@/features/notifications'
import { VenueLocationCard } from './VenueLocationCard'
import { VenueShowsList } from './VenueShowsList'
import { VenueEditForm } from '@/components/forms/VenueEditForm'
import { DeleteVenueDialog } from './DeleteVenueDialog'
import { FavoriteVenueButton } from './FavoriteVenueButton'
import { Button } from '@/components/ui/button'

interface VenueDetailProps {
  venueId: string | number
}

/**
 * Extract a display-friendly domain from a URL
 * e.g., "https://www.therebelphx.com/events" -> "therebelphx.com"
 */
function getDisplayDomain(url: string): string {
  try {
    const parsed = new URL(url.startsWith('http') ? url : `https://${url}`)
    return parsed.hostname.replace(/^www\./, '')
  } catch {
    return url
  }
}

/**
 * Normalize a URL to ensure it has a protocol
 */
function normalizeUrl(url: string): string {
  if (url.startsWith('http://') || url.startsWith('https://')) {
    return url
  }
  return `https://${url}`
}

function VenueGenreProfile({ venueId }: { venueId: number }) {
  const { data } = useVenueGenres(venueId)

  if (!data?.genres || data.genres.length === 0) {
    return null
  }

  return (
    <div className="rounded-lg border bg-card p-4 mt-4">
      <h3 className="text-sm font-semibold mb-3">Genre Profile</h3>
      <div className="flex flex-wrap gap-1.5">
        {data.genres.map((genre) => (
          <TagPill
            key={genre.tag_id}
            label={genre.name}
            href={`/tags/${genre.slug}`}
          />
        ))}
      </div>
    </div>
  )
}

export function VenueDetail({ venueId }: VenueDetailProps) {
  const [isEditingVenue, setIsEditingVenue] = useState(false)
  const [isDeleteVenueOpen, setIsDeleteVenueOpen] = useState(false)
  const { isAuthenticated, user } = useAuthContext()
  const queryClient = useQueryClient()
  const router = useRouter()
  const pathname = usePathname()
  const { pushBreadcrumb } = useNavigationBreadcrumbs()

  const { data: venue, isLoading, error } = useVenue({ venueId })

  // User can edit if they're an admin OR if they submitted the venue
  const canEdit =
    isAuthenticated &&
    venue &&
    (user?.is_admin ||
      (venue.submitted_by != null && venue.submitted_by === Number(user?.id)))

  // Push breadcrumb when venue data is loaded
  useEffect(() => {
    if (venue) {
      pushBreadcrumb(venue.name, pathname)
    }
  }, [venue, pathname, pushBreadcrumb])

  const handleVenueUpdated = () => {
    // Invalidate venue detail query
    queryClient.invalidateQueries({
      queryKey: queryKeys.venues.detail(String(venueId)),
    })
  }

  const handleShowAdded = () => {
    // Invalidate venue shows queries
    queryClient.invalidateQueries({
      queryKey: queryKeys.venues.shows(venueId),
    })
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
      error instanceof Error ? error.message : 'Failed to load venue'
    const is404 = (error as ApiError).status === 404

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Venue Not Found' : 'Error Loading Venue'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The venue you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/venues">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Venues
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!venue) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Venue Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The venue you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/venues">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Venues
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="container max-w-5xl mx-auto px-4 py-6">
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        fallback={{ href: '/venues', label: 'Venues' }}
        currentPage={venue.name}
      />

      {/* Main Content - Two Column Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-[1fr_400px] gap-8">
        {/* Main Column - Header + Shows */}
        <div className="order-2 lg:order-1">
          {/* Header */}
          <header className="mb-8">
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1">
                <div className="flex items-center gap-2 flex-wrap">
                  <h1 className="text-2xl md:text-3xl font-bold leading-8 md:leading-9">{venue.name}</h1>
                  {venue.verified && (
                    <BadgeCheck
                      className="h-6 w-6 text-primary shrink-0"
                      aria-label="Verified venue"
                    />
                  )}
                  <FavoriteVenueButton venueId={venue.id} size="md" />
                  <FollowButton entityType="venues" entityId={venue.id} />
                  <NotifyMeButton entityType="venue" entityId={venue.id} entityName={venue.name} />
                </div>
                <p className="text-muted-foreground mt-1">
                  {venue.city}, {venue.state}
                </p>
                {venue.social?.website && (
                  <a
                    href={normalizeUrl(venue.social.website)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-sm text-primary hover:underline mt-1"
                  >
                    {getDisplayDomain(venue.social.website)}
                    <ExternalLink className="h-3 w-3" />
                  </a>
                )}
              </div>

              {canEdit && (
                <div className="flex items-center gap-2 shrink-0">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setIsEditingVenue(true)}
                  >
                    <Pencil className="h-4 w-4 mr-2" />
                    Edit
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setIsDeleteVenueOpen(true)}
                    className="text-destructive hover:text-destructive hover:bg-destructive/10"
                  >
                    <Trash2 className="h-4 w-4 mr-2" />
                    Delete
                  </Button>
                </div>
              )}
            </div>

            {/* Social Links */}
            {venue.social && (
              <SocialLinks social={venue.social} className="mt-4" />
            )}
          </header>

          {/* Shows List */}
          <VenueShowsList
            venueId={venue.id}
            venueSlug={venue.slug}
            venueName={venue.name}
            venueCity={venue.city}
            venueState={venue.state}
            venueAddress={venue.address}
            venueVerified={venue.verified}
            onShowAdded={handleShowAdded}
          />
        </div>

        {/* Sidebar - Location Card + Genre Profile */}
        <div className="order-1 lg:order-2">
          <VenueLocationCard
            name={venue.name}
            address={venue.address}
            city={venue.city}
            state={venue.state}
            zipcode={venue.zipcode}
            verified={venue.verified}
          />
          <VenueGenreProfile venueId={venue.id} />
        </div>
      </div>

      {/* Revision History */}
      <RevisionHistory
        entityType="venue"
        entityId={venue.id}
        isAdmin={!!user?.is_admin}
      />

      {/* Venue Edit Form Dialog */}
      {venue && (
        <VenueEditForm
          venue={venue}
          open={isEditingVenue}
          onOpenChange={setIsEditingVenue}
          onSuccess={handleVenueUpdated}
        />
      )}

      {/* Delete Venue Dialog */}
      {venue && (
        <DeleteVenueDialog
          venue={venue}
          open={isDeleteVenueOpen}
          onOpenChange={setIsDeleteVenueOpen}
          onSuccess={() => router.push('/venues')}
        />
      )}
    </div>
  )
}
