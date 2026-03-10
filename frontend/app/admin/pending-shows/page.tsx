'use client'

import { useState, useCallback, useEffect } from 'react'
import { Clock, Loader2, Inbox, XCircle, CheckCircle, Filter } from 'lucide-react'
import {
  usePendingShows,
  useRejectedShows,
  useBatchApproveShows,
  useBatchRejectShows,
} from '@/lib/hooks/admin/useAdminShows'
import { PendingShowCard, RejectedShowCard } from '@/components/admin'
import { BatchRejectDialog } from '@/components/admin/BatchRejectDialog'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import type { ShowResponse } from '@/lib/types/show'

type ShowView = 'pending' | 'rejected'

export default function PendingShowsPage() {
  const [view, setView] = useState<ShowView>('pending')
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [sourceFilter, setSourceFilter] = useState<string>('')
  const [venueFilter, setVenueFilter] = useState<string>('')
  const [showBatchRejectDialog, setShowBatchRejectDialog] = useState(false)
  const [quickRejectIds, setQuickRejectIds] = useState<number[]>([])
  const [showQuickRejectDialog, setShowQuickRejectDialog] = useState(false)

  const { user } = useAuthContext()
  const isAdmin = !!user?.is_admin
  const {
    data: pendingData,
    isLoading: pendingLoading,
    error: pendingError,
  } = usePendingShows({
    enabled: isAdmin,
    source: sourceFilter || undefined,
  })
  const {
    data: rejectedData,
    isLoading: rejectedLoading,
    error: rejectedError,
  } = useRejectedShows({ enabled: isAdmin })

  const batchApproveMutation = useBatchApproveShows()
  const batchRejectMutation = useBatchRejectShows()

  const isLoading = view === 'pending' ? pendingLoading : rejectedLoading
  const error = view === 'pending' ? pendingError : rejectedError

  // Filter shows client-side by venue name
  const filteredShows = pendingData?.shows.filter(show => {
    if (!venueFilter) return true
    return show.venues.some(v =>
      v.name.toLowerCase().includes(venueFilter.toLowerCase())
    )
  }) ?? []

  // Get unique venue names for the filter
  const venueNames = Array.from(
    new Set(
      (pendingData?.shows ?? []).flatMap(s => s.venues.map(v => v.name))
    )
  ).sort()

  // Get unique sources for the filter
  const sources = Array.from(
    new Set(
      (pendingData?.shows ?? [])
        .map(s => s.source)
        .filter((s): s is string => !!s)
    )
  ).sort()

  // Selection helpers
  const toggleSelect = useCallback((id: number, selected: boolean) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (selected) {
        next.add(id)
      } else {
        next.delete(id)
      }
      return next
    })
  }, [])

  const selectAll = useCallback(() => {
    setSelectedIds(new Set(filteredShows.map(s => s.id)))
  }, [filteredShows])

  const selectNone = useCallback(() => {
    setSelectedIds(new Set())
  }, [])

  const allSelected = filteredShows.length > 0 && selectedIds.size === filteredShows.length
  const someSelected = selectedIds.size > 0
  const selectedCount = selectedIds.size

  // Clear selection when filters change
  useEffect(() => {
    setSelectedIds(new Set())
  }, [sourceFilter, venueFilter])

  // Batch approve
  const handleBatchApprove = useCallback(() => {
    if (selectedIds.size === 0) return
    batchApproveMutation.mutate(Array.from(selectedIds), {
      onSuccess: () => setSelectedIds(new Set()),
    })
  }, [selectedIds, batchApproveMutation])

  // Quick reject (not music) for a single show
  const handleQuickRejectNotMusic = useCallback((showId: number) => {
    setQuickRejectIds([showId])
    setShowQuickRejectDialog(true)
  }, [])

  // Keyboard shortcuts
  useEffect(() => {
    if (view !== 'pending') return

    const handler = (e: KeyboardEvent) => {
      // Don't trigger in inputs/textareas
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return

      if (e.key === 'a' && !e.metaKey && !e.ctrlKey && selectedCount > 0) {
        e.preventDefault()
        handleBatchApprove()
      } else if (e.key === 'r' && !e.metaKey && !e.ctrlKey && selectedCount > 0) {
        e.preventDefault()
        setShowBatchRejectDialog(true)
      }
    }

    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [view, selectedCount, handleBatchApprove])

  return (
    <div className="space-y-4">
      <div className="mb-6">
        <h2 className="text-xl font-semibold flex items-center gap-2">
          <Clock className="h-5 w-5" />
          Show Review
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          Review and approve or reject pending shows.
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
              {/* Filter bar */}
              {(sources.length > 1 || venueNames.length > 1) && (
                <div className="flex flex-wrap items-center gap-3 rounded-lg border border-border bg-card/50 p-3">
                  <Filter className="h-4 w-4 text-muted-foreground" />
                  {sources.length > 1 && (
                    <select
                      value={sourceFilter}
                      onChange={e => setSourceFilter(e.target.value)}
                      className="h-8 rounded-md border border-input bg-transparent px-2 py-1 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                    >
                      <option value="">All sources</option>
                      {sources.map(s => (
                        <option key={s} value={s}>
                          {s === 'discovery' ? 'Discovery' : s === 'user' ? 'User submitted' : s}
                        </option>
                      ))}
                    </select>
                  )}
                  {venueNames.length > 1 && (
                    <select
                      value={venueFilter}
                      onChange={e => setVenueFilter(e.target.value)}
                      className="h-8 rounded-md border border-input bg-transparent px-2 py-1 text-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                    >
                      <option value="">All venues</option>
                      {venueNames.map(v => (
                        <option key={v} value={v}>
                          {v}
                        </option>
                      ))}
                    </select>
                  )}
                  {(sourceFilter || venueFilter) && (
                    <button
                      onClick={() => {
                        setSourceFilter('')
                        setVenueFilter('')
                      }}
                      className="text-xs text-muted-foreground hover:text-foreground underline"
                    >
                      Clear filters
                    </button>
                  )}
                </div>
              )}

              {/* Batch action bar */}
              <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                  <Checkbox
                    checked={allSelected}
                    onCheckedChange={checked => {
                      if (checked) selectAll()
                      else selectNone()
                    }}
                    aria-label="Select all shows"
                  />
                  <p className="text-sm text-muted-foreground">
                    {someSelected
                      ? `${selectedCount} of ${filteredShows.length} selected`
                      : `${filteredShows.length} pending ${filteredShows.length === 1 ? 'show' : 'shows'}`}
                  </p>
                </div>

                {someSelected && (
                  <div className="flex items-center gap-2">
                    <Button
                      size="sm"
                      className="gap-1.5"
                      onClick={handleBatchApprove}
                      disabled={batchApproveMutation.isPending}
                    >
                      {batchApproveMutation.isPending ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <CheckCircle className="h-3.5 w-3.5" />
                      )}
                      Approve ({selectedCount})
                      <kbd className="ml-1 hidden sm:inline rounded border border-border bg-muted px-1 py-0.5 text-[10px] font-mono text-muted-foreground">
                        A
                      </kbd>
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="gap-1.5 text-destructive hover:text-destructive"
                      onClick={() => setShowBatchRejectDialog(true)}
                    >
                      <XCircle className="h-3.5 w-3.5" />
                      Reject ({selectedCount})
                      <kbd className="ml-1 hidden sm:inline rounded border border-border bg-muted px-1 py-0.5 text-[10px] font-mono text-muted-foreground">
                        R
                      </kbd>
                    </Button>
                  </div>
                )}
              </div>

              {/* Show cards */}
              {filteredShows.map(show => (
                <PendingShowCard
                  key={show.id}
                  show={show}
                  selected={selectedIds.has(show.id)}
                  onSelectChange={checked => toggleSelect(show.id, checked)}
                  onQuickRejectNotMusic={handleQuickRejectNotMusic}
                />
              ))}

              {filteredShows.length === 0 && (pendingData?.shows.length ?? 0) > 0 && (
                <div className="rounded-lg border border-border bg-card/50 p-6 text-center">
                  <p className="text-sm text-muted-foreground">
                    No shows match the current filters.
                  </p>
                </div>
              )}
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
                {rejectedData!.total} rejected{' '}
                {rejectedData!.total === 1 ? 'show' : 'shows'}
              </p>
              {rejectedData!.shows.map(show => (
                <RejectedShowCard key={show.id} show={show} />
              ))}
            </div>
          )}
        </>
      )}

      {/* Batch reject dialog */}
      <BatchRejectDialog
        showIds={Array.from(selectedIds)}
        open={showBatchRejectDialog}
        onOpenChange={setShowBatchRejectDialog}
        onSuccess={() => setSelectedIds(new Set())}
      />

      {/* Quick reject (not music) dialog */}
      <BatchRejectDialog
        showIds={quickRejectIds}
        open={showQuickRejectDialog}
        onOpenChange={setShowQuickRejectDialog}
        defaultCategory="non_music"
        defaultReason="Not a music event"
      />
    </div>
  )
}
