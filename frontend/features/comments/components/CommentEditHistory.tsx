'use client'

/**
 * CommentEditHistory — admin-only modal that walks back through a comment's
 * edit history (PSY-297).
 *
 * UX notes:
 * - Presented as a Dialog (modal) — matches the app's existing pattern for
 *   admin affordances (ReportEntityDialog et al.) and avoids adding a new
 *   primitive for this single use case.
 * - Order: current body at the top, then edits oldest-at-bottom so the user
 *   reads forward-in-time by scanning top→down through each `current → prior`
 *   transition. Each pair is rendered as a two-column before/after block.
 * - Diff rendering: intentionally NOT importing a diff lib. The app has no
 *   existing diff dependency, the ticket says "if no diff lib, show
 *   before/after blocks", and the admin viewer is low-traffic. Two side-by-side
 *   code blocks (previous vs current) are sufficient for moderation
 *   investigations; richer diffing can follow if real usage demands it.
 */

import { Loader2, Clock, AlertCircle } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import {
  useAdminCommentEditHistory,
  type CommentEditHistoryEntry,
  type CommentEditHistoryResponse,
} from '@/lib/hooks/admin/useAdminComments'

interface CommentEditHistoryProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  commentId: number
}

export function CommentEditHistory({
  open,
  onOpenChange,
  commentId,
}: CommentEditHistoryProps) {
  const { data, isLoading, error } = useAdminCommentEditHistory(commentId, open)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-2xl max-h-[85vh] overflow-y-auto"
        data-testid="comment-edit-history-dialog"
      >
        <DialogHeader>
          <DialogTitle>Edit history</DialogTitle>
          <DialogDescription>
            Chronological walkback of this comment&rsquo;s body. Current version at the top.
          </DialogDescription>
        </DialogHeader>

        {isLoading && (
          <div className="flex items-center justify-center py-8" data-testid="edit-history-loading">
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        )}

        {error && (
          <div
            className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
            data-testid="edit-history-error"
          >
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>
              {error instanceof Error
                ? error.message
                : 'Failed to load edit history.'}
            </span>
          </div>
        )}

        {data && <EditHistoryBody data={data} />}
      </DialogContent>
    </Dialog>
  )
}

// ─── Body (split for testability) ─────────────────────────────────────────────

export function EditHistoryBody({ data }: { data: CommentEditHistoryResponse }) {
  const editCount = data.edits.length

  if (editCount === 0) {
    return (
      <div className="space-y-4">
        <section data-testid="edit-history-current">
          <h3 className="text-sm font-medium mb-2 flex items-center gap-2">
            Current body
            <Badge variant="outline" className="text-[10px] px-1.5 py-0">
              now
            </Badge>
          </h3>
          <pre className="whitespace-pre-wrap break-words rounded-md border bg-muted/40 p-3 text-sm font-mono">
            {data.current_body}
          </pre>
        </section>

        <p
          className="text-sm text-muted-foreground italic"
          data-testid="edit-history-empty"
        >
          This comment has never been edited.
        </p>
      </div>
    )
  }

  // Build a transition list: for each edit row, the "next" body is either the
  // next edit's old_body (for older edits) or the current body (for the most
  // recent edit). Rendered newest-first so the walkback moves top→down toward
  // the original body.
  const newestFirst = [...data.edits].reverse()
  const transitions = newestFirst.map((edit, idx) => {
    // The body that REPLACED this old_body. For the newest edit, that's the
    // current body; for older edits, it's the old_body of the more-recent one.
    const nextBody =
      idx === 0 ? data.current_body : newestFirst[idx - 1].old_body
    return { edit, nextBody }
  })

  return (
    <div className="space-y-5">
      <section data-testid="edit-history-current">
        <h3 className="text-sm font-medium mb-2 flex items-center gap-2">
          Current body
          <Badge variant="outline" className="text-[10px] px-1.5 py-0">
            {editCount} edit{editCount !== 1 ? 's' : ''}
          </Badge>
        </h3>
        <pre className="whitespace-pre-wrap break-words rounded-md border bg-muted/40 p-3 text-sm font-mono">
          {data.current_body}
        </pre>
      </section>

      <ol className="space-y-5" data-testid="edit-history-list">
        {transitions.map(({ edit, nextBody }) => (
          <EditTransition key={edit.id} edit={edit} nextBody={nextBody} />
        ))}
      </ol>
    </div>
  )
}

// ─── Single transition row (before/after) ─────────────────────────────────────

function EditTransition({
  edit,
  nextBody,
}: {
  edit: CommentEditHistoryEntry
  nextBody: string
}) {
  // editor_name is server-resolved via the canonical chain (PSY-612), so
  // "unknown editor" only fires for absent / anonymous payloads.
  const editorLabel = edit.editor_username
    ? `@${edit.editor_username}`
    : edit.editor_name || 'unknown editor'

  return (
    <li
      className="rounded-md border bg-background p-3 space-y-3"
      data-testid="edit-history-entry"
    >
      <header className="flex items-center gap-2 text-xs text-muted-foreground">
        <Clock className="h-3.5 w-3.5" />
        <span data-testid="edit-history-editor">{editorLabel}</span>
        <span aria-hidden="true">·</span>
        <time dateTime={edit.edited_at} title={edit.edited_at}>
          {formatRelativeTime(edit.edited_at)}
        </time>
      </header>

      <div className="grid gap-2 md:grid-cols-2">
        <div data-testid="edit-history-before">
          <div className="text-[10px] uppercase tracking-wide text-muted-foreground mb-1">
            Previous
          </div>
          <pre className="whitespace-pre-wrap break-words rounded border bg-red-500/5 border-red-500/20 p-2 text-xs font-mono">
            {edit.old_body}
          </pre>
        </div>
        <div data-testid="edit-history-after">
          <div className="text-[10px] uppercase tracking-wide text-muted-foreground mb-1">
            Replaced with
          </div>
          <pre className="whitespace-pre-wrap break-words rounded border bg-green-500/5 border-green-500/20 p-2 text-xs font-mono">
            {nextBody}
          </pre>
        </div>
      </div>
    </li>
  )
}
