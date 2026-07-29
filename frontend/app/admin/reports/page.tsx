'use client'

import { Loader2, Flag, Inbox } from 'lucide-react'
import { usePendingReports } from '@/lib/hooks/admin/useAdminReports'
import { AdminEmptyState } from '@/components/admin'
import { ShowReportCard } from '@/features/shows/admin'

/**
 * Pending SHOW reports.
 *
 * Every other entity type — artists included since PSY-1633 — reports through
 * the generic entity pipeline and is reviewed in /admin/moderation. Shows keep
 * a queue of their own because they keep a report table of their own, with a
 * card whose cancel / sold-out actions the generic report card has no concept
 * of.
 */
export default function AdminReportsPage() {
  const { data, isLoading, error } = usePendingReports()

  const reports = data?.reports || []
  const totalCount = data?.total || 0

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

  if (reports.length === 0) {
    return (
      <AdminEmptyState
        icon={Inbox}
        title="No Pending Reports"
        message="All show reports have been reviewed. Reports about artists, venues and other entities appear in the moderation queue."
      />
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
        {reports.map(report => (
          <ShowReportCard key={`show-${report.id}`} report={report} />
        ))}
      </div>
    </div>
  )
}
