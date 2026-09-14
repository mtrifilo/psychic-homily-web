'use client'

import { useCallback, useMemo, useTransition } from 'react'
import Link from 'next/link'
import { useSearchParams, useRouter } from 'next/navigation'
import { parseAsInteger, useQueryState } from 'nuqs'
import { useShowsCalendar, useShowCities, useShowMonths } from '../hooks/useShows'
import { useShowSaveCountBatch } from '../hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/features/auth'
import type { CityState } from '@/components/filters'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { DensityToggle, MonthStrip } from '@/components/shared'
import {
  Pagination,
  usePaginationFocusTarget,
} from '@/components/shared/Pagination'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { DayGroupedShowList } from './DayGroupedShowList'
import { ShowListSkeleton } from './ShowListSkeleton'
import {
  clampPage,
  MAX_ARCHIVE_PAGE,
  pageRangeLabelsForWindow,
} from '../showArchive'
import { SHOWS_PAGE_SIZE, showsPageHref } from '../showsListNavigation'
import {
  SHOWS_ROOT,
  adjacentMonths,
  shortCalendarMonthLabel,
  showsMonthPath,
  showsWindowPath,
  type ShowsCalendarWindow,
} from '../showsCalendarRoute'
import { CityFilters, type CityWithCount } from '@/components/filters'
import {
  citiesEqual,
  citiesParser,
  ALL_CITIES,
} from '@/components/filters/cityParams'
import {
  useGeoDefaultCity,
  shouldShowGeoAffordance,
} from '@/components/filters/useGeoDefaultCity'
import { GeoDefaultAffordance } from '@/components/filters/GeoDefaultAffordance'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'
import {
  formatCount,
  navLinkClass,
  navStripClass,
  navStripListClass,
  navStripSeparatorClass,
} from '@/components/shared/paginationChrome'
import { suggestAlternativeCities } from '../suggestCities'

export interface ShowListProps {
  /**
   * The venue-local calendar window this list is scoped to, or undefined on
   * the unwindowed root.
   *
   * It decides three things together, which is why it is one prop rather than
   * a set: which rows the list requests, which URL its pager and its filter
   * writes address, and which month the strip marks as current. A list whose
   * rows were windowed but whose pager was not would page a month's second
   * page onto the root's URL.
   */
  window?: ShowsCalendarWindow
}

export function ShowList({ window }: ShowListProps = {}) {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { user, isAuthenticated, authStatus } = useAuthContext()
  const isAdmin = user?.is_admin ?? false
  const [isPending, startTransition] = useTransition()
  const { data: profileData } = useProfile()
  // NOTE (PSY-1624): `useDensity` reads localStorage through a server snapshot,
  // so the server HTML and the hydration render are ALWAYS 'comfortable'. A
  // viewer who chose compact or expanded now gets one whole-list re-layout on
  // the commit after hydration, where before this ticket the cards first
  // mounted post-hydration already holding the right value. Accepted rather
  // than unnoticed: the alternative is either not server-rendering the rows
  // (the thing this ticket exists to do) or persisting density somewhere the
  // server can read, which is per-visitor state on a cacheable route. Bounding
  // the shift by reserving row height across densities is the cheap follow-up.
  const { density, setDensity } = useDensity('shows')

  // Read favorites from profile — the per-user default city.
  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  // `?cities=` is the source of truth, read/written via nuqs. Three states:
  //   null        → param absent → apply the default (favorites/geo), derived below
  //   ALL_CITIES  → ?cities=all → explicit "all cities"
  //   CityState[] → explicit selection
  // Filter changes push a history entry so the back button steps through them.
  const [citiesState, setCities] = useQueryState(
    'cities',
    citiesParser.withOptions({ history: 'push', startTransition })
  )

  // Legacy single-city params (?city=&state=) — read-only, for back-compat.
  const legacyCity = searchParams.get('city')
  const legacyState = searchParams.get('state')

  // Parse multi-tag from URL (PSY-309)
  const tagsParam = searchParams.get('tags')
  const tagMatchParam = searchParams.get('tag_match')
  const selectedTags = useMemo(() => parseTagsParam(tagsParam), [tagsParam])
  const tagMatch: 'all' | 'any' = tagMatchParam === 'any' ? 'any' : 'all'

  // Any explicit selection (?cities=<pick>, ?cities=all, or legacy single-city)
  // means geo must not seed. Authed favorites also stand the geo hook down
  // (handled inside the hook via favoriteCities + isAuthenticated).
  const hasExistingSelection = citiesState !== null || !!(legacyCity && legacyState)

  // The page in view. `parseAsInteger.withDefault(1)` is the archive family's
  // reading of `?page=`, and `clampPage` bounds a hand-edited one so it becomes
  // an empty page rather than an arbitrarily large offset the backend will
  // happily scan for (its `offset` carries a minimum and no maximum).
  const [rawPage, setPage] = useQueryState(
    'page',
    parseAsInteger.withDefault(1).withOptions({ history: 'push', startTransition })
  )
  const page = clampPage(rawPage, MAX_ARCHIVE_PAGE)
  const offset = (page - 1) * SHOWS_PAGE_SIZE

  // A filter change answers a different question, so it starts at page 1 again.
  // Written through nuqs alongside `setCities`, which batches both into ONE
  // history entry; a `router.push` in the same tick would abort nuqs's pending
  // queue and the reset could be dropped.
  const resetPage = useCallback(() => {
    void setPage(null)
  }, [setPage])

  const {
    data: citiesData,
    isLoading: citiesLoading,
    isFetching: citiesFetching,
    isPlaceholderData: citiesArePlaceholder,
  } = useShowCities()

  // Map ShowCity → CityWithCount (the has-shows list). Lifted above the early
  // returns so the geo hook can read it unconditionally.
  const cities: CityWithCount[] = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.show_count,
        // Geocoded centroid (PSY-981) — drives the nearest-has-shows-city geo
        // default when the visitor's exact city has no shows.
        latitude: c.latitude,
        longitude: c.longitude,
      })) ?? [],
    [citiesData?.cities]
  )

  // IP-geo soft default for anon visitors (PSY-946). /shows reads geo via the
  // `/api/geo` edge route handler client-side (the page stays ISR — it must
  // not read `next/headers`). The hook RETURNS the derived canonical city;
  // it's folded into the derived selection below (never written to the URL).
  // Favorites and an existing `?cities=`/legacy selection both win (the hook
  // stands down).
  const { appliedGeoDefault, notifyUserInteracted } = useGeoDefaultCity({
    cities,
    authStatus,
    favoriteCities,
    hasExistingSelection,
    enableClientFetch: true,
  })

  // The effective city filter, DERIVED during render — never seeded into the
  // URL by an effect. A bare /shows resolves to the user's favorite (or, for
  // anon visitors, the geo default); an explicit ?cities=all or ?cities=<pick>
  // wins. Deriving (rather than writing the default from a mount effect) is
  // what makes the default survive client-side navigation: the URL, not a
  // mount ref, is the source of truth.
  const selectedCities: CityState[] = useMemo(() => {
    if (citiesState === ALL_CITIES) return []
    if (citiesState) return citiesState
    if (legacyCity && legacyState) return [{ city: legacyCity, state: legacyState }]
    if (favoriteCities.length > 0) return favoriteCities
    return appliedGeoDefault ? [appliedGeoDefault] : []
  }, [citiesState, legacyCity, legacyState, favoriteCities, appliedGeoDefault])

  const listFilters = useMemo(
    () => ({
      cities: selectedCities.length > 0 ? selectedCities : undefined,
      tags: selectedTags.length > 0 ? selectedTags : undefined,
      tagMatch,
    }),
    [selectedCities, selectedTags, tagMatch]
  )

  // The URL this list's own navigation is rooted at. Every href and every
  // router write below is built from it, so a windowed list never writes a
  // page or a filter onto the root's address.
  const basePath = window ? showsWindowPath(window) : SHOWS_ROOT

  const {
    data,
    isLoading,
    isFetching,
    isPlaceholderData,
    error,
    refetch,
  } = useShowsCalendar({
    offset,
    limit: SHOWS_PAGE_SIZE,
    window,
    ...listFilters,
  })

  // The month histogram that labels every page link before the reader spends a
  // click on it. Filter-keyed, so paging does not re-request it.
  const { data: monthsData, isPlaceholderData: monthsArePlaceholder } =
    useShowMonths(listFilters)

  const pageShows = useMemo(() => data?.shows ?? [], [data?.shows])

  // Batch-check saved status for all visible shows (1 request instead of N)
  const allShowIds = useMemo(() => pageShows.map(s => s.id), [pageShows])
  const { data: saveCounts } = useShowSaveCountBatch(
    allShowIds,
    isAuthenticated,
    user?.id
  )

  const listTotal = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(listTotal / SHOWS_PAGE_SIZE))


  // Facts about the current slice are only stated while the rows on screen
  // answer the current request. `keepPreviousData` holds the outgoing page in
  // place across a page change, and "Showing 51-100" over rows 1-50 is a wrong
  // number rather than a stale one.
  const rowsAnswerCurrentRequest = !isPlaceholderData

  // The buckets the labels may be derived from, and NOTHING while the histogram
  // holds its own previous filters' data.
  //
  // Withholding the total is not enough on its own: the premise check inside
  // `monthRangeLabelsByPage` is skipped when no total is supplied (the
  // histogram is then trusted alone), and the current page's label is the only
  // one dropped. A stale histogram would go on labelling every OTHER page in
  // the window with the wrong months. Handing it no buckets is what makes the
  // family's rule hold here: a label is verified or absent.
  const labelBuckets = monthsArePlaceholder ? [] : (monthsData?.months ?? [])

  // The URL named a page this list does not have. Distinguished from a genuinely
  // empty list, which is the same zero rows and a very different sentence.
  // Only once the rows on screen answer THIS request: while `keepPreviousData`
  // holds the previous page, `page` and `totalPages` can disagree transiently.
  const pageIsBeyondEnd =
    rowsAnswerCurrentRequest && pageShows.length === 0 && listTotal > 0

  // Month-range page labels: what is behind a page number, before the reader
  // spends a click on it. The window derivation and the withhold-while-stale
  // rule live in `pageRangeLabelsForWindow`, which the past-shows archive
  // shares. There is no row-derived fallback for the current page here, and
  // that is deliberate: this list spans venues, so a fallback would read each
  // row's own zone while the histogram buckets venue-locally, and the two can
  // disagree at a month boundary.
  const rangeLabels = useMemo(
    () =>
      pageRangeLabelsForWindow({
        // Soonest first from the API, which is the order this list pages in.
        months: labelBuckets,
        page,
        totalPages,
        pageSize: SHOWS_PAGE_SIZE,
        // The count that arrived WITH the rows, and whether those rows answer
        // this request. The premise being checked is that the histogram's
        // ordinals are the list's ordinals, and only the list can attest to
        // that, so a disagreement blanks every label rather than printing a
        // span the page does not cover.
        total: data?.total,
        rowsAnswerCurrentRequest,
        // The list spans years, so a label may never elide the year.
        scope: 'all-years',
      }),
    [labelBuckets, page, totalPages, rowsAnswerCurrentRequest, data?.total]
  )

  // The strip's bars. The histogram is the ONLY thing that says which months
  // are documents, so passing it through unfiltered is what keeps the strip
  // from offering a link the route answers with a not-found.
  const monthEntries = useMemo(
    () =>
      (monthsData?.months ?? []).map(entry => ({
        year: entry.year,
        month: entry.month,
        count: entry.count,
      })),
    [monthsData?.months]
  )

  // The month the strip marks. A day page marks its own month, which is why
  // the relation below is not always 'page'.
  const currentMonth = useMemo(
    () => (window ? { year: window.year, month: window.month } : null),
    [window]
  )

  // The neighbours THAT HAVE SHOWS, nearest first, as rendered links. Empty on
  // the root and on day pages, and empty for a month at either end of the
  // histogram.
  const adjacentLinks = useMemo(() => {
    if (!window || window.day !== undefined) return []
    const { previous, next } = adjacentMonths(monthEntries, window)
    const links: Array<{ href: string; label: string }> = []
    if (previous) {
      links.push({
        href: showsMonthPath(previous.year, previous.month),
        label: `‹ ${shortCalendarMonthLabel(previous.year, previous.month)}`,
      })
    }
    if (next) {
      links.push({
        href: showsMonthPath(next.year, next.month),
        label: `${shortCalendarMonthLabel(next.year, next.month)} ›`,
      })
    }
    return links
  }, [window, monthEntries])

  // The frame's title-row scope: the size of the whole matching set, and the
  // metro when exactly one is selected. NOT the rows on screen, which is what
  // the old "50 of 268 shows" line reported and what the pager's caption
  // already says exactly ("Showing 51-100 of 268").
  //
  // No leading separator. The frame prints this beside the `<h1>`, where the
  // middot joins the two; this renders it on its own line (the heading is
  // server-rendered in the route's shell and this total is client-derived from
  // the city filter, so the two cannot share a row without moving the heading
  // out of the shell), and a separator with nothing to its left is a defect
  // rather than a style.
  const scopeLabel = useMemo(() => {
    const total = formatCount(listTotal)
    if (selectedCities.length === 1) {
      const { city, state } = selectedCities[0]
      return `${total} in ${city}, ${state}`
    }
    return `${total} upcoming`
  }, [listTotal, selectedCities])

  const { targetProps, focusTarget } = usePaginationFocusTarget<HTMLParagraphElement>()

  // Spreads the params ALREADY on screen and overrides only `page`, so the city
  // filter, the tag filter and any foreign param survive a page click.
  const pageHref = useCallback(
    (targetPage: number) => showsPageHref(searchParams, targetPage, basePath),
    [searchParams, basePath]
  )

  // City filter changes write the `?cities=` param via nuqs (which preserves
  // other params). An empty selection becomes the explicit ALL_CITIES sentinel
  // (?cities=all), NOT a bare URL — a bare URL means "apply my default".
  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      // Any manual city change is an override — block a still-in-flight geo seed
      // and drop the affordance.
      notifyUserInteracted()
      resetPage()
      void setCities(cities.length > 0 ? cities : ALL_CITIES)
    },
    [notifyUserInteracted, resetPage, setCities]
  )

  // Tag changes rewrite only the tag params via the router, preserving the raw
  // `?cities=` state (absent / all / selection) so a tag change never
  // materializes the derived default into the URL.
  const writeTags = useCallback(
    (nextTags: string[], nextMatch: 'all' | 'any') => {
      const params = new URLSearchParams(searchParams.toString())
      params.delete('tags')
      params.delete('tag_match')
      // A different tag set is a different question, answered from page 1. Done
      // inside this ONE router write rather than through nuqs beside it: a
      // foreign history update aborts nuqs's pending queue, so the reset could
      // be dropped.
      params.delete('page')
      if (nextTags.length > 0) {
        params.set('tags', buildTagsParam(nextTags))
        if (nextMatch === 'any') params.set('tag_match', 'any')
      }
      const queryString = params.toString()
      startTransition(() => {
        router.push(queryString ? `${basePath}?${queryString}` : basePath, {
          scroll: false,
        })
      })
    },
    [searchParams, router, basePath]
  )

  const handleTagsChange = useCallback(
    (nextTags: string[]) => writeTags(nextTags, tagMatch),
    [tagMatch, writeTags]
  )

  const handleTagsClear = useCallback(
    () => writeTags([], tagMatch),
    [tagMatch, writeTags]
  )

  // "Clear filters" (the empty-state affordance) resets tags AND cities in a
  // SINGLE navigation. Doing it as two writes would race: the router push for
  // tags and nuqs's throttled `setCities` fire in the same tick, and nuqs's
  // adapter aborts its pending queue when it sees a foreign history update — so
  // the `?cities=all` reset could be silently dropped. One write avoids that.
  const handleClearFilters = useCallback(() => {
    notifyUserInteracted()
    startTransition(() => {
      router.push(`${basePath}?cities=all`, { scroll: false })
    })
  }, [notifyUserInteracted, router, basePath])

  // Keep tags, drop the city constraint (PSY-1433 empty-state suggestion).
  const handleSameTagsAllCities = useCallback(() => {
    notifyUserInteracted()
    const params = new URLSearchParams()
    params.set('cities', 'all')
    if (selectedTags.length > 0) {
      params.set('tags', buildTagsParam(selectedTags))
      if (tagMatch === 'any') params.set('tag_match', 'any')
    }
    startTransition(() => {
      router.push(`${basePath}?${params.toString()}`, { scroll: false })
    })
  }, [notifyUserInteracted, router, selectedTags, tagMatch, basePath])

  const alternativeCities = useMemo(
    () =>
      pageShows.length === 0 && selectedCities.length > 0
        ? suggestAlternativeCities(cities, selectedCities, 3)
        : [],
    [pageShows.length, selectedCities, cities]
  )

  // Determine if "Save as default" / "Clear defaults" should show
  const selectionDiffersFromFavorites = !citiesEqual(selectedCities, favoriteCities)

  // Only show skeleton on FIRST load (no data yet)
  if ((isLoading && !data) || (citiesLoading && !citiesData)) {
    return <ShowListSkeleton />
  }

  // Track if we're updating (fetching but already have data)
  // Dim only while the rows on screen belong to a DIFFERENT query than the one
  // being awaited, which now means a filter change and nothing else:
  // `keepPreviousData` is holding the old page in place. `isPlaceholderData`
  // says exactly that; raw `isFetching` does not, and using it would dim a
  // same-key background revalidation. That distinction became visible in
  // PSY-1624: the server-seeded first screen arrives stale by construction
  // (`seedFirstScreen` stamps `dataUpdatedAt: 0`), so `isFetching` is true on
  // the very first client commit and the freshly server-rendered list would
  // fade to 60% the instant it hydrated, which is the opposite of the point.
  const isUpdating =
    (isFetching && isPlaceholderData) ||
    (citiesFetching && citiesArePlaceholder) ||
    isPending

  // Report an error only when nothing on screen answers the CURRENT query.
  //
  // Plain `if (error)` was wrong once the first screen came from the server:
  // that entry is stale by construction (`seedFirstScreen`), so every load
  // revalidates it, and one failed background refetch would swap a fully
  // rendered list for an error message having lost nothing the reader was
  // looking at. But `error && !data` alone is wrong in the other direction —
  // `keepPreviousData` means `data` survives a key change, so a filter change
  // whose request fails would leave the PREVIOUS city's shows on screen under
  // the new filter chip, silently, presented as the answer. `isPlaceholderData`
  // is exactly "these rows belong to a different query", which is the case that
  // has to surface.
  if (error && (!data || isPlaceholderData)) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load shows. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  const showGeoAffordance = shouldShowGeoAffordance(
    appliedGeoDefault,
    selectedCities
  )

  const renderPager = (position: 'top' | 'bottom') => (
    <Pagination
      currentPage={page}
      totalPages={totalPages}
      pageHref={pageHref}
      ariaLabel={`Upcoming shows pagination, ${position} of list`}
      rangeLabels={rangeLabels}
      // Omitted while the rows on screen belong to the previous page: the
      // caption states an exact range, and "Showing 51-100" over rows 1-50 is a
      // wrong number, not a stale one. The pager falls back to "Page 2 of 6",
      // which stays true throughout.
      captionRange={
        rowsAnswerCurrentRequest && pageShows.length > 0
          ? { start: offset + 1, end: offset + pageShows.length, total: listTotal }
          : undefined
      }
      // ONE of the two instances owns the announcement, or a screen reader hears
      // "Page 2 of 6" twice on every click. The top pager keeps it: it is beside
      // the line the pager moves focus to, and first in DOM order.
      announce={position === 'top'}
      // The list runs soonest first, so paging BACK moves toward tonight.
      previousLabel="Sooner"
      nextLabel="Later"
      onNavigate={focusTarget}
      className={position === 'top' ? 'mb-4' : 'mt-6'}
    />
  )

  return (
    <section className="w-full max-w-6xl">
      {/* Show the filter whenever ≥1 city has shows (PSY-932) — consistent
          with /venues and /artists; hidden only when there are no cities. */}
      {cities.length > 0 && (
        <div className="mb-6">
          <CityFilters
            cities={cities}
            selectedCities={selectedCities}
            onFilterChange={handleFilterChange}
          >
            {isAuthenticated && selectionDiffersFromFavorites && (
              <SaveDefaultsButton
                selectedCities={selectedCities}
                favoriteCities={favoriteCities}
              />
            )}
          </CityFilters>
          {showGeoAffordance && (
            <GeoDefaultAffordance
              city={appliedGeoDefault}
              onChange={() => handleFilterChange([])}
            />
          )}
        </div>
      )}

      {/* Mobile: Sheet trigger + density toggle. Desktop hides the Sheet (the
          bar below takes over) but keeps the density toggle on this row. */}
      <div className="flex items-center justify-between mb-4 gap-2">
        <TagFacetSheet
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          title="Filter shows by tag"
          entityType="show"
          selectedCities={selectedCities}
        />
        <DensityToggle density={density} onDensityChange={setDensity} />
      </div>

      {/* PSY-1000: full-width top-bar tag filter above a full-width list (no
          left rail). Desktop only — mobile uses the Sheet trigger above. */}
      <div className="mb-4 hidden lg:block">
        <TagFacetPanel
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          heading="Filter shows by tag"
          entityType="show"
          selectedCities={selectedCities}
          layout="bar"
        />
      </div>

      <div className={cn('min-w-0', isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75')}>
        {/* The month axis. Inside the dimming wrapper rather than above it
            because its counts come from the same filtered query family as the
            rows: a strip that stayed bright while the rows faded would be
            asserting counts for a filter the list has already left.

            Bounded to the months the histogram carries, so no link here can
            address a month the route 404s. */}
        <MonthStrip
          months={monthEntries}
          hrefFor={showsMonthPath}
          allHref={SHOWS_ROOT}
          allLabel="All upcoming"
          allCount={monthsData?.total}
          current={currentMonth}
          // A day page sits INSIDE the marked month rather than being it, so
          // the mark is a section relation there and `aria-current="page"`
          // would point a reader at a link that navigates away.
          currentRelation={window?.day === undefined ? 'page' : 'section'}
          ariaLabel="Filter shows by month"
          className="mb-3"
        />

        {/* The two months either side of this one, for a reader walking
            forward or back without returning to the strip. Month pages only:
            on the root there is no current month to be adjacent to, and on a
            day page the neighbouring MONTHS are the wrong axis. */}
        {adjacentLinks.length > 0 && (
          <nav
            aria-label="Adjacent months"
            className={cn(navStripClass, 'mb-3')}
            data-testid="month-adjacent"
          >
            <ul className={navStripListClass}>
              {adjacentLinks.map((link, index) => (
                <li key={link.href}>
                  {index > 0 && (
                    <span aria-hidden="true" className={navStripSeparatorClass}>
                      ·
                    </span>
                  )}
                  <Link href={link.href} className={navLinkClass}>
                    {link.label}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
        )}

        {/* The list's SCOPE, and the pager's focus target.
            A page change only swaps the rows, which would otherwise leave
            focus on a control that has moved or unmounted. It is a stable
            LANDMARK at the top of the list, not a report of the change: the
            pager's live region announces the new position, and the pager's
            own caption states which rows are on screen. This line states the
            whole matching set instead, which is what the frame's title row
            carries. */}
        <p
          className="mb-3 font-mono text-[13px] text-muted-foreground"
          data-testid="show-count"
          {...targetProps}
        >
          {scopeLabel}
          {selectedTags.length > 0 && ` matching ${selectedTags.join(', ')}`}
        </p>

        {renderPager('top')}

        {pageIsBeyondEnd ? (
          /* An empty page over a NON-empty list: a stale bookmark, a
             hand-typed number, or a page that existed until shows graduated
             out of the upcoming set. Saying "no upcoming shows" here would be
             a flatly false claim about the catalogue, and the filter
             suggestions below answer a question nobody asked. The link is the
             way back and is always rendered: the pagers are not, since they
             return null on a list of one page, which a filtered list past its
             end can be. */
          <div
            className="text-center py-12 text-muted-foreground"
            data-testid="shows-page-beyond-end"
          >
            <p>That page is past the end of this list.</p>
            <p className="mt-2 text-sm">
              <Link href={pageHref(1)} className="text-primary hover:underline">
                Back to the first page
              </Link>
            </p>
          </div>
        ) : pageShows.length === 0 ? (
          <div
            className="text-center py-12 text-muted-foreground"
            data-testid="shows-zero-result"
          >
            <p>
              {selectedTags.length > 0 || selectedCities.length > 0
                ? 'No upcoming shows match the current filters.'
                : 'No upcoming shows at this time.'}
            </p>
            {(selectedTags.length > 0 || selectedCities.length > 0) && (
              <div className="mt-4 flex flex-col items-center gap-3 text-sm">
                {alternativeCities.length > 0 ? (
                  <p data-testid="shows-city-suggestions">
                    Try{' '}
                    {alternativeCities.map((city, index) => (
                      <span key={`${city.city}-${city.state}`}>
                        {index > 0
                          ? index === alternativeCities.length - 1
                            ? ', or '
                            : ', '
                          : null}
                        <button
                          type="button"
                          onClick={() =>
                            handleFilterChange([
                              { city: city.city, state: city.state },
                            ])
                          }
                          className="text-primary hover:underline"
                          data-testid={`shows-suggest-city-${city.city}-${city.state}`
                            .toLowerCase()
                            .replace(/\s+/g, '-')}
                        >
                          {city.city}
                        </button>
                      </span>
                    ))}
                    .
                  </p>
                ) : null}
                <div className="flex flex-wrap items-center justify-center gap-x-4 gap-y-2">
                  {selectedTags.length > 0 && selectedCities.length > 0 ? (
                    <button
                      type="button"
                      onClick={handleSameTagsAllCities}
                      className="text-primary hover:underline"
                      data-testid="shows-suggest-same-tags-all-cities"
                    >
                      Same tags, all cities
                    </button>
                  ) : null}
                  <button
                    type="button"
                    onClick={handleClearFilters}
                    className="text-primary hover:underline"
                  >
                    Clear filters
                  </button>
                </div>
              </div>
            )}
          </div>
        ) : (
          <DayGroupedShowList
            shows={pageShows}
            density={density}
            isAdmin={isAdmin}
            userId={user?.id}
            saveCounts={saveCounts}
            // From the FILTER: exactly one selected metro is the only case
            // where every row would repeat the same city.
            showCity={selectedCities.length !== 1}
          />
        )}

        {renderPager('bottom')}
      </div>
    </section>
  )
}
