'use client'

import { useState } from 'react'
import { ChevronUp, ChevronDown, MessageSquare, Pencil, Trash2, ChevronRight, Flag, History, Lock, Clock } from 'lucide-react'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { CommentForm } from './CommentForm'
import { CommentEditHistory } from './CommentEditHistory'
import { ReplyPermissionSelect } from './ReplyPermissionSelect'
import { ReportEntityDialog } from '@/features/contributions'
import {
  useReplyToComment,
  useUpdateComment,
  useUpdateReplyPermission,
  useDeleteComment,
  useVoteComment,
  useUnvoteComment,
  useCommentThread,
} from '../hooks'
import {
  REPLY_PERMISSION_BADGE_LABELS,
  type Comment,
  type ReplyPermission,
} from '../types'

interface CommentCardProps {
  comment: Comment
  entityType: string
  entityId: number
  /** Nested replies already loaded at the top-level list */
  replies?: Comment[]
}

export function CommentCard({
  comment,
  entityType,
  entityId,
  replies = [],
}: CommentCardProps) {
  const { user, isAuthenticated } = useAuthContext()
  const currentUserId = user?.id ? Number(user.id) : null
  const isOwner = currentUserId === comment.user_id
  const isAdmin = Boolean(user?.is_admin)

  const [isReplying, setIsReplying] = useState(false)
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteConfirm, setIsDeleteConfirm] = useState(false)
  const [showReplies, setShowReplies] = useState(true)
  const [loadedThread, setLoadedThread] = useState(false)
  const [isReportOpen, setIsReportOpen] = useState(false)
  // PSY-297: admin edit history viewer. Gated by is_admin and only fetched
  // when the dialog is opened (hook is `enabled` on open).
  const [isEditHistoryOpen, setIsEditHistoryOpen] = useState(false)

  const replyMutation = useReplyToComment()
  const updateMutation = useUpdateComment()
  const updateReplyPermissionMutation = useUpdateReplyPermission()
  const deleteMutation = useDeleteComment()
  const voteMutation = useVoteComment()
  const unvoteMutation = useUnvoteComment()

  // Load thread on demand if no inline replies were provided
  const hasInlineReplies = replies.length > 0
  const { data: threadData } = useCommentThread(comment.id, loadedThread && !hasInlineReplies)
  const threadReplies = hasInlineReplies ? replies : (threadData?.replies ?? [])

  const isDeleted = comment.visibility === 'hidden_by_user' || comment.visibility === 'hidden_by_mod'

  const handleVote = (direction: 1 | -1) => {
    if (!isAuthenticated) return
    if (comment.user_vote === direction) {
      // Toggle off
      unvoteMutation.mutate({ commentId: comment.id, entityType, entityId })
    } else {
      voteMutation.mutate({ commentId: comment.id, direction, entityType, entityId })
    }
  }

  const handleReply = (body: string, replyPermission?: ReplyPermission) => {
    replyMutation.mutate(
      {
        commentId: comment.id,
        body,
        entityType,
        entityId,
        replyPermission,
      },
      { onSuccess: () => setIsReplying(false) }
    )
  }

  // PSY-296: owner changes who can reply to this comment.
  const handleChangeReplyPermission = (next: ReplyPermission) => {
    if (next === comment.reply_permission) return
    updateReplyPermissionMutation.mutate({
      commentId: comment.id,
      permission: next,
      entityType,
      entityId,
    })
  }

  const handleEdit = (body: string) => {
    updateMutation.mutate(
      { commentId: comment.id, body, entityType, entityId },
      { onSuccess: () => setIsEditing(false) }
    )
  }

  const handleDelete = () => {
    deleteMutation.mutate(
      { commentId: comment.id, entityType, entityId },
      { onSuccess: () => setIsDeleteConfirm(false) }
    )
  }

  // Indentation based on depth (max depth 2)
  const depthMargin = comment.depth > 0 ? `ml-${Math.min(comment.depth, 2) * 8}` : ''

  if (isDeleted) {
    return (
      <div className={`${depthMargin} py-3 text-sm text-muted-foreground italic`} data-testid="comment-deleted">
        {comment.visibility === 'hidden_by_user' ? '[deleted]' : '[removed]'}
      </div>
    )
  }

  return (
    <div className={depthMargin} data-testid="comment-card">
      {/* Header: author + timestamp */}
      <div className="flex items-center gap-2 text-sm">
        <span className="font-medium text-foreground">{comment.author_name}</span>
        <span className="text-muted-foreground">
          {formatRelativeTime(comment.created_at)}
        </span>
        {comment.is_edited && (
          <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
            Edited
          </Badge>
        )}
        {/* PSY-513: pending-review badge for the author of a queued comment.
            Gated on isOwner so other viewers don't see queued comments even if
            one slips into the cache. The badge mirrors the existing reply-
            permission badge pattern (outline + small icon). */}
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
        {/* PSY-296 start: reply-permission badge (only for non-default values). */}
        {comment.reply_permission !== 'anyone' &&
          REPLY_PERMISSION_BADGE_LABELS[
            comment.reply_permission as ReplyPermission
          ] && (
            <Badge
              variant="outline"
              className="text-[10px] px-1.5 py-0 gap-1"
              data-testid="reply-permission-badge"
            >
              <Lock className="h-2.5 w-2.5" />
              {REPLY_PERMISSION_BADGE_LABELS[comment.reply_permission as ReplyPermission]}
            </Badge>
          )}
        {/* PSY-296 end */}
      </div>

      {/* Body */}
      {isEditing ? (
        <div className="mt-2">
          <CommentForm
            onSubmit={handleEdit}
            initialBody={comment.body}
            submitLabel="Save"
            onCancel={() => setIsEditing(false)}
            isPending={updateMutation.isPending}
          />
        </div>
      ) : (
        <div
          className="mt-1 text-sm prose prose-sm dark:prose-invert max-w-none"
          dangerouslySetInnerHTML={{ __html: comment.body_html }}
        />
      )}

      {/* Actions row: votes + reply + edit + delete */}
      {!isEditing && (
        <div className="flex items-center gap-1 mt-2">
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

          {/* Reply button (hidden at depth >= 2). PSY-296: for
              author_only we hide the button for non-authors; for followers
              we show the button and let the server reject with 403 (the
              frontend has no public is-following check yet). */}
          {isAuthenticated &&
            comment.depth < 2 &&
            (comment.reply_permission !== 'author_only' || isOwner) && (
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

          {/* Edit button (own comments) */}
          {isOwner && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs text-muted-foreground"
              onClick={() => setIsEditing(true)}
            >
              <Pencil className="h-3.5 w-3.5 mr-1" />
              Edit
            </Button>
          )}

          {/* Delete button (own comments) */}
          {isOwner && !isDeleteConfirm && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs text-muted-foreground"
              onClick={() => setIsDeleteConfirm(true)}
            >
              <Trash2 className="h-3.5 w-3.5 mr-1" />
              Delete
            </Button>
          )}

          {/* Delete confirmation */}
          {isDeleteConfirm && (
            <div className="flex items-center gap-1 ml-1">
              <span className="text-xs text-destructive">Delete?</span>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs text-destructive"
                onClick={handleDelete}
                disabled={deleteMutation.isPending}
              >
                Yes
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs text-muted-foreground"
                onClick={() => setIsDeleteConfirm(false)}
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
              data-testid="report-comment-button"
            >
              <Flag className="h-3.5 w-3.5 mr-1" />
              Report
            </Button>
          )}
          {/* PSY-296 start: owner-only reply-permission control. */}
          {isOwner && (
            <label className="flex items-center gap-1 text-xs text-muted-foreground ml-1">
              <span className="sr-only">Who can reply</span>
              <ReplyPermissionSelect
                value={comment.reply_permission as ReplyPermission}
                onChange={handleChangeReplyPermission}
                disabled={updateReplyPermissionMutation.isPending}
                ariaLabel="Who can reply"
              />
            </label>
          )}
          {/* PSY-296 end */}
        </div>
      )}

      {/* PSY-297: Admin-only edit history trigger.
          Kept in its own section (below the public/owner action row) so the
          admin affordance is visually separated from the standard comment
          controls. Only rendered when (a) the viewer is an admin and
          (b) the comment has at least one recorded edit. */}
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

      {/* Inline reply form */}
      {isReplying && (
        <div className="mt-3 ml-4">
          <CommentForm
            onSubmit={handleReply}
            placeholder={`Reply to ${comment.author_name}...`}
            submitLabel="Reply"
            onCancel={() => setIsReplying(false)}
            isPending={replyMutation.isPending}
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
            <ChevronRight className={`h-3.5 w-3.5 mr-1 transition-transform ${showReplies ? 'rotate-90' : ''}`} />
            {showReplies ? 'Hide' : 'Show'} {threadReplies.length} {threadReplies.length === 1 ? 'reply' : 'replies'}
          </Button>

          {showReplies && (
            <div className="mt-1 space-y-3 border-l-2 border-border/50 pl-3">
              {threadReplies.map((reply) => (
                <CommentCard
                  key={reply.id}
                  comment={reply}
                  entityType={entityType}
                  entityId={entityId}
                />
              ))}
            </div>
          )}
        </div>
      )}

      {/* Load replies button for top-level comments with no inline replies.
          PSY-514: also gate on reply_count > 0 so we don't render a "Show
          replies" affordance on threads that have none — clicking did
          nothing, and on `author_only` comments it was actively misleading.
          Comments fetched by routes that don't populate reply_count (e.g.
          single-comment endpoints) leave the field undefined; treat the
          missing-field case the same as 0 since there's no signal to act on. */}
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
          entityName={`Comment by ${comment.author_name}`}
        />
      )}

      {/* PSY-297: Admin edit history dialog. Mounted on-demand (only when an
          admin has clicked the trigger) so we don't fetch history for every
          comment on the page. */}
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
