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
import { formatCount } from '@/components/shared/paginationChrome'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { Button } from '@/components/ui/button'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

/**
 * The largest `?page=` this list will act on.
 *
 * Not a product limit — a serialization one. `page` is multiplied by the page
 * size to build `offset`, and the API takes `offset` as an integer, so the
 * ceiling exists to keep that product inside a range `Number.toString()` still
 * renders in plain decimal. 100,000 pages is 5,000,000 artists, several orders
 * of magnitude past any real catalogue, so nothing reachable is cut off.
 */
const MAX_ARTIST_PAGE = 100_000

/**
 * The search params this list owns. Always carried into a page link verbatim,
 * however long they are: losing one would drop a filter on the next click.
 */
const OWNED_PARAMS = new Set(['page', 'cities', 'tags', 'tag_match'])

/**
 * Whether a param this page knows nothing about is short enough to carry into
 * every page link.
 *
 * Foreign params are carried on purpose — `utm_*`, `gclid` and share tokens
 * have to survive pagination — but the pager writes up to nine hrefs into the
 * server HTML, so whatever arrives is reflected roughly a dozen times over.
 * One 15 KB junk param turned a 316 KB document into 496 KB. These bounds are
 * far above any real campaign or share token and remove the multiplier.
 */
const isCarryableParam = (key: string, value: string) =>
  key.length <= 64 && value.length <= 512

export function ArtistList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  // NOTE (PSY-1774): now that this list is server-rendered, `useDensity` reads
  // localStorage through a server snapshot, so the server HTML and the
  // hydration render are ALWAYS 'comfortable'. A viewer who chose compact or
  // expanded gets one whole-list re-layout on the commit after hydration, where
  // before the seed the cards first mounted post-hydration already holding the
  // right value. `/artists` inherits this from `/shows` exactly (PSY-1624,
  // ShowList.tsx) — same cause, same accepted trade, same cheap follow-up
  // (reserve row height across densities). Recorded here so it is accepted
  // rather than unnoticed on a second route.
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
  // Clamped at BOTH ends, because `?page=` is user-typed and crawler-followed.
  // The ceiling is the load-bearing one: an unbounded page number multiplies
  // into an offset outside JavaScript's integer range, which `toString()`
  // serialises in exponential form ("5e+21"), which the API rejects as a
  // non-integer — so `?page=99999999999999999999` rendered "Failed to load
  // artists" instead of the past-the-end notice it deserves. Any value at or
  // above the cap is past the end of every real result set, so clamping loses
  // nothing a reader could have wanted.
  const currentPage = Math.min(Math.max(1, pageParam), MAX_ARTIST_PAGE)
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
      const params = new URLSearchParams()
      for (const [key, value] of new URLSearchParams(searchParams.toString())) {
        // Params this page owns are carried verbatim — dropping one would
        // silently lose a filter on the next page click, which is far worse
        // than the thing the length cap below is for.
        if (OWNED_PARAMS.has(key) || isCarryableParam(key, value)) {
          params.append(key, value)
        }
      }
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
   *
   * Crawlers will follow these but will not index the deep pages: the route
   * pins a static `canonical` at `/artists` for every query variant, matching
   * `/releases`. Two consequences worth naming rather than discovering later —
   * this mints ~1 crawlable URL per 50 artists, and each of them re-serves the
   * page-1 JSON-LD `ItemList`, which describes content that page does not
   * contain. Whether pagination should be indexable, and which URLs should
   * carry the `ItemList`, are both indexing-policy calls this change does not
   * make; PSY-1794 owns the `ItemList` half.
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
  // The offset the ROWS ON SCREEN came back under, not the one the URL asks
  // for. `keepPreviousData` holds the previous page's rows through the next
  // page's fetch, so a URL-derived offset would caption "Showing 51-100" over
  // artists 1-50 for the length of every page change.
  const renderedOffset = data?.offset ?? 0
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
        {/*
          Formatted through the pager's own helper: this line and the pager
          caption below it report the same number, and "6200 artists" above
          "Showing 1-50 of 6,200" is two spellings of one fact.
        */}
        <p className="mb-3 text-sm text-muted-foreground" data-testid="artist-count">
          {formatCount(total)} {total === 1 ? 'artist' : 'artists'}
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

        {/*
          Hides itself at one page, so it needs no guard for that — but it IS
          guarded on the past-the-end state. `Pagination` clamps `currentPage`
          into range for display, so on `?page=99` of a 3-page list it would
          caption "Page 3 of 3" and mark page 3 `aria-current` directly beneath
          a sentence saying the reader is past the end. The two archive pagers
          avoid the same contradiction by returning before their pager renders.
        */}
        {!isPastLastPage && (
        <Pagination
          currentPage={currentPage}
          totalPages={totalPages}
          pageHref={artistPageHref}
          ariaLabel="Artists pagination"
          previousLabel="Previous"
          nextLabel="Next"
          captionRange={
            artists.length > 0
              ? {
                  start: renderedOffset + 1,
                  end: renderedOffset + artists.length,
                  total,
                }
              : undefined
          }
          onNavigate={scrollToTop}
          className="mt-8"
        />
        )}
      </div>
    </section>
  )
}
