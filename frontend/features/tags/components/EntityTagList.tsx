'use client'

import { useState, useEffect, useMemo } from 'react'
import Link from 'next/link'
import { Plus, ThumbsUp, ThumbsDown, X, Search, Loader2, ChevronDown, ChevronUp, MoreHorizontal } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
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
import { TagOfficialIndicator } from './TagOfficialIndicator'
import { useAuthContext } from '@/lib/context/AuthContext'
import { TIERS_HELP_PATH } from '@/lib/tiers'

interface EntityTagListProps {
  entityType: string
  entityId: number
  isAuthenticated?: boolean
}

// Desktop collapses after 5 pills (preserves pre-PSY-460 behavior). Mobile
// collapses much earlier and defers the rest to a Sheet — 3 pills is the
// sweet spot at 320-414px where a typical pill (~60-120px wide including
// vote buttons) plus the "+ Add" and "Show all" chips still fit on one or
// two rows before the Sheet takes over.
const DEFAULT_VISIBLE_COUNT = 5
const MOBILE_VISIBLE_COUNT = 3

export function EntityTagList({ entityType, entityId, isAuthenticated }: EntityTagListProps) {
  const { data, isLoading } = useEntityTags(entityType, entityId)
  const voteMutation = useVoteOnTag()
  const removeVoteMutation = useRemoveTagVote()
  const [addDialogOpen, setAddDialogOpen] = useState(false)
  const [sheetOpen, setSheetOpen] = useState(false)
  const [expanded, setExpanded] = useState(false)

  const tags = data?.tags ?? []

  // Sort by Wilson score (highest confidence first)
  const sortedTags = useMemo(
    () => [...tags].sort((a, b) => b.wilson_score - a.wilson_score),
    [tags]
  )

  const hasMoreDesktop = sortedTags.length > DEFAULT_VISIBLE_COUNT
  const desktopVisibleTags = expanded ? sortedTags : sortedTags.slice(0, DEFAULT_VISIBLE_COUNT)
  const hiddenDesktopCount = sortedTags.length - DEFAULT_VISIBLE_COUNT

  const hasMoreMobile = sortedTags.length > MOBILE_VISIBLE_COUNT
  const mobileVisibleTags = sortedTags.slice(0, MOBILE_VISIBLE_COUNT)
  const hiddenMobileCount = sortedTags.length - MOBILE_VISIBLE_COUNT

  if (isLoading) {
    return (
      <div className="py-4">
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      </div>
    )
  }

  // PSY-481: previously this returned null for the (zero tags + logged-out)
  // case, which hid the entire TAGS section on every untagged entity. The
  // wrapper now always renders so:
  //   • logged-out users see a muted "No tags yet." one-liner — they get a
  //     visible signal that tagging is supported on this entity, and the
  //     detail-page layout no longer collapses on sparse entities.
  //   • logged-in users see the same empty-state line plus a
  //     "+ Add the first tag" CTA that opens the existing add-tag dialog
  //     (the existing-tag application path works for every tier; only
  //     creating brand-new tag terms is gated — PSY-483).
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

  // The Add dialog is rendered once and reused by both the desktop "Add"
  // chip next to the row heading and the sheet-header "Add" action. Lifting
  // it out of the trigger tree keeps a single Dialog instance (one piece of
  // state, one Radix portal) and avoids the "dialog closes when the sheet
  // closes" problem that happens if the trigger unmounts mid-flow.
  const addTagDialog = isAuthenticated ? (
    <Dialog open={addDialogOpen} onOpenChange={setAddDialogOpen}>
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
  ) : null

  const addTagButton = isAuthenticated ? (
    <button
      onClick={() => setAddDialogOpen(true)}
      className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
      aria-label="Add tag"
    >
      <Plus className="h-3 w-3" />
      Add
    </button>
  ) : null

  return (
    <div className="py-4">
      <div className="flex items-center gap-2 mb-3">
        <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Tags
        </h3>
        {addTagButton}
      </div>

      {addTagDialog}

      {tags.length === 0 ? (
        // PSY-481 — render an empty-state row that reads as a muted one-liner
        // for logged-out users and adds a visible "+ Add the first tag" CTA
        // for logged-in users. The CTA reuses the same Dialog instance as the
        // chip next to the heading, so there's only ever one Radix portal.
        <div
          className="flex flex-wrap items-center gap-2"
          data-testid="entity-tag-list-empty"
        >
          <p className="text-sm text-muted-foreground">No tags yet.</p>
          {isAuthenticated && (
            <button
              onClick={() => setAddDialogOpen(true)}
              data-testid="entity-tag-list-empty-add-cta"
              className="inline-flex items-center gap-1 rounded-full border border-dashed border-border px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              aria-label="Add the first tag"
            >
              <Plus className="h-3 w-3" />
              Add the first tag
            </button>
          )}
        </div>
      ) : (
        <>
          {/* Mobile: first N pills + "Show all" chip that opens a Sheet.
              Hidden at >=sm where the desktop top-5 cap takes over. */}
          <div
            className="flex flex-wrap gap-2 items-center sm:hidden"
            data-testid="entity-tag-list-mobile-row"
          >
            {mobileVisibleTags.map(tag => (
              <TagWithVotes
                key={tag.tag_id}
                tag={tag}
                isAuthenticated={isAuthenticated}
                onVote={handleVote}
              />
            ))}
            {hasMoreMobile && (
              <button
                onClick={() => setSheetOpen(true)}
                data-testid="entity-tag-list-mobile-show-all"
                className="inline-flex items-center gap-1 rounded-full border border-dashed border-border px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                aria-label={`Show all ${sortedTags.length} tags`}
              >
                <MoreHorizontal className="h-3 w-3" />
                Show all tags ({hiddenMobileCount} more)
              </button>
            )}
          </div>

          {/* Desktop: unchanged from pre-PSY-460 — top 5 with inline
              expand/collapse. Hidden on narrow viewports where the Sheet
              flow takes over. */}
          <div
            className="hidden sm:flex flex-wrap gap-2 items-center"
            data-testid="entity-tag-list-desktop-row"
          >
            {desktopVisibleTags.map(tag => (
              <TagWithVotes
                key={tag.tag_id}
                tag={tag}
                isAuthenticated={isAuthenticated}
                onVote={handleVote}
              />
            ))}
            {hasMoreDesktop && (
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
                    Show {hiddenDesktopCount} more
                  </>
                )}
              </button>
            )}
          </div>
        </>
      )}

      <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
        <SheetContent
          side="bottom"
          className="max-h-[85vh] overflow-y-auto"
          data-testid="entity-tag-list-mobile-sheet"
        >
          <SheetHeader>
            <SheetTitle className="flex items-center gap-3">
              <span>All tags ({sortedTags.length})</span>
              {isAuthenticated && (
                <button
                  onClick={() => {
                    // Close the sheet first so the Add dialog doesn't
                    // stack a second Radix Portal on top of another
                    // Portal — keeps focus-trap + overlay behavior clean.
                    setSheetOpen(false)
                    setAddDialogOpen(true)
                  }}
                  data-testid="entity-tag-list-sheet-add"
                  className="inline-flex items-center gap-1 rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                  aria-label="Add tag"
                >
                  <Plus className="h-3 w-3" />
                  Add
                </button>
              )}
            </SheetTitle>
          </SheetHeader>
          <div className="px-4 pb-6 pt-2">
            <div className="flex flex-wrap gap-2 items-center">
              {sortedTags.map(tag => (
                <TagWithVotes
                  key={tag.tag_id}
                  tag={tag}
                  isAuthenticated={isAuthenticated}
                  onVote={handleVote}
                />
              ))}
            </div>
          </div>
        </SheetContent>
      </Sheet>
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

  // Controlled open state for the attribution hover card. Radix HoverCard
  // opens on hover and focus out of the box; this state lets us *also* toggle
  // on click/tap so touch users (where :hover doesn't fire) still have a path
  // to the attribution info (PSY-441 mobile fallback).
  const [open, setOpen] = useState(false)

  const handleTriggerClick = (e: React.MouseEvent) => {
    // Don't toggle the card when the click originated on the inner tag Link
    // (which navigates) or the vote buttons (which mutate). Those elements
    // stop propagation via their native semantics / explicit handlers below;
    // this guard covers any future children we add.
    const target = e.target as HTMLElement
    if (target.closest('a, button')) return
    setOpen(v => !v)
  }

  const handleTriggerKeyDown = (e: React.KeyboardEvent) => {
    // Enter/Space on the pill wrapper toggles the card — matches the
    // mouse-click affordance and keeps keyboard users on par with pointer
    // users. Radix already opens on focus, so this is an explicit toggle.
    if (e.key === 'Enter' || e.key === ' ') {
      // Only handle keystrokes that land on the wrapper itself; inner
      // focusable elements (the Link, vote buttons) handle their own keys.
      if (e.target !== e.currentTarget) return
      e.preventDefault()
      setOpen(v => !v)
    }
  }

  return (
    <HoverCard open={open} onOpenChange={setOpen} openDelay={120} closeDelay={80}>
      <HoverCardTrigger asChild>
        <div
          role="group"
          tabIndex={0}
          aria-label={`${tag.name} tag details`}
          onClick={handleTriggerClick}
          onKeyDown={handleTriggerKeyDown}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs cursor-pointer focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-1 focus:ring-offset-background',
            // Official tags get a distinct primary-accent background that
            // overrides the per-category color, making curated tags visibly
            // different at a glance (ISSUE-004 from tags-audit-2).
            tag.is_official
              ? 'border-primary/40 bg-primary/10 text-foreground'
              : getCategoryColor(tag.category)
          )}
        >
          {tag.is_official && (
            <TagOfficialIndicator size="sm" tagName={tag.name} />
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
                onClick={e => {
                  e.stopPropagation()
                  onVote(tag, true)
                }}
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
                onClick={e => {
                  e.stopPropagation()
                  onVote(tag, false)
                }}
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
      </HoverCardTrigger>
      <HoverCardContent
        align="start"
        side="top"
        className="w-[280px] text-sm"
        data-testid={`tag-attribution-card-${tag.tag_id}`}
      >
        <TagAttributionContent tag={tag} />
      </HoverCardContent>
    </HoverCard>
  )
}

// ──────────────────────────────────────────────
// Attribution hover-card body
// ──────────────────────────────────────────────

// PSY-441 — surfaces who added the tag + vote counts + a direct link to the
// tag detail page. Lives as a separate component so the test suite can assert
// on the rendered content without driving the Radix hover interaction.
function TagAttributionContent({ tag }: { tag: EntityTag }) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5">
        <Link
          href={`/tags/${tag.slug}`}
          className="font-semibold text-foreground hover:underline"
        >
          #{tag.name}
        </Link>
        {tag.is_official && (
          <TagOfficialIndicator size="sm" tagName={tag.name} />
        )}
      </div>

      {/* Added-by line. Skip entirely when the backend has no username —
          contributors who tagged anonymously or under a since-deleted account
          won't leak a dangling "Added by @undefined". */}
      {tag.added_by_username && (
        <p className="text-xs text-muted-foreground">
          Added by{' '}
          <Link
            href={`/users/${tag.added_by_username}`}
            className="text-foreground hover:underline"
          >
            @{tag.added_by_username}
          </Link>
          {tag.added_at && (
            <>
              {' · '}
              <span>{formatRelativeTime(tag.added_at)}</span>
            </>
          )}
        </p>
      )}

      <p className="text-xs text-muted-foreground tabular-nums">
        <span className="font-medium text-foreground">{tag.upvotes}</span>{' '}
        {tag.upvotes === 1 ? 'upvote' : 'upvotes'}
        {' · '}
        <span className="font-medium text-foreground">{tag.downvotes}</span>{' '}
        {tag.downvotes === 1 ? 'downvote' : 'downvotes'}
      </p>

      <Link
        href={`/tags/${tag.slug}`}
        className="inline-block text-xs text-primary hover:underline"
      >
        View tag details
      </Link>
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
  const { user } = useAuthContext()
  // Only `new_user` tier is blocked from creating new tags server-side
  // (CodeTagCreationForbidden in backend/internal/errors/tag.go). Mirror the
  // same gate client-side so users see a tooltip instead of a dead-end 403.
  const canCreateTags = user?.user_tier !== 'new_user'
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
    } else if (!searchLoading && !alreadyAppliedMatch && canCreateTags) {
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
          {addMutation.error instanceof Error &&
            /Contributor tier/i.test(addMutation.error.message) && (
              <>
                {' '}
                <Link
                  href={TIERS_HELP_PATH}
                  className="underline hover:no-underline"
                >
                  Learn more
                </Link>
              </>
            )}
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
                  disabled={!canCreateTags}
                  className="text-xs rounded border border-input bg-background px-2 py-1 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <option value="genre">Genre</option>
                  <option value="locale">Locale</option>
                  <option value="other">Other</option>
                </select>
              </div>
              {canCreateTags ? (
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
              ) : (
                <TooltipProvider delayDuration={150}>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      {/* span wrapper lets the tooltip fire on a disabled button */}
                      <span
                        className="inline-block"
                        data-testid="tag-create-disabled-wrapper"
                      >
                        <Button
                          size="sm"
                          variant="outline"
                          disabled
                          aria-disabled="true"
                          data-testid="tag-create-disabled"
                        >
                          <Plus className="h-3.5 w-3.5 mr-1.5" />
                          Create &quot;{searchQuery.trim()}&quot;
                        </Button>
                      </span>
                    </TooltipTrigger>
                    <TooltipContent
                      side="top"
                      data-testid="tag-create-disabled-tooltip"
                    >
                      <p className="text-xs">
                        Reach Contributor tier to create new tags.{' '}
                        <Link
                          href={TIERS_HELP_PATH}
                          className="underline"
                        >
                          Learn more
                        </Link>
                      </p>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
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
