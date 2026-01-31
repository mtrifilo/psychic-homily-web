'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { ArrowLeft, BadgeCheck, Pencil, Trash2, Loader2, ExternalLink } from 'lucide-react'
import { useVenue } from '@/lib/hooks/useVenues'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/lib/queryClient'
import { SocialLinks } from '@/components/SocialLinks'
import { VenueLocationCard } from '@/components/VenueLocationCard'
import { VenueShowsList } from '@/components/VenueShowsList'
import { VenueEditForm } from '@/components/forms/VenueEditForm'
import { DeleteVenueDialog } from '@/components/DeleteVenueDialog'
import { FavoriteVenueButton } from '@/components/FavoriteVenueButton'
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

export function VenueDetail({ venueId }: VenueDetailProps) {
  const [isEditingVenue, setIsEditingVenue] = useState(false)
  const [isDeleteVenueOpen, setIsDeleteVenueOpen] = useState(false)
  const { isAuthenticated, user } = useAuthContext()
  const queryClient = useQueryClient()
  const router = useRouter()

  const { data: venue, isLoading, error } = useVenue({ venueId })

  // User can edit if they're an admin OR if they submitted the venue
  const canEdit =
    isAuthenticated &&
    venue &&
    (user?.is_admin ||
      (venue.submitted_by != null && venue.submitted_by === Number(user?.id)))

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
    const is404 = errorMessage.includes('not found') || errorMessage.includes('404')

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
      {/* Back Navigation */}
      <div className="mb-6">
        <Link
          href="/venues"
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4 mr-1" />
          Back to Venues
        </Link>
      </div>

      {/* Main Content - Two Column Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-[1fr_400px] gap-8">
        {/* Main Column - Header + Shows */}
        <div className="order-2 lg:order-1">
          {/* Header */}
          <header className="mb-8">
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1">
                <div className="flex items-center gap-2 flex-wrap">
                  <h1 className="text-2xl md:text-3xl font-bold">{venue.name}</h1>
                  {venue.verified && (
                    <BadgeCheck
                      className="h-6 w-6 text-primary shrink-0"
                      aria-label="Verified venue"
                    />
                  )}
                  <FavoriteVenueButton venueId={venue.id} size="md" />
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

        {/* Sidebar - Location Card */}
        <div className="order-1 lg:order-2">
          <VenueLocationCard
            name={venue.name}
            address={venue.address}
            city={venue.city}
            state={venue.state}
            zipcode={venue.zipcode}
            verified={venue.verified}
          />
        </div>
      </div>

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
