'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Loader2, Calendar, History } from 'lucide-react'
import { useArtistShows } from '@/lib/hooks/useArtists'
import type { ArtistShow, ArtistTimeFilter } from '@/lib/types/artist'
import {
  formatDateInTimezone,
  formatDateWithYearInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'

interface ArtistShowsListProps {
  artistId: number
  artistName: string
  className?: string
}

/**
 * Format a date string to "Mon, Dec 1" format in venue timezone
 * If includeYear is true, formats as "Mon, Dec 1, 2024"
 */
function formatDate(dateString: string, state?: string | null, includeYear = false): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return includeYear
    ? formatDateWithYearInTimezone(dateString, timezone)
    : formatDateInTimezone(dateString, timezone)
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

interface ShowItemProps {
  show: ArtistShow
  currentArtistId: number
  isPastShow?: boolean
}

function ShowItem({ show, currentArtistId, isPastShow = false }: ShowItemProps) {
  const venueState = show.venue?.state || 'AZ'

  // Filter out the current artist from the "with" list
  const otherArtists = show.artists.filter(a => a.id !== currentArtistId)

  return (
    <div className="py-4 border-b border-border/30 last:border-b-0">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          {/* Date */}
          <div className="text-sm font-medium text-primary">
            {formatDate(show.event_date, venueState, isPastShow)}
          </div>

          {/* Venue */}
          {show.venue && (
            <div className="mt-1">
              {show.venue.slug ? (
                <Link
                  href={`/venues/${show.venue.slug}`}
                  className="font-semibold hover:text-primary transition-colors"
                >
                  {show.venue.name}
                </Link>
              ) : (
                <span className="font-semibold">{show.venue.name}</span>
              )}
              <span className="text-muted-foreground">
                {' '}
                &middot; {show.venue.city}, {show.venue.state}
              </span>
            </div>
          )}

          {/* Other artists on the bill */}
          {otherArtists.length > 0 && (
            <div className="text-sm text-muted-foreground mt-1">
              w/{' '}
              {otherArtists.map((artist, index) => (
                <span key={artist.id}>
                  {index > 0 && ', '}
                  {artist.slug ? (
                    <Link
                      href={`/artists/${artist.slug}`}
                      className="hover:text-foreground transition-colors"
                    >
                      {artist.name}
                    </Link>
                  ) : (
                    <span>{artist.name}</span>
                  )}
                </span>
              ))}
            </div>
          )}
        </div>

        {/* Time and Price */}
        <div className="text-right text-sm text-muted-foreground shrink-0">
          <div>{formatTime(show.event_date, venueState)}</div>
          {show.price != null && <div>{formatPrice(show.price)}</div>}
        </div>
      </div>
    </div>
  )
}

interface ShowsTabContentProps {
  artistId: number
  timeFilter: ArtistTimeFilter
  enabled: boolean
}

function ShowsTabContent({ artistId, timeFilter, enabled }: ShowsTabContentProps) {
  const { data, isLoading, error } = useArtistShows({
    artistId,
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    timeFilter,
    enabled,
    limit: 50,
  })

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="py-8 text-center text-sm text-destructive">
        Failed to load shows
      </div>
    )
  }

  if (!data?.shows || data.shows.length === 0) {
    return (
      <div className="py-8 text-center text-sm text-muted-foreground">
        {timeFilter === 'past' ? 'No past shows' : 'No upcoming shows'}
      </div>
    )
  }

  const isPastShow = timeFilter === 'past'

  return (
    <div>
      {data.shows.map(show => (
        <ShowItem key={show.id} show={show} currentArtistId={artistId} isPastShow={isPastShow} />
      ))}
      {data.total > data.shows.length && (
        <div className="text-center text-sm text-muted-foreground pt-4">
          Showing {data.shows.length} of {data.total} shows
        </div>
      )}
    </div>
  )
}

export function ArtistShowsList({
  artistId,
  artistName,
  className,
}: ArtistShowsListProps) {
  const [activeTab, setActiveTab] = useState<ArtistTimeFilter>('upcoming')

  return (
    <div className={className}>
      <Tabs
        value={activeTab}
        onValueChange={value => setActiveTab(value as ArtistTimeFilter)}
      >
        <TabsList>
          <TabsTrigger value="upcoming" className="gap-2">
            <Calendar className="h-4 w-4" />
            Upcoming
          </TabsTrigger>
          <TabsTrigger value="past" className="gap-2">
            <History className="h-4 w-4" />
            Past Shows
          </TabsTrigger>
        </TabsList>

        <TabsContent value="upcoming" className="mt-4">
          <ShowsTabContent
            artistId={artistId}
            timeFilter="upcoming"
            enabled={activeTab === 'upcoming'}
          />
        </TabsContent>

        <TabsContent value="past" className="mt-4">
          <ShowsTabContent
            artistId={artistId}
            timeFilter="past"
            enabled={activeTab === 'past'}
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}
