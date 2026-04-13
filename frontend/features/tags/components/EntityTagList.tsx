'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Plus, ThumbsUp, ThumbsDown, X, Search, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  useEntityTags,
  useAddTagToEntity,
  useRemoveTagFromEntity,
  useVoteOnTag,
  useRemoveTagVote,
  useSearchTags,
} from '../hooks'
import { getCategoryColor } from '../types'
import type { EntityTag, TagListItem } from '../types'

interface EntityTagListProps {
  entityType: string
  entityId: number
  isAuthenticated?: boolean
}

export function EntityTagList({ entityType, entityId, isAuthenticated }: EntityTagListProps) {
  const { data, isLoading } = useEntityTags(entityType, entityId)
  const voteMutation = useVoteOnTag()
  const removeVoteMutation = useRemoveTagVote()
  const [addDialogOpen, setAddDialogOpen] = useState(false)

  const tags = data?.tags ?? []

  if (isLoading) {
    return (
      <div className="py-4">
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (tags.length === 0 && !isAuthenticated) {
    return null
  }

  const handleVote = (tag: EntityTag, isUpvote: boolean) => {
    if (!isAuthenticated) return
    const currentVote = tag.user_vote ?? 0
    const newVote = isUpvote ? 1 : -1

    if (currentVote === newVote) {
      // Toggle off
      removeVoteMutation.mutate({ tagId: tag.tag_id, entityType, entityId })
    } else {
      voteMutation.mutate({ tagId: tag.tag_id, entityType, entityId, is_upvote: isUpvote })
    }
  }

  return (
    <div className="py-4">
      <div className="flex items-center gap-2 mb-3">
        <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Tags
        </h3>
        {isAuthenticated && (
          <Dialog open={addDialogOpen} onOpenChange={setAddDialogOpen}>
            <DialogTrigger asChild>
              <button
                className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                aria-label="Add tag"
              >
                <Plus className="h-3 w-3" />
                Add
              </button>
            </DialogTrigger>
            <DialogContent className="sm:max-w-md" aria-describedby={undefined}>
              <DialogHeader>
                <DialogTitle>Add Tag</DialogTitle>
              </DialogHeader>
              <AddTagForm
                entityType={entityType}
                entityId={entityId}
                existingTagIds={tags.map(t => t.tag_id)}
                onSuccess={() => setAddDialogOpen(false)}
              />
            </DialogContent>
          </Dialog>
        )}
      </div>

      {tags.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No tags yet. Be the first to add one!
        </p>
      ) : (
        <div className="flex flex-wrap gap-2">
          {tags.map(tag => (
            <TagWithVotes
              key={tag.tag_id}
              tag={tag}
              isAuthenticated={isAuthenticated}
              onVote={handleVote}
            />
          ))}
        </div>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Tag pill with voting
// ──────────────────────────────────────────────

function TagWithVotes({
  tag,
  isAuthenticated,
  onVote,
}: {
  tag: EntityTag
  isAuthenticated?: boolean
  onVote: (tag: EntityTag, isUpvote: boolean) => void
}) {
  const userVote = tag.user_vote ?? 0
  const score = tag.upvotes - tag.downvotes

  return (
    <div
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs',
        getCategoryColor(tag.category)
      )}
    >
      <Link
        href={`/tags/${tag.slug}`}
        className="font-medium hover:underline"
      >
        {tag.name}
      </Link>

      {(tag.upvotes > 0 || tag.downvotes > 0) && (
        <span className="text-[10px] opacity-70 tabular-nums">
          {score >= 0 ? `+${score}` : score}
        </span>
      )}

      {isAuthenticated && (
        <span className="inline-flex items-center gap-0.5 ml-0.5">
          <button
            onClick={() => onVote(tag, true)}
            className={cn(
              'rounded p-0.5 transition-colors',
              userVote === 1
                ? 'text-green-500'
                : 'text-current opacity-40 hover:opacity-100 hover:text-green-500'
            )}
            title="Upvote"
            aria-label={`Upvote ${tag.name}`}
          >
            <ThumbsUp className="h-3 w-3" />
          </button>
          <button
            onClick={() => onVote(tag, false)}
            className={cn(
              'rounded p-0.5 transition-colors',
              userVote === -1
                ? 'text-red-500'
                : 'text-current opacity-40 hover:opacity-100 hover:text-red-500'
            )}
            title="Downvote"
            aria-label={`Downvote ${tag.name}`}
          >
            <ThumbsDown className="h-3 w-3" />
          </button>
        </span>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Add Tag Form
// ──────────────────────────────────────────────

function AddTagForm({
  entityType,
  entityId,
  existingTagIds,
  onSuccess,
}: {
  entityType: string
  entityId: number
  existingTagIds: number[]
  onSuccess: () => void
}) {
  const addMutation = useAddTagToEntity()
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [createCategory, setCreateCategory] = useState<string>('genre')
  const { data: searchResults, isLoading: searchLoading } = useSearchTags(debouncedQuery, 10)

  // Debounce search input
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(searchQuery)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  const handleSelectTag = (tag: TagListItem) => {
    addMutation.mutate(
      { entityType, entityId, tag_id: tag.id },
      {
        onSuccess: () => {
          setSearchQuery('')
          setDebouncedQuery('')
          onSuccess()
        },
      }
    )
  }

  const handleCreateTag = () => {
    const name = searchQuery.trim()
    if (!name) return
    addMutation.mutate(
      { entityType, entityId, tag_name: name, category: createCategory },
      {
        onSuccess: () => {
          setSearchQuery('')
          setDebouncedQuery('')
          onSuccess()
        },
      }
    )
  }

  const filteredResults = searchResults?.tags?.filter(
    tag => !existingTagIds.includes(tag.id)
  ) ?? []

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== 'Enter') return
    e.preventDefault()

    if (addMutation.isPending) return

    const query = searchQuery.trim()
    if (!query || debouncedQuery.length < 2) return

    if (filteredResults.length > 0) {
      handleSelectTag(filteredResults[0])
    } else if (!searchLoading) {
      handleCreateTag()
    }
  }

  return (
    <div className="space-y-4">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          value={searchQuery}
          onChange={e => setSearchQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Search tags or type a new one..."
          className="pl-9"
          autoFocus
        />
        {searchQuery && (
          <button
            onClick={() => {
              setSearchQuery('')
              setDebouncedQuery('')
            }}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            aria-label="Clear search"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {addMutation.error && (
        <p className="text-sm text-destructive">
          {addMutation.error instanceof Error
            ? addMutation.error.message
            : 'Failed to add tag'}
        </p>
      )}

      {searchLoading && debouncedQuery.length >= 2 && (
        <div className="flex justify-center py-4">
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        </div>
      )}

      {debouncedQuery.length >= 2 && !searchLoading && (
        <div className="space-y-1 max-h-48 overflow-y-auto">
          {filteredResults.map(tag => (
            <button
              key={tag.id}
              onClick={() => handleSelectTag(tag)}
              disabled={addMutation.isPending}
              className="flex items-center gap-2 w-full rounded-md px-3 py-2 text-sm hover:bg-muted transition-colors text-left"
            >
              <span
                className={cn(
                  'inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-medium',
                  getCategoryColor(tag.category)
                )}
              >
                {tag.category}
              </span>
              <span className="font-medium">{tag.name}</span>
              <span className="ml-auto text-xs text-muted-foreground">
                {tag.usage_count} {tag.usage_count === 1 ? 'use' : 'uses'}
              </span>
            </button>
          ))}

          {filteredResults.length === 0 && (
            <div className="px-3 py-2">
              <p className="text-sm text-muted-foreground mb-2">
                No matching tags found.
              </p>
              <div className="flex items-center gap-2 mb-2">
                <label className="text-xs text-muted-foreground">Category:</label>
                <select
                  value={createCategory}
                  onChange={e => setCreateCategory(e.target.value)}
                  className="text-xs rounded border border-input bg-background px-2 py-1"
                >
                  <option value="genre">Genre</option>
                  <option value="locale">Locale</option>
                  <option value="mood">Mood</option>
                  <option value="era">Era</option>
                  <option value="instrument">Instrument</option>
                  <option value="style">Style</option>
                  <option value="descriptor">Descriptor</option>
                  <option value="other">Other</option>
                </select>
              </div>
              <Button
                size="sm"
                variant="outline"
                onClick={handleCreateTag}
                disabled={addMutation.isPending || !searchQuery.trim()}
              >
                {addMutation.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                ) : (
                  <Plus className="h-3.5 w-3.5 mr-1.5" />
                )}
                Create &quot;{searchQuery.trim()}&quot;
              </Button>
            </div>
          )}
        </div>
      )}

      {debouncedQuery.length < 2 && searchQuery.length > 0 && (
        <p className="text-sm text-muted-foreground px-1">
          Type at least 2 characters to search...
        </p>
      )}
    </div>
  )
}
