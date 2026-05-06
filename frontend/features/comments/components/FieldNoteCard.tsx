'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ChevronUp, ChevronDown, MessageSquare, Star, CheckCircle, Eye, EyeOff, Flag, Clock } from 'lucide-react'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { CommentForm } from './CommentForm'
import { MutationErrorBanner } from './MutationErrorBanner'
import { ReportEntityDialog } from '@/features/contributions'
import {
  useReplyToComment,
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

  const [isReplying, setIsReplying] = useState(false)
  const [showSpoiler, setShowSpoiler] = useState(false)
  const [showReplies, setShowReplies] = useState(true)
  const [loadedThread, setLoadedThread] = useState(false)
  const [isReportOpen, setIsReportOpen] = useState(false)

  const replyMutation = useReplyToComment()
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
      {/* Header: author + verified badge + timestamp. PSY-552: link byline
          to the author's profile when author_username is set; otherwise
          plain text. */}
      <div className="flex items-center gap-2 text-sm">
        {comment.author_username ? (
          <Link
            href={`/users/${comment.author_username}`}
            className="font-medium text-foreground hover:underline"
            data-testid="field-note-author-link"
          >
            {comment.author_name}
          </Link>
        ) : (
          <span
            className="font-medium text-foreground"
            data-testid="field-note-author-name"
          >
            {comment.author_name}
          </span>
        )}
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

      {/* Body — with spoiler handling */}
      {isSpoiler && !showSpoiler ? (
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
      {voteError.error !== null && (
        <MutationErrorBanner
          testId="vote-error-banner"
          marginTop="mt-3"
          message={
            formatCommentSubmissionError(voteError.error) ??
            'Vote failed. Please try again.'
          }
        />
      )}

      {/* Actions row: votes + reply + report */}
      <div className="flex items-center gap-1 mt-3">
        {/* Vote buttons */}
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
    </div>
  )
}
