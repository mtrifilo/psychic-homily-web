'use client'

import { useState } from 'react'
import { Shield, Music, MapPin, Loader2 } from 'lucide-react'
import { usePendingShows } from '@/lib/hooks/useAdminShows'
import { usePendingVenueEdits } from '@/lib/hooks/useAdminVenueEdits'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PendingShowCard } from '@/components/admin/PendingShowCard'
import VenueEditsPage from './venue-edits/page'

export default function AdminPage() {
  const [activeTab, setActiveTab] = useState('pending-shows')
  const { data, isLoading, error } = usePendingShows()
  const {
    data: venueEditsData,
    isLoading: venueEditsLoading,
    error: venueEditsError,
  } = usePendingVenueEdits()

  return (
    <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
      <div className="mx-auto max-w-4xl">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center gap-3 mb-2">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
              <Shield className="h-5 w-5 text-primary" />
            </div>
            <h1 className="text-2xl font-bold tracking-tight">Admin Console</h1>
          </div>
          <p className="text-sm text-muted-foreground">
            Manage pending submissions, venues, and users.
          </p>
        </div>

        {/* Tabs */}
        <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
          <TabsList className="mb-6">
            <TabsTrigger value="pending-shows" className="gap-2">
              <Music className="h-4 w-4" />
              Pending Shows
              {data?.total !== undefined && data.total > 0 && (
                <span className="ml-1 rounded-full bg-amber-500 px-2 py-0.5 text-xs font-medium text-white">
                  {data.total}
                </span>
              )}
            </TabsTrigger>
            <TabsTrigger value="pending-venue-edits" className="gap-2">
              <MapPin className="h-4 w-4" />
              Venue Edits
              {venueEditsData?.total !== undefined &&
                venueEditsData.total > 0 && (
                  <span className="ml-1 rounded-full bg-amber-500 px-2 py-0.5 text-xs font-medium text-white">
                    {venueEditsData.total}
                  </span>
                )}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="pending-shows" className="space-y-4">
            {isLoading && (
              <div className="flex items-center justify-center py-12">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            )}

            {error && (
              <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-destructive">
                Failed to load pending shows. Please try again.
              </div>
            )}

            {!isLoading && !error && data?.shows.length === 0 && (
              <div className="rounded-lg border border-border bg-card/50 p-8 text-center">
                <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
                  <Music className="h-6 w-6 text-muted-foreground" />
                </div>
                <h3 className="font-medium mb-1">No Pending Shows</h3>
                <p className="text-sm text-muted-foreground">
                  All show submissions have been reviewed. Check back later for
                  new submissions.
                </p>
              </div>
            )}

            {!isLoading && !error && data?.shows && data.shows.length > 0 && (
              <div className="space-y-4">
                <p className="text-sm text-muted-foreground">
                  {data.total} pending{' '}
                  {data.total === 1 ? 'submission' : 'submissions'} awaiting
                  review
                </p>
                {data.shows.map(show => (
                  <PendingShowCard key={show.id} show={show} />
                ))}
              </div>
            )}
          </TabsContent>

          <TabsContent value="pending-venue-edits" className="space-y-4">
            <VenueEditsPage />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
