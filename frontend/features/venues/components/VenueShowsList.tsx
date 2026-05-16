'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Loader2, Plus } from 'lucide-react'
import {
  BracketLink,
  SectionHeader,
  DenseTable,
} from '@/components/shared'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms/ShowForm'
import { useAuthContext } from '@/lib/context/AuthContext'
import { NotifyMeButton } from '@/features/notifications'
import { dedupVenueShows } from '@/features/shows'
import { formatShowDate, formatShowTime } from '@/lib/utils/formatters'
import { useVenueShows } from '../hooks/useVenues'
import type { VenueShow } from '../types'

interface VenueShowsListProps {
  venueId: number
  venueSlug: string
  venueName: string
  venueCity: string
  venueState: string
  venueAddress?: string | null
  venueVerified?: boolean
  className?: string
  onShowAdded?: () => void
}

const TIMEZONE = Intl.DateTimeFormat().resolvedOptions().timeZone

function ShowsLoader() {
  return (
    <div className="flex justify-center py-6">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  )
}

function ShowRow({ show, venueState }: { show: VenueShow; venueState: string }) {
  // Per-show `state` falls back to the venue's state for date/time
  // formatting when the show row didn't carry one.
  const state = show.state ?? venueState
  const detailsHref = `/shows/${show.slug || show.id}`
  return (
    <tr>
      <td className="whitespace-nowrap">
        <Link
          href={detailsHref}
          className="hover:text-primary hover:underline underline-offset-2"
        >
          {formatShowDate(show.event_date, state)}
        </Link>
      </td>
      <td className="text-muted-foreground">
        {show.artists.length > 0 ? (
          show.artists.map((a, i) => (
            <span key={a.id}>
              {i > 0 && ', '}
              <Link
                href={`/artists/${a.slug}`}
                className="hover:text-foreground hover:underline"
              >
                {a.name}
              </Link>
            </span>
          ))
        ) : (
          '—'
        )}
      </td>
      <td className="text-right whitespace-nowrap text-muted-foreground">
        {formatShowTime(show.event_date, state)}
      </td>
    </tr>
  )
}

function ShowsTable({
  shows,
  total,
  venueState,
  isPast,
}: {
  shows: VenueShow[]
  total: number
  venueState: string
  isPast: boolean
}) {
  return (
    <>
      <DenseTable
        variant="alternating"
        aria-label={isPast ? 'Past shows' : 'Upcoming shows'}
      >
        <thead>
          <tr>
            <th>Date</th>
            <th>Bill</th>
            <th className="text-right">Time</th>
          </tr>
        </thead>
        <tbody>
          {shows.map(show => (
            <ShowRow key={show.id} show={show} venueState={venueState} />
          ))}
        </tbody>
      </DenseTable>
      {total > shows.length && (
        <p className="text-xs text-muted-foreground mt-2">
          Showing {shows.length} of {total} shows
        </p>
      )}
    </>
  )
}

/**
 * Both fetches fire eagerly so the past-shows-when-zero branch can hide
 * the whole section without an expand round-trip.
 */
export function VenueShowsList({
  venueId,
  venueSlug,
  venueName,
  venueCity,
  venueState,
  venueAddress,
  venueVerified,
  className,
  onShowAdded,
}: VenueShowsListProps) {
  const [pastOpen, setPastOpen] = useState(false)
  const [isAddingShow, setIsAddingShow] = useState(false)
  const { isAuthenticated } = useAuthContext()

  const upcoming = useVenueShows({
    venueId,
    timezone: TIMEZONE,
    timeFilter: 'upcoming',
    enabled: true,
    limit: 50,
  })
  const past = useVenueShows({
    venueId,
    timezone: TIMEZONE,
    timeFilter: 'past',
    enabled: true,
    limit: 50,
  })

  const upcomingShows = upcoming.data?.shows
    ? dedupVenueShows(upcoming.data.shows)
    : []
  const pastShows = past.data?.shows ? dedupVenueShows(past.data.shows) : []

  return (
    <div className={className}>
      <section>
        <SectionHeader title="Upcoming shows" as="h2" size="md" />
        {upcoming.isLoading ? (
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
          <ShowsTable
            shows={upcomingShows}
            total={upcoming.data?.total ?? upcomingShows.length}
            venueState={venueState}
            isPast={false}
          />
        )}
      </section>

      {pastShows.length > 0 && (
        <section className="mt-8">
          <SectionHeader
            title="Past shows"
            as="h2"
            size="md"
            action={
              <BracketLink
                label={pastOpen ? 'Hide' : 'Show'}
                onClick={() => setPastOpen(!pastOpen)}
              />
            }
          />
          {pastOpen && (
            <ShowsTable
              shows={pastShows}
              total={past.data?.total ?? pastShows.length}
              venueState={venueState}
              isPast={true}
            />
          )}
        </section>
      )}

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
