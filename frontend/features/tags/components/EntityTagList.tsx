'use client'

import { useState, useEffect, useMemo } from 'react'
import Link from 'next/link'
import { Plus, ThumbsUp, ThumbsDown, X, Search, Loader2, BadgeCheck, ChevronDown, ChevronUp } from 'lucide-react'
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
import { getCategoryColor, TAG_CATEGORIES, getCategoryLabel } from '../types'
import type { EntityTag, TagListItem } from '../types'

interface EntityTagListProps {
  entityType: string
  entityId: number
  isAuthenticated?: boolean
}

const DEFAULT_VISIBLE_COUNT = 5

export function EntityTagList({ entityType, entityId, isAuthenticated }: EntityTagListProps) {
  const { data, isLoading } = useEntityTags(entityType, entityId)
  const voteMutation = useVoteOnTag()
  const removeVoteMutation = useRemoveTagVote()
  const [addDialogOpen, setAddDialogOpen] = useState(false)
  const [expanded, setExpanded] = useState(false)

  const tags = data?.tags ?? []

  // Sort by Wilson score (highest confidence first)
  const sortedTags = useMemo(
    () => [...tags].sort((a, b) => b.wilson_score - a.wilson_score),
    [tags]
  )

  const hasMore = sortedTags.length > DEFAULT_VISIBLE_COUNT
  const visibleTags = expanded ? sortedTags : sortedTags.slice(0, DEFAULT_VISIBLE_COUNT)
  const hiddenCount = sortedTags.length - DEFAULT_VISIBLE_COUNT

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
        <div className="flex flex-wrap gap-2 items-center">
          {visibleTags.map(tag => (
            <TagWithVotes
              key={tag.tag_id}
              tag={tag}
              isAuthenticated={isAuthenticated}
              onVote={handleVote}
            />
          ))}
          {hasMore && (
            <button
              onClick={() => setExpanded(!expanded)}
              className="inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            >
              {expanded ? (
                <>
                  <ChevronUp className="h-3 w-3" />
                  Show less
                </>
              ) : (
                <>
                  <ChevronDown className="h-3 w-3" />
                  Show {hiddenCount} more
                </>
              )}
            </button>
          )}
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
        // Official tags get a distinct primary-accent background that
        // overrides the per-category color, making curated tags visibly
        // different at a glance (ISSUE-004 from tags-audit-2).
        tag.is_official
          ? 'border-primary/40 bg-primary/10 text-foreground'
          : getCategoryColor(tag.category)
      )}
    >
      {tag.is_official && (
        <span title="Official tag" aria-label="Official tag" role="img">
          <BadgeCheck
            className="h-3.5 w-3.5 text-primary shrink-0"
            aria-hidden="true"
          />
        </span>
      )}
      <Link
        href={`/tags/${tag.slug}`}
        className="font-medium hover:underline"
        title={tag.is_official ? `${tag.name} (Official)` : tag.name}
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
  const [filterCategory, setFilterCategory] = useState<string>('')
  const [createCategory, setCreateCategory] = useState<string>('genre')
  const { data: searchResults, isLoading: searchLoading } = useSearchTags(
    debouncedQuery,
    10,
    filterCategory || undefined
  )

  // Debounce search input
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(searchQuery)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  // Sync create category with filter category
  useEffect(() => {
    if (filterCategory) {
      setCreateCategory(filterCategory)
    }
  }, [filterCategory])

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

  const allResults = searchResults?.tags ?? []
  const filteredResults = allResults.filter(
    tag => !existingTagIds.includes(tag.id)
  )

  // When the search matches a tag that's already applied (canonical name OR
  // via an alias), surface an "already applied" row instead of the Create CTA
  // so the user doesn't accidentally create a duplicate tag (PSY-452).
  const alreadyAppliedMatch = allResults.find(tag =>
    existingTagIds.includes(tag.id)
  )

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== 'Enter') return
    e.preventDefault()

    if (addMutation.isPending) return

    const query = searchQuery.trim()
    if (!query || debouncedQuery.length < 2) return

    if (filteredResults.length > 0) {
      handleSelectTag(filteredResults[0])
    } else if (!searchLoading && !alreadyAppliedMatch) {
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

      <div className="flex items-center gap-1.5">
        <span className="text-xs text-muted-foreground">Category:</span>
        <button
          onClick={() => setFilterCategory('')}
          className={cn(
            'rounded-full px-2 py-0.5 text-[11px] font-medium border transition-colors',
            filterCategory === ''
              ? 'bg-foreground/10 text-foreground border-foreground/20'
              : 'text-muted-foreground border-transparent hover:text-foreground hover:bg-muted'
          )}
        >
          All
        </button>
        {TAG_CATEGORIES.map(cat => (
          <button
            key={cat}
            onClick={() => setFilterCategory(filterCategory === cat ? '' : cat)}
            className={cn(
              'rounded-full px-2 py-0.5 text-[11px] font-medium border transition-colors',
              filterCategory === cat
                ? getCategoryColor(cat)
                : 'text-muted-foreground border-transparent hover:text-foreground hover:bg-muted'
            )}
          >
            {getCategoryLabel(cat)}
          </button>
        ))}
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
              className="flex items-start gap-2 w-full rounded-md px-3 py-2 text-sm hover:bg-muted transition-colors text-left"
            >
              <span
                className={cn(
                  'inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-medium shrink-0 mt-0.5',
                  getCategoryColor(tag.category)
                )}
              >
                {tag.category}
              </span>
              <div className="min-w-0 flex-1">
                <span className="font-medium">{tag.name}</span>
                {tag.matched_via_alias && (
                  <span
                    className="block text-[11px] text-muted-foreground truncate"
                    data-testid="tag-autocomplete-matched-alias"
                  >
                    matched &ldquo;{tag.matched_via_alias}&rdquo;
                  </span>
                )}
              </div>
              <span className="ml-auto shrink-0 text-xs text-muted-foreground mt-0.5">
                {tag.usage_count} {tag.usage_count === 1 ? 'use' : 'uses'}
              </span>
            </button>
          ))}

          {filteredResults.length === 0 && alreadyAppliedMatch && (
            <div
              className="px-3 py-2"
              data-testid="tag-autocomplete-already-applied"
            >
              <p className="text-sm text-muted-foreground">
                &ldquo;{alreadyAppliedMatch.name}&rdquo; is already applied to
                this {entityType}.
              </p>
              {alreadyAppliedMatch.matched_via_alias && (
                <p
                  className="text-[11px] text-muted-foreground mt-1"
                  data-testid="tag-autocomplete-matched-alias"
                >
                  matched &ldquo;{alreadyAppliedMatch.matched_via_alias}&rdquo;
                </p>
              )}
            </div>
          )}

          {filteredResults.length === 0 && !alreadyAppliedMatch && (
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
