'use client'

import { useCallback, useMemo, useState, useEffect, useRef, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { Search, ChevronLeft, ChevronRight } from 'lucide-react'
import { useReleases } from '../hooks/useReleases'
import { useLabels } from '@/features/labels/hooks/useLabels'
import { ReleaseCard } from './ReleaseCard'
import { LoadingSpinner, DensityToggle } from '@/components/shared'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { RELEASE_TYPES, RELEASE_TYPE_LABELS, RELEASE_SORT_OPTIONS } from '../types'
import type { ReleaseType, ReleaseSortOption } from '../types'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

const PAGE_SIZE = 50

export function ReleaseList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const { density, setDensity } = useDensity('releases')

  // Parse filters from URL
  const typeParam = searchParams.get('type') as ReleaseType | null
  const yearParam = searchParams.get('year')
  const searchParam = searchParams.get('search') ?? ''
  const sortParam = (searchParams.get('sort') as ReleaseSortOption) ?? 'newest'
  const labelIdParam = searchParams.get('label_id')
  const pageParam = searchParams.get('page')

  const currentPage = pageParam ? Math.max(1, parseInt(pageParam, 10)) : 1
  const offset = (currentPage - 1) * PAGE_SIZE

  // Parse multi-tag from URL (PSY-309)
  const tagsParam = searchParams.get('tags')
  const tagMatchParam = searchParams.get('tag_match')
  const selectedTags = useMemo(() => parseTagsParam(tagsParam), [tagsParam])
  const tagMatch: 'all' | 'any' = tagMatchParam === 'any' ? 'any' : 'all'

  // Local state for debounced search
  const [searchInput, setSearchInput] = useState(searchParam)
  const [yearInput, setYearInput] = useState(yearParam ?? '')
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Sync search input when URL changes externally
  useEffect(() => {
    setSearchInput(searchParam)
  }, [searchParam])

  // Fetch releases
  const { data, isLoading, isFetching, error, refetch } = useReleases({
    releaseType: typeParam ?? undefined,
    year: yearParam ? parseInt(yearParam, 10) : undefined,
    search: searchParam || undefined,
    sort: sortParam,
    labelId: labelIdParam ? parseInt(labelIdParam, 10) : undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
    limit: PAGE_SIZE,
    offset,
  })

  // Fetch labels for filter dropdown
  const { data: labelsData } = useLabels()
  const labels = labelsData?.labels ?? []

  // URL update helper — preserves existing params unless explicitly overridden
  const updateFilters = (params: Record<string, string | null>) => {
    const newParams = new URLSearchParams()

    // Merge current and new params
    const mergedType = params.type !== undefined ? params.type : typeParam
    const mergedYear = params.year !== undefined ? params.year : yearParam
    const mergedSearch = params.search !== undefined ? params.search : searchParam
    const mergedSort = params.sort !== undefined ? params.sort : sortParam
    const mergedLabelId = params.label_id !== undefined ? params.label_id : labelIdParam
    const mergedPage = params.page !== undefined ? params.page : null // Reset page on filter change unless explicitly set
    const mergedTags = params.tags !== undefined ? params.tags : (selectedTags.length > 0 ? buildTagsParam(selectedTags) : null)
    const mergedTagMatch = params.tag_match !== undefined ? params.tag_match : (tagMatch === 'any' ? 'any' : null)

    if (mergedType) newParams.set('type', mergedType)
    if (mergedYear) newParams.set('year', mergedYear)
    if (mergedSearch) newParams.set('search', mergedSearch)
    if (mergedSort && mergedSort !== 'newest') newParams.set('sort', mergedSort)
    if (mergedLabelId) newParams.set('label_id', mergedLabelId)
    if (mergedPage && mergedPage !== '1') newParams.set('page', mergedPage)
    if (mergedTags) newParams.set('tags', mergedTags)
    if (mergedTagMatch) newParams.set('tag_match', mergedTagMatch)

    const queryString = newParams.toString()
    startTransition(() => {
      router.push(queryString ? `/releases?${queryString}` : '/releases', { scroll: false })
    })
  }

  const handleTagsChange = useCallback((nextTags: string[]) => {
    updateFilters({
      tags: nextTags.length > 0 ? buildTagsParam(nextTags) : null,
      page: null,
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [typeParam, yearParam, searchParam, sortParam, labelIdParam, tagMatch])

  const handleTagsClear = useCallback(() => {
    updateFilters({ tags: null, page: null })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [typeParam, yearParam, searchParam, sortParam, labelIdParam])

  // Debounced search
  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setSearchInput(value)

    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      updateFilters({ search: value || null, page: null })
    }, 300)
  }

  const handleTypeChange = (type: string | null) => {
    updateFilters({ type, page: null })
  }

  const handleSortChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    updateFilters({ sort: e.target.value || null, page: null })
  }

  const handleLabelChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    updateFilters({ label_id: e.target.value || null, page: null })
  }

  const handleYearSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = yearInput.trim()
    if (trimmed && /^\d{4}$/.test(trimmed)) {
      updateFilters({ year: trimmed, page: null })
    } else if (!trimmed) {
      updateFilters({ year: null, page: null })
    }
  }

  const handlePageChange = (page: number) => {
    updateFilters({ page: page > 1 ? page.toString() : null })
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const clearFilters = () => {
    setSearchInput('')
    setYearInput('')
    startTransition(() => {
      router.push('/releases')
    })
  }

  if (isLoading && !data) {
    return (
      <div className="flex justify-center items-center py-12">
        <LoadingSpinner />
      </div>
    )
  }

  const isUpdating = isFetching || isPending

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load releases. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  const releases = data?.releases ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / PAGE_SIZE)
  const hasFilters = !!typeParam || !!yearParam || !!searchParam || !!labelIdParam || sortParam !== 'newest'

  return (
    <section className="w-full max-w-6xl">
      {/* Filters */}
      <div className="mb-6 space-y-4">
        {/* Search + Sort + Label row */}
        <div className="flex flex-wrap items-center gap-3">
          {/* Search */}
          <div className="relative w-full max-w-xs">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
            <Input
              type="text"
              value={searchInput}
              onChange={handleSearchChange}
              placeholder="Search by title or artist..."
              autoComplete="off"
              className="pl-8"
            />
          </div>

          {/* Sort */}
          <select
            value={sortParam}
            onChange={handleSortChange}
            className="h-9 rounded-md border border-border bg-background px-3 text-sm focus:outline-none focus:ring-1 focus:ring-ring"
          >
            {RELEASE_SORT_OPTIONS.map(opt => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>

          {/* Label filter */}
          {labels.length > 0 && (
            <select
              value={labelIdParam ?? ''}
              onChange={handleLabelChange}
              className="h-9 rounded-md border border-border bg-background px-3 text-sm focus:outline-none focus:ring-1 focus:ring-ring max-w-[200px]"
            >
              <option value="">All Labels</option>
              {labels.map(label => (
                <option key={label.id} value={label.id}>
                  {label.name}
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Release Type Filter */}
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-muted-foreground mr-1">Type:</span>
          <button
            onClick={() => handleTypeChange(null)}
            className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${
              !typeParam
                ? 'bg-background text-foreground shadow-sm border border-border/50'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            All
          </button>
          {RELEASE_TYPES.map(type => (
            <button
              key={type}
              onClick={() => handleTypeChange(type)}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${
                typeParam === type
                  ? 'bg-background text-foreground shadow-sm border border-border/50'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {RELEASE_TYPE_LABELS[type]}
            </button>
          ))}
        </div>

        {/* Year Filter */}
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground mr-1">Year:</span>
          <form onSubmit={handleYearSubmit} className="flex items-center gap-2">
            <input
              type="text"
              inputMode="numeric"
              pattern="\d{4}"
              maxLength={4}
              placeholder="e.g. 2024"
              value={yearInput}
              onChange={e => setYearInput(e.target.value)}
              className="w-24 rounded-md border border-border/50 bg-background px-2.5 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-ring"
            />
            <Button type="submit" variant="outline" size="sm" className="text-xs h-7">
              Filter
            </Button>
          </form>
          {hasFilters && (
            <button
              onClick={clearFilters}
              className="text-xs text-muted-foreground hover:text-foreground underline"
            >
              Clear filters
            </button>
          )}
        </div>
      </div>

      <div className="flex items-center justify-between mb-4">
        <span className="text-sm text-muted-foreground" data-testid="release-count">
          {total} {total === 1 ? 'release' : 'releases'}
          {selectedTags.length > 0 && ` matching ${selectedTags.join(', ')}`}
        </span>
        <div className="flex items-center gap-2">
          <TagFacetSheet
            selectedSlugs={selectedTags}
            onToggle={handleTagsChange}
            onClear={handleTagsClear}
            title="Filter releases by tag"
            entityType="release"
          />
          <DensityToggle density={density} onDensityChange={setDensity} />
        </div>
      </div>

      <div className="flex flex-col gap-6 lg:flex-row">
        <aside className="hidden lg:block lg:w-64 lg:shrink-0">
          <TagFacetPanel
            selectedSlugs={selectedTags}
            onToggle={handleTagsChange}
            onClear={handleTagsClear}
            heading="Filter releases by tag"
            entityType="release"
          />
        </aside>

        <div className={`flex-1 min-w-0 ${isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}`}>
          {releases.length === 0 ? (
            <div className="text-center py-12 text-muted-foreground">
              <p>
                {hasFilters || selectedTags.length > 0
                  ? 'No releases found matching your filters.'
                  : 'No releases available at this time.'}
              </p>
              {(hasFilters || selectedTags.length > 0) && (
                <button
                  onClick={() => {
                    clearFilters()
                    if (selectedTags.length > 0) handleTagsClear()
                  }}
                  className="mt-4 text-primary hover:underline"
                >
                  View all releases
                </button>
              )}
            </div>
          ) : (
            <div className="@container">
              <div
                className={
                  density === 'compact'
                    ? 'flex flex-col gap-px'
                    : density === 'expanded'
                      ? 'grid grid-cols-1 gap-5'
                      : 'grid grid-cols-1 @sm:grid-cols-2 @2xl:grid-cols-3 gap-3'
                }
              >
                {releases.map(release => (
                  <ReleaseCard
                    key={release.id}
                    release={release}
                    density={density}
                  />
                ))}
              </div>
            </div>
          )}

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-center gap-2 mt-8">
              <Button
                variant="outline"
                size="sm"
                disabled={currentPage <= 1}
                onClick={() => handlePageChange(currentPage - 1)}
              >
                <ChevronLeft className="h-4 w-4 mr-1" />
                Previous
              </Button>
              <span className="text-sm text-muted-foreground px-3">
                Page {currentPage} of {totalPages}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={currentPage >= totalPages}
                onClick={() => handlePageChange(currentPage + 1)}
              >
                Next
                <ChevronRight className="h-4 w-4 ml-1" />
              </Button>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}
