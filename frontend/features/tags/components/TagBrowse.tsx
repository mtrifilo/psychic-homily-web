'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Search, Hash, Loader2 } from 'lucide-react'
import { useDebounce } from 'use-debounce'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useTags } from '../hooks'
import { TAG_CATEGORIES, getCategoryColor, getCategoryLabel } from '../types'
import type { TagListItem } from '../types'

const PAGE_SIZE = 50
const SEARCH_DEBOUNCE_MS = 300

export function TagBrowse() {
  const [category, setCategory] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch] = useDebounce(searchInput.trim(), SEARCH_DEBOUNCE_MS)
  const [offset, setOffset] = useState(0)

  const { data, isLoading, error, refetch } = useTags({
    category: category || undefined,
    search: debouncedSearch || undefined,
    limit: PAGE_SIZE,
    offset,
    sort: 'usage',
  })

  const tags = data?.tags ?? []
  const total = data?.total ?? 0
  const hasMore = offset + PAGE_SIZE < total

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

  return (
    <section className="w-full max-w-6xl">
      {/* Search */}
      <div className="mb-6">
        <div className="relative max-w-md">
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

      {/* Tag cards grid */}
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
            <p>No tags found.</p>
          )}
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
// Tag Card
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
            <Badge variant="secondary" className="text-[10px] px-1.5 py-0 shrink-0">
              Official
            </Badge>
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
