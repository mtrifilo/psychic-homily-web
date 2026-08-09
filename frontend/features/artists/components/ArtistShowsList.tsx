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
 * The venue twin is `VenueShowsList` (PSY-1753). Two deliberate divergences,
 * both in the rows:
 *
 *  - Each row names where it happened, because an artist's shows span venues.
 *  - Each row carries the WHOLE bill, including this artist. The pre-PSY-1754
 *    list filtered the page's own artist out and showed only "w/ …". That hid
 *    the thing the reader most often wants from a past date — who they opened
 *    for — and it made every row of a support slot start with "w/" and no lead.
 *    The cost is the artist's own name repeating down the page when they
 *    headline; the addendum accepted that trade. `ArtistShowsList.test.tsx`
 *    pins it, so a future reader can tell this was chosen rather than lost.
 *
 * There is no add-show affordance here, because a show is created against a
 * venue, not against a bill.
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

  // No `dedupArtistShows` here, and none in the past archive either.
  //
  // Two reasons, and only the second is unconditional. The duplicate class it
  // filtered — the same artist, at the same venue, at the same instant — is the
  // key of the PSY-576 unique index on `show_artists (artist_id, venue_id,
  // event_date)`, verified clean on stage and on prod. That index is PARTIAL
  // (`WHERE event_date IS NOT NULL AND venue_id IS NOT NULL`), so it does not
  // cover a show with no venue link, which this page does render ("Venue TBA").
  // Duplicates in that residual class would now reach the reader.
  //
  // The unconditional reason is that filtering rows after the fact is actively
  // WRONG in a server-paged list, residual class or not: it would render fewer
  // rows than the pager's own "Showing 51-100 of 161" claims, while `total` and
  // the year histogram still count what the page dropped. A client-side filter
  // cannot be the answer here; closing the residual class belongs in the index
  // or the ingest path.
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
