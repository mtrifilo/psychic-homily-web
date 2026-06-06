'use client'

import { useState, useCallback } from 'react'
import Link from 'next/link'
import {
  Loader2,
  Inbox,
  Pencil,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Trash2,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useMyPendingEdits } from '../hooks/useMyPendingEdits'
import { useCancelPendingEdit } from '../hooks/useCancelPendingEdit'
import type { PendingEditResponse, PendingEditStatus } from '../types'

const PAGE_SIZE = 20

/**
 * Builds the public URL for a pending-edit row's affected entity.
 * Falls back to '#' when the slug is missing — entity routes are slug-only,
 * so /artists/:id-style URLs always 404 in the live app.
 */
function getEntityUrl(entityType: string, slug?: string | null): string {
  if (!slug) return '#'
  switch (entityType) {
    case 'artist':
      return `/artists/${slug}`
    case 'venue':
      return `/venues/${slug}`
    case 'festival':
      return `/festivals/${slug}`
    case 'release':
      return `/releases/${slug}`
    case 'label':
      return `/labels/${slug}`
    default:
      return '#'
  }
}

function entityTypeLabel(entityType: string): string {
  return entityType.charAt(0).toUpperCase() + entityType.slice(1)
}

function timeAgo(dateStr: string): string {
  const now = new Date()
  const date = new Date(dateStr)
  const seconds = Math.floor((now.getTime() - date.getTime()) / 1000)

  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  const months = Math.floor(days / 30)
  return `${months}mo ago`
}

function renderValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return '(empty)'
  return String(value)
}

interface StatusBadgeProps {
  status: PendingEditStatus
}

function StatusBadge({ status }: StatusBadgeProps) {
  switch (status) {
    case 'pending':
      return (
        <Badge
          variant="secondary"
          className="bg-pending text-pending-foreground border-pending-foreground"
        >
          <Clock className="h-3 w-3 mr-1" />
          Pending
        </Badge>
      )
    case 'approved':
      return (
        <Badge
          variant="secondary"
          className="bg-success text-success-foreground border-success-foreground"
        >
          <CheckCircle2 className="h-3 w-3 mr-1" />
          Approved
        </Badge>
      )
    case 'rejected':
      return (
        <Badge
          variant="secondary"
          className="bg-rose-500/10 text-rose-700 dark:text-rose-400 border-rose-200 dark:border-rose-800"
        >
          <XCircle className="h-3 w-3 mr-1" />
          Rejected
        </Badge>
      )
  }
}

interface PendingEditRowProps {
  edit: PendingEditResponse
}

function PendingEditRow({ edit }: PendingEditRowProps) {
  const [expanded, setExpanded] = useState(false)
  const cancelMutation = useCancelPendingEdit()

  const entityLabel =
    edit.entity_name || `${entityTypeLabel(edit.entity_type)} #${edit.entity_id}`
  const entityUrl = getEntityUrl(edit.entity_type, edit.entity_slug)
  const hasLink = entityUrl !== '#'

  const fieldChanges = edit.field_changes ?? []
  const fieldCount = fieldChanges.length

  const handleCancel = useCallback(() => {
    cancelMutation.mutate(edit.id)
  }, [cancelMutation, edit.id])

  return (
    <Card className="overflow-hidden" data-testid="pending-edit-row">
      <CardContent className="p-4">
        {/* Header row */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1 flex-wrap">
            <Badge
              variant="secondary"
              className="shrink-0 bg-blue-500/10 text-blue-700 dark:text-blue-400 border-blue-200 dark:border-blue-800"
            >
              <Pencil className="h-3 w-3 mr-1" />
              Edit
            </Badge>
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(edit.entity_type)}
            </Badge>
            <StatusBadge status={edit.status} />
            {hasLink ? (
              <Link
                href={entityUrl}
                className="text-sm font-medium text-foreground hover:underline truncate"
              >
                {entityLabel}
                <ExternalLink className="h-3 w-3 inline ml-1 opacity-50" />
              </Link>
            ) : (
              <span className="text-sm font-medium text-muted-foreground truncate">
                {entityLabel}
              </span>
            )}
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(edit.created_at)}
          </span>
        </div>

        {edit.summary && (
          edit.summary_html ? (
            <div
              className="mt-2 text-sm text-muted-foreground prose prose-sm max-w-none"
              dangerouslySetInnerHTML={{ __html: edit.summary_html }}
            />
          ) : (
            <p className="mt-2 text-sm text-muted-foreground">
              {edit.summary}
            </p>
          )
        )}

        {/* Rejection reason — visible by default since it's the moderator's
            response to YOUR edit and the most actionable detail on the row.
            Renders sanitised HTML when the backend ships rejection_reason_html
            (post-PSY-605); falls back to plain whitespace-preserved text for
            legacy rows that pre-date the markdown roundtrip. */}
        {edit.status === 'rejected' && edit.rejection_reason && (
          <div
            className="mt-3 rounded-md border border-rose-200 dark:border-rose-800 bg-rose-50/50 dark:bg-rose-950/30 p-3"
            data-testid="rejection-reason"
          >
            <div className="flex items-start gap-2">
              <AlertCircle className="h-4 w-4 text-rose-600 dark:text-rose-400 shrink-0 mt-0.5" />
              <div className="min-w-0 flex-1">
                <p className="text-xs font-medium text-rose-700 dark:text-rose-400 uppercase tracking-wide">
                  Moderator response
                </p>
                {edit.rejection_reason_html ? (
                  <div
                    className="mt-1 text-sm text-foreground prose prose-sm max-w-none"
                    dangerouslySetInnerHTML={{
                      __html: edit.rejection_reason_html,
                    }}
                  />
                ) : (
                  <p className="mt-1 text-sm text-foreground whitespace-pre-wrap break-words">
                    {edit.rejection_reason}
                  </p>
                )}
              </div>
            </div>
          </div>
        )}

        {/* Field changes preview */}
        <button
          type="button"
          onClick={() => setExpanded(v => !v)}
          className="mt-3 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
        >
          {expanded ? (
            <ChevronDown className="h-3 w-3" />
          ) : (
            <ChevronRight className="h-3 w-3" />
          )}
          {fieldCount} field change{fieldCount !== 1 ? 's' : ''}
        </button>

        {expanded && (
          <div className="mt-2 space-y-1.5 rounded-md border bg-muted/30 p-3 text-sm">
            {fieldChanges.map((change, idx) => (
              <div key={idx} className="space-y-0.5">
                <span className="font-medium text-muted-foreground">
                  {change.field}:
                </span>
                <div className="flex gap-2 flex-wrap text-xs font-mono">
                  <span className="bg-red-500/10 text-red-700 dark:text-red-400 rounded px-1.5 py-0.5 line-through">
                    {renderValue(change.old_value)}
                  </span>
                  <span className="text-muted-foreground">&rarr;</span>
                  <span className="bg-green-500/10 text-green-700 dark:text-green-400 rounded px-1.5 py-0.5">
                    {renderValue(change.new_value)}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Cancel — only for still-pending edits the user owns */}
        {edit.status === 'pending' && (
          <div className="mt-3 flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={handleCancel}
              disabled={cancelMutation.isPending}
              data-testid="cancel-pending-edit"
            >
              {cancelMutation.isPending ? (
                <Loader2 className="h-3 w-3 animate-spin mr-1" />
              ) : (
                <Trash2 className="h-3 w-3 mr-1" />
              )}
              Cancel edit
            </Button>
            {cancelMutation.isError && (
              <span className="text-xs text-destructive">
                {cancelMutation.error?.message || 'Cancel failed'}
              </span>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

/**
 * Renders the signed-in user's pending entity edits, latest first, with
 * inline pagination. Each row links to the affected entity (when slug-
 * resolvable), shows status (pending / approved / rejected), and surfaces
 * the moderator's rejection reason when applicable. Owner-only by
 * construction — the backend filters on `submitted_by = current_user.id`,
 * so even if an attacker mutated the React state they could not see
 * another user's edits.
 *
 * PSY-600: contributor-facing pending-edits feedback loop. Replaces the
 * broken email-verification gate that previously occupied /submissions.
 */
export function MyPendingEditsList() {
  const [page, setPage] = useState(0)
  const offset = page * PAGE_SIZE

  const { data, isLoading, isError, error } = useMyPendingEdits(
    PAGE_SIZE,
    offset
  )

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (isError) {
    return (
      <Card className="border-destructive/30 bg-destructive/5">
        <CardContent className="p-6 text-center">
          <AlertCircle className="h-8 w-8 mx-auto mb-3 text-destructive" />
          <p className="text-sm text-foreground font-medium">
            Failed to load your pending edits
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            {error?.message || 'Please try again later.'}
          </p>
        </CardContent>
      </Card>
    )
  }

  const edits = data?.edits ?? []
  const total = data?.total ?? 0

  if (edits.length === 0 && page === 0) {
    return (
      <Card className="border-border/50 bg-card/50">
        <CardContent className="p-8 text-center">
          <Inbox className="h-10 w-10 mx-auto mb-3 text-muted-foreground/40" />
          <p className="text-base font-medium text-foreground">
            No pending edits yet
          </p>
          <p className="mt-1 text-sm text-muted-foreground max-w-md mx-auto">
            Suggest edits to artists, venues, festivals, releases, or labels
            from any entity page. Your edits appear here as they wait for
            moderator review.
          </p>
        </CardContent>
      </Card>
    )
  }

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-3">
      {edits.map(edit => (
        <PendingEditRow key={edit.id} edit={edit} />
      ))}

      {/* Pagination — only render when needed */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between gap-2 pt-2">
          <p className="text-xs text-muted-foreground">
            Showing {offset + 1}–{Math.min(offset + edits.length, total)} of{' '}
            {total}
          </p>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => setPage(p => Math.max(0, p - 1))}
              disabled={page === 0}
            >
              Previous
            </Button>
            <span className="text-xs text-muted-foreground">
              Page {page + 1} of {totalPages}
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setPage(p => p + 1)}
              disabled={page + 1 >= totalPages}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
