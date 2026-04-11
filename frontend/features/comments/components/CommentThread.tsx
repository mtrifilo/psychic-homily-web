'use client'

import { useState } from 'react'
import { MessageSquare } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { useComments, useCreateComment } from '../hooks'
import { CommentForm } from './CommentForm'
import { CommentCard } from './CommentCard'
import type { Comment } from '../types'

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

  const { data, isLoading } = useComments(entityType, entityId, sort)
  const createMutation = useCreateComment()

  const comments = data?.comments ?? []
  const total = data?.total ?? 0

  // Separate top-level comments and replies
  const topLevel = comments.filter((c) => c.depth === 0)
  const repliesByParent = comments.reduce<Record<number, Comment[]>>((acc, c) => {
    if (c.parent_id) {
      if (!acc[c.parent_id]) acc[c.parent_id] = []
      acc[c.parent_id].push(c)
    }
    return acc
  }, {})

  const handleCreate = (body: string) => {
    createMutation.mutate({ entityType, entityId, body })
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
      ) : topLevel.length === 0 ? (
        <p className="text-sm text-muted-foreground py-8 text-center" data-testid="empty-state">
          No comments yet. Be the first to share your thoughts.
        </p>
      ) : (
        <div className="space-y-4 divide-y divide-border/50">
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
