'use client'

import { useCallback, useMemo, useRef, useState, useTransition } from 'react'
import Link from 'next/link'
import { useSearchParams, useRouter } from 'next/navigation'
import { parseAsInteger, parseAsStringLiteral, useQueryState } from 'nuqs'
import { useVenues, useVenueCities } from '../hooks/useVenues'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/features/auth'
import type { VenueWithShowCount } from '../types'
import { VenueSearch } from './VenueSearch'
import { VenueTable, type VenueTableControl } from './VenueTable'
import { VenueSortControl } from './VenueSortControl'
import { VenueCityChooser } from './VenueCityChooser'
import {
  VenueMiniAtlasPane,
  useMiniAtlasViewport,
} from './VenueMiniAtlasPane'
import {
  CityFilters,
  type CityFiltersControl,
  type CityWithCount,
  type CityState,
} from '@/components/filters'
import {
  buildCitiesParam,
  citiesParser,
  ALL_CITIES,
  cityKey,
  cityLabel,
} from '@/components/filters/cityParams'
import { RemovableFilterChip } from '@/components/filters/RemovableFilterChip'
import { useGeoDefaultCity } from '@/components/filters/useGeoDefaultCity'
import { Breadcrumb } from '@/components/shared'
import {
  Pagination,
  usePaginationFocusTarget,
} from '@/components/shared/Pagination'
import { clampPage, MAX_ARCHIVE_PAGE } from '@/features/shows/showArchive'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
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
  countLabel,
  facetCityFor,
  nearbyCitiesWithRooms,
  venuesCityHref,
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
        <Skeleton key={i} className="h-8" />
      ))}
    </div>
  )
}

/**
 * The `/venues` directory: one city's rooms.
 *
 * The city is DERIVED during render and never written to the URL, through the
 * shared `useGeoDefaultCity`: the viewer's favourite cities, else, for a
 * SETTLED ANONYMOUS viewer, the IP-geo city. A bare `/venues` is therefore the
 * viewer's own city, `?cities=City,ST` is the shareable address for one city,
 * and `?cities=all` is an explicit whole catalogue. Deriving rather than
 * seeding is what makes the default survive client-side navigation: the URL is
 * the source of truth, not a mount ref.
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

  // Legacy single-city params, read-only: older deep links predate the
  // `?cities=` wire format.
  const legacyCity = searchParams.get('city')
  const legacyState = searchParams.get('state')

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
  // addresses.
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
    error: citiesError,
    refetch: refetchCities,
  } = useVenueCities({ tags: selectedTags, tagMatch })

  // Map VenueCity → CityWithCount. Lifted above every early return so the geo
  // hook reads it unconditionally.
  //
  // No centroids: `/venues/cities` does not serve them, so `matchByGeo` here is
  // an EXACT city match rather than a nearest-city one. A visitor in a suburb
  // with no rooms of its own therefore lands on the choose-a-city state.
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

  // The effective city filter and WHERE IT CAME FROM, resolved together in one
  // walk so the line that names the source cannot describe a different branch
  // than the one that chose the city. Derived during render and never seeded
  // into the URL by an effect.
  const { selectedCities, derivedFrom } = useMemo((): {
    selectedCities: CityState[]
    derivedFrom: 'favourites' | 'location' | null
  } => {
    if (citiesState === ALL_CITIES) return { selectedCities: [], derivedFrom: null }
    if (citiesState) return { selectedCities: citiesState, derivedFrom: null }
    if (legacyCity && legacyState) {
      return {
        selectedCities: [{ city: legacyCity, state: legacyState }],
        derivedFrom: null,
      }
    }
    if (favoriteCities.length > 0) {
      return { selectedCities: favoriteCities, derivedFrom: 'favourites' }
    }
    if (appliedGeoDefault) {
      return { selectedCities: [appliedGeoDefault], derivedFrom: 'location' }
    }
    return { selectedCities: [], derivedFrom: null }
  }, [citiesState, legacyCity, legacyState, favoriteCities, appliedGeoDefault])

  // "Not known yet" as distinct from "no city". Both the city list and the
  // identity/geo read have to have answered before a null selection means the
  // viewer has no city.
  const derivationPending =
    !hasExplicitSelection && (isResolving || (citiesLoading && !citiesData))

  // The city this page is ABOUT, as the facet spells it.
  //
  // The RULE lives in `facetCityFor`, shared with `generateMetadata`; the
  // INPUTS differ, and deliberately. The server sees only the URL, while
  // `selectedCities` also carries the city derived from favourites or location.
  // So a bare `/venues` renders a city heading under the generic title, which
  // is the point: the URL is the same for every viewer and the heading is not.
  const scopeCity = useMemo(
    () => facetCityFor(selectedCities, cities),
    [selectedCities, cities]
  )

  // A city the facet does not offer holds no verified rooms, so there is no
  // table to draw and no name this page is willing to print. It gets the same
  // state a placeless viewer gets, and `generateMetadata` marks it `noindex`
  // off the same fact.
  //
  // Only while no tag is applied: the facet read here is SCOPED to the tags, so
  // under a tag filter an absent city means "no rooms with this tag", which the
  // zero-result state says better and offers a way out of. It is also the only
  // scoping under which this agrees with the server, whose facet is unscoped.
  //
  // `cities.length > 0` is the facet having ANSWERED: an empty one cannot tell
  // a city it does not carry from a city it has not loaded.
  const cityIsUnknown =
    selectedTags.length === 0 &&
    cities.length > 0 &&
    selectedCities.length === 1 &&
    scopeCity === null

  const showCityChooser =
    !derivationPending &&
    citiesState !== ALL_CITIES &&
    (selectedCities.length === 0 || cityIsUnknown)

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

  // Whether the tag facet has anything to offer, answered by the busiest venue
  // tag's count: if the most-used tag is unused, every count is zero. A tag
  // already applied through the URL keeps the facet visible so it can be taken
  // off again.
  //
  // `TagFacetPanel` already hides ITSELF on that condition. This probe exists
  // for `TagFacetSheet`, whose trigger button renders unconditionally and so
  // opens an empty drawer on a catalogue with no venue tags.
  const { data: topTagData } = useTags({
    entity_type: 'venue',
    sort: 'usage',
    limit: 1,
  })
  // Not in the choose-a-city state: there are no rows there for a tag to
  // narrow, so the facet would be a control with nothing under it.
  const showTagFacet =
    !showCityChooser &&
    (selectedTags.length > 0 || (topTagData?.tags?.[0]?.usage_count ?? 0) > 0)

  const { targetProps, focusTarget } =
    usePaginationFocusTarget<HTMLHeadingElement>()

  // The derived-city line's "change" opens the city picker, which owns its own
  // overlay state; this is the one verb it exposes.
  const cityFilterControl = useRef<CityFiltersControl | null>(null)

  // ONE hover id for the whole page (PSY-2079). The table reports a row and the
  // mini Atlas reports a pin into the same state, and both read it back, so a
  // room cannot be lit in one view and not the other.
  const [hoveredVenueId, setHoveredVenueId] = useState<number | null>(null)
  // Whether the viewport is wide enough for the map pane. False until
  // hydration, so the pane is absent from the server HTML and from every
  // viewport under 1280, not merely hidden there.
  const miniAtlasViewport = useMiniAtlasViewport()

  // The table's one verb, the same shape the city picker's control uses: a pin
  // click asks the table to reveal a room and the table owns how its rows are
  // found. Null while no table is mounted, so every call site stays null-safe.
  const venueTableControl = useRef<VenueTableControl | null>(null)
  const revealVenueRow = useCallback((venueId: number) => {
    venueTableControl.current?.revealRow(venueId)
  }, [])

  // Spreads the params ALREADY on screen and overrides only `page`, so the city
  // filter, the sort order and any foreign param survive a page click.
  const pageHref = useCallback(
    (targetPage: number) => venuesPageHref(searchParams, targetPage),
    [searchParams]
  )

  // A city change answers a different question, so it starts at page 1 again.
  // Both writes go through nuqs in the same tick, which batches them into ONE
  // history entry; a `router.push` beside them would abort nuqs's pending queue
  // and the reset could be dropped.
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
      // The order and the city come from the nuqs values rather than from
      // `searchParams`, which lags a write still in flight: picking a city and
      // then a tag would otherwise push the previous city. `citiesState` carries
      // the RAW state (absent / all / a selection), so copying it preserves the
      // rule that a tag change never materializes the derived default.
      params.delete('sort')
      if (sortState) params.set('sort', sortState)
      params.delete('cities')
      if (citiesState === ALL_CITIES) params.set('cities', ALL_CITIES)
      else if (citiesState) params.set('cities', buildCitiesParam(citiesState))
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
    [searchParams, router, sortState, citiesState]
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
  // navigation, for the same nuqs/router race reason as above. The order is
  // carried through: it is how the reader reads the list, not one of the
  // filters being cleared.
  const handleClearFilters = useCallback(() => {
    notifyUserInteracted()
    const params = new URLSearchParams()
    params.set('cities', ALL_CITIES)
    if (sortState) params.set('sort', sortState)
    startTransition(() => {
      router.push(`${VENUES_ROOT}?${params.toString()}`, { scroll: false })
    })
  }, [notifyUserInteracted, router, sortState])

  const venues: VenueWithShowCount[] = data?.venues ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / VENUES_PAGE_SIZE))

  // Facts about the current slice are only stated while the rows on screen
  // answer the current request. Two ways they do not: there are no rows yet,
  // and `keepPreviousData` is holding the previous query's.
  const rowsAnswerCurrentRequest = data !== undefined && !isPlaceholderData

  const heading = scopeCity ? `Venues in ${cityLabel(scopeCity)}` : 'Venues'
  // Every selected city, named. A derived selection of two favourites filters
  // the list just as hard as one, and without this the reader is shown a subset
  // of the catalogue under a bare "Venues" with nothing saying which cities.
  const selectionLabel = selectedCities.map(cityLabel).join(' and ')

  // `upcoming_show_total` spans the whole filtered set, so the heading states
  // it on every page of a paged city. A response without it comes from a
  // backend older than the field, and the only honest number left there is the
  // sum of the rows on screen: this page's, labelled as such. That fallback
  // (and the optional field in VenuesListResponse) exists only for the window
  // in which this frontend is live before that backend; once production's
  // backend serves the field it can be deleted.
  const cityUpcoming = data?.upcoming_show_total
  const upcomingCount =
    cityUpcoming ?? venues.reduce((sum, v) => sum + v.upcoming_show_count, 0)
  // Null while the rows on screen answer a DIFFERENT request: the heading flips
  // to the new city on the same render the filter changes, and a count from the
  // outgoing city beneath it is a wrong number rather than a stale one.
  const roomsLabel = rowsAnswerCurrentRequest ? countLabel(total, 'room') : null
  const upcomingLabel = rowsAnswerCurrentRequest
    ? countLabel(upcomingCount, 'upcoming show')
    : null
  // Whether that label describes the same set the room count does, which is
  // what lets it stand beside it in the heading. The city-wide field does; a
  // page sum does only when this page is the whole set. Read only beside a
  // non-null upcomingLabel, which already requires rowsAnswerCurrentRequest.
  const upcomingLabelIsAboutTheSet = cityUpcoming != null || totalPages === 1

  // Sorting the whole facet is work only the empty state spends, so the
  // condition that renders it is inside the memo rather than around it: the
  // hook stays unconditional, the sort does not run on the path with rows.
  const nearbyCities = useMemo(
    () =>
      scopeCity && rowsAnswerCurrentRequest && venues.length === 0
        ? nearbyCitiesWithRooms(cities, scopeCity)
        : [],
    [cities, scopeCity, rowsAnswerCurrentRequest, venues.length]
  )

  const chooserScope = useMemo(() => {
    const rooms = cities.reduce((sum, c) => sum + c.count, 0)
    return `${countLabel(rooms, 'room')} in ${countLabel(cities.length, 'city', 'cities')}`
  }, [cities])

  // Dim only while the rows on screen belong to a DIFFERENT query than the one
  // being awaited. `isPlaceholderData` says exactly that; raw `isFetching` does
  // not, and using it would fade a background revalidation.
  const isUpdating = (isFetching && isPlaceholderData) || isPending

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
        {/* The pager's focus target: a stable landmark at the top of the list,
            and unlike the count beside it never empty. */}
        <h1 className="text-2xl font-bold sm:text-3xl" {...targetProps}>
          {heading}
        </h1>
        <p className="font-mono text-[13px] text-muted-foreground">
          {showCityChooser ? chooserScope : (roomsLabel ?? '')}
          {!showCityChooser && upcomingLabelIsAboutTheSet && upcomingLabel
            ? ` · ${upcomingLabel}`
            : ''}
        </p>
      </div>

      {/* Gated on the picker being on screen: "change" opens it, and with no
          cities at all there is nothing to change to. */}
      {derivedFrom && selectedCities.length > 0 && cities.length > 0 && (
        <p
          className="mb-3 text-sm text-muted-foreground"
          data-testid="venues-derived-city"
        >
          Showing {selectionLabel} from your{' '}
          {derivedFrom === 'favourites' ? 'favourites' : 'location'}
          {' · '}
          <button
            type="button"
            onClick={() => cityFilterControl.current?.open()}
            aria-label="Change city"
            className="inline-flex min-h-11 items-center text-primary hover:underline underline-offset-4"
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
            // The choose-a-city state offers the busiest cities itself.
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

  // At 1280 and up the content is bounded to 720px and the space beside it is
  // left EMPTY rather than reserved: an empty box would promise content this
  // page does not have.
  const frame = (children: React.ReactNode, aside?: React.ReactNode) => (
    <section className="w-full max-w-6xl">
      {chrome}
      <div className="xl:flex xl:items-start xl:gap-8">
        <div className="min-w-0 xl:max-w-[720px] xl:flex-1">{children}</div>
        {aside}
      </div>
    </section>
  )

  // The city facet is what the page DERIVES and OFFERS a city from: the picker,
  // the busiest-city chips, and the geo match. When it fails there is nothing to
  // choose and nothing to derive, so the page says so and offers the retry
  // rather than rendering an empty invitation to pick.
  //
  // Not when the URL already names the scope. There the facet is decorative, the
  // rows are on their way, and blanking them would take a shared link out over a
  // filter bar.
  if (citiesError && cities.length === 0 && !hasExplicitSelection) {
    return frame(
      <div className="py-12 text-center text-destructive" data-testid="venues-cities-error">
        <p>Failed to load cities. Please try again later.</p>
        <Button
          variant="outline"
          className="mt-4"
          onClick={() => refetchCities()}
        >
          Retry
        </Button>
      </div>
    )
  }

  if (showCityChooser) {
    return frame(<VenueCityChooser cities={cities} params={searchParams} />)
  }

  if (derivationPending || isLoading) {
    return frame(<VenueTableSkeleton />)
  }

  // Report an error only when nothing on screen answers the CURRENT query. A
  // plain `if (error)` would discard a rendered page over a failed background
  // refetch; `isPlaceholderData` covers the opposite hazard, where
  // `keepPreviousData` would present the previous city's rooms as the new
  // filter's answer.
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

  // Anything the reader can take off to widen the list. The city counts: the
  // control that clears it widens to every city, so a sentence offering it has
  // to be true about the city too.
  const hasNarrowingFilter =
    selectedTags.length > 0 || selectedCities.length > 0

  // The pane is for ONE city's rooms: it fits them in a 368px frame, and its
  // link opens the Atlas on that city. A whole-catalogue or multi-city page has
  // no single place to be about, so it gets the table alone.
  //
  // The CITY rather than a flag, so the condition and the value the pane needs
  // are one expression and cannot disagree.
  const miniAtlasCity =
    miniAtlasViewport && venues.length > 0 ? scopeCity : null

  // Nothing can be hovered that the reader cannot point at. Two ways the id
  // outlives its pointer, neither of which fires a mouseleave: the row is
  // removed from under the cursor by a page or filter change, and the window
  // narrows past 1280, which takes the pane away and unbinds the rows' own
  // handlers. Without this, widening again relights a room the pointer is
  // nowhere near. Adjusted during render rather than in an effect, which would
  // paint that frame first.
  if (
    hoveredVenueId !== null &&
    (miniAtlasCity === null || !venues.some(v => v.id === hoveredVenueId))
  ) {
    setHoveredVenueId(null)
  }

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
        {roomsLabel && upcomingLabel && (
          <p
            className="hidden text-xs text-muted-foreground sm:block"
            data-testid="venues-count-rule"
          >
            {roomsLabel} &middot; {upcomingLabel}
            {upcomingLabelIsAboutTheSet ? '' : ' on this page'} at verified
            rooms, on each venue&apos;s local calendar
          </p>
        )}
      </div>

      <div
        className={cn(
          'min-w-0 transition-opacity duration-75',
          isUpdating && 'opacity-60'
        )}
      >
        {/* Not rendered over zero rows: the pager clamps a hand-typed page to
            the last real one and would caption a position no row on screen
            occupies. The link in the beyond-end body is the way back. */}
        {venues.length > 0 && renderPager('top')}

        {pageIsBeyondEnd ? (
          <div
            className="py-12 text-center text-muted-foreground"
            data-testid="venues-page-beyond-end"
          >
            <p>That page is past the end of this list.</p>
            <p className="mt-2 text-sm">
              <Link
                href={pageHref(1)}
                className="inline-flex min-h-11 items-center text-primary hover:underline"
              >
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
              {/* The named city only when the facet knows it: a value the facet
                  cannot vouch for is never echoed back. */}
              {scopeCity && selectedTags.length === 0
                ? `No verified rooms in ${cityLabel(scopeCity)} yet.`
                : hasNarrowingFilter
                  ? 'No verified rooms match the current filters.'
                  : 'No verified rooms yet.'}
            </p>
            {/* The way on from a city with nothing in it. Rendered only with a
                city in hand: "nearby" needs something to be near, and the
                facet carries no coordinates, so the order is same state first
                and then busiest (`nearbyCitiesWithRooms`). */}
            {scopeCity && nearbyCities.length > 0 && (
              <p className="mt-3 text-sm" data-testid="venues-nearby-cities">
                <span className="text-muted-foreground">
                  Nearby cities with rooms:{' '}
                </span>
                {nearbyCities.map((city, index) => (
                  <span key={cityKey(city)}>
                    {index > 0 && (
                      <span aria-hidden="true" className="text-muted-foreground">
                        {' '}
                        &middot;{' '}
                      </span>
                    )}
                    <Link
                      href={venuesCityHref(searchParams, city.city, city.state)}
                      className="text-primary hover:underline"
                    >
                      {cityLabel(city)}
                    </Link>{' '}
                    <span className="text-muted-foreground">
                      ({countLabel(city.count, 'room')})
                    </span>
                  </span>
                ))}
              </p>
            )}
            <p className="mt-3 text-sm">
              {hasNarrowingFilter && !(scopeCity && selectedTags.length === 0) ? (
                <button
                  type="button"
                  onClick={handleClearFilters}
                  className="inline-flex min-h-11 items-center text-primary hover:underline"
                >
                  Clear filters
                </button>
              ) : (
                <Link
                  href="/contribute"
                  className="inline-flex min-h-11 items-center text-primary hover:underline"
                >
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
              // From the SCOPE: exactly one selected city is the only case
              // where every row would repeat the same city.
              showCity={selectedCities.length !== 1}
              // Only while the pane is on screen. Without a map beside them the
              // rows carry no hover reporting and no focus target, which is
              // what keeps them identical at every width below 1280.
              hoveredVenueId={miniAtlasCity ? hoveredVenueId : null}
              onHoverVenue={miniAtlasCity ? setHoveredVenueId : undefined}
              controlRef={venueTableControl}
            />
            {/* A key for the mark beside each room name. Hidden from assistive
                tech entirely: the mark itself is `aria-hidden` there and each
                row already says "Verified room" in its own text. */}
            <p
              aria-hidden="true"
              className="mt-3 text-xs text-muted-foreground"
              data-testid="venues-verified-legend"
            >
              &#10003; verified room
            </p>
          </>
        )}

        {venues.length > 0 && renderPager('bottom')}
      </div>
    </>,
    miniAtlasCity ? (
      <VenueMiniAtlasPane
        venues={venues}
        scopeCity={miniAtlasCity}
        hoveredVenueId={hoveredVenueId}
        onHoverVenue={setHoveredVenueId}
        onSelectVenue={revealVenueRow}
      />
    ) : null
  )
}
