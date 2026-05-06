'use client'

import { useState, useMemo, useCallback } from 'react'
import {
  Loader2,
  Inbox,
  Pencil,
  Flag,
  Check,
  X,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  MessageSquare,
  History,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { UserAttribution } from '@/components/shared'
import {
  useAdminPendingEdits,
  useApprovePendingEdit,
  useRejectPendingEdit,
} from '@/lib/hooks/admin/useAdminPendingEdits'
import {
  useAdminEntityReports,
  useResolveEntityReport,
  useDismissEntityReport,
  useAdminHideCollection,
} from '@/lib/hooks/admin/useAdminEntityReports'
import {
  useAdminPendingComments,
  useAdminApproveComment,
  useAdminRejectComment,
  useAdminHideComment,
} from '@/lib/hooks/admin/useAdminComments'
import { CommentEditHistory } from '@/features/comments'
import type { PendingEditResponse } from '@/lib/hooks/admin/useAdminPendingEdits'
import type { EntityReportResponse } from '@/lib/hooks/admin/useAdminEntityReports'
import type { PendingComment } from '@/lib/hooks/admin/useAdminComments'

// ─── Helpers ─────────────────────────────────────────────────────────────────

function getEntityUrl(entityType: string, entityId: number, entitySlug?: string): string {
  switch (entityType) {
    case 'artist':
      return `/artists/${entityId}`
    case 'venue':
      return `/venues/${entityId}`
    case 'festival':
      return `/festivals/${entityId}`
    case 'show':
      return `/shows/${entityId}`
    case 'comment':
      return '#'
    // PSY-357: collections are addressed by slug, not numeric ID. Fall back
    // to '#' if the slug couldn't be resolved (deleted collection, etc.).
    case 'collection':
      return entitySlug ? `/collections/${entitySlug}` : '#'
    default:
      return '#'
  }
}

function entityTypeLabel(entityType: string): string {
  return entityType.charAt(0).toUpperCase() + entityType.slice(1)
}

function reportTypeLabel(reportType: string): string {
  return reportType
    .split('_')
    .map(w => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ')
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

// ─── Filter Types ────────────────────────────────────────────────────────────

type ItemTypeFilter = 'all' | 'edits' | 'reports' | 'comments'
type EntityTypeFilter = '' | 'artist' | 'venue' | 'festival' | 'show' | 'collection'

// ─── Unified Item Type ───────────────────────────────────────────────────────

type ModerationItem =
  | { type: 'edit'; data: PendingEditResponse }
  | { type: 'report'; data: EntityReportResponse }
  | { type: 'comment'; data: PendingComment }

// ─── Pending Edit Card ───────────────────────────────────────────────────────

function PendingEditCard({ edit }: { edit: PendingEditResponse }) {
  const [expanded, setExpanded] = useState(false)
  const [rejecting, setRejecting] = useState(false)
  const [rejectionReason, setRejectionReason] = useState('')

  const approveMutation = useApprovePendingEdit()
  const rejectMutation = useRejectPendingEdit()

  const isActioning = approveMutation.isPending || rejectMutation.isPending

  const handleApprove = useCallback(() => {
    approveMutation.mutate(edit.id)
  }, [approveMutation, edit.id])

  const handleReject = useCallback(() => {
    if (!rejectionReason.trim()) return
    rejectMutation.mutate(
      { editId: edit.id, reason: rejectionReason.trim() },
      { onSuccess: () => { setRejecting(false); setRejectionReason('') } }
    )
  }, [rejectMutation, edit.id, rejectionReason])

  return (
    <Card className="overflow-hidden">
      <CardContent className="p-4">
        {/* Header row */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <Badge variant="secondary" className="shrink-0 bg-blue-500/10 text-blue-700 dark:text-blue-400 border-blue-200 dark:border-blue-800">
              <Pencil className="h-3 w-3 mr-1" />
              Edit
            </Badge>
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(edit.entity_type)}
            </Badge>
            <a
              href={getEntityUrl(edit.entity_type, edit.entity_id)}
              className="text-sm font-medium text-foreground hover:underline truncate"
              target="_blank"
              rel="noopener noreferrer"
            >
              {edit.entity_name || `${entityTypeLabel(edit.entity_type)} #${edit.entity_id}`}
              <ExternalLink className="h-3 w-3 inline ml-1 opacity-50" />
            </a>
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(edit.created_at)}
          </span>
        </div>

        {/* Meta. PSY-613: byline via shared UserAttribution. The
            moderation contract doesn't ship submitter_username yet, so we
            pass null and the primitive renders plain text. */}
        <div className="mt-2 text-sm text-muted-foreground">
          <span>
            by{' '}
            <UserAttribution name={edit.submitter_name} username={null} />
          </span>
          {edit.summary && (
            <span className="ml-1">
              &mdash; {edit.summary}
            </span>
          )}
        </div>

        {/* Changes preview / expand */}
        <button
          onClick={() => setExpanded(!expanded)}
          className="mt-2 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
        >
          {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
          {edit.field_changes.length} field change{edit.field_changes.length !== 1 ? 's' : ''}
        </button>

        {expanded && (
          <div className="mt-2 space-y-1.5 rounded-md border bg-muted/30 p-3 text-sm">
            {edit.field_changes.map((change, idx) => (
              <div key={idx} className="space-y-0.5">
                <span className="font-medium text-muted-foreground">{change.field}:</span>
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

        {/* Rejection reason input */}
        {rejecting && (
          <div className="mt-3 space-y-2">
            <textarea
              value={rejectionReason}
              onChange={e => setRejectionReason(e.target.value)}
              placeholder="Rejection reason (required) -- be specific to help the contributor learn"
              className="w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
              rows={2}
              autoFocus
            />
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="destructive"
                onClick={handleReject}
                disabled={!rejectionReason.trim() || isActioning}
              >
                {rejectMutation.isPending ? (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                ) : (
                  <X className="h-3 w-3 mr-1" />
                )}
                Confirm Reject
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => { setRejecting(false); setRejectionReason('') }}
                disabled={isActioning}
              >
                Cancel
              </Button>
            </div>
          </div>
        )}

        {/* Action buttons */}
        {!rejecting && (
          <div className="mt-3 flex items-center gap-2">
            <Button
              size="sm"
              onClick={handleApprove}
              disabled={isActioning}
            >
              {approveMutation.isPending ? (
                <Loader2 className="h-3 w-3 animate-spin mr-1" />
              ) : (
                <Check className="h-3 w-3 mr-1" />
              )}
              Approve
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setRejecting(true)}
              disabled={isActioning}
            >
              <X className="h-3 w-3 mr-1" />
              Reject
            </Button>
          </div>
        )}

        {/* Error display */}
        {(approveMutation.isError || rejectMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(approveMutation.error || rejectMutation.error)?.message || 'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Entity Report Card ──────────────────────────────────────────────────────

function EntityReportCard({ report }: { report: EntityReportResponse }) {
  const [showNotes, setShowNotes] = useState(false)
  const [notes, setNotes] = useState('')
  const [action, setAction] = useState<'resolve' | 'dismiss' | null>(null)

  const resolveMutation = useResolveEntityReport()
  const dismissMutation = useDismissEntityReport()

  const isActioning = resolveMutation.isPending || dismissMutation.isPending

  const handleAction = useCallback(() => {
    if (action === 'resolve') {
      resolveMutation.mutate(
        { reportId: report.id, notes: notes.trim() || undefined },
        { onSuccess: () => { setShowNotes(false); setNotes(''); setAction(null) } }
      )
    } else if (action === 'dismiss') {
      dismissMutation.mutate(
        { reportId: report.id, notes: notes.trim() || undefined },
        { onSuccess: () => { setShowNotes(false); setNotes(''); setAction(null) } }
      )
    }
  }, [action, resolveMutation, dismissMutation, report.id, notes])

  const startAction = useCallback((type: 'resolve' | 'dismiss') => {
    setAction(type)
    setShowNotes(true)
  }, [])

  return (
    <Card className="overflow-hidden">
      <CardContent className="p-4">
        {/* Header row */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <Badge variant="secondary" className="shrink-0 bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-200 dark:border-amber-800">
              <Flag className="h-3 w-3 mr-1" />
              Report
            </Badge>
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(report.entity_type)}
            </Badge>
            <a
              href={getEntityUrl(report.entity_type, report.entity_id, report.entity_slug)}
              className="text-sm font-medium text-foreground hover:underline truncate"
              target="_blank"
              rel="noopener noreferrer"
            >
              {report.entity_name || `${entityTypeLabel(report.entity_type)} #${report.entity_id}`}
              <ExternalLink className="h-3 w-3 inline ml-1 opacity-50" />
            </a>
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(report.created_at)}
          </span>
        </div>

        {/* Meta */}
        <div className="mt-2 space-y-1">
          <div className="flex items-center gap-2 text-sm">
            <Badge variant="outline" className="text-xs">
              {reportTypeLabel(report.report_type)}
            </Badge>
            <span className="text-muted-foreground">
              {/* PSY-613: byline via shared UserAttribution. The
                  moderation report contract doesn't ship reporter_username
                  yet, so we pass null and the primitive renders plain text. */}
              by{' '}
              <UserAttribution
                name={report.reporter_name}
                username={null}
              />
            </span>
          </div>
          {report.details && (
            <p className="text-sm text-muted-foreground italic">
              &ldquo;{report.details}&rdquo;
            </p>
          )}
        </div>

        {/* Notes input */}
        {showNotes && (
          <div className="mt-3 space-y-2">
            <textarea
              value={notes}
              onChange={e => setNotes(e.target.value)}
              placeholder={`Admin notes (optional)${action === 'resolve' ? ' -- describe the action taken' : ' -- explain why this was dismissed'}`}
              className="w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
              rows={2}
              autoFocus
            />
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant={action === 'resolve' ? 'default' : 'outline'}
                onClick={handleAction}
                disabled={isActioning}
              >
                {isActioning ? (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                ) : action === 'resolve' ? (
                  <Check className="h-3 w-3 mr-1" />
                ) : (
                  <X className="h-3 w-3 mr-1" />
                )}
                {action === 'resolve' ? 'Confirm Resolve' : 'Confirm Dismiss'}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => { setShowNotes(false); setNotes(''); setAction(null) }}
                disabled={isActioning}
              >
                Cancel
              </Button>
            </div>
          </div>
        )}

        {/* Action buttons */}
        {!showNotes && (
          <div className="mt-3 flex items-center gap-2">
            <Button
              size="sm"
              onClick={() => startAction('resolve')}
              disabled={isActioning}
            >
              <Check className="h-3 w-3 mr-1" />
              Resolve
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => startAction('dismiss')}
              disabled={isActioning}
            >
              <X className="h-3 w-3 mr-1" />
              Dismiss
            </Button>
          </div>
        )}

        {/* Error display */}
        {(resolveMutation.isError || dismissMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(resolveMutation.error || dismissMutation.error)?.message || 'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Pending Comment Card ───────────────────────────────────────────────────

function PendingCommentCard({ comment }: { comment: PendingComment }) {
  const [rejecting, setRejecting] = useState(false)
  const [rejectionReason, setRejectionReason] = useState('')
  // PSY-297: edit history viewer, opened on demand
  const [isEditHistoryOpen, setIsEditHistoryOpen] = useState(false)

  const approveMutation = useAdminApproveComment()
  const rejectMutation = useAdminRejectComment()

  const isActioning = approveMutation.isPending || rejectMutation.isPending

  const handleApprove = useCallback(() => {
    approveMutation.mutate(comment.id)
  }, [approveMutation, comment.id])

  const handleReject = useCallback(() => {
    if (!rejectionReason.trim()) return
    rejectMutation.mutate(
      { commentId: comment.id, reason: rejectionReason.trim() },
      { onSuccess: () => { setRejecting(false); setRejectionReason('') } }
    )
  }, [rejectMutation, comment.id, rejectionReason])

  const entityUrl = getEntityUrl(comment.entity_type, comment.entity_id)
  const editCount = comment.edit_count ?? 0
  const hasEdits = editCount > 0

  return (
    <Card className="overflow-hidden" data-testid="pending-comment-card">
      <CardContent className="p-4">
        {/* Header row */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <Badge variant="secondary" className="shrink-0 bg-violet-500/10 text-violet-700 dark:text-violet-400 border-violet-200 dark:border-violet-800">
              <MessageSquare className="h-3 w-3 mr-1" />
              Comment
            </Badge>
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(comment.entity_type)}
            </Badge>
            {comment.entity_name && (
              <a
                href={entityUrl}
                className="text-sm font-medium text-foreground hover:underline truncate"
                target="_blank"
                rel="noopener noreferrer"
              >
                {comment.entity_name}
                <ExternalLink className="h-3 w-3 inline ml-1 opacity-50" />
              </a>
            )}
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(comment.created_at)}
          </span>
        </div>

        {/* Meta. PSY-613: byline via shared UserAttribution. The moderation
            comment contract doesn't ship author_username yet, so we pass
            null and the primitive renders plain text. */}
        <div className="mt-2 text-sm text-muted-foreground flex items-center flex-wrap gap-2">
          <span>
            by{' '}
            <UserAttribution name={comment.author_name} username={null} />
          </span>
          {comment.trust_tier && (
            <Badge variant="outline" className="text-[10px] px-1.5 py-0">
              {comment.trust_tier}
            </Badge>
          )}
          {/* PSY-297: edit count badge + click-to-view-history.
              Only rendered when the comment has at least one recorded edit. */}
          {hasEdits && (
            <button
              type="button"
              onClick={() => setIsEditHistoryOpen(true)}
              className="inline-flex items-center gap-1 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-700 dark:text-amber-400 hover:bg-amber-500/20 transition-colors"
              data-testid="pending-comment-edit-badge"
              aria-label={`View edit history (${editCount} edit${editCount !== 1 ? 's' : ''})`}
            >
              <History className="h-3 w-3" />
              {editCount} edit{editCount !== 1 ? 's' : ''}
            </button>
          )}
        </div>

        {/* Comment body */}
        <div
          className="mt-2 rounded-md border bg-muted/30 p-3 text-sm prose prose-sm dark:prose-invert max-w-none"
          dangerouslySetInnerHTML={{ __html: comment.body_html }}
          data-testid="comment-body"
        />

        {/* PSY-297: edit history dialog, mounted on demand. */}
        {isEditHistoryOpen && (
          <CommentEditHistory
            open={isEditHistoryOpen}
            onOpenChange={setIsEditHistoryOpen}
            commentId={comment.id}
          />
        )}

        {/* Rejection reason input */}
        {rejecting && (
          <div className="mt-3 space-y-2">
            <textarea
              value={rejectionReason}
              onChange={e => setRejectionReason(e.target.value)}
              placeholder="Rejection reason (required)"
              className="w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
              rows={2}
              autoFocus
            />
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="destructive"
                onClick={handleReject}
                disabled={!rejectionReason.trim() || isActioning}
              >
                {rejectMutation.isPending ? (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                ) : (
                  <X className="h-3 w-3 mr-1" />
                )}
                Confirm Reject
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => { setRejecting(false); setRejectionReason('') }}
                disabled={isActioning}
              >
                Cancel
              </Button>
            </div>
          </div>
        )}

        {/* Action buttons */}
        {!rejecting && (
          <div className="mt-3 flex items-center gap-2">
            <Button
              size="sm"
              onClick={handleApprove}
              disabled={isActioning}
            >
              {approveMutation.isPending ? (
                <Loader2 className="h-3 w-3 animate-spin mr-1" />
              ) : (
                <Check className="h-3 w-3 mr-1" />
              )}
              Approve
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setRejecting(true)}
              disabled={isActioning}
            >
              <X className="h-3 w-3 mr-1" />
              Reject
            </Button>
          </div>
        )}

        {/* Error display */}
        {(approveMutation.isError || rejectMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(approveMutation.error || rejectMutation.error)?.message || 'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Comment Report Card ────────────────────────────────────────────────────

function CommentReportCard({ report }: { report: EntityReportResponse }) {
  const [showNotes, setShowNotes] = useState(false)
  const [notes, setNotes] = useState('')
  const [action, setAction] = useState<'hide' | 'dismiss' | null>(null)

  const hideMutation = useAdminHideComment()
  const dismissMutation = useDismissEntityReport()

  const isActioning = hideMutation.isPending || dismissMutation.isPending

  const handleAction = useCallback(() => {
    if (action === 'hide') {
      hideMutation.mutate(
        { commentId: report.entity_id, reason: notes.trim() || 'Hidden via report review' },
        { onSuccess: () => { setShowNotes(false); setNotes(''); setAction(null) } }
      )
    } else if (action === 'dismiss') {
      dismissMutation.mutate(
        { reportId: report.id, notes: notes.trim() || undefined },
        { onSuccess: () => { setShowNotes(false); setNotes(''); setAction(null) } }
      )
    }
  }, [action, hideMutation, dismissMutation, report.id, report.entity_id, notes])

  const startAction = useCallback((type: 'hide' | 'dismiss') => {
    setAction(type)
    setShowNotes(true)
  }, [])

  // Truncate comment body for preview
  const bodyPreview = report.details
    ? (report.details.length > 200 ? report.details.substring(0, 200) + '...' : report.details)
    : undefined

  return (
    <Card className="overflow-hidden" data-testid="comment-report-card">
      <CardContent className="p-4">
        {/* Header row */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <Badge variant="secondary" className="shrink-0 bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-200 dark:border-amber-800">
              <Flag className="h-3 w-3 mr-1" />
              Report
            </Badge>
            <Badge variant="outline" className="shrink-0">
              Comment
            </Badge>
            <span className="text-sm text-muted-foreground truncate">
              {report.entity_name || `Comment #${report.entity_id}`}
            </span>
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(report.created_at)}
          </span>
        </div>

        {/* Meta */}
        <div className="mt-2 space-y-1">
          <div className="flex items-center gap-2 text-sm">
            <Badge variant="outline" className="text-xs">
              {reportTypeLabel(report.report_type)}
            </Badge>
            <span className="text-muted-foreground">
              {/* PSY-613: byline via shared UserAttribution. The
                  moderation report contract doesn't ship reporter_username
                  yet, so we pass null and the primitive renders plain text. */}
              by{' '}
              <UserAttribution
                name={report.reporter_name}
                username={null}
              />
            </span>
          </div>
          {bodyPreview && (
            <p className="text-sm text-muted-foreground italic">
              &ldquo;{bodyPreview}&rdquo;
            </p>
          )}
        </div>

        {/* Notes input */}
        {showNotes && (
          <div className="mt-3 space-y-2">
            <textarea
              value={notes}
              onChange={e => setNotes(e.target.value)}
              placeholder={action === 'hide' ? 'Reason for hiding (optional)' : 'Notes for dismissal (optional)'}
              className="w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
              rows={2}
              autoFocus
            />
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant={action === 'hide' ? 'destructive' : 'outline'}
                onClick={handleAction}
                disabled={isActioning}
              >
                {isActioning ? (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                ) : action === 'hide' ? (
                  <X className="h-3 w-3 mr-1" />
                ) : (
                  <Check className="h-3 w-3 mr-1" />
                )}
                {action === 'hide' ? 'Confirm Hide' : 'Confirm Dismiss'}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => { setShowNotes(false); setNotes(''); setAction(null) }}
                disabled={isActioning}
              >
                Cancel
              </Button>
            </div>
          </div>
        )}

        {/* Action buttons */}
        {!showNotes && (
          <div className="mt-3 flex items-center gap-2">
            <Button
              size="sm"
              variant="destructive"
              onClick={() => startAction('hide')}
              disabled={isActioning}
            >
              <X className="h-3 w-3 mr-1" />
              Hide Comment
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => startAction('dismiss')}
              disabled={isActioning}
            >
              <Check className="h-3 w-3 mr-1" />
              Dismiss Report
            </Button>
          </div>
        )}

        {/* Error display */}
        {(hideMutation.isError || dismissMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(hideMutation.error || dismissMutation.error)?.message || 'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Collection Report Card ────────────────────────────────────────────────

/**
 * PSY-357: admin moderation card for collection reports. Mirrors
 * `CommentReportCard` — a single click both hides the collection from
 * public browse (PUT /collections/{slug} with is_public=false) AND marks
 * the report resolved. The "Dismiss" path leaves the collection alone and
 * just clears the report.
 *
 * Hide is unavailable when the slug couldn't be resolved (i.e. the
 * collection was deleted between report submission and review). In that
 * case the only useful action is Dismiss.
 */
function CollectionReportCard({ report }: { report: EntityReportResponse }) {
  const [showNotes, setShowNotes] = useState(false)
  const [notes, setNotes] = useState('')
  const [action, setAction] = useState<'hide' | 'dismiss' | null>(null)

  const hideMutation = useAdminHideCollection()
  const resolveMutation = useResolveEntityReport()
  const dismissMutation = useDismissEntityReport()

  const isActioning =
    hideMutation.isPending || resolveMutation.isPending || dismissMutation.isPending

  const handleAction = useCallback(() => {
    if (action === 'hide') {
      if (!report.entity_slug) return
      // Hide first, then resolve the report so the moderation queue
      // reflects the action taken (rather than two separate concerns).
      hideMutation.mutate(
        { slug: report.entity_slug },
        {
          onSuccess: () => {
            resolveMutation.mutate(
              { reportId: report.id, notes: notes.trim() || undefined },
              {
                onSuccess: () => {
                  setShowNotes(false)
                  setNotes('')
                  setAction(null)
                },
              }
            )
          },
        }
      )
    } else if (action === 'dismiss') {
      dismissMutation.mutate(
        { reportId: report.id, notes: notes.trim() || undefined },
        {
          onSuccess: () => {
            setShowNotes(false)
            setNotes('')
            setAction(null)
          },
        }
      )
    }
  }, [action, hideMutation, resolveMutation, dismissMutation, report.id, report.entity_slug, notes])

  const startAction = useCallback((type: 'hide' | 'dismiss') => {
    setAction(type)
    setShowNotes(true)
  }, [])

  const entityUrl = getEntityUrl(report.entity_type, report.entity_id, report.entity_slug)
  const hasSlug = Boolean(report.entity_slug)

  return (
    <Card className="overflow-hidden" data-testid="collection-report-card">
      <CardContent className="p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <Badge variant="secondary" className="shrink-0 bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-200 dark:border-amber-800">
              <Flag className="h-3 w-3 mr-1" />
              Report
            </Badge>
            <Badge variant="outline" className="shrink-0">
              Collection
            </Badge>
            {hasSlug ? (
              <a
                href={entityUrl}
                className="text-sm font-medium text-foreground hover:underline truncate"
                target="_blank"
                rel="noopener noreferrer"
              >
                {report.entity_name || `Collection #${report.entity_id}`}
                <ExternalLink className="h-3 w-3 inline ml-1 opacity-50" />
              </a>
            ) : (
              <span className="text-sm font-medium text-muted-foreground truncate">
                {report.entity_name || `Collection #${report.entity_id}`} (deleted)
              </span>
            )}
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(report.created_at)}
          </span>
        </div>

        <div className="mt-2 space-y-1">
          <div className="flex items-center gap-2 text-sm">
            <Badge variant="outline" className="text-xs">
              {reportTypeLabel(report.report_type)}
            </Badge>
            <span className="text-muted-foreground">
              {/* PSY-613: byline via shared UserAttribution. The
                  moderation report contract doesn't ship reporter_username
                  yet, so we pass null and the primitive renders plain text. */}
              by{' '}
              <UserAttribution
                name={report.reporter_name}
                username={null}
              />
            </span>
          </div>
          {report.details && (
            <p className="text-sm text-muted-foreground italic">
              &ldquo;{report.details}&rdquo;
            </p>
          )}
        </div>

        {showNotes && (
          <div className="mt-3 space-y-2">
            <textarea
              value={notes}
              onChange={e => setNotes(e.target.value)}
              placeholder={
                action === 'hide'
                  ? 'Reason for hiding from public browse (optional)'
                  : 'Notes for dismissal (optional)'
              }
              className="w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
              rows={2}
              autoFocus
            />
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant={action === 'hide' ? 'destructive' : 'outline'}
                onClick={handleAction}
                disabled={isActioning}
              >
                {isActioning ? (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                ) : action === 'hide' ? (
                  <X className="h-3 w-3 mr-1" />
                ) : (
                  <Check className="h-3 w-3 mr-1" />
                )}
                {action === 'hide' ? 'Confirm Hide' : 'Confirm Dismiss'}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => { setShowNotes(false); setNotes(''); setAction(null) }}
                disabled={isActioning}
              >
                Cancel
              </Button>
            </div>
          </div>
        )}

        {!showNotes && (
          <div className="mt-3 flex items-center gap-2">
            <Button
              size="sm"
              variant="destructive"
              onClick={() => startAction('hide')}
              disabled={isActioning || !hasSlug}
              title={hasSlug ? undefined : 'Cannot hide — collection was deleted'}
            >
              <X className="h-3 w-3 mr-1" />
              Hide from Public Browse
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => startAction('dismiss')}
              disabled={isActioning}
            >
              <Check className="h-3 w-3 mr-1" />
              Dismiss Report
            </Button>
          </div>
        )}

        {(hideMutation.isError || resolveMutation.isError || dismissMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(hideMutation.error || resolveMutation.error || dismissMutation.error)?.message ||
              'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Main Component ──────────────────────────────────────────────────────────

export function ModerationQueue() {
  const [itemTypeFilter, setItemTypeFilter] = useState<ItemTypeFilter>('all')
  const [entityTypeFilter, setEntityTypeFilter] = useState<EntityTypeFilter>('')

  // Fetch pending edits
  const {
    data: editsData,
    isLoading: editsLoading,
    error: editsError,
  } = useAdminPendingEdits({
    status: 'pending',
    entity_type: entityTypeFilter || undefined,
  })

  // Fetch pending entity reports
  const {
    data: reportsData,
    isLoading: reportsLoading,
    error: reportsError,
  } = useAdminEntityReports({
    status: 'pending',
    entity_type: entityTypeFilter || undefined,
  })

  // Fetch pending comments
  const {
    data: commentsData,
    isLoading: commentsLoading,
    error: commentsError,
  } = useAdminPendingComments()

  const isLoading = editsLoading || reportsLoading || commentsLoading
  const error = editsError || reportsError || commentsError

  // Merge and sort items by created_at (oldest first for review fairness)
  const items = useMemo<ModerationItem[]>(() => {
    const editItems: ModerationItem[] = (editsData?.edits || []).map(e => ({
      type: 'edit' as const,
      data: e,
    }))
    // All reports (entity + comment reports) are of type 'report' in the unified list
    const reportItems: ModerationItem[] = (reportsData?.reports || []).map(r => ({
      type: 'report' as const,
      data: r,
    }))
    const commentItems: ModerationItem[] = (commentsData?.comments || []).map(c => ({
      type: 'comment' as const,
      data: c,
    }))

    let merged = [...editItems, ...reportItems, ...commentItems]

    // Apply item type filter
    if (itemTypeFilter === 'edits') {
      merged = merged.filter(i => i.type === 'edit')
    } else if (itemTypeFilter === 'reports') {
      merged = merged.filter(i => i.type === 'report')
    } else if (itemTypeFilter === 'comments') {
      merged = merged.filter(i => i.type === 'comment')
    }

    // Sort oldest first (review fairness)
    merged.sort(
      (a, b) =>
        new Date(a.data.created_at).getTime() - new Date(b.data.created_at).getTime()
    )

    return merged
  }, [editsData, reportsData, commentsData, itemTypeFilter])

  const totalEdits = editsData?.total || 0
  const totalReports = reportsData?.total || 0
  const totalComments = commentsData?.total || 0
  const totalItems = totalEdits + totalReports + totalComments

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
            : 'Failed to load moderation queue. Please try again.'}
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-3">
        {/* Item type filter */}
        <div className="flex items-center gap-1 rounded-lg border bg-muted/30 p-0.5">
          <FilterButton
            active={itemTypeFilter === 'all'}
            onClick={() => setItemTypeFilter('all')}
            label="All"
            count={totalItems}
          />
          <FilterButton
            active={itemTypeFilter === 'edits'}
            onClick={() => setItemTypeFilter('edits')}
            label="Edits"
            count={totalEdits}
          />
          <FilterButton
            active={itemTypeFilter === 'reports'}
            onClick={() => setItemTypeFilter('reports')}
            label="Reports"
            count={totalReports}
          />
          <FilterButton
            active={itemTypeFilter === 'comments'}
            onClick={() => setItemTypeFilter('comments')}
            label="Comments"
            count={totalComments}
          />
        </div>

        {/* Entity type filter */}
        <select
          value={entityTypeFilter}
          onChange={e => setEntityTypeFilter(e.target.value as EntityTypeFilter)}
          className="rounded-md border bg-background px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
        >
          <option value="">All entity types</option>
          <option value="artist">Artists</option>
          <option value="venue">Venues</option>
          <option value="festival">Festivals</option>
          <option value="show">Shows</option>
          <option value="collection">Collections</option>
        </select>

        {/* Summary count */}
        <span className="text-sm text-muted-foreground ml-auto">
          {items.length} item{items.length !== 1 ? 's' : ''} pending review
        </span>
      </div>

      {/* Empty state */}
      {items.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Inbox className="h-8 w-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">Queue Clear</h3>
          <p className="text-sm text-muted-foreground max-w-sm">
            {itemTypeFilter === 'edits'
              ? 'No pending entity edits to review.'
              : itemTypeFilter === 'reports'
                ? 'No pending entity reports to review.'
                : itemTypeFilter === 'comments'
                  ? 'No pending comments to review.'
                  : 'No items need moderation. Pending entity edits, reports, and comments will appear here when users submit them.'}
          </p>
        </div>
      )}

      {/* Items list */}
      {items.length > 0 && (
        <div className="grid gap-3">
          {items.map(item => {
            if (item.type === 'edit') {
              return <PendingEditCard key={`edit-${item.data.id}`} edit={item.data as PendingEditResponse} />
            }
            if (item.type === 'comment') {
              return <PendingCommentCard key={`comment-${item.data.id}`} comment={item.data as PendingComment} />
            }
            // Reports — type-specific cards for kinds that need bespoke
            // moderation actions (hide-comment, hide-collection); generic
            // EntityReportCard for the other entity types.
            const report = item.data as EntityReportResponse
            if (report.entity_type === 'comment') {
              return <CommentReportCard key={`comment-report-${report.id}`} report={report} />
            }
            if (report.entity_type === 'collection') {
              return <CollectionReportCard key={`collection-report-${report.id}`} report={report} />
            }
            return <EntityReportCard key={`report-${report.id}`} report={report} />
          })}
        </div>
      )}
    </div>
  )
}

// ─── Filter Button ───────────────────────────────────────────────────────────

function FilterButton({
  active,
  onClick,
  label,
  count,
}: {
  active: boolean
  onClick: () => void
  label: string
  count: number
}) {
  return (
    <button
      onClick={onClick}
      className={`rounded-md px-3 py-1 text-sm font-medium transition-colors ${
        active
          ? 'bg-background text-foreground shadow-sm'
          : 'text-muted-foreground hover:text-foreground'
      }`}
    >
      {label}
      {count > 0 && (
        <span className={`ml-1.5 text-xs ${active ? 'text-muted-foreground' : 'opacity-70'}`}>
          {count}
        </span>
      )}
    </button>
  )
}
