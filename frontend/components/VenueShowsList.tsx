'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Loader2, Calendar, History, Plus } from 'lucide-react'
import { useVenueShows, type TimeFilter } from '@/lib/hooks/useVenues'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { VenueShow } from '@/lib/types/venue'
import {
  formatDateInTimezone,
  formatDateWithYearInTimezone,
  formatTimeInTimezone,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms/ShowForm'

interface VenueShowsListProps {
  venueId: number
  venueName: string
  venueCity: string
  venueState: string
  venueAddress?: string | null
  venueVerified?: boolean
  className?: string
  onShowAdded?: () => void
}

/**
 * Format a date string to "Mon, Dec 1" format in venue timezone
 * If includeYear is true, formats as "Mon, Dec 1, 2024"
 */
function formatDate(dateString: string, state: string, includeYear = false): string {
  const timezone = getTimezoneForState(state)
  return includeYear
    ? formatDateWithYearInTimezone(dateString, timezone)
    : formatDateInTimezone(dateString, timezone)
}

/**
 * Format a date string to "7:30 PM" format in venue timezone
 */
function formatTime(dateString: string, state: string): string {
  const timezone = getTimezoneForState(state)
  return formatTimeInTimezone(dateString, timezone)
}

/**
 * Format price as $XX.XX
 */
function formatPrice(price: number): string {
  return `$${price.toFixed(2)}`
}

interface ShowItemProps {
  show: VenueShow
  state: string
  isPastShow?: boolean
}

function ShowItem({ show, state, isPastShow = false }: ShowItemProps) {
  return (
    <div className="py-3 border-b border-border/30 last:border-b-0">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium text-primary">
            {formatDate(show.event_date, state, isPastShow)}
          </div>
          <div className="text-base font-semibold">
            {show.artists.map((artist, index) => (
              <span key={artist.id}>
                {index > 0 && (
                  <span className="text-muted-foreground/60 font-normal">
                    {' '}
                    &bull;{' '}
                  </span>
                )}
                <Link
                  href={`/artists/${artist.id}`}
                  className="hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors"
                >
                  {artist.name}
                </Link>
              </span>
            ))}
            {show.artists.length === 0 && 'TBA'}
          </div>
        </div>
        <div className="text-right text-sm text-muted-foreground shrink-0">
          <div>{formatTime(show.event_date, state)}</div>
          {show.price != null && <div>{formatPrice(show.price)}</div>}
        </div>
      </div>
    </div>
  )
}

interface ShowsTabContentProps {
  venueId: number
  venueState: string
  timeFilter: TimeFilter
  enabled: boolean
}

function ShowsTabContent({
  venueId,
  venueState,
  timeFilter,
  enabled,
}: ShowsTabContentProps) {
  const { data, isLoading, error } = useVenueShows({
    venueId,
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
    <div className="divide-y divide-border/30">
      {data.shows.map(show => (
        <ShowItem key={show.id} show={show} state={venueState} isPastShow={isPastShow} />
      ))}
    </div>
  )
}

export function VenueShowsList({
  venueId,
  venueName,
  venueCity,
  venueState,
  venueAddress,
  venueVerified,
  className,
  onShowAdded,
}: VenueShowsListProps) {
  const [activeTab, setActiveTab] = useState<TimeFilter>('upcoming')
  const [isAddingShow, setIsAddingShow] = useState(false)
  const { isAuthenticated } = useAuthContext()

  return (
    <div className={className}>
      <Tabs
        value={activeTab}
        onValueChange={value => setActiveTab(value as TimeFilter)}
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
            venueId={venueId}
            venueState={venueState}
            timeFilter="upcoming"
            enabled={activeTab === 'upcoming'}
          />
        </TabsContent>

        <TabsContent value="past" className="mt-4">
          <ShowsTabContent
            venueId={venueId}
            venueState={venueState}
            timeFilter="past"
            enabled={activeTab === 'past'}
          />
        </TabsContent>
      </Tabs>

      {/* Add Show Section */}
      {isAuthenticated && (
        <div className="mt-6 pt-4 border-t border-border/50">
          {isAddingShow ? (
            <ShowForm
              mode="create"
              prefilledVenue={{
                id: venueId,
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
