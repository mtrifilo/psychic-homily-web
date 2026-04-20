'use client'

import { useState, useTransition } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { Search, Hash, Loader2, LayoutGrid, Cloud } from 'lucide-react'
import { useDebounce } from 'use-debounce'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { useTags } from '../hooks'
import {
  TAG_CATEGORIES,
  TAG_SORT_OPTIONS,
  DEFAULT_TAG_SORT,
  DEFAULT_TAG_VIEW,
  getCategoryColor,
  getCategoryLabel,
} from '../types'
import type { TagListItem, TagSortOption, TagView } from '../types'
import { TagOfficialIndicator } from './TagOfficialIndicator'

const PAGE_SIZE = 50
const SEARCH_DEBOUNCE_MS = 300

// Cloud view: font-size interpolates on a log scale so a long usage_count
// tail (a handful of mega-tags + many small ones) stays readable instead of
// being dominated by one giant word.
const CLOUD_MIN_PX = 13
const CLOUD_MAX_PX = 30

function toSort(value: string | null): TagSortOption {
  const match = TAG_SORT_OPTIONS.find(o => o.value === value)
  return match?.value ?? DEFAULT_TAG_SORT
}

function toView(value: string | null): TagView {
  return value === 'cloud' ? 'cloud' : 'grid'
}

function sortToBackend(sort: TagSortOption): string {
  return TAG_SORT_OPTIONS.find(o => o.value === sort)?.backend ?? 'usage'
}

export function cloudFontSizePx(
  usageCount: number,
  minUsage: number,
  maxUsage: number
): number {
  if (maxUsage <= minUsage) return (CLOUD_MIN_PX + CLOUD_MAX_PX) / 2
  const lo = Math.log(Math.max(1, minUsage) + 1)
  const hi = Math.log(Math.max(1, maxUsage) + 1)
  const v = Math.log(Math.max(1, usageCount) + 1)
  const t = (v - lo) / (hi - lo)
  return CLOUD_MIN_PX + t * (CLOUD_MAX_PX - CLOUD_MIN_PX)
}

export function TagBrowse() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()

  const sort = toSort(searchParams.get('sort'))
  const view = toView(searchParams.get('view'))

  const [category, setCategory] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch] = useDebounce(searchInput.trim(), SEARCH_DEBOUNCE_MS)
  const [offset, setOffset] = useState(0)

  const { data, isLoading, error, refetch } = useTags({
    category: category || undefined,
    search: debouncedSearch || undefined,
    limit: PAGE_SIZE,
    offset,
    sort: sortToBackend(sort),
  })

  const tags = data?.tags ?? []
  const total = data?.total ?? 0
  const hasMore = offset + PAGE_SIZE < total

  const updateParams = (patch: Partial<{ sort: TagSortOption; view: TagView }>) => {
    const next = new URLSearchParams(searchParams.toString())
    const nextSort = patch.sort ?? sort
    const nextView = patch.view ?? view
    if (nextSort === DEFAULT_TAG_SORT) next.delete('sort')
    else next.set('sort', nextSort)
    if (nextView === DEFAULT_TAG_VIEW) next.delete('view')
    else next.set('view', nextView)
    const qs = next.toString()
    startTransition(() => {
      router.replace(qs ? `/tags?${qs}` : '/tags', { scroll: false })
    })
  }

  const handleCategoryChange = (newCategory: string) => {
    setCategory(newCategory)
    setOffset(0)
  }

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load tags. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  const emptyStateMessage = debouncedSearch
    ? null
    : category
      ? `No ${getCategoryLabel(category).toLowerCase()} tags yet.`
      : 'No tags found.'

  const usageValues = tags.map(t => t.usage_count)
  const minUsage = usageValues.length ? Math.min(...usageValues) : 0
  const maxUsage = usageValues.length ? Math.max(...usageValues) : 0

  return (
    <section className="w-full max-w-6xl" aria-busy={isPending}>
      {/* Search + controls */}
      <div className="mb-6 flex flex-wrap items-center gap-3">
        <div className="relative w-full max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            type="search"
            value={searchInput}
            onChange={e => {
              setSearchInput(e.target.value)
              setOffset(0)
            }}
            placeholder="Search tags..."
            className="pl-9"
            aria-label="Search tags"
          />
        </div>

        <label className="sr-only" htmlFor="tag-sort">
          Sort tags
        </label>
        <select
          id="tag-sort"
          value={sort}
          onChange={e => updateParams({ sort: toSort(e.target.value) })}
          className="h-9 rounded-md border border-border bg-background px-3 text-sm focus:outline-none focus:ring-1 focus:ring-ring"
          aria-label="Sort tags"
        >
          {TAG_SORT_OPTIONS.map(opt => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>

        <div
          className="inline-flex items-center rounded-lg border border-border/50 bg-muted/30 p-0.5"
          role="radiogroup"
          aria-label="Tag layout"
        >
          {(
            [
              { value: 'grid' as const, label: 'Grid', Icon: LayoutGrid },
              { value: 'cloud' as const, label: 'Cloud', Icon: Cloud },
            ]
          ).map(({ value, label, Icon }) => (
            <button
              key={value}
              type="button"
              role="radio"
              aria-checked={view === value}
              onClick={() => updateParams({ view: value })}
              data-testid={`view-${value}`}
              className={cn(
                'flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md transition-colors duration-100',
                view === value
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              )}
            >
              <Icon className="h-3.5 w-3.5" />
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* Category filter tabs */}
      <div className="flex items-center gap-1.5 flex-wrap mb-6">
        <button
          onClick={() => handleCategoryChange('')}
          className={cn(
            'rounded-full px-3 py-1.5 text-xs font-medium transition-colors',
            !category
              ? 'bg-foreground text-background'
              : 'bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground'
          )}
        >
          All
        </button>
        {TAG_CATEGORIES.map(cat => (
          <button
            key={cat}
            onClick={() => handleCategoryChange(cat)}
            className={cn(
              'rounded-full px-3 py-1.5 text-xs font-medium transition-colors border',
              category === cat
                ? getCategoryColor(cat)
                : 'bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground border-transparent'
            )}
          >
            {getCategoryLabel(cat)}
          </button>
        ))}
      </div>

      {/* Results count */}
      {total > 0 && (
        <p className="text-sm text-muted-foreground mb-4">
          {total} {total === 1 ? 'tag' : 'tags'} found
        </p>
      )}

      {/* Loading state */}
      {isLoading && !data && (
        <div className="flex justify-center items-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {/* Tag list */}
      {!isLoading && tags.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          {debouncedSearch ? (
            <>
              <p>
                No tags match <span className="font-medium">&ldquo;{debouncedSearch}&rdquo;</span>.
              </p>
              <p className="text-sm mt-2">Try a different search term.</p>
            </>
          ) : (
            <p>{emptyStateMessage}</p>
          )}
        </div>
      ) : view === 'cloud' ? (
        <div
          className="flex flex-wrap items-center gap-x-3 gap-y-2 leading-tight"
          data-testid="tag-cloud"
        >
          {tags.map(tag => (
            <TagCloudItem
              key={tag.id}
              tag={tag}
              fontSizePx={cloudFontSizePx(tag.usage_count, minUsage, maxUsage)}
            />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {tags.map((tag: TagListItem) => (
            <TagCard key={tag.id} tag={tag} />
          ))}
        </div>
      )}

      {/* Pagination */}
      {(offset > 0 || hasMore) && (
        <div className="flex items-center justify-center gap-3 mt-8">
          <Button
            variant="outline"
            size="sm"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
          >
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {Math.floor(offset / PAGE_SIZE) + 1} of{' '}
            {Math.ceil(total / PAGE_SIZE)}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasMore}
            onClick={() => setOffset(offset + PAGE_SIZE)}
          >
            Next
          </Button>
        </div>
      )}
    </section>
  )
}

// ──────────────────────────────────────────────
// Tag Card (grid view)
// ──────────────────────────────────────────────

function TagCard({ tag }: { tag: TagListItem }) {
  return (
    <Link
      href={`/tags/${tag.slug}`}
      className="group flex items-start gap-3 rounded-lg border border-border/50 bg-card p-4 hover:border-border hover:bg-muted/30 transition-colors"
    >
      <div className="mt-0.5">
        <Hash className="h-4 w-4 text-muted-foreground group-hover:text-foreground transition-colors" />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <span className="font-medium text-sm group-hover:text-foreground truncate">
            {tag.name}
          </span>
          {tag.is_official && (
            <TagOfficialIndicator size="md" tagName={tag.name} />
          )}
        </div>
        <div className="flex items-center gap-2">
          <span
            className={cn(
              'inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-medium',
              getCategoryColor(tag.category)
            )}
          >
            {getCategoryLabel(tag.category)}
          </span>
          <span className="text-xs text-muted-foreground">
            {tag.usage_count} {tag.usage_count === 1 ? 'use' : 'uses'}
          </span>
        </div>
      </div>
    </Link>
  )
}

// ──────────────────────────────────────────────
// Tag Cloud Item (cloud view)
// ──────────────────────────────────────────────

function TagCloudItem({
  tag,
  fontSizePx,
}: {
  tag: TagListItem
  fontSizePx: number
}) {
  return (
    <Link
      href={`/tags/${tag.slug}`}
      data-testid={`tag-cloud-item-${tag.slug}`}
      style={{ fontSize: `${fontSizePx}px` }}
      className={cn(
        'inline-flex items-center gap-1 rounded-md px-2 py-0.5',
        'text-muted-foreground hover:text-foreground hover:bg-muted/40 transition-colors'
      )}
      title={`${tag.name} (${tag.usage_count} ${tag.usage_count === 1 ? 'use' : 'uses'})`}
    >
      <span>{tag.name}</span>
      {tag.is_official && <TagOfficialIndicator size="sm" tagName={tag.name} />}
    </Link>
  )
}
