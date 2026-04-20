'use client'

import { useCallback, useMemo, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useLabels } from '../hooks/useLabels'
import { LabelCard } from './LabelCard'
import { LabelSearch } from './LabelSearch'
import { LoadingSpinner, DensityToggle } from '@/components/shared'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { Button } from '@/components/ui/button'
import { LABEL_STATUSES, LABEL_STATUS_LABELS } from '../types'
import type { LabelStatus } from '../types'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

export function LabelList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const { density, setDensity } = useDensity('labels')

  // Parse filters from URL
  const statusParam = searchParams.get('status') as LabelStatus | null
  const tagsParam = searchParams.get('tags')
  const tagMatchParam = searchParams.get('tag_match')
  const selectedTags = useMemo(() => parseTagsParam(tagsParam), [tagsParam])
  const tagMatch: 'all' | 'any' = tagMatchParam === 'any' ? 'any' : 'all'

  const { data, isLoading, isFetching, error, refetch } = useLabels({
    status: statusParam ?? undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
  })

  const writeParams = useCallback(
    (
      nextStatus: string | null = statusParam,
      nextTags: string[] = selectedTags,
      nextMatch: 'all' | 'any' = tagMatch
    ) => {
      const params = new URLSearchParams()
      if (nextStatus) params.set('status', nextStatus)
      if (nextTags.length > 0) {
        params.set('tags', buildTagsParam(nextTags))
        if (nextMatch === 'any') params.set('tag_match', 'any')
      }
      const queryString = params.toString()
      startTransition(() => {
        router.push(queryString ? `/labels?${queryString}` : '/labels', {
          scroll: false,
        })
      })
    },
    [statusParam, selectedTags, tagMatch, router]
  )

  const handleStatusChange = (status: string | null) => {
    writeParams(status, selectedTags, tagMatch)
  }

  const handleTagsChange = useCallback(
    (nextTags: string[]) => writeParams(statusParam, nextTags, tagMatch),
    [statusParam, tagMatch, writeParams]
  )

  const handleTagsClear = useCallback(
    () => writeParams(statusParam, [], tagMatch),
    [statusParam, tagMatch, writeParams]
  )

  const clearFilters = () => {
    startTransition(() => {
      router.push('/labels')
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
        <p>Failed to load labels. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  const labels = data?.labels ?? []
  const hasFilters = !!statusParam || selectedTags.length > 0

  return (
    <section className="w-full max-w-6xl">
      {/* Filters */}
      <div className="mb-6 space-y-4">
        <LabelSearch />
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
          {LABEL_STATUSES.map(status => (
            <button
              key={status}
              onClick={() => handleStatusChange(status)}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors ${
                statusParam === status
                  ? 'bg-background text-foreground shadow-sm border border-border/50'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {LABEL_STATUS_LABELS[status]}
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
          title="Filter labels by tag"
        />
        <DensityToggle density={density} onDensityChange={setDensity} />
      </div>

      <div className="flex flex-col gap-6 lg:flex-row">
        <aside className="hidden lg:block lg:w-64 lg:shrink-0">
          <TagFacetPanel
            selectedSlugs={selectedTags}
            onToggle={handleTagsChange}
            onClear={handleTagsClear}
            heading="Filter labels by tag"
          />
        </aside>

        <div className={`flex-1 min-w-0 ${isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}`}>
          <p className="mb-3 text-sm text-muted-foreground" data-testid="label-count">
            {labels.length} {labels.length === 1 ? 'label' : 'labels'}
            {selectedTags.length > 0 && ` matching ${selectedTags.join(', ')}`}
          </p>
          {labels.length === 0 ? (
            <div className="text-center py-12 text-muted-foreground">
              <p>
                {hasFilters
                  ? 'No labels found matching your filters.'
                  : 'No labels available at this time.'}
              </p>
              {hasFilters && (
                <button
                  onClick={clearFilters}
                  className="mt-4 text-primary hover:underline"
                >
                  View all labels
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
                {labels.map(label => (
                  <LabelCard key={label.id} label={label} density={density} />
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}
