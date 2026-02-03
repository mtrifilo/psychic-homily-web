'use client'

import { Loader2, Flag, Inbox } from 'lucide-react'
import { usePendingReports } from '@/lib/hooks/useAdminReports'
import { ShowReportCard } from '@/components/admin'

export default function AdminReportsPage() {
  const { data, isLoading, error } = usePendingReports()

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

  const reports = data?.reports || []

  if (reports.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
          <Inbox className="h-8 w-8 text-muted-foreground" />
        </div>
        <h3 className="text-lg font-medium mb-1">No Pending Reports</h3>
        <p className="text-sm text-muted-foreground max-w-sm">
          All user reports have been reviewed. New reports will appear here
          when users flag shows as cancelled, sold out, or inaccurate.
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
          {data?.total} pending report{data?.total !== 1 ? 's' : ''} requiring
          review
        </span>
      </div>

      {/* Reports Grid */}
      <div className="grid gap-4 md:grid-cols-2">
        {reports.map(report => (
          <ShowReportCard key={report.id} report={report} />
        ))}
      </div>
    </div>
  )
}
