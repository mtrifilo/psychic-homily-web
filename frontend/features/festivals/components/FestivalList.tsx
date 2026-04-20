'use client'

import { useCallback, useMemo, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useFestivals } from '../hooks/useFestivals'
import { FestivalCard } from './FestivalCard'
import { LoadingSpinner, DensityToggle } from '@/components/shared'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { Button } from '@/components/ui/button'
import { FESTIVAL_STATUSES, FESTIVAL_STATUS_LABELS } from '../types'
import type { FestivalStatus } from '../types'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

export function FestivalList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const { density, setDensity } = useDensity('festivals')

  // Parse filters from URL
  const statusParam = searchParams.get('status') as FestivalStatus | null
  const yearParam = searchParams.get('year')
  const cityParam = searchParams.get('city')
  const tagsParam = searchParams.get('tags')
  const tagMatchParam = searchParams.get('tag_match')
  const selectedTags = useMemo(() => parseTagsParam(tagsParam), [tagsParam])
  const tagMatch: 'all' | 'any' = tagMatchParam === 'any' ? 'any' : 'all'

  const { data, isLoading, isFetching, error, refetch } = useFestivals({
    status: statusParam ?? undefined,
    year: yearParam ? parseInt(yearParam, 10) : undefined,
    city: cityParam ?? undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
  })

  const updateFilters = (params: {
    status?: string | null
    year?: string | null
    city?: string | null
    tags?: string | null
    tag_match?: string | null
  }) => {
    const newParams = new URLSearchParams()
    const newStatus =
      params.status !== undefined ? params.status : statusParam
    const newYear = params.year !== undefined ? params.year : yearParam
    const newCity = params.city !== undefined ? params.city : cityParam
    const newTags = params.tags !== undefined
      ? params.tags
      : (selectedTags.length > 0 ? buildTagsParam(selectedTags) : null)
    const newTagMatch = params.tag_match !== undefined
      ? params.tag_match
      : (tagMatch === 'any' ? 'any' : null)

    if (newStatus) newParams.set('status', newStatus)
    if (newYear) newParams.set('year', newYear)
    if (newCity) newParams.set('city', newCity)
    if (newTags) newParams.set('tags', newTags)
    if (newTagMatch) newParams.set('tag_match', newTagMatch)

    const queryString = newParams.toString()
    startTransition(() => {
      router.push(queryString ? `/festivals?${queryString}` : '/festivals', { scroll: false })
    })
  }

  const handleStatusChange = (status: string | null) => {
    updateFilters({ status })
  }

  const handleTagsChange = useCallback((nextTags: string[]) => {
    updateFilters({
      tags: nextTags.length > 0 ? buildTagsParam(nextTags) : null,
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusParam, yearParam, cityParam, tagMatch])

  const handleTagsClear = useCallback(() => {
    updateFilters({ tags: null })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusParam, yearParam, cityParam])

  const clearFilters = () => {
    startTransition(() => {
      router.push('/festivals')
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
        <p>Failed to load festivals. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  const festivals = data?.festivals ?? []
  const hasFilters = !!statusParam || !!yearParam || !!cityParam || selectedTags.length > 0

  return (
    <section className="w-full max-w-6xl">
      {/* Filters */}
      <div className="mb-6 space-y-4">
        {/* Status Filter */}
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-muted-foreground mr-1">Status:</span>
          <button
            onClick={() => handleStatusChange(null)}
            className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${
              !statusParam
                ? 'bg-background text-foreground shadow-sm border border-border/50'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            All
          </button>
          {FESTIVAL_STATUSES.map(status => (
            <button
              key={status}
              onClick={() => handleStatusChange(status)}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${
                statusParam === status
                  ? 'bg-background text-foreground shadow-sm border border-border/50'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {FESTIVAL_STATUS_LABELS[status]}
            </button>
          ))}
          {hasFilters && (
            <button
              onClick={clearFilters}
              className="text-xs text-muted-foreground hover:text-foreground underline ml-2"
            >
              Clear filters
            </button>
          )}
        </div>
      </div>

      <div className="flex items-center justify-between mb-4 gap-2">
        <TagFacetSheet
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          title="Filter festivals by tag"
          entityType="festival"
        />
        <DensityToggle density={density} onDensityChange={setDensity} />
      </div>

      <div className="flex flex-col gap-6 lg:flex-row">
        <aside className="hidden lg:block lg:w-64 lg:shrink-0">
          <TagFacetPanel
            selectedSlugs={selectedTags}
            onToggle={handleTagsChange}
            onClear={handleTagsClear}
            heading="Filter festivals by tag"
            entityType="festival"
          />
        </aside>

        <div className={`flex-1 min-w-0 ${isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}`}>
          <p className="mb-3 text-sm text-muted-foreground" data-testid="festival-count">
            {festivals.length} {festivals.length === 1 ? 'festival' : 'festivals'}
            {selectedTags.length > 0 && ` matching ${selectedTags.join(', ')}`}
          </p>
          {festivals.length === 0 ? (
            <div className="text-center py-12 text-muted-foreground">
              <p>
                {hasFilters
                  ? 'No festivals found matching your filters.'
                  : 'No festivals available at this time.'}
              </p>
              {hasFilters && (
                <button
                  onClick={clearFilters}
                  className="mt-4 text-primary hover:underline"
                >
                  View all festivals
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
                {festivals.map(festival => (
                  <FestivalCard
                    key={festival.id}
                    festival={festival}
                    density={density}
                  />
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}
