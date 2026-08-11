'use client'

import { useCallback, useMemo, useTransition } from 'react'
import Link from 'next/link'
import { useSearchParams, useRouter } from 'next/navigation'
import { parseAsInteger, useQueryState } from 'nuqs'
import { useArtists, useArtistCities } from '../hooks/useArtists'
import { ARTIST_LIST_PAGE_LIMIT } from '../api'
import { ArtistCard } from './ArtistCard'
import { ArtistSearch } from './ArtistSearch'
import { CityFilters, type CityWithCount, type CityState } from '@/components/filters'
import { citiesParser, ALL_CITIES } from '@/components/filters/cityParams'
import { LoadingSpinner, DensityToggle, Pagination } from '@/components/shared'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { Button } from '@/components/ui/button'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

export function ArtistList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const { density, setDensity } = useDensity('artists')

  // PSY-496: city filter is page-scoped — we don't auto-apply the user's
  // profile-level favorite_cities here. Favorites are shows-centric (the
  // canonical homepage) and inheriting them on /artists produced the
  // "0 artists" confusion where most artists have city: null. Users can
  // still filter by city on /artists manually — the URL drives state.

  // `?cities=` via nuqs (shared parser). /artists has NO derived default —
  // absent (null) simply means "all artists" (PSY-496 above) — so unlike
  // /shows, clearing writes a bare URL, not the ALL_CITIES sentinel. The
  // sentinel is still tolerated on deep links (?cities=all → []).
  const [citiesState, setCities] = useQueryState(
    'cities',
    citiesParser.withOptions({ history: 'push', startTransition })
  )
  const selectedCities: CityState[] = useMemo(() => {
    if (citiesState === ALL_CITIES || citiesState === null) return []
    return citiesState
  }, [citiesState])

  // `?page=` via nuqs so a filter change can clear it in the SAME URL write as
  // the filter itself — nuqs batches setters called in one tick into a single
  // navigation. The pager writes this param as a plain `<Link href>` instead
  // (crawlable, middle-clickable); nuqs reads it back off the URL either way.
  const [pageParam, setPage] = useQueryState(
    'page',
    parseAsInteger.withDefault(1).withOptions({ history: 'push', startTransition })
  )
  const currentPage = Math.max(1, pageParam)
  const offset = (currentPage - 1) * ARTIST_LIST_PAGE_LIMIT

  // Parse multi-tag from URL (PSY-309)
  const tagsParam = searchParams.get('tags')
  const tagMatchParam = searchParams.get('tag_match')
  const selectedTags = useMemo(() => parseTagsParam(tagsParam), [tagsParam])
  const tagMatch: 'all' | 'any' = tagMatchParam === 'any' ? 'any' : 'all'

  const { data: citiesData, isLoading: citiesLoading, isFetching: citiesFetching } = useArtistCities()
  const { data, isLoading, isFetching, error, refetch } = useArtists({
    cities: selectedCities.length > 0 ? selectedCities : undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
    limit: ARTIST_LIST_PAGE_LIMIT,
    offset,
  })

  /**
   * Serializes a `/artists` URL from the params CURRENTLY in the address bar,
   * overriding only the named keys (a `null` deletes one).
   *
   * Built by editing the live params rather than from a fixed key list so that
   * anything this component knows nothing about — `utm_*`, `gclid`, share
   * tokens — survives a page change. A from-scratch builder drops them at the
   * first pagination click, which is exactly the bug PSY-1755 and PSY-1754
   * each shipped once.
   *
   * Every URL this component writes goes through here — the pager's hrefs AND
   * the tag filter's imperative push — so a page link cannot drift from what a
   * filter change would have written.
   */
  const buildArtistsHref = useCallback(
    (overrides: Record<string, string | null>) => {
      const params = new URLSearchParams(searchParams.toString())
      for (const [key, value] of Object.entries(overrides)) {
        if (value === null) params.delete(key)
        else params.set(key, value)
      }
      const queryString = params.toString()
      return queryString ? `/artists?${queryString}` : '/artists'
    },
    [searchParams]
  )

  /**
   * Page links are real URLs so the strip is middle-clickable and shareable;
   * the `<Link>` navigation writes the param and this component reads it back.
   * Page one writes NO `?page=`, so the head of the list has one URL rather
   * than two.
   */
  const artistPageHref = useCallback(
    (nextPage: number) =>
      buildArtistsHref({ page: nextPage > 1 ? String(nextPage) : null }),
    [buildArtistsHref]
  )

  const scrollToTop = useCallback(
    () => window.scrollTo({ top: 0, behavior: 'smooth' }),
    []
  )

  // City changes write `?cities=` via nuqs (empty → null → bare URL; no
  // default derivation on /artists, see PSY-496 note above), and reset the
  // pager in the same write: a filter that narrows the set while `?page=5`
  // survives lands the reader on an empty page they did not ask for.
  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      void setCities(cities.length > 0 ? cities : null)
      void setPage(null)
    },
    [setCities, setPage]
  )

  // Tag changes rewrite only the tag params, preserving `?cities=` verbatim,
  // and drop `?page=` for the same reason the city filter does.
  const writeTags = useCallback(
    (nextTags: string[], nextMatch: 'all' | 'any') => {
      const href = buildArtistsHref({
        tags: nextTags.length > 0 ? buildTagsParam(nextTags) : null,
        tag_match: nextTags.length > 0 && nextMatch === 'any' ? 'any' : null,
        page: null,
      })
      startTransition(() => {
        router.push(href, { scroll: false })
      })
    },
    [buildArtistsHref, router]
  )

  const handleTagsChange = useCallback(
    (nextTags: string[]) => writeTags(nextTags, tagMatch),
    [tagMatch, writeTags]
  )

  const handleTagsClear = useCallback(() => {
    writeTags([], tagMatch)
  }, [tagMatch, writeTags])

  // "Clear filters" resets tags AND cities in a SINGLE navigation — mixing a
  // router push (tags) with nuqs's throttled setCities in one tick races
  // (nuqs aborts its queue on a foreign history update; see PSY-1388).
  const handleClearFilters = useCallback(() => {
    startTransition(() => {
      router.push('/artists', { scroll: false })
    })
  }, [router])

  // Only show full spinner on FIRST load (no data yet)
  if ((isLoading && !data) || (citiesLoading && !citiesData)) {
    return (
      <div className="flex justify-center items-center py-12">
        <LoadingSpinner />
      </div>
    )
  }

  // Track if we're updating (fetching but already have data)
  const isUpdating = isFetching || citiesFetching || isPending

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load artists. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  // Map ArtistCity to CityWithCount
  const cities: CityWithCount[] = citiesData?.cities?.map(c => ({
    city: c.city,
    state: c.state,
    count: c.artist_count,
  })) ?? []

  const artists = data?.artists ?? []
  // The whole matching set, not this page. `artists.length` here would caption
  // "50 artists" over a catalogue of thousands.
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / ARTIST_LIST_PAGE_LIMIT)
  // Only meaningful once a response has landed: before that `total` is 0 and
  // every page looks past the end.
  const isPastLastPage = total > 0 && currentPage > totalPages
  const hasTagFilter = selectedTags.length > 0
  const hasAnyFilter = hasTagFilter || selectedCities.length > 0

  // Hoisted out of the JSX so the empty state stays one branch deep. A page
  // past the end is its own state, not an empty catalogue: a stale bookmark, or
  // a filter that shrank the set under a deep `?page=`. Reported as such rather
  // than as "no artists", which would tell the reader the list is empty when it
  // is not. Wording matches the archive pagers (`ArtistPastShows`,
  // `VenuePastShows`), which say the same thing about their own lists.
  const emptyState = isPastLastPage ? (
    <p>
      That page is past the end of the list.{' '}
      <Link href={artistPageHref(1)} className="text-primary hover:underline">
        Back to the first page
      </Link>
      .
    </p>
  ) : (
    <>
      <p>
        {hasAnyFilter
          ? 'No artists match the current filters.'
          : 'No artists available at this time.'}
      </p>
      {hasAnyFilter && (
        <button
          onClick={handleClearFilters}
          className="mt-4 text-primary hover:underline"
        >
          Clear filters
        </button>
      )}
    </>
  )

  return (
    <section className="w-full max-w-6xl">
      <div className="mb-6 space-y-4">
        <ArtistSearch />
        {cities.length > 0 && (
          <CityFilters
            cities={cities}
            selectedCities={selectedCities}
            onFilterChange={handleFilterChange}
          />
        )}
      </div>

      {/* Mobile: Sheet trigger + density toggle. Desktop hides the Sheet (the
          bar below takes over) but keeps the density toggle on this row. */}
      <div className="flex items-center justify-between mb-4 gap-2">
        <TagFacetSheet
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          title="Filter artists by tag"
          entityType="artist"
        />
        <DensityToggle density={density} onDensityChange={setDensity} />
      </div>

      {/* PSY-1001: full-width top-bar tag filter above a full-width list (no
          left rail). Desktop only — mobile uses the Sheet trigger above. */}
      <div className="mb-4 hidden lg:block">
        <TagFacetPanel
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          heading="Filter artists by tag"
          entityType="artist"
          layout="bar"
        />
      </div>

      <div className={`min-w-0 ${isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}`}>
        <p className="mb-3 text-sm text-muted-foreground" data-testid="artist-count">
          {total} {total === 1 ? 'artist' : 'artists'}
          {hasTagFilter && ` matching ${selectedTags.join(', ')}`}
        </p>
        {artists.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            {emptyState}
          </div>
        ) : (
          <div className="@container">
            <div className={
              density === 'compact'
                ? 'flex flex-col gap-px'
                : density === 'expanded'
                  ? 'grid grid-cols-1 gap-5'
                  : 'grid grid-cols-1 @sm:grid-cols-2 @2xl:grid-cols-3 gap-3'
            }>
              {artists.map(artist => (
                <ArtistCard key={artist.id} artist={artist} density={density} />
              ))}
            </div>
          </div>
        )}

        {/* Hides itself at one page, so it needs no guard here. */}
        <Pagination
          currentPage={currentPage}
          totalPages={totalPages}
          pageHref={artistPageHref}
          ariaLabel="Artists pagination"
          previousLabel="Previous"
          nextLabel="Next"
          captionRange={
            artists.length > 0
              ? { start: offset + 1, end: offset + artists.length, total }
              : undefined
          }
          onNavigate={scrollToTop}
          className="mt-8"
        />
      </div>
    </section>
  )
}
