'use client'

import { useState } from 'react'
import { ChevronUp, ChevronDown, MessageSquare, Star, CheckCircle, Eye, EyeOff, Flag, Clock, Pencil, Trash2, History } from 'lucide-react'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { UserAttribution } from '@/components/shared'
import { CommentForm } from './CommentForm'
import { CommentEditHistory } from './CommentEditHistory'
import { MutationErrorBanner } from './MutationErrorBanner'
import { ReportEntityDialog } from '@/features/contributions'
import {
  useReplyToComment,
  useUpdateComment,
  useDeleteComment,
  useVoteComment,
  useUnvoteComment,
  useCommentThread,
  useAutoDismissError,
  formatCommentSubmissionError,
} from '../hooks'
import type { Comment } from '../types'

interface ShowArtist {
  id: number
  name: string
}

interface FieldNoteCardProps {
  comment: Comment
  showId: number
  artists?: ShowArtist[]
  replies?: Comment[]
}

function StarDisplay({ value, testId }: { value: number; testId?: string }) {
  return (
    <span className="inline-flex items-center gap-0.5" data-testid={testId}>
      {[1, 2, 3, 4, 5].map((star) => (
        <Star
          key={star}
          className={`h-3.5 w-3.5 ${
            star <= value
              ? 'fill-yellow-400 text-yellow-400'
              : 'text-muted-foreground/30'
          }`}
        />
      ))}
    </span>
  )
}

export function FieldNoteCard({
  comment,
  showId,
  artists = [],
  replies = [],
}: FieldNoteCardProps) {
  const { user, isAuthenticated } = useAuthContext()
  const currentUserId = user?.id ? Number(user.id) : null
  const isOwner = currentUserId === comment.user_id
  const isAdmin = Boolean(user?.is_admin)

  const [isReplying, setIsReplying] = useState(false)
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteConfirm, setIsDeleteConfirm] = useState(false)
  const [showSpoiler, setShowSpoiler] = useState(false)
  const [showReplies, setShowReplies] = useState(true)
  const [loadedThread, setLoadedThread] = useState(false)
  const [isReportOpen, setIsReportOpen] = useState(false)
  // PSY-590: admin edit history viewer — gated by is_admin and only fetched
  // when the dialog is opened (mirrors CommentCard / PSY-297 pattern).
  const [isEditHistoryOpen, setIsEditHistoryOpen] = useState(false)

  const replyMutation = useReplyToComment()
  const updateMutation = useUpdateComment()
  const deleteMutation = useDeleteComment()
  const voteMutation = useVoteComment()
  const unvoteMutation = useUnvoteComment()
  // PSY-608: optimistic vote/unvote rollback hides the failure visually.
  // Show a brief auto-dismissing banner so the user knows the action was
  // reverted, mirroring SaveButton / FavoriteVenueButton (~3s).
  const voteError = useAutoDismissError()

  const hasInlineReplies = replies.length > 0
  const { data: threadData } = useCommentThread(comment.id, loadedThread && !hasInlineReplies)
  const threadReplies = hasInlineReplies ? replies : (threadData?.replies ?? [])

  const sd = comment.structured_data
  const isSpoiler = sd?.setlist_spoiler === true
  const isVerified = sd?.is_verified_attendee === true

  // Find artist name from show_artist_id
  const artistName = sd?.show_artist_id
    ? artists.find((a) => a.id === sd.show_artist_id)?.name
    : null

  const handleVote = (direction: 1 | -1) => {
    if (!isAuthenticated) return
    if (comment.user_vote === direction) {
      unvoteMutation.mutate(
        { commentId: comment.id, entityType: 'show', entityId: showId },
        { onError: (err) => voteError.show(err) }
      )
    } else {
      voteMutation.mutate(
        { commentId: comment.id, direction, entityType: 'show', entityId: showId },
        { onError: (err) => voteError.show(err) }
      )
    }
  }

  const handleReply = (body: string) => {
    replyMutation.mutate(
      { commentId: comment.id, body, entityType: 'show', entityId: showId },
      { onSuccess: () => setIsReplying(false) }
    )
  }

  // PSY-590: edit + delete mirror the CommentCard wiring. The backend
  // PUT/DELETE /comments/{id} endpoints operate on the row regardless of
  // kind, so field-note edits go through the same useUpdateComment hook
  // and inherit the comment_edits history (admin-visible via PSY-297).
  // Structured fields (sound/crowd/notable/etc.) are intentionally NOT
  // editable here — only the body, matching the existing comment-edit
  // surface and the "mirror comment behavior" decision on this ticket.
  const handleEdit = (body: string) => {
    updateMutation.mutate(
      { commentId: comment.id, body, entityType: 'show', entityId: showId },
      { onSuccess: () => setIsEditing(false) }
    )
  }

  const handleDelete = () => {
    deleteMutation.mutate(
      { commentId: comment.id, entityType: 'show', entityId: showId },
      { onSuccess: () => setIsDeleteConfirm(false) }
    )
  }

  const isDeleted = comment.visibility === 'hidden_by_user' || comment.visibility === 'hidden_by_mod'

  if (isDeleted) {
    return (
      <div className="py-3 text-sm text-muted-foreground italic" data-testid="field-note-deleted">
        {comment.visibility === 'hidden_by_user' ? '[deleted]' : '[removed]'}
      </div>
    )
  }

  return (
    <div data-testid="field-note-card" className="rounded-lg border border-border/50 bg-card p-4">
      <div className="flex items-center gap-2 text-sm">
        <UserAttribution
          name={comment.author_name}
          username={comment.author_username}
          className="font-medium text-foreground hover:underline"
          testId={comment.author_username ? 'field-note-author-link' : 'field-note-author-name'}
        />
        {isVerified && (
          <Badge
            variant="secondary"
            className="text-[10px] px-1.5 py-0 bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400"
            data-testid="verified-badge"
          >
            <CheckCircle className="h-3 w-3 mr-0.5" />
            Verified Attendee
          </Badge>
        )}
        <span className="text-muted-foreground">
          {formatRelativeTime(comment.created_at)}
        </span>
        {comment.is_edited && (
          <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
            Edited
          </Badge>
        )}
        {/* PSY-513: pending-review badge for the author of a queued field
            note. Mirrors the CommentCard pattern. */}
        {comment.visibility === 'pending_review' && isOwner && (
          <Badge
            variant="outline"
            className="text-[10px] px-1.5 py-0 gap-1 border-amber-700/50 text-amber-500"
            data-testid="pending-review-badge"
          >
            <Clock className="h-2.5 w-2.5" />
            Pending review
          </Badge>
        )}
      </div>

      {/* Artist attribution + song position */}
      {(artistName || sd?.song_position) && (
        <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
          {artistName && (
            <span data-testid="artist-attribution">
              During {artistName}&apos;s set
            </span>
          )}
          {artistName && sd?.song_position && <span>&middot;</span>}
          {sd?.song_position && (
            <span data-testid="song-position">Song #{sd.song_position}</span>
          )}
        </div>
      )}

      {/* Structured data display: ratings */}
      {(sd?.sound_quality || sd?.crowd_energy) && (
        <div className="flex items-center gap-4 mt-2" data-testid="ratings-display">
          {sd?.sound_quality && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span>Sound:</span>
              <StarDisplay value={sd.sound_quality} testId="sound-quality-display" />
            </div>
          )}
          {sd?.crowd_energy && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span>Crowd:</span>
              <StarDisplay value={sd.crowd_energy} testId="crowd-energy-display" />
            </div>
          )}
        </div>
      )}

      {/* Body — with spoiler handling. PSY-590: edit mode replaces the
          rendered body with a CommentForm; only the body field is editable
          (mirrors comment-edit). Structured fields (sound/crowd/notable/etc.)
          remain saved as-is on the row. */}
      {isEditing ? (
        <div className="mt-2">
          <CommentForm
            onSubmit={handleEdit}
            initialBody={comment.body}
            submitLabel="Save"
            onCancel={() => setIsEditing(false)}
            isPending={updateMutation.isPending}
            errorMessage={formatCommentSubmissionError(updateMutation.error)}
          />
        </div>
      ) : isSpoiler && !showSpoiler ? (
        <div className="mt-2" data-testid="spoiler-gate">
          <button
            onClick={() => setShowSpoiler(true)}
            className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            <EyeOff className="h-4 w-4" />
            Contains setlist spoilers — click to reveal
          </button>
        </div>
      ) : (
        <div className="mt-2">
          {isSpoiler && (
            <button
              onClick={() => setShowSpoiler(false)}
              className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors mb-1"
              data-testid="spoiler-hide"
            >
              <Eye className="h-3.5 w-3.5" />
              Hide spoilers
            </button>
          )}
          <div
            className="text-sm prose prose-sm dark:prose-invert max-w-none"
            dangerouslySetInnerHTML={{ __html: comment.body_html }}
            data-testid="field-note-body"
          />
        </div>
      )}

      {/* Notable moments */}
      {sd?.notable_moments && (!isSpoiler || showSpoiler) && (
        <div
          className="mt-2 rounded-md bg-primary/5 border border-primary/10 px-3 py-2 text-sm"
          data-testid="notable-moments"
        >
          <span className="font-medium text-xs text-muted-foreground uppercase tracking-wider">
            Notable:
          </span>{' '}
          {sd.notable_moments}
        </div>
      )}

      {/* PSY-608: auto-dismiss banner for vote/unvote failures. The
          optimistic-rollback restores the cached state silently; without
          this, the user sees the icon flip back with no explanation. */}
      {!isEditing && voteError.error !== null && (
        <MutationErrorBanner
          testId="vote-error-banner"
          marginTop="mt-3"
          message={
            formatCommentSubmissionError(voteError.error) ??
            'Vote failed. Please try again.'
          }
        />
      )}

      {/* PSY-590: sticky banner for delete failures — mirrors CommentCard
          (the inline edit-form banner is owned by CommentForm via
          errorMessage above). */}
      {!isEditing && deleteMutation.isError && (
        <MutationErrorBanner
          testId="delete-error-banner"
          message={
            formatCommentSubmissionError(deleteMutation.error) ??
            'Failed to delete field note. Please try again.'
          }
        />
      )}

      {/* Actions row: votes + reply + edit + delete + report */}
      {!isEditing && (
        <div className="flex items-center gap-1 mt-3">
          {/* Vote buttons. PSY-593: authors cannot vote on their own field
              notes (matches HN/Lobsters / CommentCard). Hide the up/down
              buttons on own comments and render the score as a plain span. */}
          {!isOwner ? (
            <>
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
              <span className="text-xs font-medium min-w-[1.5rem] text-center" data-testid="vote-score">
                {comment.ups - comment.downs}
              </span>
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
            </>
          ) : (
            <span
              className="text-xs font-medium min-w-[1.5rem] text-center text-muted-foreground"
              data-testid="vote-score"
            >
              {comment.ups - comment.downs}
            </span>
          )}

          {/* Reply button */}
          {isAuthenticated && comment.depth < 2 && comment.reply_permission !== 'author_only' && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs text-muted-foreground"
              onClick={() => setIsReplying(!isReplying)}
            >
              <MessageSquare className="h-3.5 w-3.5 mr-1" />
              Reply
            </Button>
          )}

          {/* PSY-590: Edit button (own field notes). Mirrors CommentCard. */}
          {isOwner && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs text-muted-foreground"
              onClick={() => setIsEditing(true)}
              data-testid="edit-field-note-button"
            >
              <Pencil className="h-3.5 w-3.5 mr-1" />
              Edit
            </Button>
          )}

          {/* PSY-590: Delete button (own field notes) with inline Yes/No
              confirmation, mirroring CommentCard. */}
          {isOwner && !isDeleteConfirm && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs text-muted-foreground"
              onClick={() => setIsDeleteConfirm(true)}
              data-testid="delete-field-note-button"
            >
              <Trash2 className="h-3.5 w-3.5 mr-1" />
              Delete
            </Button>
          )}

          {isDeleteConfirm && (
            <div className="flex items-center gap-1 ml-1" data-testid="delete-field-note-confirm">
              <span className="text-xs text-destructive">Delete?</span>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs text-destructive"
                onClick={handleDelete}
                disabled={deleteMutation.isPending}
                data-testid="delete-field-note-yes"
              >
                Yes
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs text-muted-foreground"
                onClick={() => setIsDeleteConfirm(false)}
                data-testid="delete-field-note-no"
              >
                No
              </Button>
            </div>
          )}

          {/* Report button (non-owner, authenticated) */}
          {isAuthenticated && !isOwner && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs text-muted-foreground"
              onClick={() => setIsReportOpen(true)}
              data-testid="report-field-note-button"
            >
              <Flag className="h-3.5 w-3.5 mr-1" />
              Report
            </Button>
          )}
        </div>
      )}

      {/* PSY-590: admin-only edit history trigger. Mirrors CommentCard
          (PSY-297) — gated on is_admin and rendered only when at least one
          edit has been recorded. */}
      {!isEditing && isAdmin && comment.edit_count > 0 && (
        <div className="mt-1 pt-1 border-t border-border/40 flex items-center">
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-[11px] text-muted-foreground hover:text-foreground"
            onClick={() => setIsEditHistoryOpen(true)}
            data-testid="admin-edit-history-button"
            aria-label="View edit history"
          >
            <History className="h-3 w-3 mr-1" />
            Edit history ({comment.edit_count})
          </Button>
        </div>
      )}

      {/* Inline reply form. PSY-608: surface 4xx (e.g. 429) inline so reply
          mutations don't fail silently — same pattern as CommentCard. */}
      {isReplying && (
        <div className="mt-3 ml-4">
          <CommentForm
            onSubmit={handleReply}
            placeholder={`Reply to ${comment.author_name}...`}
            submitLabel="Reply"
            onCancel={() => setIsReplying(false)}
            isPending={replyMutation.isPending}
            errorMessage={formatCommentSubmissionError(replyMutation.error)}
          />
        </div>
      )}

      {/* Nested replies */}
      {threadReplies.length > 0 && (
        <div className="mt-2">
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-1 text-xs text-muted-foreground"
            onClick={() => setShowReplies(!showReplies)}
          >
            {showReplies ? 'Hide' : 'Show'} {threadReplies.length}{' '}
            {threadReplies.length === 1 ? 'reply' : 'replies'}
          </Button>

          {showReplies && (
            <div className="mt-1 space-y-3 border-l-2 border-border/50 pl-3">
              {threadReplies.map((reply) => (
                <FieldNoteCard
                  key={reply.id}
                  comment={reply}
                  showId={showId}
                  artists={artists}
                />
              ))}
            </div>
          )}
        </div>
      )}

      {/* Load replies button. PSY-514: same gating as CommentCard — suppress
          when reply_count is 0 (or missing), otherwise the click reads as a
          no-op since there are no replies to fetch. */}
      {!hasInlineReplies &&
        !loadedThread &&
        comment.depth === 0 &&
        (comment.reply_count ?? 0) > 0 && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-1 text-xs text-muted-foreground mt-1"
            onClick={() => setLoadedThread(true)}
            data-testid="show-replies-button"
          >
            <MessageSquare className="h-3.5 w-3.5 mr-1" />
            Show replies
          </Button>
        )}

      {/* Report dialog */}
      {isAuthenticated && !isOwner && (
        <ReportEntityDialog
          open={isReportOpen}
          onOpenChange={setIsReportOpen}
          entityType="comment"
          entityId={comment.id}
          entityName={`Field note by ${comment.author_name}`}
        />
      )}

      {/* PSY-590: admin edit history dialog. Mounted on-demand so we don't
          fetch history for every field note on the page (mirrors PSY-297). */}
      {isAdmin && isEditHistoryOpen && (
        <CommentEditHistory
          open={isEditHistoryOpen}
          onOpenChange={setIsEditHistoryOpen}
          commentId={comment.id}
        />
      )}
    </div>
  )
}
