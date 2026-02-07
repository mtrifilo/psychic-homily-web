'use client'

import { Clock, Loader2, Inbox } from 'lucide-react'
import { usePendingShows } from '@/lib/hooks/useAdminShows'
import { PendingShowCard } from '@/components/admin'

export default function PendingShowsPage() {
  const { data, isLoading, error } = usePendingShows()

  return (
    <div className="space-y-4">
      <div className="mb-6">
        <h2 className="text-xl font-semibold flex items-center gap-2">
          <Clock className="h-5 w-5" />
          Pending Shows
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          Review and approve or reject user-submitted shows.
        </p>
      </div>

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
            <Inbox className="h-6 w-6 text-muted-foreground" />
          </div>
          <h3 className="font-medium mb-1">No Pending Shows</h3>
          <p className="text-sm text-muted-foreground">
            All show submissions have been reviewed. Check back later for new
            submissions.
          </p>
        </div>
      )}

      {!isLoading && !error && data?.shows && data.shows.length > 0 && (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {data.total} pending {data.total === 1 ? 'show' : 'shows'} awaiting
            review
          </p>
          {data.shows.map(show => (
            <PendingShowCard key={show.id} show={show} />
          ))}
        </div>
      )}
    </div>
  )
}
