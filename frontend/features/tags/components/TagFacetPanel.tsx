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
import type { CityState } from '@/components/filters'
import { useTags } from '../hooks'
import {
  TAG_CATEGORIES,
  getCategoryColor,
  getCategoryLabel,
} from '../types'
import type { TagCategory, TagEntityType, TagListItem } from '../types'

// Copy for the info tooltip next to the facet heading, keyed by entity type.
// `show` explains the multi-select behavior (combine tags to filter shows).
// `festival` surfaces transitive semantics (PSY-499): filtering by genre matches
// the container entity whose lineup includes a tagged artist. Other entity types
// use direct-tag semantics so they don't get a tooltip.
const FACET_TOOLTIP_COPY: Partial<Record<TagEntityType, string>> = {
  show: 'Select one or more tags to filter shows based on any tag combination.',
  festival:
    'Filtering by genre matches festivals whose lineup artists have that tag.',
}

const DEFAULT_TAGS_PER_CATEGORY = 20

/**
 * Layout mode for the facet panel.
 * - `rail` (default): vertical sidebar — categories stacked, each with its
 *   own heading. The original PSY-309 layout used by the non-migrated browse
 *   pages and the mobile Sheet.
 * - `bar` (PSY-1000): horizontal top bar — chips from all categories flow into
 *   a single wrapping row beside the heading, the list renders full-width
 *   below. Same data + behavior (live counts, disabled zero-result facets,
 *   selection, "show all tags"); only the container layout changes.
 */
export type TagFacetLayout = 'rail' | 'bar'

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
  /**
   * Container layout (PSY-1000). `rail` (default) is the vertical sidebar;
   * `bar` is the horizontal top-bar used on `/shows`. Default stays vertical
   * so non-migrated callers (the other browse pages, the mobile Sheet) are
   * unaffected.
   */
  layout?: TagFacetLayout
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
  /**
   * Active city filter passed through to the facet count query (PSY-982).
   * Used only by `/shows` (entityType="show"): the backend scopes each tag's
   * count to shows in these cities so the facet matches the city-filtered
   * shows list. While a city is selected, zero-in-city chips are shown
   * DISABLED (greyed, non-clickable) rather than hidden — the user sees the
   * tag exists but has no shows here, instead of it silently vanishing and
   * instead of dead-ending at "0 shows matching <tag>". Omit for the global,
   * city-agnostic facet (every other browse page).
   */
  selectedCities?: CityState[]
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
  layout = 'rail',
  className,
  entityType,
  selectedCities,
}: TagFacetPanelProps) {
  const selectedSet = useMemo(() => new Set(selectedSlugs), [selectedSlugs])
  // When entity_type is set we hide zero-count chips — they would lie about
  // the available results. The expander lets curious users still browse the
  // full vocabulary. Without entity_type (e.g. `/tags` browse) every chip is
  // a valid destination so we show them all.
  const [showAll, setShowAll] = useState(false)
  // City-scoped mode (PSY-982): while a city is selected on /shows the facet
  // counts are city-scoped, so a zero-count chip means "no shows here". We
  // SHOW those chips DISABLED instead of hiding them — the expander is
  // redundant in this mode, and a disabled chip can never dead-end on click.
  const cityScoped = (selectedCities?.length ?? 0) > 0
  const isBar = layout === 'bar'
  const handleToggle = (slug: string) => {
    if (selectedSet.has(slug)) {
      onToggle(selectedSlugs.filter(s => s !== slug))
    } else {
      onToggle([...selectedSlugs, slug])
    }
  }

  // The "Show all tags" expander only matters when we hide zero-count chips.
  // In city-scoped mode they are shown disabled instead, so the expander is
  // redundant and omitted. Shared between both layouts; bar mode renders it
  // inline at the end of the chip row, rail mode stacks it below the groups.
  const showAllExpander = entityType && !cityScoped && (
    <button
      type="button"
      onClick={() => setShowAll(prev => !prev)}
      className="text-xs text-muted-foreground hover:text-foreground underline underline-offset-2"
      data-testid="tag-facet-show-all"
    >
      {showAll ? 'Hide tags with no matches' : 'Show all tags'}
    </button>
  )

  // Shared "Clear all" control — the same button, positioned per layout.
  const clearButton = selectedSlugs.length > 0 && (
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
  )

  // Bar layout (PSY-1000): the full vocabulary flows into ONE wrapping row so
  // the chips sit above a full-width list. We flatten the categories rather
  // than stacking per-category headings — the chip colors (from
  // getCategoryColor) already encode the category, matching the Figma
  // `474:9` "Filter by tag" row. The heading, chips, the "show all"
  // expander, and "Clear all" all share that single flex-wrap row.
  if (isBar) {
    return (
      <div
        className={cn('flex flex-wrap items-center gap-x-3 gap-y-2', className)}
        data-testid="tag-facet-panel"
        data-layout="bar"
      >
        {!hideHeading && (
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <TagIcon className="h-3.5 w-3.5" aria-hidden />
            {heading}
            {entityType && FACET_TOOLTIP_COPY[entityType] && (
              <TransitiveTagTooltip
                text={FACET_TOOLTIP_COPY[entityType] ?? ''}
              />
            )}
          </h2>
        )}

        {TAG_CATEGORIES.map(cat => (
          <CategoryGroup
            key={cat}
            category={cat}
            limit={tagsPerCategory}
            selectedSet={selectedSet}
            onToggle={handleToggle}
            entityType={entityType}
            selectedCities={selectedCities}
            showAll={showAll}
            cityScoped={cityScoped}
            inline
          />
        ))}

        {showAllExpander}
        {clearButton}
      </div>
    )
  }

  // Rail layout (default, PSY-309): vertical sidebar with stacked,
  // per-category groups.
  return (
    <aside
      className={cn('space-y-5', className)}
      data-testid="tag-facet-panel"
      data-layout="rail"
    >
      {!hideHeading && (
        <div className="flex items-center justify-between">
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <TagIcon className="h-3.5 w-3.5" aria-hidden />
            {heading}
            {entityType && FACET_TOOLTIP_COPY[entityType] && (
              <TransitiveTagTooltip
                text={FACET_TOOLTIP_COPY[entityType] ?? ''}
              />
            )}
          </h2>
          {clearButton}
        </div>
      )}

      {hideHeading && selectedSlugs.length > 0 && (
        <div className="flex justify-end">{clearButton}</div>
      )}

      {TAG_CATEGORIES.map(cat => (
        <CategoryGroup
          key={cat}
          category={cat}
          limit={tagsPerCategory}
          selectedSet={selectedSet}
          onToggle={handleToggle}
          entityType={entityType}
          selectedCities={selectedCities}
          showAll={showAll}
          cityScoped={cityScoped}
        />
      ))}

      {showAllExpander && <div className="pt-1">{showAllExpander}</div>}
    </aside>
  )
}

interface CategoryGroupProps {
  category: TagCategory
  limit: number
  selectedSet: Set<string>
  onToggle: (slug: string) => void
  entityType?: TagEntityType
  selectedCities?: CityState[]
  showAll: boolean
  /** When true, zero-count chips are shown disabled instead of hidden. */
  cityScoped: boolean
  /**
   * Bar layout (PSY-1000): render the chips bare so they flow into the
   * parent's single wrapping row — no per-category heading, no own
   * container. Rail layout (default) keeps the stacked heading + group.
   */
  inline?: boolean
}

function CategoryGroup({
  category,
  limit,
  selectedSet,
  onToggle,
  entityType,
  selectedCities,
  showAll,
  cityScoped,
  inline = false,
}: CategoryGroupProps) {
  const { data, isLoading } = useTags({
    category,
    sort: 'usage',
    limit,
    entity_type: entityType,
    // Only the show facet honours cities backend-side; passing it for other
    // entity types is harmless (the backend ignores it) but we keep it to the
    // city-scoped case so cache keys stay clean for the global pages.
    cities: cityScoped ? selectedCities : undefined,
  })
  const allTags = data?.tags ?? []
  // When entity_type is set, the API returns per-entity-type counts.
  // - Default (city-agnostic) mode: hide zero-count chips so the surface area
  //   shrinks to *useful* filters; the user can expand to see the full vocab.
  // - City-scoped mode (PSY-982): show every chip, but render zero-in-city
  //   chips disabled — the user sees the tag exists with no shows here rather
  //   than it vanishing, and a disabled chip can never dead-end on click.
  // Selected chips stay visible regardless of count so the selection controls
  // remain reachable (preserves clear/toggle UX across navigation).
  const visibleTags =
    entityType && !cityScoped && !showAll
      ? allTags.filter(
          tag => tag.usage_count > 0 || selectedSet.has(tag.slug),
        )
      : allTags
  const label = getCategoryLabel(category)

  if (!isLoading && visibleTags.length === 0) return null

  const chips = visibleTags.map(tag => (
    <TagChip
      key={tag.id}
      tag={tag}
      selected={selectedSet.has(tag.slug)}
      onToggle={onToggle}
      // Disable only an unselected zero-in-city chip: a selected chip
      // must stay interactive so the user can always deselect it.
      disabled={
        cityScoped && tag.usage_count === 0 && !selectedSet.has(tag.slug)
      }
    />
  ))

  const skeletons = Array.from({ length: 6 }).map((_, i) => (
    <Skeleton key={i} className="h-6 w-16 rounded-full" />
  ))

  // Bar layout (PSY-1000): emit chips bare so they flow into the parent's
  // single wrapping row. No heading, no own container — the chip color
  // already signals the category. We still tag the group with a testid so
  // tests can scope assertions to a category in either layout.
  if (inline) {
    return (
      <span
        className="contents"
        data-testid={`tag-facet-category-${category}`}
      >
        {isLoading ? skeletons : chips}
      </span>
    )
  }

  return (
    <div className="space-y-2" data-testid={`tag-facet-category-${category}`}>
      <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </h3>
      <div className="flex flex-wrap gap-1.5">{isLoading ? skeletons : chips}</div>
    </div>
  )
}

interface TagChipProps {
  tag: TagListItem
  selected: boolean
  onToggle: (slug: string) => void
  /** Zero-match chip in city-scoped mode: shown but non-interactive. */
  disabled?: boolean
}

function TagChip({ tag, selected, onToggle, disabled = false }: TagChipProps) {
  const catColors = getCategoryColor(tag.category)
  return (
    <button
      type="button"
      onClick={() => onToggle(tag.slug)}
      aria-pressed={selected}
      disabled={disabled}
      title={disabled ? `No shows for ${tag.name} in the selected city` : undefined}
      data-testid={`tag-facet-chip-${tag.slug}`}
      className={cn(
        'inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium',
        'transition-all duration-100 select-none',
        disabled
          ? 'cursor-not-allowed opacity-40'
          : 'cursor-pointer',
        selected
          ? 'bg-primary text-primary-foreground border-primary shadow-sm ring-1 ring-primary/40'
          : `${catColors} ${disabled ? '' : 'hover:bg-muted/60'}`
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
