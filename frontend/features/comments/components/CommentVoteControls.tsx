'use client'

import { type ReactNode } from 'react'
import { ChevronUp, ChevronDown } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { MutationErrorBanner } from './MutationErrorBanner'
import {
  useVoteComment,
  useUnvoteComment,
  formatCommentSubmissionError,
} from '../hooks'
import { useAutoDismissBanner } from '@/lib/hooks/common/useAutoDismissBanner'
import { type Comment } from '../types'

interface CommentVoteControlsProps {
  /** Comment or field note row being voted on. Reads `id`, `user_id`,
   *  `user_vote`, `ups`, `downs`. */
  comment: Comment
  /** Entity polymorphic discriminator (`'show'`, `'artist'`, …). Forwarded
   *  to the vote/unvote mutation cache-keys so optimistic updates land on
   *  the correct cached list. */
  entityType: string
  /** Entity id forwarded to the mutation cache-keys. */
  entityId: number
  /** Action-row children placed AFTER the chevrons + score, inside the
   *  same flex row. Cards pass their Reply / Edit / Delete / Report
   *  buttons here so the row layout is preserved. */
  children?: ReactNode
  /** Margin-top for both the action row and the inline error banner.
   *  CommentCard uses the default `mt-2`; FieldNoteCard uses `mt-3` to
   *  match its pre-extraction visual rhythm. */
  marginTop?: 'mt-2' | 'mt-3'
}

/**
 * Vote chevrons + score + auto-dismiss error banner, extracted from
 * CommentCard and FieldNoteCard (PSY-632). Encapsulates:
 *
 *  - The two mutation hooks (`useVoteComment` / `useUnvoteComment`) and the
 *    toggle-vs-set logic (clicking the same direction unvotes).
 *  - The PSY-593 "authors don't see vote buttons on their own rows" rule
 *    — Upvote and Downvote are hidden when the viewer is the author; the
 *    score still renders, muted, so authors can see their own score.
 *  - The PSY-608 auto-dismiss banner that surfaces the optimistic-rollback
 *    failure (without the banner, the icon flips back with no explanation).
 *
 * Layout: the primitive renders the action-row `<div>` itself; consumers
 * pass their remaining action buttons (Reply / Edit / Delete / Report) as
 * `children`, which sit inside the same flex row immediately after the
 * chevrons. The banner is a sibling below the row so it doesn't disrupt
 * the inline flex.
 *
 * Anonymous viewers see the chevrons (disabled) so the affordance stays
 * discoverable; clicks are no-ops via the auth gate in `handleVote`. The
 * `disabled` attribute also blocks the keyboard path.
 */
export function CommentVoteControls({
  comment,
  entityType,
  entityId,
  children,
  marginTop = 'mt-2',
}: CommentVoteControlsProps) {
  const { user, isAuthenticated } = useAuthContext()
  const currentUserId = user?.id ? Number(user.id) : null
  const isOwner = currentUserId === comment.user_id

  const voteMutation = useVoteComment()
  const unvoteMutation = useUnvoteComment()
  // PSY-608: optimistic vote/unvote rollback hides the failure visually.
  // Show a brief auto-dismissing banner so the user knows the action was
  // reverted, mirroring SaveButton / FavoriteVenueButton (~3s). PSY-958:
  // routed through the shared useAutoDismissBanner primitive (was a
  // comments-local useAutoDismissError, now removed).
  const { value: voteError, show: showVoteError } =
    useAutoDismissBanner<unknown>(3000)

  const handleVote = (direction: 1 | -1) => {
    if (!isAuthenticated) return
    if (comment.user_vote === direction) {
      unvoteMutation.mutate(
        { commentId: comment.id, entityType, entityId },
        { onError: (err) => showVoteError(err) }
      )
    } else {
      voteMutation.mutate(
        { commentId: comment.id, direction, entityType, entityId },
        { onError: (err) => showVoteError(err) }
      )
    }
  }

  return (
    <>
      <div className={`flex items-center gap-1 ${marginTop}`}>
        {/* PSY-593: authors don't see vote buttons on their own rows
            (matches HN/Lobsters — authors must not self-promote). Score
            still renders so the author can see it. */}
        {!isOwner && (
          <Button
            variant="ghost"
            size="sm"
            className={`h-7 w-7 p-0 ${comment.user_vote === 1 ? 'text-primary' : 'text-muted-foreground'}`}
            onClick={() => handleVote(1)}
            disabled={!isAuthenticated}
            aria-label="Upvote"
            data-testid="upvote-button"
          >
            <ChevronUp className="h-4 w-4" />
          </Button>
        )}
        <span
          className={`text-xs font-medium min-w-[1.5rem] text-center ${isOwner ? 'text-muted-foreground' : ''}`}
          data-testid="vote-score"
        >
          {comment.ups - comment.downs}
        </span>
        {!isOwner && (
          <Button
            variant="ghost"
            size="sm"
            className={`h-7 w-7 p-0 ${comment.user_vote === -1 ? 'text-destructive' : 'text-muted-foreground'}`}
            onClick={() => handleVote(-1)}
            disabled={!isAuthenticated}
            aria-label="Downvote"
            data-testid="downvote-button"
          >
            <ChevronDown className="h-4 w-4" />
          </Button>
        )}
        {children}
      </div>

      {/* PSY-608: auto-dismiss banner for vote/unvote failures. The
          optimistic-rollback restores the cached state silently; without
          this, the user sees the icon flip back with no explanation. */}
      {voteError !== null && (
        <MutationErrorBanner
          testId="vote-error-banner"
          marginTop={marginTop}
          message={
            formatCommentSubmissionError(voteError) ??
            'Vote failed. Please try again.'
          }
        />
      )}
    </>
  )
}
