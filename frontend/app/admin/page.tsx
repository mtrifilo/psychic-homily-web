'use client'

import { useState, useDeferredValue } from 'react'
import dynamic from 'next/dynamic'
import { Shield, Music, MapPin, Loader2, XCircle, Search, X, Upload, BadgeCheck } from 'lucide-react'
import { usePendingShows, useRejectedShows } from '@/lib/hooks/useAdminShows'
import { usePendingVenueEdits } from '@/lib/hooks/useAdminVenueEdits'
import { useUnverifiedVenues } from '@/lib/hooks/useAdminVenues'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { PendingShowCard } from '@/components/admin/PendingShowCard'
import { RejectedShowCard } from '@/components/admin/RejectedShowCard'

// Dynamic imports for heavy components - only loaded when their tab is active
const ShowImportPanel = dynamic(
  () => import('@/components/admin/ShowImportPanel').then(m => m.ShowImportPanel),
  {
    loading: () => (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    ),
  }
)

const VenueEditsPage = dynamic(() => import('./venue-edits/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

const UnverifiedVenuesPage = dynamic(() => import('./unverified-venues/page'), {
  loading: () => (
    <div className="flex items-center justify-center py-12">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  ),
})

export default function AdminPage() {
  const [activeTab, setActiveTab] = useState('pending-shows')
  const [rejectedSearch, setRejectedSearch] = useState('')
  const deferredRejectedSearch = useDeferredValue(rejectedSearch)

  const { data, isLoading, error } = usePendingShows()
  const {
    data: venueEditsData,
    isLoading: venueEditsLoading,
    error: venueEditsError,
  } = usePendingVenueEdits()
  const {
    data: rejectedData,
    isLoading: rejectedLoading,
    error: rejectedError,
  } = useRejectedShows({ search: deferredRejectedSearch || undefined })
  const {
    data: unverifiedVenuesData,
  } = useUnverifiedVenues()

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
            <TabsTrigger value="unverified-venues" className="gap-2">
              <BadgeCheck className="h-4 w-4" />
              Unverified Venues
              {unverifiedVenuesData?.total !== undefined &&
                unverifiedVenuesData.total > 0 && (
                  <span className="ml-1 rounded-full bg-orange-500 px-2 py-0.5 text-xs font-medium text-white">
                    {unverifiedVenuesData.total}
                  </span>
                )}
            </TabsTrigger>
            <TabsTrigger value="rejected-shows" className="gap-2">
              <XCircle className="h-4 w-4" />
              Rejected Shows
              {rejectedData?.total !== undefined && rejectedData.total > 0 && (
                <span className="ml-1 rounded-full bg-destructive px-2 py-0.5 text-xs font-medium text-white">
                  {rejectedData.total}
                </span>
              )}
            </TabsTrigger>
            <TabsTrigger value="import-show" className="gap-2">
              <Upload className="h-4 w-4" />
              Import Show
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

          <TabsContent value="unverified-venues" className="space-y-4">
            <UnverifiedVenuesPage />
          </TabsContent>

          <TabsContent value="rejected-shows" className="space-y-4">
            {/* Search Input */}
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search by title or rejection reason..."
                value={rejectedSearch}
                onChange={e => setRejectedSearch(e.target.value)}
                className="pl-9 pr-9"
              />
              {rejectedSearch && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="absolute right-1 top-1/2 h-7 w-7 -translate-y-1/2 p-0"
                  onClick={() => setRejectedSearch('')}
                >
                  <X className="h-4 w-4" />
                </Button>
              )}
            </div>

            {rejectedLoading && (
              <div className="flex items-center justify-center py-12">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            )}

            {rejectedError && (
              <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-destructive">
                Failed to load rejected shows. Please try again.
              </div>
            )}

            {!rejectedLoading &&
              !rejectedError &&
              rejectedData?.shows.length === 0 && (
                <div className="rounded-lg border border-border bg-card/50 p-8 text-center">
                  <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
                    <XCircle className="h-6 w-6 text-muted-foreground" />
                  </div>
                  <h3 className="font-medium mb-1">No Rejected Shows</h3>
                  <p className="text-sm text-muted-foreground">
                    {rejectedSearch
                      ? 'No rejected shows match your search.'
                      : 'No show submissions have been rejected yet.'}
                  </p>
                </div>
              )}

            {!rejectedLoading &&
              !rejectedError &&
              rejectedData?.shows &&
              rejectedData.shows.length > 0 && (
                <div className="space-y-4">
                  <p className="text-sm text-muted-foreground">
                    {rejectedData.total} rejected{' '}
                    {rejectedData.total === 1 ? 'submission' : 'submissions'}
                    {rejectedSearch && ' matching your search'}
                  </p>
                  {rejectedData.shows.map(show => (
                    <RejectedShowCard key={show.id} show={show} />
                  ))}
                </div>
              )}
          </TabsContent>

          <TabsContent value="import-show" className="space-y-4">
            <ShowImportPanel />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
