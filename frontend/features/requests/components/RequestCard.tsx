'use client'

import Link from 'next/link'
import { ThumbsUp, ThumbsDown } from 'lucide-react'
import { cn } from '@/lib/utils'
import { UserAttribution } from '@/components/shared'
import { formatTimeAgo } from '../types'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useVoteRequest, useRemoveVoteRequest } from '../hooks'
import {
  getEntityTypeLabel,
  getEntityTypeColor,
  getStatusLabel,
  getStatusColor,
} from '../types'
import type { Request } from '../types'

interface RequestCardProps {
  request: Request
}

export function RequestCard({ request }: RequestCardProps) {
  const { isAuthenticated } = useAuthContext()
  const voteMutation = useVoteRequest()
  const removeVoteMutation = useRemoveVoteRequest()

  const userVote = request.user_vote ?? 0

  const handleUpvote = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!isAuthenticated) return

    if (userVote === 1) {
      removeVoteMutation.mutate({ requestId: request.id })
    } else {
      voteMutation.mutate({ requestId: request.id, is_upvote: true })
    }
  }

  const handleDownvote = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!isAuthenticated) return

    if (userVote === -1) {
      removeVoteMutation.mutate({ requestId: request.id })
    } else {
      voteMutation.mutate({ requestId: request.id, is_upvote: false })
    }
  }

  const isVoting = voteMutation.isPending || removeVoteMutation.isPending

  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <div className="flex gap-3">
        {/* Vote widget */}
        <div className="flex flex-col items-center gap-0.5 shrink-0 pt-0.5">
          <button
            onClick={handleUpvote}
            disabled={isVoting || !isAuthenticated}
            className={cn(
              'rounded p-1 transition-colors',
              userVote === 1
                ? 'text-green-600 dark:text-green-400'
                : 'text-muted-foreground/50 hover:text-green-600 dark:hover:text-green-400',
              !isAuthenticated && 'cursor-default opacity-50'
            )}
            title={isAuthenticated ? 'Upvote' : 'Log in to vote'}
            aria-label="Upvote"
          >
            <ThumbsUp className="h-4 w-4" />
          </button>
          <span
            className={cn(
              'text-sm font-semibold tabular-nums min-w-[1.5rem] text-center',
              request.vote_score > 0 && 'text-green-600 dark:text-green-400',
              request.vote_score < 0 && 'text-red-600 dark:text-red-400',
              request.vote_score === 0 && 'text-muted-foreground'
            )}
          >
            {request.vote_score}
          </span>
          <button
            onClick={handleDownvote}
            disabled={isVoting || !isAuthenticated}
            className={cn(
              'rounded p-1 transition-colors',
              userVote === -1
                ? 'text-red-600 dark:text-red-400'
                : 'text-muted-foreground/50 hover:text-red-600 dark:hover:text-red-400',
              !isAuthenticated && 'cursor-default opacity-50'
            )}
            title={isAuthenticated ? 'Downvote' : 'Log in to vote'}
            aria-label="Downvote"
          >
            <ThumbsDown className="h-4 w-4" />
          </button>
        </div>

        {/* Text content */}
        <div className="flex-1 min-w-0">
          <Link href={`/requests/${request.id}`} className="block group">
            <h3 className="font-bold text-foreground group-hover:text-primary transition-colors truncate">
              {request.title}
            </h3>
          </Link>

          <div className="flex items-center gap-2 flex-wrap mt-1">
            <span
              className={cn(
                'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium',
                getEntityTypeColor(request.entity_type)
              )}
            >
              {getEntityTypeLabel(request.entity_type)}
            </span>
            <span
              className={cn(
                'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium',
                getStatusColor(request.status)
              )}
            >
              {getStatusLabel(request.status)}
            </span>
          </div>

          {request.description && (
            <p className="text-sm text-muted-foreground mt-1.5 line-clamp-2">
              {request.description}
            </p>
          )}

          <div className="mt-1.5 flex items-center gap-3 text-xs text-muted-foreground">
            <span>
              by{' '}
              {/* PSY-613: Backend ships requester_name via the canonical
                  resolver chain (PSY-612); username field is not yet on
                  the request contract, so we render plain text. */}
              <UserAttribution
                name={request.requester_name}
                username={null}
              />
            </span>
            <span>{formatTimeAgo(request.created_at)}</span>
          </div>
        </div>
      </div>
    </article>
  )
}
