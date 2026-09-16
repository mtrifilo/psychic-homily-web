'use client'

import { useCallback, useMemo, useRef, useTransition } from 'react'
import Link from 'next/link'
import { useSearchParams, useRouter } from 'next/navigation'
import { parseAsInteger, parseAsStringLiteral, useQueryState } from 'nuqs'
import { useVenues, useVenueCities } from '../hooks/useVenues'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/features/auth'
import type { VenueWithShowCount } from '../types'
import { VenueSearch } from './VenueSearch'
import { VenueTable } from './VenueTable'
import { VenueSortControl } from './VenueSortControl'
import { VenueCityChooser } from './VenueCityChooser'
import {
  CityFilters,
  type CityFiltersControl,
  type CityWithCount,
  type CityState,
} from '@/components/filters'
import { citiesParser, ALL_CITIES, cityLabel } from '@/components/filters/cityParams'
import { RemovableFilterChip } from '@/components/filters/RemovableFilterChip'
import { useGeoDefaultCity } from '@/components/filters/useGeoDefaultCity'
import { Breadcrumb } from '@/components/shared'
import {
  Pagination,
  usePaginationFocusTarget,
} from '@/components/shared/Pagination'
import { formatCount } from '@/components/shared/paginationChrome'
import { clampPage, MAX_ARCHIVE_PAGE } from '@/features/shows/showArchive'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
  useTags,
} from '@/features/tags'
import {
  DEFAULT_VENUE_SORT,
  VENUES_PAGE_SIZE,
  VENUES_ROOT,
  VENUE_SORTS,
  venuesPageHref,
  type VenueSort,
} from '../venuesListNavigation'

/** Placeholder rows held while the page's city is still being derived. */
function VenueTableSkeleton() {
  return (
    <div
      className="space-y-2"
      role="status"
      aria-label="Loading rooms"
      data-testid="venues-skeleton"
    >
      {Array.from({ length: 8 }, (_, i) => (
        <div key={i} className="h-8 animate-pulse rounded bg-muted/50" />
      ))}
    </div>
  )
}

/**
 * The `/venues` directory: one city's rooms, densest register the site has.
 *
 * The city is DERIVED during render and never written to the URL, the same rule
 * and the same hook `/shows` uses (favourites, else the IP-geo city). A bare
 * `/venues` is therefore the viewer's own city, `?cities=City,ST` is the
 * shareable address for one city, and `?cities=all` is an explicit whole
 * catalogue. Deriving rather than seeding is what makes the default survive
 * client-side navigation: the URL is the source of truth, not a mount ref.
 *
 * The rows are not requested until the derivation settles. A surface whose
 * CONTENT is the derived city cannot treat "not known yet" as "no city": doing
 * so would either flash the choose-a-city state at every viewer who has one, or
 * paint the whole catalogue's first page (which opens on London) under a
 * heading that is about to change.
 */
export function VenueList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const { authStatus } = useAuthContext()
  const { data: profileData } = useProfile()

  // The per-user default city. Shows-centric by origin, and deliberately shared
  // with this page: a viewer's metro is one fact, not one per browse surface.
  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  // `?cities=` is the source of truth, read/written via nuqs. Three states:
  //   null        → param absent → apply the derived default
  //   ALL_CITIES  → ?cities=all → explicit whole catalogue
  //   CityState[] → explicit selection
  const [citiesState, setCities] = useQueryState(
    'cities',
    citiesParser.withOptions({ history: 'push', startTransition })
  )

  // Legacy single-city params (?city=&state=) — read-only, for back-compat
  // with older scene deep links that predate the ?cities= wire format.
  const legacyCity = searchParams.get('city')
  const legacyState = searchParams.get('state')

  // Multi-tag filter (PSY-309).
  const tagsParam = searchParams.get('tags')
  const tagMatchParam = searchParams.get('tag_match')
  const selectedTags = useMemo(() => parseTagsParam(tagsParam), [tagsParam])
  const tagMatch: 'all' | 'any' = tagMatchParam === 'any' ? 'any' : 'all'

  // The page in view. `clampPage` bounds a hand-edited number so it becomes an
  // empty page rather than an arbitrarily large offset the backend will happily
  // scan for (its `offset` carries a minimum and no maximum).
  const [rawPage, setPage] = useQueryState(
    'page',
    parseAsInteger.withDefault(1).withOptions({ history: 'push', startTransition })
  )
  const page = clampPage(rawPage, MAX_ARCHIVE_PAGE)
  const offset = (page - 1) * VENUES_PAGE_SIZE

  // The row order. The default is never written: it would give one order two
  // addresses, and the second would miss the server-seeded first screen.
  const [sortState, setSort] = useQueryState(
    'sort',
    parseAsStringLiteral(VENUE_SORTS).withOptions({
      history: 'push',
      startTransition,
    })
  )
  const sort: VenueSort = sortState ?? DEFAULT_VENUE_SORT

  // Scoped to the tag filter, so the picker's counts and the sheet's apply
  // button describe the rows this page is about to render rather than the whole
  // catalog. NOT scoped to the city selection: the response is the per-city
  // breakdown, so narrowing it to a city leaves the picker offering only the
  // city already picked.
  const {
    data: citiesData,
    isLoading: citiesLoading,
    isFetching: citiesFetching,
    isPlaceholderData: citiesArePlaceholder,
  } = useVenueCities({ tags: selectedTags, tagMatch })

  // Map VenueCity → CityWithCount. Lifted above every early return so the geo
  // hook reads it unconditionally.
  //
  // No centroids: `/venues/cities` does not serve them, so `matchByGeo` here is
  // an EXACT city match, without the nearest-city fallback `/shows` gets. A
  // visitor in a suburb with no rooms of its own therefore lands on the
  // choose-a-city state rather than on the nearest metro.
  const cities: CityWithCount[] = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.venue_count,
      })) ?? [],
    [citiesData?.cities]
  )

  const hasExplicitSelection =
    citiesState !== null || !!(legacyCity && legacyState)

  const { appliedGeoDefault, notifyUserInteracted, isResolving } =
    useGeoDefaultCity({
      cities,
      authStatus,
      favoriteCities,
      hasExistingSelection: hasExplicitSelection,
      enableClientFetch: true,
    })

  // The effective city filter, DERIVED during render and never seeded into the
  // URL by an effect.
  const selectedCities: CityState[] = useMemo(() => {
    if (citiesState === ALL_CITIES) return []
    if (citiesState) return citiesState
    if (legacyCity && legacyState) return [{ city: legacyCity, state: legacyState }]
    if (favoriteCities.length > 0) return favoriteCities
    return appliedGeoDefault ? [appliedGeoDefault] : []
  }, [citiesState, legacyCity, legacyState, favoriteCities, appliedGeoDefault])

  // Where a derived city came from, for the line that says so. Null whenever the
  // URL named the city, which is the case where nothing was derived at all.
  const derivedFrom: 'favourites' | 'location' | null = hasExplicitSelection
    ? null
    : favoriteCities.length > 0
      ? 'favourites'
      : appliedGeoDefault
        ? 'location'
        : null

  // "Not known yet" as distinct from "no city". Both the city list and the
  // identity/geo read have to have answered before a null selection means the
  // viewer has no city.
  const derivationPending =
    !hasExplicitSelection && (isResolving || (citiesLoading && !citiesData))

  const hasExplicitAll = citiesState === ALL_CITIES
  const showCityChooser =
    !derivationPending && !hasExplicitAll && selectedCities.length === 0

  const {
    data,
    isLoading,
    isFetching,
    isPlaceholderData,
    error,
    refetch,
  } = useVenues({
    cities: selectedCities.length > 0 ? selectedCities : undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
    limit: VENUES_PAGE_SIZE,
    offset,
    sort,
    // An unscoped GET /venues is a whole-catalogue page, so it is requested only
    // once the scope is settled and is something other than "no city".
    enabled: !derivationPending && !showCityChooser,
  })

  // Whether the tag facet has anything to offer. The busiest venue tag's count
  // answers it in one small request: if the most-used tag is unused, every count
  // is zero and the facet is a bar of dead chips (and, on mobile, an empty
  // sheet). A tag already applied through the URL keeps the facet visible so it
  // can be taken off again.
  const { data: topTagData } = useTags({
    entity_type: 'venue',
    sort: 'usage',
    limit: 1,
  })
  const showTagFacet =
    selectedTags.length > 0 || (topTagData?.tags?.[0]?.usage_count ?? 0) > 0

  const { targetProps, focusTarget } =
    usePaginationFocusTarget<HTMLParagraphElement>()

  // The derived-city line's "change" opens the city picker, which owns its own
  // overlay state; this is the one verb it exposes.
  const cityFilterControl = useRef<CityFiltersControl | null>(null)

  // Spreads the params ALREADY on screen and overrides only `page`, so the city
  // filter, the sort order and any foreign param survive a page click.
  const pageHref = useCallback(
    (targetPage: number) => venuesPageHref(searchParams, targetPage),
    [searchParams]
  )

  // A city change answers a different question, so it starts at page 1 again.
  // Both writes go through nuqs in the same tick, which batches them into ONE
  // history entry; a `router.push` beside them would abort nuqs's pending queue
  // and the reset could be dropped (PSY-1388).
  const handleFilterChange = useCallback(
    (nextCities: CityState[]) => {
      notifyUserInteracted()
      void setPage(null)
      void setCities(nextCities.length > 0 ? nextCities : ALL_CITIES)
    },
    [notifyUserInteracted, setPage, setCities]
  )

  const handleSortChange = useCallback(
    (nextSort: VenueSort) => {
      void setPage(null)
      void setSort(nextSort === DEFAULT_VENUE_SORT ? null : nextSort)
    },
    [setPage, setSort]
  )

  // Tag changes rewrite only the tag params, preserving the raw `?cities=`
  // state (absent / all / selection) so a tag change never materializes the
  // derived default into the URL. The page reset rides in the SAME write for
  // the reason above.
  const writeTags = useCallback(
    (nextTags: string[], nextMatch: 'all' | 'any') => {
      const params = new URLSearchParams(searchParams.toString())
      params.delete('tags')
      params.delete('tag_match')
      params.delete('page')
      if (nextTags.length > 0) {
        params.set('tags', buildTagsParam(nextTags))
        if (nextMatch === 'any') params.set('tag_match', 'any')
      }
      const queryString = params.toString()
      startTransition(() => {
        router.push(
          queryString ? `${VENUES_ROOT}?${queryString}` : VENUES_ROOT,
          { scroll: false }
        )
      })
    },
    [searchParams, router]
  )

  const handleTagsChange = useCallback(
    (nextTags: string[]) => writeTags(nextTags, tagMatch),
    [tagMatch, writeTags]
  )

  const handleTagsClear = useCallback(
    () => writeTags([], tagMatch),
    [tagMatch, writeTags]
  )

  // "Clear filters" drops the tags and widens to every city in a SINGLE
  // navigation, for the same nuqs/router race reason as above.
  const handleClearFilters = useCallback(() => {
    notifyUserInteracted()
    startTransition(() => {
      router.push(`${VENUES_ROOT}?cities=all`, { scroll: false })
    })
  }, [notifyUserInteracted, router])

  const venues: VenueWithShowCount[] = data?.venues ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / VENUES_PAGE_SIZE))

  // Facts about the current slice are only stated while the rows on screen
  // answer the current request.
  const rowsAnswerCurrentRequest = !isPlaceholderData

  const scopeCity = selectedCities.length === 1 ? selectedCities[0] : null
  const heading = scopeCity
    ? `Venues in ${cityLabel(scopeCity)}`
    : 'Venues'

  // The count line. The room total comes from the response; the upcoming total
  // is a SUM OF THE ROWS ON SCREEN, so it is labelled as this page's whenever
  // the page is not the whole set. The API serves no city-wide sum.
  const pageUpcoming = venues.reduce((sum, v) => sum + v.upcoming_show_count, 0)
  const wholeSetOnScreen = rowsAnswerCurrentRequest && totalPages === 1
  const roomsLabel = `${formatCount(total)} ${total === 1 ? 'room' : 'rooms'}`
  const upcomingLabel = `${formatCount(pageUpcoming)} upcoming ${
    pageUpcoming === 1 ? 'show' : 'shows'
  }`

  const chooserScope = useMemo(() => {
    const rooms = cities.reduce((sum, c) => sum + c.count, 0)
    return `${formatCount(rooms)} ${rooms === 1 ? 'room' : 'rooms'} in ${formatCount(cities.length)} ${cities.length === 1 ? 'city' : 'cities'}`
  }, [cities])

  // Dim only while the rows on screen belong to a DIFFERENT query than the one
  // being awaited. `isPlaceholderData` says exactly that; raw `isFetching` does
  // not, and using it would fade the server-seeded first screen the instant it
  // hydrated (that entry is stale by construction).
  const isUpdating =
    (isFetching && isPlaceholderData) ||
    (citiesFetching && citiesArePlaceholder) ||
    isPending

  const chrome = (
    <>
      {scopeCity && (
        <div className="hidden sm:block">
          <Breadcrumb
            fallback={{ href: VENUES_ROOT, label: 'Venues' }}
            currentPage={cityLabel(scopeCity)}
          />
        </div>
      )}

      <div className="mb-2 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h1 className="text-2xl font-bold sm:text-3xl">{heading}</h1>
        <p className="font-mono text-[13px] text-muted-foreground" {...targetProps}>
          {showCityChooser ? chooserScope : roomsLabel}
          {!showCityChooser && wholeSetOnScreen ? ` · ${upcomingLabel}` : ''}
        </p>
      </div>

      {derivedFrom && scopeCity && (
        <p
          className="mb-3 text-sm text-muted-foreground"
          data-testid="venues-derived-city"
        >
          Showing {cityLabel(scopeCity)} from your{' '}
          {derivedFrom === 'favourites' ? 'favourites' : 'location'}
          {' · '}
          <button
            type="button"
            onClick={() => cityFilterControl.current?.open()}
            className="text-primary hover:underline underline-offset-4"
            data-testid="venues-derived-city-change"
          >
            change
          </button>
        </p>
      )}

      <div className="mb-3 space-y-3">
        <VenueSearch />
        {cities.length > 0 && (
          <CityFilters
            cities={cities}
            selectedCities={selectedCities}
            onFilterChange={handleFilterChange}
            resultNoun={{ singular: 'venue', plural: 'venues' }}
            allLabel="All cities"
            // The chooser state below offers the busiest cities itself, and the
            // row is what overflowed the directory at 390.
            showPopularCities={false}
            controlRef={cityFilterControl}
          />
        )}
        {selectedTags.length > 0 && (
          <div className="flex flex-wrap items-center gap-2">
            {selectedTags.map(slug => (
              <RemovableFilterChip
                key={slug}
                label={slug}
                onRemove={() =>
                  handleTagsChange(selectedTags.filter(s => s !== slug))
                }
                data-testid={`venue-tag-chip-${slug}`}
              />
            ))}
          </div>
        )}
      </div>

      {showTagFacet && (
        <>
          <div className="mb-3 flex items-center justify-between gap-2">
            <TagFacetSheet
              selectedSlugs={selectedTags}
              onToggle={handleTagsChange}
              onClear={handleTagsClear}
              title="Filter venues by tag"
              entityType="venue"
            />
          </div>
          <div className="mb-3 hidden lg:block">
            <TagFacetPanel
              selectedSlugs={selectedTags}
              onToggle={handleTagsChange}
              onClear={handleTagsClear}
              heading="Filter venues by tag"
              entityType="venue"
              layout="bar"
            />
          </div>
        </>
      )}
    </>
  )

  // The mini Atlas pane (PSY-2079) takes the space to the right of the table at
  // 1280 and up. Its slot is left EMPTY rather than reserved: an empty box would
  // promise content this page does not yet have.
  const frame = (children: React.ReactNode) => (
    <section className="w-full max-w-6xl">
      {chrome}
      <div className="xl:max-w-[720px]">{children}</div>
    </section>
  )

  if (showCityChooser) {
    return frame(<VenueCityChooser cities={cities} />)
  }

  if (derivationPending || (isLoading && !data) || (citiesLoading && !citiesData)) {
    return frame(<VenueTableSkeleton />)
  }

  // Report an error only when nothing on screen answers the CURRENT query. The
  // server-seeded first screen is stale by construction, so a plain `if (error)`
  // would discard a rendered page over a failed background refetch;
  // `isPlaceholderData` covers the opposite hazard, where `keepPreviousData`
  // would present the previous city's rooms as the new filter's answer.
  if (error && (!data || isPlaceholderData)) {
    return frame(
      <div className="py-12 text-center text-destructive">
        <p>Failed to load venues. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  // The URL named a page this list does not have. The same zero rows as an
  // empty city, and a very different sentence.
  const pageIsBeyondEnd =
    rowsAnswerCurrentRequest && venues.length === 0 && total > 0

  const hasNarrowingFilter = selectedTags.length > 0

  const renderPager = (position: 'top' | 'bottom') => (
    <Pagination
      currentPage={page}
      totalPages={totalPages}
      pageHref={pageHref}
      ariaLabel={`Venues pagination, ${position} of list`}
      captionRange={
        rowsAnswerCurrentRequest && venues.length > 0
          ? { start: offset + 1, end: offset + venues.length, total }
          : undefined
      }
      // ONE of the two instances owns the announcement, or a screen reader hears
      // the new position twice on every click. The top pager keeps it: it is
      // beside the line the pager moves focus to, and first in DOM order.
      announce={position === 'top'}
      onNavigate={focusTarget}
      className={position === 'top' ? 'mb-3' : 'mt-4'}
    />
  )

  return frame(
    <>
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-2">
        <VenueSortControl sort={sort} onSortChange={handleSortChange} />
        <p
          className="hidden text-xs text-muted-foreground sm:block"
          data-testid="venues-count-rule"
        >
          {roomsLabel} &middot; {upcomingLabel}
          {wholeSetOnScreen ? '' : ' on this page'} at verified rooms, on each
          venue&apos;s local calendar
        </p>
      </div>

      <div
        className={cn(
          'min-w-0 transition-opacity duration-75',
          isUpdating && 'opacity-60'
        )}
      >
        {renderPager('top')}

        {pageIsBeyondEnd ? (
          <div
            className="py-12 text-center text-muted-foreground"
            data-testid="venues-page-beyond-end"
          >
            <p>That page is past the end of this list.</p>
            <p className="mt-2 text-sm">
              <Link href={pageHref(1)} className="text-primary hover:underline">
                Back to the first page
              </Link>
            </p>
          </div>
        ) : venues.length === 0 ? (
          <div
            className="py-8 text-muted-foreground"
            data-testid="venues-zero-result"
          >
            <p className="text-foreground">
              {hasNarrowingFilter
                ? 'No verified rooms match the current filters.'
                : scopeCity
                  ? `No verified rooms in ${cityLabel(scopeCity)} yet.`
                  : 'No verified rooms yet.'}
            </p>
            <p className="mt-3 text-sm">
              {hasNarrowingFilter ? (
                <button
                  type="button"
                  onClick={handleClearFilters}
                  className="text-primary hover:underline"
                >
                  Clear filters
                </button>
              ) : (
                <Link href="/contribute" className="text-primary hover:underline">
                  Add a venue
                </Link>
              )}
            </p>
          </div>
        ) : (
          <>
            <VenueTable
              venues={venues}
              sort={sort}
              onSortChange={handleSortChange}
            />
            <p
              className="mt-3 text-xs text-muted-foreground"
              data-testid="venues-verified-legend"
            >
              &#10003; verified room
            </p>
          </>
        )}

        {renderPager('bottom')}
      </div>
    </>
  )
}
