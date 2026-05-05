'use client'

import { useState } from 'react'
import { MessageSquare, Clock } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import {
  useComments,
  useCreateComment,
  formatCommentSubmissionError,
} from '../hooks'
import { CommentForm } from './CommentForm'
import { CommentCard } from './CommentCard'
import type { Comment, ReplyPermission } from '../types'

interface CommentThreadProps {
  entityType: string
  entityId: number
}

type SortOption = 'best' | 'new' | 'top'

const sortLabels: Record<SortOption, string> = {
  best: 'Best',
  new: 'New',
  top: 'Top',
}

export function CommentThread({ entityType, entityId }: CommentThreadProps) {
  const { isAuthenticated } = useAuthContext()
  const [sort, setSort] = useState<SortOption>('best')
  // PSY-513: track the author's just-submitted pending-review comment so we
  // can render it optimistically. The public comments list will not include
  // pending_review rows (server-side filter), so this local state is the
  // source of truth until a moderator approves it (after which a refetch
  // surfaces the canonical row and the optimistic entry is de-duped by id).
  const [pendingComment, setPendingComment] = useState<Comment | null>(null)
  // PSY-589: bumped on every successful submit so the form can clear its
  // textarea via `resetSignal`. The form keeps the draft on error so the
  // user can retry without retyping.
  const [submitGeneration, setSubmitGeneration] = useState(0)

  const { data, isLoading } = useComments(entityType, entityId, sort)
  const createMutation = useCreateComment()

  const comments = data?.comments ?? []
  const total = data?.total ?? 0

  // Drop the optimistic entry once the canonical row appears in the list.
  const hasCanonicalPending =
    pendingComment !== null && comments.some((c) => c.id === pendingComment.id)
  const effectivePending = hasCanonicalPending ? null : pendingComment

  // Separate top-level comments and replies
  const topLevel = comments.filter((c) => c.depth === 0)
  const repliesByParent = comments.reduce<Record<number, Comment[]>>((acc, c) => {
    if (c.parent_id) {
      if (!acc[c.parent_id]) acc[c.parent_id] = []
      acc[c.parent_id].push(c)
    }
    return acc
  }, {})

  const handleCreate = (body: string, replyPermission?: ReplyPermission) => {
    createMutation.mutate(
      { entityType, entityId, body, replyPermission },
      {
        onSuccess: (created) => {
          // Only top-level (parent_id == null) submissions land here; replies
          // go through useReplyToComment in CommentCard.
          if (created.visibility === 'pending_review') {
            setPendingComment(created)
          }
          // PSY-589: clear the form ONLY on success. On 4xx the form
          // retains the draft so the user can retry.
          setSubmitGeneration((g) => g + 1)
        },
      }
    )
  }

  return (
    <section className="mt-8" data-testid="comment-thread">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <MessageSquare className="h-5 w-5" />
          Discussion
          {total > 0 && (
            <span className="text-sm font-normal text-muted-foreground">
              ({total})
            </span>
          )}
        </h2>

        {/* Sort selector */}
        {comments.length > 0 && (
          <div className="flex items-center gap-1">
            {(Object.keys(sortLabels) as SortOption[]).map((option) => (
              <Button
                key={option}
                variant={sort === option ? 'secondary' : 'ghost'}
                size="sm"
                className="h-7 px-2 text-xs"
                onClick={() => setSort(option)}
              >
                {sortLabels[option]}
              </Button>
            ))}
          </div>
        )}
      </div>

      {/* Comment form for authenticated users */}
      {isAuthenticated ? (
        <div className="mb-6">
          <CommentForm
            onSubmit={handleCreate}
            placeholder="Share your thoughts..."
            isPending={createMutation.isPending}
            allowReplyPermission
            errorMessage={formatCommentSubmissionError(createMutation.error)}
            resetSignal={submitGeneration}
          />
        </div>
      ) : (
        <p className="text-sm text-muted-foreground mb-6" data-testid="auth-gate">
          <a href="/login" className="text-primary hover:underline">
            Sign in
          </a>{' '}
          to join the discussion.
        </p>
      )}

      {/* PSY-513: pending-review confirmation banner. Inline banner pattern
          mirrors EntityEditDrawer's "Edit submitted for review" success state
          since the codebase has no toast primitive. Only the author sees this. */}
      {effectivePending && (
        <div
          className="mb-4 rounded-md border border-amber-700/50 bg-amber-950/40 p-3 flex items-start gap-2"
          role="status"
          data-testid="pending-review-banner"
        >
          <Clock className="h-4 w-4 mt-0.5 text-amber-500 shrink-0" />
          <p className="text-sm text-amber-200">
            Comment submitted — awaiting moderation. You&apos;ll see it here once an admin approves it.
          </p>
        </div>
      )}

      {/* Comments list */}
      {isLoading ? (
        <div className="space-y-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="animate-pulse space-y-2">
              <div className="h-3 w-32 bg-muted rounded" />
              <div className="h-4 w-full bg-muted rounded" />
              <div className="h-4 w-3/4 bg-muted rounded" />
            </div>
          ))}
        </div>
      ) : topLevel.length === 0 && !effectivePending ? (
        <p className="text-sm text-muted-foreground py-8 text-center" data-testid="empty-state">
          No comments yet. Be the first to share your thoughts.
        </p>
      ) : (
        <div className="space-y-4 divide-y divide-border/50">
          {/* Optimistic pending comment, rendered first so the author can see
              what they posted. Visible only to the author (gated above by
              setPendingComment, which only fires for the submitter). */}
          {effectivePending && (
            <div className="pt-4 first:pt-0">
              <CommentCard
                comment={effectivePending}
                entityType={entityType}
                entityId={entityId}
              />
            </div>
          )}
          {topLevel.map((comment) => (
            <div key={comment.id} className="pt-4 first:pt-0">
              <CommentCard
                comment={comment}
                entityType={entityType}
                entityId={entityId}
                replies={repliesByParent[comment.id] ?? []}
              />
            </div>
          ))}
        </div>
      )}

      {/* Load more */}
      {data?.has_more && (
        <div className="mt-4 text-center">
          <Button variant="outline" size="sm">
            Load more comments
          </Button>
        </div>
      )}
    </section>
  )
}
