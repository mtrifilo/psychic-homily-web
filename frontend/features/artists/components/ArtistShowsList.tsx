'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import {
  BracketLink,
  SectionHeader,
  DenseTable,
} from '@/components/shared'
import { NotifyMeButton } from '@/features/notifications'
import { dedupArtistShows } from '@/features/shows'
import { formatShowDate, formatShowTime } from '@/lib/utils/formatters'
import { useArtistShows } from '../hooks/useArtists'
import type { ArtistShow } from '../types'

interface ArtistShowsListProps {
  artistId: number
  artistName: string
  className?: string
}

const TIMEZONE = Intl.DateTimeFormat().resolvedOptions().timeZone

function ShowsLoader() {
  return (
    <div className="flex justify-center py-6">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  )
}

function ShowRow({ show, artistId }: { show: ArtistShow; artistId: number }) {
  const state = show.venue?.state ?? null
  const otherArtists = show.artists.filter(a => a.id !== artistId)
  const detailsHref = `/shows/${show.slug || show.id}`
  return (
    <tr>
      <td className="whitespace-nowrap">
        <Link
          href={detailsHref}
          className="hover:text-primary hover:underline underline-offset-2"
        >
          {formatShowDate(show.event_date, state, false, show.venue?.timezone)}
        </Link>
      </td>
      <td>
        {show.venue ? (
          <span>
            <Link
              href={`/venues/${show.venue.slug}`}
              className="font-medium hover:text-primary hover:underline"
            >
              {show.venue.name}
            </Link>
            <span className="text-muted-foreground">
              {' · '}
              {[show.venue.city, show.venue.state].filter(Boolean).join(', ')}
            </span>
          </span>
        ) : (
          <span className="text-muted-foreground">Venue TBA</span>
        )}
      </td>
      <td className="text-muted-foreground">
        {otherArtists.length > 0 ? (
          <span>
            <span className="italic">w/</span>{' '}
            {otherArtists.map((a, i) => (
              <span key={a.id}>
                {i > 0 && ', '}
                <Link
                  href={`/artists/${a.slug}`}
                  className="hover:text-foreground hover:underline"
                >
                  {a.name}
                </Link>
              </span>
            ))}
          </span>
        ) : (
          '—'
        )}
      </td>
      <td className="text-right whitespace-nowrap text-muted-foreground">
        {formatShowTime(show.event_date, state, show.venue?.timezone)}
      </td>
    </tr>
  )
}

function ShowsTable({
  shows,
  total,
  artistId,
  isPast,
}: {
  shows: ArtistShow[]
  total: number
  artistId: number
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
            <th>Venue · Location</th>
            <th>Bill</th>
            <th className="text-right">Time</th>
          </tr>
        </thead>
        <tbody>
          {shows.map(show => (
            <ShowRow key={show.id} show={show} artistId={artistId} />
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
 * Artist shows — two density-first sections (PSY-644).
 *
 * - **Upcoming shows**: always rendered. Empty state shows an inline
 *   `[Notify me]` affordance because shows are PH's primary signal —
 *   landing on an artist with zero upcoming shows should let the user
 *   subscribe immediately, not get a bare "no shows yet" message.
 * - **Past shows**: separate section, collapsed by default with a
 *   `[Show]`/`[Hide]` toggle. The whole section hides when the artist has
 *   zero past shows. Fetch fires eagerly on page load so the empty-section
 *   hide can happen without an expand round-trip.
 *
 * Replaces the pre-PSY-644 internal Upcoming/Past Radix tabs.
 */
export function ArtistShowsList({
  artistId,
  artistName,
  className,
}: ArtistShowsListProps) {
  const [pastOpen, setPastOpen] = useState(false)
  const upcoming = useArtistShows({
    artistId,
    timezone: TIMEZONE,
    timeFilter: 'upcoming',
    enabled: true,
    limit: 50,
  })
  const past = useArtistShows({
    artistId,
    timezone: TIMEZONE,
    timeFilter: 'past',
    enabled: true,
    limit: 50,
  })

  const upcomingShows = upcoming.data?.shows
    ? dedupArtistShows(upcoming.data.shows)
    : []
  const pastShows = past.data?.shows ? dedupArtistShows(past.data.shows) : []

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
              entityType="artist"
              entityId={artistId}
              entityName={artistName}
              variant="bracket"
            />
          </div>
        ) : (
          <ShowsTable
            shows={upcomingShows}
            total={upcoming.data?.total ?? upcomingShows.length}
            artistId={artistId}
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
              artistId={artistId}
              isPast={true}
            />
          )}
        </section>
      )}
    </div>
  )
}
