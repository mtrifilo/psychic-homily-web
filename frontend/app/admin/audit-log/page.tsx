'use client'

import { Loader2, ScrollText, Inbox } from 'lucide-react'
import { useAuditLogs } from '@/lib/hooks/useAdminAuditLogs'
import { AuditLogEntry } from '@/components/admin'

export default function AdminAuditLogPage() {
  const { data, isLoading, error } = useAuditLogs()

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
            : 'Failed to load audit logs. Please try again.'}
        </p>
      </div>
    )
  }

  const logs = data?.logs || []

  if (logs.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
          <Inbox className="h-8 w-8 text-muted-foreground" />
        </div>
        <h3 className="text-lg font-medium mb-1">No Audit Logs</h3>
        <p className="text-sm text-muted-foreground max-w-sm">
          Admin actions will be recorded here. Approve a show, verify a venue,
          or resolve a report to see entries appear.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <ScrollText className="h-4 w-4" />
        <span>
          {data?.total} audit log entr{data?.total !== 1 ? 'ies' : 'y'}
        </span>
      </div>

      <div className="space-y-2">
        {logs.map(entry => (
          <AuditLogEntry key={entry.id} entry={entry} />
        ))}
      </div>
    </div>
  )
}
