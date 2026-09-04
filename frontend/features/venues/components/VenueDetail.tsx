'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { ArrowLeft, BadgeCheck, Pencil, Trash2, Loader2, ExternalLink, Flag } from 'lucide-react'
import { useVenue, useVenueGenres } from '../hooks/useVenues'
import type { VenueShowYearsResponse } from '../types'
import { useVenueUpdate } from '../hooks/useVenueEdit'
import type { ApiError } from '@/lib/api'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/lib/queryClient'
import { SocialLinks, RevisionHistory, FollowButton, Breadcrumb, TagPill, EntityDescription, AddToCollectionButton, EntityHeader } from '@/components/shared'
import { EntityCollections } from '@/features/collections'
import { EntityChartRankBadge } from '@/features/charts'
import { CommentThread } from '@/features/comments'
import { EntityTagList } from '@/features/tags'
import { FollowAlertsReveal } from '@/components/shared/FollowAlertsReveal'
import { VenueLocationCard } from './VenueLocationCard'
import { VenueShowsList } from './VenueShowsList'
import { VenueBillNetwork, VENUE_SHOWS_ANCHOR } from './VenueBillNetwork'
import { EntityEditDrawer, EntitySaveSuccessBanner, useEntitySaveSuccessBanner, AttributionLine, ReportEntityDialog, useSuggestEdit, type EntityEditSuccess } from '@/features/contributions'
import { DeleteVenueDialog } from './DeleteVenueDialog'
import { Button } from '@/components/ui/button'
import { socialLinkHref } from '@/lib/socialLinks'

interface VenueDetailProps {
  venueId: string | number
  /**
   * The venue's past-show year histogram, already fetched by the route so the
   * archive's year strip reaches the served HTML (PSY-1756).
   *
   * Threaded to `VenuePastShows` as a prop rather than seeded through the
   * page's `HydrationBoundary`, and the reason is freshness rather than key
   * mechanics — no venue-shows key carries anything per-viewer, so any of them
   * CAN be built on the server. `seedFirstScreen` stamps `dataUpdatedAt: 0`,
   * which would make every venue page refetch a
   * histogram it just rendered; `initialData` is treated as fresh for the usual
   * staleTime, which is what a server-rendered strip should be. The cost is
   * this prop passing through two components that do nothing else with it, and
   * a strip that can lag by up to the server read's own window (an hour): on
   * the day a venue's newest show graduates from upcoming to past, the current
   * year can be missing from the strip for that long. Acceptable for an archive
   * index; it would not be for the rows themselves.
   */
  initialPastYears?: VenueShowYearsResponse
}

/**
 * The domain to print beside a website link,
 * e.g. "https://www.therebelphx.com/events" -> "therebelphx.com".
 *
 * Its argument is an href `socialLinkHref` has already parsed, so this parse
 * cannot throw and the caption names the host the click resolves to.
 */
function getDisplayDomain(href: string): string {
  return new URL(href).hostname.replace(/^www\./, '')
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

export function VenueDetail({ venueId, initialPastYears }: VenueDetailProps) {
  const [isEditingVenue, setIsEditingVenue] = useState(false)
  const [isDeleteVenueOpen, setIsDeleteVenueOpen] = useState(false)
  const [isReportOpen, setIsReportOpen] = useState(false)
  const { isAuthenticated, user } = useAuthContext()
  const queryClient = useQueryClient()
  const router = useRouter()
  const venueUpdate = useVenueUpdate()
  const suggestVenueEdit = useSuggestEdit()
  const saveBanner = useEntitySaveSuccessBanner()

  const { data: venue, isLoading, error } = useVenue({ venueId })

  // Any authenticated user can suggest edits; admins/trusted can edit directly
  const canEdit = isAuthenticated && venue
  const userTier = (user as unknown as Record<string, unknown> | undefined)?.user_tier
  const canEditDirectly = isAuthenticated && (
    user?.is_admin ||
    userTier === 'trusted_contributor' ||
    userTier === 'local_ambassador' ||
    (venue?.submitted_by != null && venue.submitted_by === Number(user?.id))
  )

  const handleVenueUpdated = (result: EntityEditSuccess) => {
    // Invalidate venue detail query
    queryClient.invalidateQueries({
      queryKey: queryKeys.venues.detail(String(venueId)),
    })
    saveBanner.handleSaveSuccess(result)
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

  // The anchor and its caption both read this one value. `website` anchors no
  // host, so what the gate buys here is an absolute, parseable destination.
  const websiteHref = socialLinkHref('website', venue.social?.website)

  return (
    // max-w-6xl matches the other 4 EntityDetailLayout-based detail pages (ArtistDetail, ReleaseDetail, LabelDetail, FestivalDetail). Previously max-w-5xl was drift from when the 2-col grid was added; the 400px sidebar + gap still fits comfortably at 6xl on desktop.
    <div className="container max-w-6xl mx-auto px-4 py-6">
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        fallback={{ href: '/venues', label: 'Venues' }}
        currentPage={venue.name}
      />

      {/* Main Content - Two Column Layout */}
      {/* PSY-1034: `minmax(0,1fr)` (not `1fr`) caps the main track's implicit
          `min-width: auto`. Without it, the ResizeObserver-measured graph in
          VenueBillNetwork grows the track, which re-fires the RO with a larger
          width, ballooning the layout rightward each cycle. Same class as
          PSY-949's Dialog fix; `min-w-0` on the main column is the
          belt-and-suspenders sibling. */}
      <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_400px] gap-8">
        {/* Main Column - Header + Shows */}
        <div className="order-2 lg:order-1 min-w-0">
          {/* Header */}
          <header className="mb-8">
            <EntityHeader
              title={venue.name}
              actionsPlacement="below"
              subtitle={
                <>
                  {venue.verified && (
                    <BadgeCheck
                      className="h-5 w-5 text-primary shrink-0"
                      aria-label="Verified venue"
                    />
                  )}
                  <span>{venue.city}, {venue.state}</span>
                </>
              }
              actions={
                <>
                  <FollowButton entityType="venues" entityId={venue.id} />
                  <AddToCollectionButton entityType="venue" entityId={venue.id} entityName={venue.name} />
                  {/* No Notify-me control: following a venue IS subscribing to
                      its alerts (PSY-1893). The on/off axis is revealed after
                      the follow, below this row (PSY-1905). */}
                  {isAuthenticated && (
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setIsEditingVenue(true)}
                      >
                        <Pencil className="h-4 w-4 mr-2" />
                        Edit
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setIsReportOpen(true)}
                        className="text-muted-foreground hover:text-foreground"
                        title="Report an issue"
                      >
                        <Flag className="h-4 w-4" />
                      </Button>
                      {user?.is_admin && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setIsDeleteVenueOpen(true)}
                          className="text-destructive hover:text-destructive hover:bg-destructive/10"
                        >
                          <Trash2 className="h-4 w-4 mr-2" />
                          Delete
                        </Button>
                      )}
                    </>
                  )}
                </>
              }
            />

            {/* Renders nothing until the venue is followed. */}
            <FollowAlertsReveal
              entityType="venues"
              entityId={venue.id}
              entityName={venue.name}
              className="mt-2"
            />

            {websiteHref && (
              <a
                href={websiteHref}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-sm text-primary hover:underline mt-2"
              >
                {getDisplayDomain(websiteHref)}
                <ExternalLink className="h-3 w-3" />
              </a>
            )}

            <div className="mt-1">
              <AttributionLine entityType="venue" entityId={venue.id} />
            </div>

            <EntitySaveSuccessBanner visible={saveBanner.isVisible} />

            {/* Social Links */}
            {venue.social && (
              <SocialLinks social={venue.social} className="mt-4" />
            )}

            {/* Tags */}
            <EntityTagList
              entityType="venue"
              entityId={venue.id}
              isAuthenticated={isAuthenticated}
            />
          </header>

          {/* Description */}
          <div className="mb-6">
            <EntityDescription
              description={venue.description}
              canEdit={!!canEditDirectly}
              onSave={async (description) => {
                await new Promise<void>((resolve, reject) => {
                  if (user?.is_admin) {
                    venueUpdate.mutate(
                      { venueId: venue.id, data: { description } },
                      {
                        onSuccess: () => {
                          queryClient.invalidateQueries({
                            queryKey: queryKeys.venues.detail(String(venueId)),
                          })
                          resolve()
                        },
                        onError: (err) => reject(err),
                      }
                    )
                    return
                  }
                  // PSY-668: trusted_contributor + local_ambassador + the venue's
                  // original submitter route through suggest-edit, which the
                  // backend auto-applies for trusted tiers via canEditDirectly
                  // (pending_edit.go) or queues for review otherwise.
                  // useSuggestEdit's own onSuccess invalidates ['venues'], which
                  // prefix-matches the detail key — no caller-side invalidate.
                  suggestVenueEdit.mutate(
                    {
                      entityType: 'venue',
                      entityId: venue.id,
                      changes: [
                        {
                          field: 'description',
                          old_value: venue.description ?? '',
                          new_value: description,
                        },
                      ],
                      summary: 'Updated description via inline editor',
                    },
                    {
                      onSuccess: () => resolve(),
                      onError: (err) => reject(err),
                    }
                  )
                })
              }}
            />
          </div>

          {/* Shows List. id="venue-shows": the mobile graph teaser's link-out
              target (VenueBillNetwork, PSY-1472). scroll-mt for the sticky
              header. */}
          <div id={VENUE_SHOWS_ANCHOR} className="scroll-mt-20">
            <VenueShowsList
              venueId={venue.id}
              venueSlug={venue.slug}
              venueName={venue.name}
              venueCity={venue.city}
              venueState={venue.state}
              venueTimezone={venue.timezone}
              venueAddress={venue.address}
              venueVerified={venue.verified}
              initialPastYears={initialPastYears}
              onShowAdded={handleShowAdded}
            />
          </div>

          {/* Bill Network — PSY-365: who plays together at this venue. The
              section returns null when the venue is too sparse, when the
              viewport is mobile, or when the active window has no co-bills,
              so we render unconditionally and let the component self-gate. */}
          <VenueBillNetwork venueIdOrSlug={venue.id} venueName={venue.name} />
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
          <EntityChartRankBadge
            entityType="venue"
            entityId={venue.id}
            className="mt-6"
          />
          <VenueGenreProfile venueId={venue.id} />
          <div className="mt-6">
            <EntityCollections entityType="venue" entityId={venue.id} />
          </div>
        </div>
      </div>

      {/* History + Discussion — already inside this page's root
          `container max-w-6xl mx-auto px-4`, so they share the same gutter +
          max-width as the rest of the page. Each section carries its own top
          margin, so no per-section wrapper is needed (PSY-1026). */}
      <RevisionHistory
        entityType="venue"
        entityId={venue.id}
        isAdmin={!!user?.is_admin}
      />
      <CommentThread entityType="venue" entityId={venue.id} />

      {/* Edit Drawer (all authenticated users) */}
      {venue && isAuthenticated && (
        <EntityEditDrawer
          open={isEditingVenue}
          onOpenChange={(open) => setIsEditingVenue(open)}
          entityType="venue"
          entityId={venue.id}
          entityName={venue.name}
          entity={venue as unknown as Record<string, unknown>}
          canEditDirectly={!!canEditDirectly}
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

      {/* Report Dialog (authenticated users) */}
      {venue && isAuthenticated && (
        <ReportEntityDialog
          open={isReportOpen}
          onOpenChange={setIsReportOpen}
          entityType="venue"
          entityId={venue.id}
          entityName={venue.name}
        />
      )}
    </div>
  )
}
