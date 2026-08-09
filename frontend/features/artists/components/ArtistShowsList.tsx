'use client'

import { Loader2 } from 'lucide-react'
import { formatCount, SectionHeader } from '@/components/shared'
import { NotifyMeButton } from '@/features/notifications'
import { useArtistShows } from '../hooks/useArtists'
import {
  ARTIST_UPCOMING_SHOWS_LIMIT,
  ARTIST_SHOWS_VIEWER_TIMEZONE,
} from '../api'
import { ArtistPastShows } from './ArtistPastShows'
import { ArtistShowsTable } from './ArtistShowsTable'

interface ArtistShowsListProps {
  artistId: number
  /** Used to build page/year hrefs in the past archive, so they are shareable. */
  artistSlug: string
  artistName: string
  className?: string
}

function ShowsLoader() {
  return (
    <div className="flex justify-center py-6">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  )
}

/**
 * An artist's shows: everything booked ahead, then the archive behind it
 * (PSY-1754).
 *
 * The two sections answer different questions and are paged differently, so
 * they own their own requests rather than slicing one. Upcoming is bounded by
 * booking horizons and comes down in a single request; the past is unbounded
 * and lives in `ArtistPastShows`, which pages it by year.
 *
 * The venue twin is `VenueShowsList` (PSY-1753). The one deliberate divergence
 * is in the rows: an artist's shows span venues, so each row names where it
 * happened. There is no add-show affordance here, because a show is created
 * against a venue, not against a bill.
 */
export function ArtistShowsList({
  artistId,
  artistSlug,
  artistName,
  className,
}: ArtistShowsListProps) {
  const upcoming = useArtistShows({
    artistId,
    timezone: ARTIST_SHOWS_VIEWER_TIMEZONE,
    timeFilter: 'upcoming',
    limit: ARTIST_UPCOMING_SHOWS_LIMIT,
  })

  // No `dedupArtistShows` here, and none in the past archive either. The
  // duplicate class it filtered — the same artist, at the same venue, at the
  // same instant — is enforced impossible by the PSY-576 structural unique
  // index on `show_artists (artist_id, venue_id, event_date)`, verified clean
  // on stage and on prod, and that index key IS the dedup key once the list is
  // scoped to one artist. Filtering rows after the fact is also actively wrong
  // in a server-paged list: it would render fewer rows than the pager's own
  // "Showing 51-100 of 161" claims, and `total` and the year histogram would
  // still count what the page dropped.
  const upcomingShows = upcoming.data?.shows ?? []
  const upcomingTotal = upcoming.data?.total ?? upcomingShows.length

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
              entityType="artist"
              entityId={artistId}
              entityName={artistName}
              variant="bracket"
            />
          </div>
        ) : (
          <>
            <ArtistShowsTable
              shows={upcomingShows}
              ariaLabel="Upcoming shows"
            />
            {/* Only when the backend cap actually bit. An artist booked further
                out than `ARTIST_UPCOMING_SHOWS_LIMIT` is hypothetical today,
                but a silently truncated list would read as the whole
                calendar. */}
            {upcomingTotal > upcomingShows.length && (
              <p className="mt-2 text-xs text-muted-foreground">
                Showing the next {formatCount(upcomingShows.length)} of{' '}
                {formatCount(upcomingTotal)} announced shows.
              </p>
            )}
          </>
        )}
      </section>

      <ArtistPastShows
        artistId={artistId}
        artistSlug={artistSlug}
        artistName={artistName}
        className="mt-8"
      />
    </div>
  )
}
