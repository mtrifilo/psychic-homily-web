'use client'

import {
  useCallback,
  useMemo,
  useState,
  useEffect,
  useRef,
  useTransition,
} from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { Search } from 'lucide-react'
import { useReleaseSaveCountBatch } from '../hooks/useSavedReleases'
import { useReleases } from '../hooks/useReleases'
import { useLabels } from '@/features/labels/hooks/useLabels'
import { ReleaseCard } from './ReleaseCard'
import { LoadingSpinner, DensityToggle, Pagination } from '@/components/shared'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  RELEASE_TYPES,
  RELEASE_TYPE_LABELS,
  RELEASE_SORT_OPTIONS,
} from '../types'
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
  // `router` identity is stable across renders in the app router, so it is safe
  // as an effect dependency below.
  const [isPending, startTransition] = useTransition()
  const { density, setDensity } = useDensity('releases')
  const { isAuthenticated, user } = useAuthContext()

  // Parse filters from URL
  const typeParam = searchParams.get('type') as ReleaseType | null
  const yearParam = searchParams.get('year')
  const searchParam = searchParams.get('search') ?? ''
  const sortParam = (searchParams.get('sort') as ReleaseSortOption) ?? 'newest'
  const labelIdParam = searchParams.get('label_id')
  const pageParam = searchParams.get('page')

  // Non-finite input collapses to page one rather than propagating: `?page=abc`
  // parses to NaN, and NaN defeats every comparison it touches: it would slip
  // past the out-of-range check below AND reach the API as `offset=NaN`.
  const parsedPage = parseInt(pageParam ?? '', 10)
  const currentPage = Number.isFinite(parsedPage) ? Math.max(1, parsedPage) : 1
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

  // The search debounce was cleared before each restart but never on unmount, so
  // the last keystroke's timer still fired ~300ms after the list was gone and
  // pushed a `setState` into a torn-down React DOM. Harmless in the browser;
  // under vitest it lands after jsdom teardown and fails the whole run with
  // `ReferenceError: window is not defined`.
  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [])

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
  const releases = data?.releases ?? []
  // Resolved here rather than beside the render that uses them: the
  // out-of-range snap-back below is an effect, so it has to run before the
  // early returns that this component makes for the loading and error states.
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / PAGE_SIZE)
  const releaseSaveCounts = useReleaseSaveCountBatch(
    releases.map(release => release.id),
    isAuthenticated,
    user?.id
  )

  /**
   * Serializes the current filter set into a `/releases` URL, with `params`
   * overriding individual values. `page` is dropped unless explicitly passed,
   * so every filter change lands the reader back on page one.
   *
   * Shared by the filter controls (which navigate imperatively) and the pager
   * (which needs the same URL as a plain `href`), so a page link can never
   * drift from what a filter change would have written.
   */
  const buildReleasesHref = (params: Record<string, string | null>) => {
    const newParams = new URLSearchParams()

    // Merge current and new params
    const mergedType = params.type !== undefined ? params.type : typeParam
    const mergedYear = params.year !== undefined ? params.year : yearParam
    const mergedSearch =
      params.search !== undefined ? params.search : searchParam
    const mergedSort = params.sort !== undefined ? params.sort : sortParam
    const mergedLabelId =
      params.label_id !== undefined ? params.label_id : labelIdParam
    const mergedPage = params.page !== undefined ? params.page : null // Reset page on filter change unless explicitly set
    const mergedTags =
      params.tags !== undefined
        ? params.tags
        : selectedTags.length > 0
          ? buildTagsParam(selectedTags)
          : null
    const mergedTagMatch =
      params.tag_match !== undefined
        ? params.tag_match
        : tagMatch === 'any'
          ? 'any'
          : null

    if (mergedType) newParams.set('type', mergedType)
    if (mergedYear) newParams.set('year', mergedYear)
    if (mergedSearch) newParams.set('search', mergedSearch)
    if (mergedSort && mergedSort !== 'newest') newParams.set('sort', mergedSort)
    if (mergedLabelId) newParams.set('label_id', mergedLabelId)
    if (mergedPage && mergedPage !== '1') newParams.set('page', mergedPage)
    if (mergedTags) newParams.set('tags', mergedTags)
    if (mergedTagMatch) newParams.set('tag_match', mergedTagMatch)

    const queryString = newParams.toString()
    return queryString ? `/releases?${queryString}` : '/releases'
  }

  const updateFilters = (params: Record<string, string | null>) => {
    const href = buildReleasesHref(params)
    startTransition(() => {
      router.push(href, { scroll: false })
    })
  }

  /**
   * Page links are real URLs so the strip is middle-clickable and shareable;
   * the `<Link>` navigation writes the param, and this component reads it back
   * off `useSearchParams`. Dropping `?page=1` is the builder's job, so page one
   * needs no special case here.
   *
   * Crawlers will follow these but will not index the deep pages: the route
   * pins a static `canonical` at `/releases` for every query variant. That is
   * now the site-wide settled call, not a placeholder. PSY-1767 weighed
   * per-page canonicals against canonicalize-to-root and kept this posture; the
   * reasoning is on `listRootCanonical` in `lib/seo/siteMetadata`.
   */
  const releasePageHref = (nextPage: number) =>
    buildReleasesHref({ page: String(nextPage) })

  /**
   * Snaps a stale deep-page URL back onto the last real page.
   *
   * `Pagination` clamps `currentPage` for DISPLAY, so without this the strip
   * shows "Page 4 of 4" while the URL still claims `?page=999`, and the reader
   * shares, bookmarks, or reloads the URL, not the strip. The list is empty
   * there too, because the offset is past the end.
   *
   * `replace`, not `push`: this is a correction of a URL the reader never chose,
   * and pushing it would make Back walk them straight into the page that was
   * just rejected (ChartDrilldownPage precedent).
   *
   * Held until the count has actually loaded. `total` is 0 while the first
   * request is in flight, which would otherwise make every page look
   * out-of-range and bounce page 2 to page 1 on a cold load.
   */
  const pageOutOfRange = !isLoading && total > 0 && currentPage > totalPages
  const lastPageHref = buildReleasesHref({ page: String(totalPages) })
  useEffect(() => {
    if (pageOutOfRange) router.replace(lastPageHref, { scroll: false })
  }, [pageOutOfRange, lastPageHref, router])

  /**
   * Restores the scroll-to-top the button pager did by hand. Reading page 2
   * from wherever page 1's grid happened to end is the behavior this replaced.
   */
  const scrollToTop = () => window.scrollTo({ top: 0, behavior: 'smooth' })

  const handleTagsChange = useCallback(
    (nextTags: string[]) => {
      updateFilters({
        tags: nextTags.length > 0 ? buildTagsParam(nextTags) : null,
        page: null,
      })
      // eslint-disable-next-line react-hooks/exhaustive-deps
    },
    [typeParam, yearParam, searchParam, sortParam, labelIdParam, tagMatch]
  )

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

  const hasFilters =
    !!typeParam ||
    !!yearParam ||
    !!searchParam ||
    !!labelIdParam ||
    sortParam !== 'newest'

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
            <Button
              type="submit"
              variant="outline"
              size="sm"
              className="text-xs h-7"
            >
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
        <span
          className="text-sm text-muted-foreground"
          data-testid="release-count"
        >
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

      {/* PSY-1002: full-width top-bar tag filter above a full-width list (no
          left rail). Desktop only — mobile uses the Sheet trigger above. */}
      <div className="mb-4 hidden lg:block">
        <TagFacetPanel
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          heading="Filter releases by tag"
          entityType="release"
          layout="bar"
        />
      </div>

      <div
        className={`min-w-0 ${isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}`}
      >
        {pageOutOfRange ? (
          // The offset is past the end, so the API legitimately returns
          // nothing, but "No releases found matching your filters" is a wrong
          // answer to a question the reader never asked. Hold the spinner for
          // the tick the snap-back above takes.
          <div className="flex justify-center items-center py-12">
            <LoadingSpinner />
          </div>
        ) : releases.length === 0 ? (
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
                  showSaveAction
                  saveData={
                    releaseSaveCounts.isError
                      ? undefined
                      : (releaseSaveCounts.data?.[String(release.id)] ?? {
                          save_count: 0,
                          is_saved: false,
                        })
                  }
                  saveDisabled={releaseSaveCounts.isLoading}
                />
              ))}
            </div>
          </div>
        )}

        {/* Hides itself at one page, so it needs no guard here. */}
        <Pagination
          currentPage={currentPage}
          totalPages={totalPages}
          pageHref={releasePageHref}
          ariaLabel="Releases pagination"
          previousLabel="Previous"
          nextLabel="Next"
          onNavigate={scrollToTop}
          className="mt-8"
        />
      </div>
    </section>
  )
}
