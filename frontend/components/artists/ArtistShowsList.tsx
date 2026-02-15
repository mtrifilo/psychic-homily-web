'use client'

import { useState } from 'react'
import { Loader2, Calendar, History } from 'lucide-react'
import { useArtistShows } from '@/lib/hooks/useArtists'
import type { ArtistTimeFilter } from '@/lib/types/artist'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { CompactShowRow } from '@/components/shows/CompactShowRow'
import { SHOW_LIST_FEATURE_POLICY } from '@/components/shows/showListFeaturePolicy'

interface ArtistShowsListProps {
  artistId: number
  className?: string
}

interface ShowsTabContentProps {
  artistId: number
  timeFilter: ArtistTimeFilter
  enabled: boolean
}

function ShowsTabContent({
  artistId,
  timeFilter,
  enabled,
}: ShowsTabContentProps) {
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
      {data.shows.map(show => {
        const otherArtists = show.artists.filter(a => a.id !== artistId)
        return (
          <CompactShowRow
            key={show.id}
            show={show}
            state={show.venue?.state}
            isPastShow={isPastShow}
            primaryLine="venue"
            venue={show.venue}
            secondaryArtists={otherArtists}
            showDetailsLink={SHOW_LIST_FEATURE_POLICY.context.showDetailsLink}
          />
        )
      })}
      {data.total > data.shows.length && (
        <div className="text-center text-sm text-muted-foreground pt-4">
          Showing {data.shows.length} of {data.total} shows
        </div>
      )}
    </div>
  )
}

export function ArtistShowsList({ artistId, className }: ArtistShowsListProps) {
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
