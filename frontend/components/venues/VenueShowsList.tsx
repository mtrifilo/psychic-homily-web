'use client'

import { useState } from 'react'
import { Loader2, Calendar, History, Plus } from 'lucide-react'
import { useVenueShows, type TimeFilter } from '@/lib/hooks/useVenues'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms/ShowForm'
import { CompactShowRow } from '@/components/shows/CompactShowRow'
import { SHOW_LIST_FEATURE_POLICY } from '@/components/shows/showListFeaturePolicy'

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
    <div>
      {data.shows.map(show => (
        <CompactShowRow
          key={show.id}
          show={show}
          state={venueState}
          isPastShow={isPastShow}
          showDetailsLink={SHOW_LIST_FEATURE_POLICY.context.showDetailsLink}
        />
      ))}
    </div>
  )
}

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
