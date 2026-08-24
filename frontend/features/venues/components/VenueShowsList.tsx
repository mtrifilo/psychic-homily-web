'use client'

import { useMemo, useState } from 'react'
import { Loader2, Plus } from 'lucide-react'
import { formatCount, SectionHeader } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { useAuthContext } from '@/lib/context/AuthContext'
import { NotifyMeButton } from '@/features/notifications'
import { ShowForm } from '@/features/shows'
import { useVenueShows } from '../hooks/useVenues'
import { VENUE_UPCOMING_SHOWS_LIMIT } from '../api'
import { VenuePastShows } from './VenuePastShows'
import { VenueShowsTable } from './VenueShowsTable'
import type { VenueShowYearsResponse, VenueShowZone } from '../types'

interface VenueShowsListProps {
  venueId: number
  venueSlug: string
  venueName: string
  venueCity: string
  venueState: string
  venueTimezone?: string | null
  venueAddress?: string | null
  venueVerified?: boolean
  /**
   * The past-show year histogram the ROUTE already fetched, so the archive's
   * year strip is in the served HTML instead of appearing after the first
   * client fetch (PSY-1756). Passed straight through to `VenuePastShows`.
   */
  initialPastYears?: VenueShowYearsResponse
  className?: string
  onShowAdded?: () => void
}

function ShowsLoader() {
  return (
    <div className="flex justify-center py-6">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  )
}

/**
 * A venue's shows: everything booked ahead, then the archive behind it.
 *
 * The two sections answer different questions and are paged differently, so
 * they own their own requests rather than slicing one. Upcoming is bounded by
 * booking horizons and comes down in a single request; the past is unbounded
 * and lives in `VenuePastShows`, which pages it by year (PSY-1753).
 */
export function VenueShowsList({
  venueId,
  venueSlug,
  venueName,
  venueCity,
  venueState,
  venueTimezone,
  venueAddress,
  venueVerified,
  initialPastYears,
  className,
  onShowAdded,
}: VenueShowsListProps) {
  const [isAddingShow, setIsAddingShow] = useState(false)
  const { isAuthenticated } = useAuthContext()

  const upcoming = useVenueShows({
    venueId,
    timeFilter: 'upcoming',
    limit: VENUE_UPCOMING_SHOWS_LIMIT,
  })

  // No `dedupVenueShows` here, and none in the past archive either (PSY-1753).
  // The duplicate class it filtered — same headliner, same instant, at this
  // venue — is enforced impossible by the PSY-576 structural unique index,
  // verified clean on stage and on prod. Filtering rows after the fact is also
  // actively wrong once a list is paged server-side: it would render fewer rows
  // than the pager's own "Showing 51-100 of 161" claims, and `total` and the
  // year histogram would still count what the page dropped. The Atlas venue
  // panel still dedups because it fetches one unpaged page.
  const upcomingShows = upcoming.data?.shows ?? []
  const upcomingTotal = upcoming.data?.total ?? upcomingShows.length
  // Stable identity so `VenueShowsTable`'s memos key on it cleanly. This
  // section does not group by month (that is the past archive's treatment), so
  // nothing expensive hangs off it here — it is kept stable to match the past
  // section, where it does matter, rather than to fix a cost on this path.
  const zone: VenueShowZone = useMemo(
    () => ({ venueState, venueTimezone }),
    [venueState, venueTimezone]
  )

  return (
    <div className={className}>
      <section>
        <SectionHeader
          title="Upcoming shows"
          as="h2"
          size="md"
          action={
            upcomingShows.length > 0 ? (
              <span className="font-mono text-xs text-muted-foreground">
                {formatCount(upcomingTotal)}
              </span>
            ) : undefined
          }
        />
        {upcoming.isPending ? (
          <ShowsLoader />
        ) : upcoming.error ? (
          <p className="py-3 text-sm text-destructive">Failed to load shows</p>
        ) : upcomingShows.length === 0 ? (
          <div className="flex items-baseline gap-3 py-2 text-sm text-muted-foreground">
            <span>No upcoming shows yet.</span>
            <NotifyMeButton
              entityType="venue"
              entityId={venueId}
              entityName={venueName}
              variant="bracket"
            />
          </div>
        ) : (
          <>
            <VenueShowsTable
              shows={upcomingShows}
              zone={zone}
              ariaLabel="Upcoming shows"
            />
            {/* Only when the backend cap actually bit. A venue booked further
                out than `VENUE_UPCOMING_SHOWS_LIMIT` is hypothetical today, but
                a silently truncated list would read as the whole calendar. */}
            {upcomingTotal > upcomingShows.length && (
              <p className="mt-2 text-xs text-muted-foreground">
                Showing the next {formatCount(upcomingShows.length)} of{' '}
                {formatCount(upcomingTotal)} announced shows.
              </p>
            )}
          </>
        )}
      </section>

      <VenuePastShows
        venueId={venueId}
        venueSlug={venueSlug}
        venueName={venueName}
        venueState={venueState}
        venueTimezone={venueTimezone}
        initialYears={initialPastYears}
        className="mt-8"
      />

      {isAuthenticated && (
        <div className="mt-6 pt-4 border-t border-border/50">
          {isAddingShow ? (
            <ShowForm
              mode="create"
              prefilledVenue={{
                id: venueId,
                slug: venueSlug,
                name: venueName,
                city: venueCity,
                state: venueState,
                address: venueAddress || undefined,
                timezone: venueTimezone,
                verified: venueVerified,
              }}
              onSuccess={() => {
                setIsAddingShow(false)
                onShowAdded?.()
              }}
              onCancel={() => setIsAddingShow(false)}
              redirectOnCreate={false}
            />
          ) : (
            <Button
              variant="outline"
              onClick={() => setIsAddingShow(true)}
              className="w-full"
            >
              <Plus className="h-4 w-4 mr-2" />
              Add a show at {venueName}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
