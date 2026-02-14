'use client'

import { useMemo } from 'react'
import { Loader2, Flag, Inbox } from 'lucide-react'
import { usePendingReports } from '@/lib/hooks/useAdminReports'
import { usePendingArtistReports } from '@/lib/hooks/useAdminArtistReports'
import { ShowReportCard, ArtistReportCard } from '@/components/admin'
import type { ShowReportResponse } from '@/lib/types/show'
import type { ArtistReportResponse } from '@/lib/types/artist'

type MergedReport =
  | { type: 'show'; report: ShowReportResponse }
  | { type: 'artist'; report: ArtistReportResponse }

export default function AdminReportsPage() {
  const {
    data: showReportsData,
    isLoading: showReportsLoading,
    error: showReportsError,
  } = usePendingReports()
  const {
    data: artistReportsData,
    isLoading: artistReportsLoading,
    error: artistReportsError,
  } = usePendingArtistReports()

  const isLoading = showReportsLoading || artistReportsLoading
  const error = showReportsError || artistReportsError

  // Merge and sort all reports by created_at DESC
  const mergedReports = useMemo<MergedReport[]>(() => {
    const showReports: MergedReport[] = (
      showReportsData?.reports || []
    ).map(r => ({ type: 'show' as const, report: r }))
    const artistReports: MergedReport[] = (
      artistReportsData?.reports || []
    ).map(r => ({ type: 'artist' as const, report: r }))

    return [...showReports, ...artistReports].sort(
      (a, b) =>
        new Date(b.report.created_at).getTime() -
        new Date(a.report.created_at).getTime()
    )
  }, [showReportsData, artistReportsData])

  const totalCount =
    (showReportsData?.total || 0) + (artistReportsData?.total || 0)

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center">
        <p className="text-destructive">
          {error instanceof Error
            ? error.message
            : 'Failed to load reports. Please try again.'}
        </p>
      </div>
    )
  }

  if (mergedReports.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
          <Inbox className="h-8 w-8 text-muted-foreground" />
        </div>
        <h3 className="text-lg font-medium mb-1">No Pending Reports</h3>
        <p className="text-sm text-muted-foreground max-w-sm">
          All user reports have been reviewed. New reports will appear here when
          users flag shows or artists with issues.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Header with count */}
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Flag className="h-4 w-4" />
        <span>
          {totalCount} pending report{totalCount !== 1 ? 's' : ''} requiring
          review
        </span>
      </div>

      {/* Reports Grid */}
      <div className="grid gap-4 md:grid-cols-2">
        {mergedReports.map(item =>
          item.type === 'show' ? (
            <ShowReportCard key={`show-${item.report.id}`} report={item.report} />
          ) : (
            <ArtistReportCard
              key={`artist-${item.report.id}`}
              report={item.report}
            />
          )
        )}
      </div>
    </div>
  )
}
