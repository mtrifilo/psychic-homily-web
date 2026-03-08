'use client'

import { useState, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useReleases } from '@/lib/hooks/useReleases'
import { ReleaseCard } from './ReleaseCard'
import { LoadingSpinner, DensityToggle } from '@/components/shared'
import { useDensity } from '@/lib/hooks/useDensity'
import { Button } from '@/components/ui/button'
import { RELEASE_TYPES, RELEASE_TYPE_LABELS } from '@/lib/types/release'
import type { ReleaseType } from '@/lib/types/release'

export function ReleaseList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const { density } = useDensity('releases')

  // Parse filters from URL
  const typeParam = searchParams.get('type') as ReleaseType | null
  const yearParam = searchParams.get('year')
  const [yearInput, setYearInput] = useState(yearParam ?? '')

  const { data, isLoading, isFetching, error, refetch } = useReleases({
    releaseType: typeParam ?? undefined,
    year: yearParam ? parseInt(yearParam, 10) : undefined,
  })

  const updateFilters = (params: { type?: string | null; year?: string | null }) => {
    const newParams = new URLSearchParams()
    const newType = params.type !== undefined ? params.type : typeParam
    const newYear = params.year !== undefined ? params.year : yearParam

    if (newType) newParams.set('type', newType)
    if (newYear) newParams.set('year', newYear)

    const queryString = newParams.toString()
    startTransition(() => {
      router.push(queryString ? `/releases?${queryString}` : '/releases')
    })
  }

  const handleTypeChange = (type: string | null) => {
    updateFilters({ type })
  }

  const handleYearSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = yearInput.trim()
    if (trimmed && /^\d{4}$/.test(trimmed)) {
      updateFilters({ year: trimmed })
    } else if (!trimmed) {
      updateFilters({ year: null })
    }
  }

  const clearFilters = () => {
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
  const hasFilters = !!typeParam || !!yearParam

  return (
    <section className="w-full max-w-6xl">
      {/* Filters */}
      <div className="mb-6 space-y-4">
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

      <div className="flex justify-end mb-4">
        <DensityToggle storageKey="releases" />
      </div>

      {/* Release Grid */}
      <div
        className={
          isUpdating
            ? 'opacity-60 transition-opacity duration-75'
            : 'transition-opacity duration-75'
        }
      >
        {releases.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            <p>
              {hasFilters
                ? 'No releases found matching your filters.'
                : 'No releases available at this time.'}
            </p>
            {hasFilters && (
              <button
                onClick={clearFilters}
                className="mt-4 text-primary hover:underline"
              >
                View all releases
              </button>
            )}
          </div>
        ) : (
          <div
            className={
              density === 'compact'
                ? 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-2'
                : density === 'expanded'
                  ? 'grid grid-cols-1 sm:grid-cols-2 gap-4'
                  : 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3'
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
        )}
      </div>
    </section>
  )
}
