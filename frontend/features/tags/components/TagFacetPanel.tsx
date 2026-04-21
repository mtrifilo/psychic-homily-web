'use client'

import { useMemo, useState } from 'react'
import { X, Tag as TagIcon, Info } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useTags } from '../hooks'
import {
  TAG_CATEGORIES,
  getCategoryColor,
  getCategoryLabel,
} from '../types'
import type { TagCategory, TagEntityType, TagListItem } from '../types'

// Copy for the info tooltip next to the facet heading, keyed by entity type.
// `show` and `festival` surface transitive semantics (PSY-499): filtering by
// genre matches the container entity whose lineup includes a tagged artist.
// Other entity types use direct-tag semantics so they don't get a tooltip.
const TRANSITIVE_TOOLTIP_COPY: Partial<Record<TagEntityType, string>> = {
  show: 'Filtering by genre matches shows whose artists have that tag.',
  festival:
    'Filtering by genre matches festivals whose lineup artists have that tag.',
}

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
  /**
   * Entity type the facet is filtering (PSY-484). When provided, the chip
   * counts reflect tags applied to *this* entity type — so on `/venues` we
   * show "punk N venues" instead of the misleading global "punk M (across
   * all entity types)" that was the dogfood bug. Zero-count chips are
   * hidden by default; a "Show all tags" expander reveals them. Omit on
   * `/tags` browse where the global count is the right signal.
   */
  entityType?: TagEntityType
}

/**
 * Sidebar/drawer used by the six browse pages (PSY-309). Groups tags by
 * category (genre / locale / other — the 3 TAG_CATEGORIES in prod)
 * and lets the user toggle chips to combine tags with AND semantics.
 *
 * Data source: `/tags?category={cat}&sort=usage&limit=N&entity_type=…` —
 * one query per category via `useTags`. Small number of categories (3)
 * keeps the request count bounded. The panel auto-grows if TAG_CATEGORIES
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
  entityType,
}: TagFacetPanelProps) {
  const selectedSet = useMemo(() => new Set(selectedSlugs), [selectedSlugs])
  // When entity_type is set we hide zero-count chips — they would lie about
  // the available results. The expander lets curious users still browse the
  // full vocabulary. Without entity_type (e.g. `/tags` browse) every chip is
  // a valid destination so we show them all.
  const [showAll, setShowAll] = useState(false)
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
            {entityType && TRANSITIVE_TOOLTIP_COPY[entityType] && (
              <TransitiveTagTooltip
                text={TRANSITIVE_TOOLTIP_COPY[entityType] ?? ''}
              />
            )}
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
          entityType={entityType}
          showAll={showAll}
        />
      ))}

      {/* "Show all tags" expander only matters when we're filtering chips */}
      {entityType && (
        <div className="pt-1">
          <button
            type="button"
            onClick={() => setShowAll(prev => !prev)}
            className="text-xs text-muted-foreground hover:text-foreground underline underline-offset-2"
            data-testid="tag-facet-show-all"
          >
            {showAll ? 'Hide tags with no matches' : 'Show all tags'}
          </button>
        </div>
      )}
    </aside>
  )
}

interface CategoryGroupProps {
  category: TagCategory
  limit: number
  selectedSet: Set<string>
  onToggle: (slug: string) => void
  entityType?: TagEntityType
  showAll: boolean
}

function CategoryGroup({
  category,
  limit,
  selectedSet,
  onToggle,
  entityType,
  showAll,
}: CategoryGroupProps) {
  const { data, isLoading } = useTags({
    category,
    sort: 'usage',
    limit,
    entity_type: entityType,
  })
  const allTags = data?.tags ?? []
  // When entity_type is set, the API returns per-entity-type counts. Hide
  // zero-count chips by default so the surface area shrinks to *useful*
  // filters; the user can expand to see the full vocabulary.
  // Selected chips stay visible regardless of count so the selection
  // controls remain reachable even when the user navigates to a different
  // browse page where the tag has zero matches (preserves clear/toggle UX).
  const visibleTags =
    entityType && !showAll
      ? allTags.filter(
          tag => tag.usage_count > 0 || selectedSet.has(tag.slug),
        )
      : allTags
  const label = getCategoryLabel(category)

  if (!isLoading && visibleTags.length === 0) return null

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
          {visibleTags.map(tag => (
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
 * Small info icon next to the facet heading that reveals the transitive
 * semantics of the filter (PSY-499). Only rendered for `show` / `festival`
 * entity types — direct-tag pages (artist, venue, label, release) don't
 * need the explainer. Keyboard- and hover-accessible via Radix Tooltip.
 */
function TransitiveTagTooltip({ text }: { text: string }) {
  return (
    <TooltipProvider delayDuration={120}>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label="How tag filtering works"
            className="inline-flex items-center rounded-full p-0.5 text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-testid="tag-facet-transitive-info"
          >
            <Info className="h-3.5 w-3.5" aria-hidden />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-xs text-xs">
          {text}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
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
