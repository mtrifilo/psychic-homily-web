'use client'

import { useMemo } from 'react'
import { X, Tag as TagIcon } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useTags } from '../hooks'
import {
  TAG_CATEGORIES,
  getCategoryColor,
  getCategoryLabel,
} from '../types'
import type { TagCategory, TagListItem } from '../types'

const DEFAULT_TAGS_PER_CATEGORY = 20

export interface TagFacetPanelProps {
  /** Currently selected tag slugs. */
  selectedSlugs: string[]
  /** Called with the next full selection when the user toggles a chip. */
  onToggle: (nextSlugs: string[]) => void
  /** Called when the user clears all selections. */
  onClear: () => void
  /** Max tags per category (default 20). Categories with fewer show all. */
  tagsPerCategory?: number
  /** Optional heading (e.g., "Filter artists by tag"). */
  heading?: string
  /** Hide the internal heading (useful when rendered inside a Sheet). */
  hideHeading?: boolean
  /** Tailwind class overrides for the root container. */
  className?: string
}

/**
 * Sidebar/drawer used by the six browse pages (PSY-309). Groups tags by
 * category (genre / locale / other — the 3 TAG_CATEGORIES in prod)
 * and lets the user toggle chips to combine tags with AND semantics.
 *
 * Data source: `/tags?category={cat}&sort=usage&limit=N` — one query
 * per category via `useTags`. Small number of categories (3) keeps
 * the request count bounded. The panel auto-grows if TAG_CATEGORIES
 * gains more entries.
 */
export function TagFacetPanel({
  selectedSlugs,
  onToggle,
  onClear,
  tagsPerCategory = DEFAULT_TAGS_PER_CATEGORY,
  heading = 'Filter by tags',
  hideHeading = false,
  className,
}: TagFacetPanelProps) {
  const selectedSet = useMemo(() => new Set(selectedSlugs), [selectedSlugs])

  const handleToggle = (slug: string) => {
    if (selectedSet.has(slug)) {
      onToggle(selectedSlugs.filter(s => s !== slug))
    } else {
      onToggle([...selectedSlugs, slug])
    }
  }

  return (
    <aside
      className={cn('space-y-5', className)}
      data-testid="tag-facet-panel"
    >
      {!hideHeading && (
        <div className="flex items-center justify-between">
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <TagIcon className="h-3.5 w-3.5" aria-hidden />
            {heading}
          </h2>
          {selectedSlugs.length > 0 && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={onClear}
              data-testid="tag-facet-clear"
            >
              <X className="mr-1 h-3 w-3" /> Clear all
            </Button>
          )}
        </div>
      )}

      {hideHeading && selectedSlugs.length > 0 && (
        <div className="flex justify-end">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-xs"
            onClick={onClear}
            data-testid="tag-facet-clear"
          >
            <X className="mr-1 h-3 w-3" /> Clear all
          </Button>
        </div>
      )}

      {TAG_CATEGORIES.map(cat => (
        <CategoryGroup
          key={cat}
          category={cat}
          limit={tagsPerCategory}
          selectedSet={selectedSet}
          onToggle={handleToggle}
        />
      ))}
    </aside>
  )
}

interface CategoryGroupProps {
  category: TagCategory
  limit: number
  selectedSet: Set<string>
  onToggle: (slug: string) => void
}

function CategoryGroup({
  category,
  limit,
  selectedSet,
  onToggle,
}: CategoryGroupProps) {
  const { data, isLoading } = useTags({
    category,
    sort: 'usage',
    limit,
  })
  const tags = data?.tags ?? []
  const label = getCategoryLabel(category)

  if (!isLoading && tags.length === 0) return null

  return (
    <div className="space-y-2" data-testid={`tag-facet-category-${category}`}>
      <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </h3>
      {isLoading ? (
        <div className="flex flex-wrap gap-1.5">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-6 w-16 rounded-full" />
          ))}
        </div>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {tags.map(tag => (
            <TagChip
              key={tag.id}
              tag={tag}
              selected={selectedSet.has(tag.slug)}
              onToggle={onToggle}
            />
          ))}
        </div>
      )}
    </div>
  )
}

interface TagChipProps {
  tag: TagListItem
  selected: boolean
  onToggle: (slug: string) => void
}

function TagChip({ tag, selected, onToggle }: TagChipProps) {
  const catColors = getCategoryColor(tag.category)
  return (
    <button
      type="button"
      onClick={() => onToggle(tag.slug)}
      aria-pressed={selected}
      data-testid={`tag-facet-chip-${tag.slug}`}
      className={cn(
        'inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium',
        'transition-all duration-100 select-none cursor-pointer',
        selected
          ? 'bg-primary text-primary-foreground border-primary shadow-sm ring-1 ring-primary/40'
          : `${catColors} hover:bg-muted/60`
      )}
    >
      <span>{tag.name}</span>
      <span className={cn('text-[10px] font-normal opacity-70')}>
        {tag.usage_count}
      </span>
    </button>
  )
}

/**
 * Parse a comma-separated `tags` query param into a trimmed, deduped
 * list of slugs. Used by the six browse pages to read URL state.
 */
export function parseTagsParam(param: string | null): string[] {
  if (!param) return []
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of param.split(',')) {
    const slug = raw.trim().toLowerCase()
    if (!slug || seen.has(slug)) continue
    seen.add(slug)
    out.push(slug)
  }
  return out
}

/** Build a `tags=` query-string value from an array of slugs. */
export function buildTagsParam(slugs: string[]): string {
  return slugs.join(',')
}
