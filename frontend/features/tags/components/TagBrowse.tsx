'use client'

import { useEffect, useRef, useState, useTransition } from 'react'
import type { KeyboardEvent } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { Search, Hash, Loader2 } from 'lucide-react'
import { useDebounce } from 'use-debounce'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { DenseTable } from '@/components/shared'
import { useTags } from '../hooks'
import {
  TAG_CATEGORIES,
  TAG_SORT_OPTIONS,
  DEFAULT_TAG_SORT,
  getCategoryColor,
  getCategoryLabel,
} from '../types'
import type { TagListItem, TagSortOption } from '../types'
import { TagOfficialIndicator } from './TagOfficialIndicator'

const PAGE_SIZE = 50
const SEARCH_DEBOUNCE_MS = 300

function toSort(value: string | null): TagSortOption {
  const match = TAG_SORT_OPTIONS.find(o => o.value === value)
  return match?.value ?? DEFAULT_TAG_SORT
}

function sortToBackend(sort: TagSortOption): string {
  return TAG_SORT_OPTIONS.find(o => o.value === sort)?.backend ?? 'usage'
}

/**
 * Per-category total counts for the facet chips, scoped to the active search
 * term but NOT to the selected category — each chip reports how many tags
 * that facet would surface, so a chip can be disabled when it has none.
 * One `useTags({category, limit:1})` per category (3 categories ⇒ 3 bounded
 * requests, same approach as TagFacetPanel). `all` is the cross-category sum.
 */
function useCategoryCounts(search: string | undefined): {
  counts: Record<string, number>
  all: number
} {
  const genre = useTags({ category: 'genre', search, limit: 1 })
  const locale = useTags({ category: 'locale', search, limit: 1 })
  const other = useTags({ category: 'other', search, limit: 1 })

  const counts: Record<string, number> = {
    genre: genre.data?.total ?? 0,
    locale: locale.data?.total ?? 0,
    other: other.data?.total ?? 0,
  }
  return { counts, all: counts.genre + counts.locale + counts.other }
}

export function TagBrowse() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()

  const sort = toSort(searchParams.get('sort'))

  const [category, setCategory] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch] = useDebounce(searchInput.trim(), SEARCH_DEBOUNCE_MS)
  const [offset, setOffset] = useState(0)

  // Autofocus the search box on mount so a returning visitor can type-to-filter
  // immediately (acceptance criterion + Figma 412:7 placeholder copy).
  const searchRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    searchRef.current?.focus()
  }, [])

  const { data, isLoading, error, refetch } = useTags({
    category: category || undefined,
    search: debouncedSearch || undefined,
    limit: PAGE_SIZE,
    offset,
    sort: sortToBackend(sort),
  })

  const { counts, all: allCount } = useCategoryCounts(debouncedSearch || undefined)

  const tags = data?.tags ?? []
  const total = data?.total ?? 0
  const hasMore = offset + PAGE_SIZE < total
  const pageStart = total === 0 ? 0 : offset + 1
  const pageEnd = Math.min(offset + PAGE_SIZE, total)

  const updateSort = (nextSort: TagSortOption) => {
    const next = new URLSearchParams(searchParams.toString())
    if (nextSort === DEFAULT_TAG_SORT) next.delete('sort')
    else next.set('sort', nextSort)
    const qs = next.toString()
    setOffset(0)
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

  return (
    <section className="w-full max-w-6xl" aria-busy={isPending}>
      {/* Search + sort controls */}
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <div className="relative min-w-0 flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            ref={searchRef}
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

        <div
          className="inline-flex items-center rounded-lg border border-border/50 bg-muted/30 p-0.5"
          role="radiogroup"
          aria-label="Sort tags"
        >
          {TAG_SORT_OPTIONS.map(opt => (
            <button
              key={opt.value}
              type="button"
              role="radio"
              aria-checked={sort === opt.value}
              onClick={() => updateSort(opt.value)}
              data-testid={`sort-${opt.value}`}
              className={cn(
                'rounded-md px-2.5 py-1 text-xs font-medium transition-colors duration-100',
                sort === opt.value
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              )}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      {/* Category facet chips + result range */}
      <div className="mb-4 flex flex-wrap items-center gap-1.5">
        <FacetChip
          label="All"
          count={allCount}
          active={!category}
          onClick={() => handleCategoryChange('')}
        />
        {TAG_CATEGORIES.map(cat => (
          <FacetChip
            key={cat}
            label={getCategoryLabel(cat)}
            count={counts[cat] ?? 0}
            active={category === cat}
            categoryTint={getCategoryColor(cat)}
            onClick={() => handleCategoryChange(cat)}
          />
        ))}
        {total > 0 && (
          <p className="ml-auto text-sm text-muted-foreground">
            {total} {total === 1 ? 'tag' : 'tags'}
          </p>
        )}
      </div>

      {/* Loading state */}
      {isLoading && !data && (
        <div className="flex justify-center items-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {/* Tag directory table */}
      {!isLoading && tags.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          {debouncedSearch ? (
            <>
              <p>
                No tags match{' '}
                <span className="font-medium">&ldquo;{debouncedSearch}&rdquo;</span>.
              </p>
              <p className="text-sm mt-2">Try a different search term.</p>
            </>
          ) : (
            <p>{emptyStateMessage}</p>
          )}
        </div>
      ) : (
        tags.length > 0 && <TagDirectoryTable tags={tags} />
      )}

      {/* Pagination */}
      {(offset > 0 || hasMore) && (
        <div className="mt-8 flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm text-muted-foreground">
            Showing {pageStart}–{pageEnd} of {total}
          </p>
          <div className="flex items-center gap-3">
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
        </div>
      )}
    </section>
  )
}

// ──────────────────────────────────────────────
// Facet chip (All / Genre / Locale / Other)
// ──────────────────────────────────────────────

function FacetChip({
  label,
  count,
  active,
  categoryTint,
  onClick,
}: {
  label: string
  count: number
  active: boolean
  /** Category tint classes (from getCategoryColor) applied when active. */
  categoryTint?: string
  onClick: () => void
}) {
  // Zero-result facets are disabled — clicking them would only show an empty
  // table. An active chip stays interactive so the user can always deselect.
  const disabled = count === 0 && !active
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-pressed={active}
      data-testid={`facet-${label.toLowerCase()}`}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors',
        active
          ? categoryTint
            ? categoryTint
            : 'border-foreground bg-foreground text-background'
          : 'border-border bg-muted/40 text-muted-foreground hover:bg-muted/70 hover:text-foreground',
        disabled && 'cursor-not-allowed opacity-40 hover:bg-muted/40 hover:text-muted-foreground'
      )}
    >
      <span>{label}</span>
      <span className="tabular-nums opacity-70">{count}</span>
    </button>
  )
}

// ──────────────────────────────────────────────
// Tag directory table (dense, sortable, zebra-striped)
// ──────────────────────────────────────────────

function TagDirectoryTable({ tags }: { tags: TagListItem[] }) {
  // Keyboard nav: ↑/↓ move focus between row links; Enter is native on the
  // anchor. Refs index by row order so arrow keys can shift focus.
  const rowRefs = useRef<(HTMLAnchorElement | null)[]>([])

  const handleKeyDown = (e: KeyboardEvent<HTMLTableSectionElement>) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    const current = rowRefs.current.findIndex(el => el === document.activeElement)
    if (current === -1) return
    e.preventDefault()
    const nextIndex =
      e.key === 'ArrowDown'
        ? Math.min(current + 1, tags.length - 1)
        : Math.max(current - 1, 0)
    rowRefs.current[nextIndex]?.focus()
  }

  return (
    <DenseTable variant="alternating">
      <thead>
        <tr>
          <th scope="col">Tag</th>
          <th scope="col">Category</th>
          <th scope="col" className="text-right">
            Uses
          </th>
        </tr>
      </thead>
      <tbody onKeyDown={handleKeyDown}>
        {tags.map((tag, i) => (
          <tr
            key={tag.id}
            className="cursor-pointer transition-colors hover:bg-muted/40"
          >
            <td>
              <Link
                ref={el => {
                  rowRefs.current[i] = el
                }}
                href={`/tags/${tag.slug}`}
                className="group inline-flex items-center gap-2 rounded-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <Hash
                  className="h-3.5 w-3.5 shrink-0 text-muted-foreground group-hover:text-foreground transition-colors"
                  aria-hidden
                />
                <span className="font-medium text-foreground group-hover:underline">
                  {tag.name}
                </span>
                {tag.is_official && (
                  <TagOfficialIndicator size="sm" tagName={tag.name} />
                )}
              </Link>
            </td>
            <td>
              <span className={cn('text-xs font-medium', categoryTextTint(tag.category))}>
                {getCategoryLabel(tag.category)}
              </span>
            </td>
            <td className="text-right tabular-nums text-muted-foreground">
              {tag.usage_count}
            </td>
          </tr>
        ))}
      </tbody>
    </DenseTable>
  )
}

/**
 * Category as a tinted TEXT label (not a pill). Derives the foreground tint
 * from `getCategoryColor` — the single source of truth for the genre→chart-6 /
 * locale→chart-8 / other→muted mapping — by keeping only its `text-*` token and
 * dropping the bg/border classes. Deriving (rather than re-hardcoding the map)
 * keeps the two surfaces from drifting. Contract: `getCategoryColor` must return
 * exactly one `text-*` token; if that ever stops holding, this falls back to
 * `text-muted-foreground` (and TagBrowse.test.tsx asserts the genre tint, so a
 * regression surfaces in CI rather than silently).
 */
function categoryTextTint(category: string): string {
  const text = getCategoryColor(category)
    .split(' ')
    .find(c => c.startsWith('text-'))
  return text ?? 'text-muted-foreground'
}
