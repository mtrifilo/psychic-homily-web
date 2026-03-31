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
import { Breadcrumb } from '@/components/shared'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useRequest,
  useUpdateRequest,
  useDeleteRequest,
  useVoteRequest,
  useRemoveVoteRequest,
  useFulfillRequest,
  useCloseRequest,
} from '../hooks'
import {
  getEntityTypeLabel,
  getEntityTypeColor,
  getStatusLabel,
  getStatusColor,
  getEntityUrl,
  formatTimeAgo,
  formatDate,
} from '../types'

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
  const closeMutation = useCloseRequest()

  const [isEditing, setIsEditing] = useState(false)

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
  const canFulfill = isAdmin && request.status !== 'fulfilled'
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

  const handleFulfill = () => {
    fulfillMutation.mutate({ requestId: request.id })
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
                    {request.requester_name && request.requester_name !== 'Unknown' ? (
                      <Link
                        href={`/users/${request.requester_name}`}
                        className="text-foreground hover:text-primary transition-colors"
                      >
                        {request.requester_name}
                      </Link>
                    ) : (
                      <span className="text-foreground">
                        User #{request.requester_id}
                      </span>
                    )}{' '}
                    {formatTimeAgo(request.created_at)}
                  </p>

                  {request.description && (
                    <p className="text-muted-foreground mt-4 whitespace-pre-line">
                      {request.description}
                    </p>
                  )}

                  {/* Requested entity link */}
                  {request.requested_entity_id && (
                    <div className="mt-4">
                      <Link
                        href={getEntityUrl(
                          request.entity_type,
                          request.requested_entity_id
                        )}
                        className="inline-flex items-center gap-1.5 text-sm text-primary hover:text-primary/80 transition-colors"
                      >
                        <ExternalLink className="h-3.5 w-3.5" />
                        View requested {getEntityTypeLabel(request.entity_type).toLowerCase()}
                      </Link>
                    </div>
                  )}

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

                {canFulfill && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleFulfill}
                    disabled={fulfillMutation.isPending}
                    className="text-green-700 hover:text-green-700 dark:text-green-400 dark:hover:text-green-400"
                  >
                    <CheckCircle className="h-4 w-4 mr-1.5" />
                    Fulfill
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
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                )}
              </div>
            </div>
          </div>
        )}
      </header>
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
