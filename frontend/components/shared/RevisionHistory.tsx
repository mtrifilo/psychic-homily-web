'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ChevronDown, ChevronRight, History, Loader2, RotateCcw } from 'lucide-react'
import { useEntityRevisions, useRollbackRevision } from '@/lib/hooks/common/useRevisions'
import type { RevisionItem, FieldChange } from '@/lib/hooks/common/useRevisions'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { formatRelativeTime } from '@/lib/formatRelativeTime'

interface RevisionHistoryProps {
  entityType: string
  entityId: string | number
  isAdmin?: boolean
}

/**
 * Format a value for display in revision diffs.
 */
function formatValue(value: unknown): string {
  if (value === null || value === undefined) return '(empty)'
  if (typeof value === 'string') return value || '(empty)'
  if (typeof value === 'boolean') return value ? 'true' : 'false'
  if (typeof value === 'number') return String(value)
  return JSON.stringify(value)
}

/**
 * Renders a single field change diff.
 */
function FieldChangeDiff({ change }: { change: FieldChange }) {
  return (
    <div className="py-1.5 text-sm">
      <span className="font-medium text-muted-foreground">{change.field}:</span>
      <div className="ml-4 mt-0.5 space-y-0.5">
        <div className="flex items-start gap-1.5">
          <span className="text-xs text-muted-foreground shrink-0 mt-0.5">-</span>
          <span className="text-red-400 line-through break-all">
            {formatValue(change.old_value)}
          </span>
        </div>
        <div className="flex items-start gap-1.5">
          <span className="text-xs text-muted-foreground shrink-0 mt-0.5">+</span>
          <span className="text-green-400 break-all">
            {formatValue(change.new_value)}
          </span>
        </div>
      </div>
    </div>
  )
}

/**
 * Renders a single revision entry with expandable field changes.
 */
function RevisionEntry({
  revision,
  isAdmin,
  onRollback,
  isRollingBack,
}: {
  revision: RevisionItem
  isAdmin: boolean
  onRollback: (revisionId: number) => void
  isRollingBack: boolean
}) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="border-b border-border/50 last:border-b-0">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-start gap-3 py-3 px-2 text-left hover:bg-muted/30 transition-colors rounded-md"
      >
        <div className="mt-0.5 shrink-0 text-muted-foreground">
          {expanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            {revision.user_name ? (
              <Link
                href={`/users/${revision.user_name}`}
                onClick={e => e.stopPropagation()}
                className="text-sm font-medium hover:underline"
              >
                {revision.user_name}
              </Link>
            ) : (
              <span className="text-sm font-medium text-muted-foreground">
                User #{revision.user_id}
              </span>
            )}
            <span className="text-xs text-muted-foreground">
              {formatRelativeTime(revision.created_at)}
            </span>
          </div>
          {revision.summary && (
            <p className="text-sm text-muted-foreground mt-0.5 truncate">
              {revision.summary}
            </p>
          )}
          <p className="text-xs text-muted-foreground mt-0.5">
            {revision.changes.length} field{revision.changes.length === 1 ? '' : 's'} changed
          </p>
        </div>
      </button>

      {expanded && (
        <div className="ml-9 pb-3 pr-2">
          <div className="border-l-2 border-border pl-3">
            {revision.changes.map((change, idx) => (
              <FieldChangeDiff key={idx} change={change} />
            ))}
          </div>

          {isAdmin && (
            <div className="mt-3">
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  if (
                    window.confirm(
                      `Are you sure you want to rollback revision #${revision.id}? This will revert the ${revision.changes.length} field change(s) to their previous values.`
                    )
                  ) {
                    onRollback(revision.id)
                  }
                }}
                disabled={isRollingBack}
                className="text-xs"
              >
                {isRollingBack ? (
                  <Loader2 className="h-3 w-3 mr-1.5 animate-spin" />
                ) : (
                  <RotateCcw className="h-3 w-3 mr-1.5" />
                )}
                Rollback
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/**
 * A collapsible section that displays revision history for any entity.
 * Shows field-level diffs and admin rollback controls.
 */
export function RevisionHistory({ entityType, entityId, isAdmin = false }: RevisionHistoryProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [limit] = useState(20)
  const [offset, setOffset] = useState(0)

  const { data, isLoading, error } = useEntityRevisions(entityType, entityId, {
    enabled: isOpen,
    limit,
    offset,
  })

  const rollback = useRollbackRevision()

  const total = data?.total ?? 0
  const revisions = data?.revisions ?? []
  const hasMore = offset + limit < total

  return (
    <div className="mt-8 border border-border/50 rounded-lg">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-full flex items-center gap-2 px-4 py-3 text-left hover:bg-muted/30 transition-colors rounded-lg"
      >
        <History className="h-4 w-4 text-muted-foreground shrink-0" />
        <span className="text-sm font-medium">History</span>
        {isOpen && total > 0 && (
          <Badge variant="secondary" className="text-xs px-1.5 py-0">
            {total}
          </Badge>
        )}
        <div className="flex-1" />
        {isOpen ? (
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-4 w-4 text-muted-foreground" />
        )}
      </button>

      {isOpen && (
        <div className="px-4 pb-4">
          {isLoading && (
            <div className="flex justify-center py-6">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
            </div>
          )}

          {error && (
            <p className="text-sm text-destructive py-4">
              Failed to load revision history
            </p>
          )}

          {!isLoading && !error && revisions.length === 0 && (
            <p className="text-sm text-muted-foreground py-4">
              No edit history
            </p>
          )}

          {!isLoading && !error && revisions.length > 0 && (
            <>
              <div className="divide-y-0">
                {revisions.map(revision => (
                  <RevisionEntry
                    key={revision.id}
                    revision={revision}
                    isAdmin={isAdmin}
                    onRollback={id => rollback.mutate(id)}
                    isRollingBack={rollback.isPending}
                  />
                ))}
              </div>

              {hasMore && (
                <div className="mt-3 flex justify-center">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setOffset(prev => prev + limit)}
                    className="text-xs"
                  >
                    Load more
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
