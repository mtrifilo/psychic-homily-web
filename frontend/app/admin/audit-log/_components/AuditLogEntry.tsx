'use client'

import {
  CheckCircle,
  XCircle,
  BadgeCheck,
  FileEdit,
  Flag,
  ShieldAlert,
} from 'lucide-react'
import type { AuditLogEntry as AuditLogEntryType } from '@/lib/types/audit'
import { Badge } from '@/components/ui/badge'
import { formatTimestamp } from '@/lib/utils/formatters'

interface AuditLogEntryProps {
  entry: AuditLogEntryType
}

const ACTION_CONFIG: Record<
  string,
  { label: string; icon: React.ReactNode; color: string }
> = {
  approve_show: {
    label: 'Approved show',
    icon: <CheckCircle className="h-4 w-4" />,
    color: 'text-green-600 dark:text-green-400',
  },
  reject_show: {
    label: 'Rejected show',
    icon: <XCircle className="h-4 w-4" />,
    color: 'text-red-600 dark:text-red-400',
  },
  verify_venue: {
    label: 'Verified venue',
    icon: <BadgeCheck className="h-4 w-4" />,
    color: 'text-blue-600 dark:text-blue-400',
  },
  approve_venue_edit: {
    label: 'Approved venue edit',
    icon: <FileEdit className="h-4 w-4" />,
    color: 'text-green-600 dark:text-green-400',
  },
  reject_venue_edit: {
    label: 'Rejected venue edit',
    icon: <XCircle className="h-4 w-4" />,
    color: 'text-red-600 dark:text-red-400',
  },
  dismiss_report: {
    label: 'Dismissed report',
    icon: <XCircle className="h-4 w-4" />,
    color: 'text-muted-foreground',
  },
  resolve_report: {
    label: 'Resolved report',
    icon: <CheckCircle className="h-4 w-4" />,
    color: 'text-green-600 dark:text-green-400',
  },
  resolve_report_with_flag: {
    label: 'Resolved report (flagged show)',
    icon: <Flag className="h-4 w-4" />,
    color: 'text-amber-600 dark:text-amber-400',
  },
}

function getActionConfig(action: string) {
  return (
    ACTION_CONFIG[action] ?? {
      label: action,
      icon: <ShieldAlert className="h-4 w-4" />,
      color: 'text-muted-foreground',
    }
  )
}

function getEntityLabel(entityType: string): string {
  switch (entityType) {
    case 'show':
      return 'Show'
    case 'venue':
      return 'Venue'
    case 'venue_edit':
      return 'Venue Edit'
    case 'show_report':
      return 'Report'
    default:
      return entityType
  }
}

export function AuditLogEntry({ entry }: AuditLogEntryProps) {
  const config = getActionConfig(entry.action)

  return (
    <div className="flex items-start gap-3 rounded-lg border bg-card p-3">
      <div className={`mt-0.5 shrink-0 ${config.color}`}>{config.icon}</div>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 flex-wrap">
          <span className="text-sm font-medium">{config.label}</span>
          <Badge variant="outline" className="text-xs">
            {getEntityLabel(entry.entity_type)} #{entry.entity_id}
          </Badge>
        </div>
        <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
          <span>{entry.actor_email || 'System'}</span>
          <span>&middot;</span>
          <span>{formatTimestamp(entry.created_at)}</span>
        </div>
        {entry.metadata && Object.keys(entry.metadata).length > 0 && (
          <div className="mt-1.5 text-xs text-muted-foreground">
            {entry.metadata.reason && (
              <span>
                Reason: {String(entry.metadata.reason)}
              </span>
            )}
            {entry.metadata.notes && (
              <span>
                Notes: {String(entry.metadata.notes)}
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
