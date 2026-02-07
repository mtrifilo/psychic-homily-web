'use client'

import { useState } from 'react'
import { Clock, Loader2, Inbox, XCircle } from 'lucide-react'
import { usePendingShows, useRejectedShows } from '@/lib/hooks/useAdminShows'
import { PendingShowCard, RejectedShowCard } from '@/components/admin'

type ShowView = 'pending' | 'rejected'

export default function PendingShowsPage() {
  const [view, setView] = useState<ShowView>('pending')
  const { data: pendingData, isLoading: pendingLoading, error: pendingError } = usePendingShows()
  const { data: rejectedData, isLoading: rejectedLoading, error: rejectedError } = useRejectedShows()

  const isLoading = view === 'pending' ? pendingLoading : rejectedLoading
  const error = view === 'pending' ? pendingError : rejectedError

  return (
    <div className="space-y-4">
      <div className="mb-6">
        <h2 className="text-xl font-semibold flex items-center gap-2">
          <Clock className="h-5 w-5" />
          Show Review
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          Review and approve or reject user-submitted shows.
        </p>
      </div>

      {/* View toggle */}
      <div className="flex gap-1 rounded-lg border border-border bg-muted/50 p-1 w-fit">
        <button
          onClick={() => setView('pending')}
          className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
            view === 'pending'
              ? 'bg-background text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <Clock className="h-3.5 w-3.5" />
          Pending
          {pendingData?.total !== undefined && pendingData.total > 0 && (
            <span className="rounded-full bg-amber-500 px-1.5 py-0.5 text-xs font-medium text-white leading-none">
              {pendingData.total}
            </span>
          )}
        </button>
        <button
          onClick={() => setView('rejected')}
          className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
            view === 'rejected'
              ? 'bg-background text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <XCircle className="h-3.5 w-3.5" />
          Rejected
          {rejectedData?.total !== undefined && rejectedData.total > 0 && (
            <span className="rounded-full bg-muted-foreground/20 px-1.5 py-0.5 text-xs font-medium leading-none">
              {rejectedData.total}
            </span>
          )}
        </button>
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center text-destructive">
          Failed to load shows. Please try again.
        </div>
      )}

      {/* Pending view */}
      {view === 'pending' && !pendingLoading && !pendingError && (
        <>
          {pendingData?.shows.length === 0 ? (
            <div className="rounded-lg border border-border bg-card/50 p-8 text-center">
              <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
                <Inbox className="h-6 w-6 text-muted-foreground" />
              </div>
              <h3 className="font-medium mb-1">No Pending Shows</h3>
              <p className="text-sm text-muted-foreground">
                All show submissions have been reviewed. Check back later for new
                submissions.
              </p>
            </div>
          ) : (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                {pendingData!.total} pending {pendingData!.total === 1 ? 'show' : 'shows'} awaiting review
              </p>
              {pendingData!.shows.map(show => (
                <PendingShowCard key={show.id} show={show} />
              ))}
            </div>
          )}
        </>
      )}

      {/* Rejected view */}
      {view === 'rejected' && !rejectedLoading && !rejectedError && (
        <>
          {rejectedData?.shows.length === 0 ? (
            <div className="rounded-lg border border-border bg-card/50 p-8 text-center">
              <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
                <Inbox className="h-6 w-6 text-muted-foreground" />
              </div>
              <h3 className="font-medium mb-1">No Rejected Shows</h3>
              <p className="text-sm text-muted-foreground">
                No shows have been rejected yet.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              <p className="text-sm text-muted-foreground">
                {rejectedData!.total} rejected {rejectedData!.total === 1 ? 'show' : 'shows'}
              </p>
              {rejectedData!.shows.map(show => (
                <RejectedShowCard key={show.id} show={show} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
