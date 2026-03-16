'use client'

import { useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { cn } from '@/lib/utils'
import { useLabels } from '../hooks/useLabels'
import { LabelCard } from './LabelCard'
import { LoadingSpinner, DensityToggle } from '@/components/shared'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { Button } from '@/components/ui/button'
import { LABEL_STATUSES, LABEL_STATUS_LABELS } from '../types'
import type { LabelStatus } from '../types'

export function LabelList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const { density } = useDensity('labels')

  // Parse filters from URL
  const statusParam = searchParams.get('status') as LabelStatus | null

  const { data, isLoading, isFetching, error, refetch } = useLabels({
    status: statusParam ?? undefined,
  })

  const updateFilters = (params: { status?: string | null }) => {
    const newParams = new URLSearchParams()
    const newStatus =
      params.status !== undefined ? params.status : statusParam

    if (newStatus) newParams.set('status', newStatus)

    const queryString = newParams.toString()
    startTransition(() => {
      router.push(queryString ? `/labels?${queryString}` : '/labels')
    })
  }

  const handleStatusChange = (status: string | null) => {
    updateFilters({ status })
  }

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
  const hasFilters = !!statusParam

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

      <div className="flex justify-end mb-4">
        <DensityToggle storageKey="labels" />
      </div>

      {/* Label Grid */}
      <div
        className={
          isUpdating
            ? 'opacity-60 transition-opacity duration-75'
            : 'transition-opacity duration-75'
        }
      >
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
          <div
            className={cn(
              '@container',
              density === 'compact'
                ? 'flex flex-col gap-px'
                : density === 'expanded'
                  ? 'grid grid-cols-1 gap-5'
                  : 'grid grid-cols-1 @sm:grid-cols-2 @2xl:grid-cols-3 gap-3'
            )}
          >
            {labels.map(label => (
              <LabelCard key={label.id} label={label} density={density} />
            ))}
          </div>
        )}
      </div>
    </section>
  )
}
