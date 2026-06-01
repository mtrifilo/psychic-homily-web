'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import {
  Loader2,
  ThumbsUp,
  ThumbsDown,
  Pencil,
  Check,
  X,
  Trash2,
  CheckCircle,
  XCircle,
  ExternalLink,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Breadcrumb, UserAttribution } from '@/components/shared'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useRequest,
  useUpdateRequest,
  useDeleteRequest,
  useVoteRequest,
  useRemoveVoteRequest,
  useFulfillRequest,
  useApproveFulfillment,
  useRejectFulfillment,
  useCloseRequest,
} from '../hooks'
import {
  getEntityTypeLabel,
  getEntityTypeArticle,
  getEntityTypeColor,
  getStatusLabel,
  getStatusColor,
  getEntityUrlBySlug,
  formatTimeAgo,
  formatDate,
} from '../types'
import { FulfillmentEntityPicker } from './FulfillmentEntityPicker'

interface RequestDetailProps {
  requestId: number
}

export function RequestDetail({ requestId }: RequestDetailProps) {
  const router = useRouter()
  const { user, isAuthenticated } = useAuthContext()
  const { data: request, isLoading, error } = useRequest(requestId)
  const deleteMutation = useDeleteRequest()
  const voteMutation = useVoteRequest()
  const removeVoteMutation = useRemoveVoteRequest()
  const fulfillMutation = useFulfillRequest()
  const approveMutation = useApproveFulfillment()
  const rejectMutation = useRejectFulfillment()
  const closeMutation = useCloseRequest()

  const [isEditing, setIsEditing] = useState(false)
  const [isRejectModalOpen, setIsRejectModalOpen] = useState(false)
  // PSY-917: the "Propose a fulfillment" picker dialog. Proposing requires
  // naming a concrete entity, so the button opens this picker rather than
  // submitting directly.
  const [isProposeModalOpen, setIsProposeModalOpen] = useState(false)

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load request'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Request Not Found' : 'Error Loading Request'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The request you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/requests">Back to Requests</Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!request) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Request Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The request you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/requests">Back to Requests</Link>
          </Button>
        </div>
      </div>
    )
  }

  const currentUserId = user?.id ? Number(user.id) : undefined
  const isRequester = currentUserId === request.requester_id
  const isAdmin = user?.is_admin === true
  const canEdit = isRequester || isAdmin
  const canDelete = isRequester || isAdmin
  // PSY-748/PSY-891: submitting a fulfillment is open to ANY authenticated
  // user (it proposes, not finalizes — the requester/admin then approves).
  // Available only while the request is open for proposals.
  const canSubmitFulfillment =
    isAuthenticated &&
    (request.status === 'pending' || request.status === 'in_progress')
  // PSY-891: approving/rejecting a proposed fulfillment is gated to the
  // original requester or an admin, and only when one is pending review.
  const canReviewFulfillment =
    request.status === 'pending_fulfillment' && (isRequester || isAdmin)
  const canClose = isAdmin && request.status !== 'cancelled' && request.status !== 'fulfilled'

  const userVote = request.user_vote ?? 0
  const isVoting = voteMutation.isPending || removeVoteMutation.isPending

  const handleUpvote = () => {
    if (!isAuthenticated) return
    if (userVote === 1) {
      removeVoteMutation.mutate({ requestId: request.id })
    } else {
      voteMutation.mutate({ requestId: request.id, is_upvote: true })
    }
  }

  const handleDownvote = () => {
    if (!isAuthenticated) return
    if (userVote === -1) {
      removeVoteMutation.mutate({ requestId: request.id })
    } else {
      voteMutation.mutate({ requestId: request.id, is_upvote: false })
    }
  }

  const handleDelete = () => {
    if (
      window.confirm(
        'Are you sure you want to delete this request? This action cannot be undone.'
      )
    ) {
      deleteMutation.mutate(
        { requestId: request.id },
        { onSuccess: () => router.push('/requests') }
      )
    }
  }

  // PSY-917: a proposal MUST name a concrete entity (picker is mandatory).
  // The selected entity id flows through as fulfilled_entity_id; the backend
  // validates type-match (PSY-748) and any 400 surfaces inline in the picker.
  const handleProposeFulfillment = (entityId: number) => {
    fulfillMutation.mutate(
      { requestId: request.id, fulfilled_entity_id: entityId },
      { onSuccess: () => setIsProposeModalOpen(false) }
    )
  }

  const openProposeModal = () => {
    fulfillMutation.reset()
    setIsProposeModalOpen(true)
  }

  const handleApprove = () => {
    approveMutation.mutate({ requestId: request.id })
  }

  const handleReject = () => {
    rejectMutation.mutate(
      { requestId: request.id },
      { onSuccess: () => setIsRejectModalOpen(false) }
    )
  }

  const handleClose = () => {
    if (
      window.confirm('Are you sure you want to close this request?')
    ) {
      closeMutation.mutate({ requestId: request.id })
    }
  }

  return (
    <div className="container max-w-4xl mx-auto px-4 py-6">
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        fallback={{ href: '/requests', label: 'Requests' }}
        currentPage={request.title}
      />

      {/* Header */}
      <header className="mb-8">
        {isEditing ? (
          <InlineEditForm
            requestId={request.id}
            title={request.title}
            description={request.description ?? ''}
            onDone={() => setIsEditing(false)}
          />
        ) : (
          <div>
            <div className="flex items-start justify-between gap-4">
              <div className="flex gap-4">
                {/* Vote widget (larger for detail) */}
                <div className="flex flex-col items-center gap-1 shrink-0">
                  <button
                    onClick={handleUpvote}
                    disabled={isVoting || !isAuthenticated}
                    className={cn(
                      'rounded p-1.5 transition-colors',
                      userVote === 1
                        ? 'text-green-600 dark:text-green-400 bg-green-500/10'
                        : 'text-muted-foreground/50 hover:text-green-600 dark:hover:text-green-400 hover:bg-green-500/10',
                      !isAuthenticated && 'cursor-default opacity-50'
                    )}
                    title={isAuthenticated ? 'Upvote' : 'Log in to vote'}
                    aria-label="Upvote"
                  >
                    <ThumbsUp className="h-5 w-5" />
                  </button>
                  <span
                    className={cn(
                      'text-lg font-bold tabular-nums',
                      request.vote_score > 0 &&
                        'text-green-600 dark:text-green-400',
                      request.vote_score < 0 &&
                        'text-red-600 dark:text-red-400',
                      request.vote_score === 0 && 'text-muted-foreground'
                    )}
                  >
                    {request.vote_score}
                  </span>
                  <button
                    onClick={handleDownvote}
                    disabled={isVoting || !isAuthenticated}
                    className={cn(
                      'rounded p-1.5 transition-colors',
                      userVote === -1
                        ? 'text-red-600 dark:text-red-400 bg-red-500/10'
                        : 'text-muted-foreground/50 hover:text-red-600 dark:hover:text-red-400 hover:bg-red-500/10',
                      !isAuthenticated && 'cursor-default opacity-50'
                    )}
                    title={isAuthenticated ? 'Downvote' : 'Log in to vote'}
                    aria-label="Downvote"
                  >
                    <ThumbsDown className="h-5 w-5" />
                  </button>
                  <div className="text-[10px] text-muted-foreground mt-1 text-center">
                    {request.upvotes} up / {request.downvotes} down
                  </div>
                </div>

                {/* Title and info */}
                <div>
                  <h1 className="text-3xl font-bold tracking-tight">
                    {request.title}
                  </h1>

                  <div className="flex items-center gap-2 mt-2 flex-wrap">
                    <span
                      className={cn(
                        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
                        getEntityTypeColor(request.entity_type)
                      )}
                    >
                      {getEntityTypeLabel(request.entity_type)}
                    </span>
                    <span
                      className={cn(
                        'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
                        getStatusColor(request.status)
                      )}
                    >
                      {getStatusLabel(request.status)}
                    </span>
                  </div>

                  <p className="text-sm text-muted-foreground mt-2">
                    Requested by{' '}
                    <UserAttribution
                      name={request.requester_name}
                      username={request.requester_username}
                      className="text-foreground"
                    />{' '}
                    {formatTimeAgo(request.created_at)}
                  </p>

                  {request.description && (
                    <p className="text-muted-foreground mt-4 whitespace-pre-line">
                      {request.description}
                    </p>
                  )}

                  {/*
                    Linked entity. The requests table reuses requested_entity_id
                    for BOTH the originally-requested entity AND (after a
                    proposal) the fulfiller's proposed entity, so the link's
                    label depends on status: "proposed" while pending review,
                    "requested" otherwise. We key off the server-resolved slug
                    (PSY-917) — entity pages route by slug, not id — and render
                    nothing when the slug is null (entity has no slug / was
                    deleted) so we never emit a dead link.
                    The dedicated review panel below ALSO surfaces this link for
                    the requester; this block covers every other viewer.
                  */}
                  {(() => {
                    const entityUrl = getEntityUrlBySlug(
                      request.entity_type,
                      request.requested_entity_slug
                    )
                    if (!entityUrl) return null
                    const isProposed =
                      request.status === 'pending_fulfillment'
                    const label =
                      request.requested_entity_name ??
                      getEntityTypeLabel(request.entity_type).toLowerCase()
                    return (
                      <div className="mt-4">
                        <Link
                          href={entityUrl}
                          className="inline-flex items-center gap-1.5 text-sm text-primary hover:text-primary/80 transition-colors"
                        >
                          <ExternalLink className="h-3.5 w-3.5" />
                          View {isProposed ? 'proposed' : 'requested'} {label}
                        </Link>
                      </div>
                    )
                  })()}

                  {/* Fulfillment info */}
                  {request.status === 'fulfilled' && (
                    <div className="mt-4 rounded-lg border border-green-500/20 bg-green-500/5 p-3">
                      <div className="flex items-center gap-2 text-sm">
                        <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
                        <span className="font-medium text-green-700 dark:text-green-400">
                          Fulfilled
                        </span>
                      </div>
                      {request.fulfiller_name && (
                        <p className="text-sm text-muted-foreground mt-1">
                          by {request.fulfiller_name}
                          {request.fulfilled_at &&
                            ` on ${formatDate(request.fulfilled_at)}`}
                        </p>
                      )}
                    </div>
                  )}

                  {/* PSY-891: pending-fulfillment review panel (requester/admin) */}
                  {canReviewFulfillment && (
                    <div className="mt-4 rounded-lg border border-primary/60 p-4">
                      <p className="text-sm font-semibold text-primary">
                        A fulfillment is awaiting your approval
                      </p>
                      {request.fulfiller_name && (
                        <p className="mt-1 text-sm text-foreground">
                          <UserAttribution
                            name={request.fulfiller_name}
                            username={request.fulfiller_username}
                            className="text-foreground"
                          />{' '}
                          proposed a fulfillment
                          {request.updated_at &&
                            ` · ${formatTimeAgo(request.updated_at)}`}
                        </p>
                      )}
                      {/*
                        PSY-917: the propose flow now captures a concrete entity
                        (fulfilled_entity_id), stored on the request and resolved
                        server-side to a slug + name. Surface it as a "View
                        proposed {entity}" link so the requester can inspect what
                        was proposed before approving. Suppressed only when the
                        slug didn't resolve (legacy proposals from before this
                        shipped carried no entity; entity since deleted).
                      */}
                      {(() => {
                        const proposedUrl = getEntityUrlBySlug(
                          request.entity_type,
                          request.requested_entity_slug
                        )
                        if (!proposedUrl) return null
                        const label =
                          request.requested_entity_name ??
                          getEntityTypeLabel(request.entity_type).toLowerCase()
                        return (
                          <Link
                            href={proposedUrl}
                            className="mt-2 inline-flex items-center gap-1.5 text-sm text-primary transition-colors hover:text-primary/80"
                            data-testid="review-panel-proposed-entity-link"
                          >
                            <ExternalLink className="h-3.5 w-3.5" />
                            View proposed {label}
                          </Link>
                        )
                      })()}
                      <div className="mt-3 flex items-center gap-2">
                        <Button
                          size="sm"
                          onClick={handleApprove}
                          disabled={
                            approveMutation.isPending || rejectMutation.isPending
                          }
                        >
                          {approveMutation.isPending && (
                            <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
                          )}
                          Approve fulfillment
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setIsRejectModalOpen(true)}
                          disabled={
                            approveMutation.isPending || rejectMutation.isPending
                          }
                        >
                          Reject
                        </Button>
                      </div>
                      {approveMutation.error && (
                        <p className="mt-2 text-sm text-destructive">
                          {approveMutation.error instanceof Error
                            ? approveMutation.error.message
                            : 'Failed to approve fulfillment'}
                        </p>
                      )}
                    </div>
                  )}

                  {/* PSY-891: pending-fulfillment, viewer is NOT requester/admin */}
                  {request.status === 'pending_fulfillment' &&
                    !canReviewFulfillment && (
                      <p className="mt-4 text-sm text-muted-foreground">
                        A fulfillment has been proposed — awaiting the
                        requester&apos;s approval.
                      </p>
                    )}

                  {/* PSY-748/PSY-891: any authed user can propose a fulfillment */}
                  {canSubmitFulfillment && (
                    <div className="mt-4">
                      <p className="mb-2 text-sm text-muted-foreground">
                        Found the{' '}
                        {getEntityTypeLabel(request.entity_type).toLowerCase()} in
                        the graph? Propose it — the requester reviews and
                        approves.
                      </p>
                      <Button
                        size="sm"
                        onClick={openProposeModal}
                        disabled={fulfillMutation.isPending}
                      >
                        {fulfillMutation.isPending && (
                          <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
                        )}
                        Propose a fulfillment
                      </Button>
                    </div>
                  )}
                </div>
              </div>

              {/* Action buttons */}
              <div className="flex items-center gap-2 shrink-0">
                {canEdit && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setIsEditing(true)}
                  >
                    <Pencil className="h-4 w-4 mr-1.5" />
                    Edit
                  </Button>
                )}

                {canClose && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleClose}
                    disabled={closeMutation.isPending}
                  >
                    <XCircle className="h-4 w-4 mr-1.5" />
                    Close
                  </Button>
                )}

                {canDelete && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleDelete}
                    disabled={deleteMutation.isPending}
                    className="text-destructive hover:text-destructive"
                    aria-label="Delete request"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                )}
              </div>
            </div>
          </div>
        )}
      </header>

      {/* PSY-891: reject-fulfillment confirm modal (no required reason) */}
      <Dialog open={isRejectModalOpen} onOpenChange={setIsRejectModalOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reject this fulfillment?</DialogTitle>
            <DialogDescription>
              The request will reopen so anyone can propose a fulfillment, and
              the contributor will be notified. You can approve a different
              proposal later.
            </DialogDescription>
          </DialogHeader>
          {rejectMutation.error && (
            <p className="text-sm text-destructive">
              {rejectMutation.error instanceof Error
                ? rejectMutation.error.message
                : 'Failed to reject fulfillment'}
            </p>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setIsRejectModalOpen(false)}
              disabled={rejectMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleReject}
              disabled={rejectMutation.isPending}
            >
              {rejectMutation.isPending && (
                <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
              )}
              Reject fulfillment
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* PSY-917: propose-fulfillment entity picker. Mandatory entity selection
          — the request can only enter pending_fulfillment with a concrete
          proposed entity so the requester's review always has something to
          inspect. */}
      <Dialog open={isProposeModalOpen} onOpenChange={setIsProposeModalOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              Propose {getEntityTypeArticle(request.entity_type)}{' '}
              {getEntityTypeLabel(request.entity_type).toLowerCase()}
            </DialogTitle>
            <DialogDescription>
              Pick the{' '}
              {getEntityTypeLabel(request.entity_type).toLowerCase()} in the
              graph that fulfills this request. The requester reviews and
              approves your proposal.
            </DialogDescription>
          </DialogHeader>
          <FulfillmentEntityPicker
            entityType={request.entity_type}
            isSubmitting={fulfillMutation.isPending}
            submitError={
              fulfillMutation.error
                ? fulfillMutation.error instanceof Error
                  ? fulfillMutation.error.message
                  : 'Failed to propose a fulfillment'
                : null
            }
            onSubmit={handleProposeFulfillment}
            onCancel={() => setIsProposeModalOpen(false)}
          />
        </DialogContent>
      </Dialog>
    </div>
  )
}

// ──────────────────────────────────────────────
// Inline Edit Form
// ──────────────────────────────────────────────

function InlineEditForm({
  requestId,
  title: initialTitle,
  description: initialDescription,
  onDone,
}: {
  requestId: number
  title: string
  description: string
  onDone: () => void
}) {
  const updateMutation = useUpdateRequest()
  const [title, setTitle] = useState(initialTitle)
  const [description, setDescription] = useState(initialDescription)

  const handleSave = () => {
    updateMutation.mutate(
      {
        requestId,
        title: title.trim(),
        description: description.trim() || undefined,
      },
      { onSuccess: () => onDone() }
    )
  }

  return (
    <div className="space-y-4 rounded-lg border border-border/50 bg-card p-4">
      <div>
        <label
          htmlFor="edit-title"
          className="text-sm font-medium mb-1.5 block"
        >
          Title
        </label>
        <Input
          id="edit-title"
          value={title}
          onChange={e => setTitle(e.target.value)}
          autoFocus
        />
      </div>

      <div>
        <label
          htmlFor="edit-description"
          className="text-sm font-medium mb-1.5 block"
        >
          Description
        </label>
        <Textarea
          id="edit-description"
          value={description}
          onChange={e => setDescription(e.target.value)}
          rows={4}
        />
      </div>

      {updateMutation.error && (
        <p className="text-sm text-destructive">
          {updateMutation.error instanceof Error
            ? updateMutation.error.message
            : 'Failed to update request'}
        </p>
      )}

      <div className="flex gap-2">
        <Button
          size="sm"
          onClick={handleSave}
          disabled={!title.trim() || updateMutation.isPending}
        >
          <Check className="h-4 w-4 mr-1" />
          {updateMutation.isPending ? 'Saving...' : 'Save'}
        </Button>
        <Button size="sm" variant="outline" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </div>
  )
}
