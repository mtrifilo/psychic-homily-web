'use client'

import { useCallback, useMemo } from 'react'
// Deep import, not the barrel: `@/features/shows`'s barrel edge drags in
// ShowForm and the whole mutation graph for one component, and pulls a
// venues -> shows -> venues value cycle in behind it (the same reason
// VenueShowsTable deep-imports ShowBill).
import {
  archiveListState,
  PastShowsArchive,
  useArchivePage,
} from '@/features/shows/components/PastShowsArchive'
import {
  archiveScope,
  archiveYearScope,
} from '@/features/shows/showArchive'
import {
  useVenueShowMonths,
  useVenueShowYears,
  useVenueShows,
} from '../hooks/useVenues'
import { venuePastShowsPageParams } from '../api'
import { venueArchiveHref, venueZoneResolver, VENUE_PAST_SHOWS_FRAGMENT } from '../showArchive'
import { VenueShowsTable } from './VenueShowsTable'
import type {
  VenueShow,
  VenueShowZone,
  VenueShowMonthsResponse,
  VenueShowsResponse,
  VenueShowYearsResponse,
} from '../types'

/**
 * Anchor the past-shows pager and year strip land on. Deliberately its own id
 * rather than `VENUE_SHOWS_ANCHOR`, which sits on the whole shows block and
 * would drop a reader who just changed page at the top of the UPCOMING list.
 *
 * Re-exported from `showArchive`, which owns the archive's URL space and has to
 * append this fragment when it builds the all-years hrefs.
 */
export const VENUE_PAST_SHOWS_ANCHOR = VENUE_PAST_SHOWS_FRAGMENT

export interface VenuePastShowsProps {
  venueId: number
  /** Used to build page/year hrefs, so they are absolute and shareable. */
  venueSlug: string
  /** Used in the document title while a year or page is active. */
  venueName: string
  venueState: string
  venueTimezone?: string | null
  /**
   * The year this archive is scoped to, or null for every year.
   *
   * A PROP, not a `?year=` read: the year lives in the path
   * (`/venues/{slug}/shows/{year}`), so the route already knows it before this
   * component mounts, and the server can render the right rows into the HTML.
   * See `venueArchiveHref` for the whole URL space and why the year is a
   * segment while the page is not.
   */
  activeYear?: number | null
  /**
   * What the SERVER already fetched for this exact scope, seeded into the query
   * cache so the first render — server AND client — has it.
   *
   * The three are seeded by DIFFERENT sets of routes, which is worth stating
   * because a reader who assumes they travel together will mis-reason about
   * what is in the HTML:
   *
   *   initialShows   year-archive route ONLY. It is what puts the rows, and
   *                  therefore the pagers, in the served document; the venue
   *                  page renders the archive only after its first client fetch.
   *   initialYears   BOTH routes. The year strip is in the HTML either way.
   *   initialMonths  year-archive route ONLY (PSY-1769), and it travels with
   *                  `initialShows` for exactly that reason: labels are pager
   *                  chrome, so seeding them is worth a server read only where a
   *                  pager actually reaches the HTML.
   */
  initialShows?: VenueShowsResponse
  initialYears?: VenueShowYearsResponse
  initialMonths?: VenueShowMonthsResponse
  className?: string
}

/**
 * The venue's past-show archive: a year strip, a paged table, and pagers above
 * and below it (PSY-1753).
 *
 * A THIN WRAPPER over `PastShowsArchive` since PSY-1842. Everything a reader can
 * see — pager placement, which pager owns the live region, how a page link gets
 * its month-range label, what an empty or past-the-end page says — lives there
 * and is shared with the artist archive, because it had been duplicated between
 * the two and the duplication had already leaked a bug in each direction. What
 * is left here is what is genuinely venue-shaped: which endpoints to read, what
 * the URLs look like, and that every row is dated on the venue's own calendar.
 *
 * Mounted on TWO surfaces, and the difference between them is one prop
 * (PSY-1756): the venue page renders it with no `activeYear` (every year, paged)
 * and the year-archive route renders it scoped to a year, with the first page
 * and the histogram already fetched server-side. One component rather than two
 * so the archive cannot drift between the surface a reader browses and the
 * surface a crawler indexes.
 */
export function VenuePastShows({
  venueId,
  venueSlug,
  venueName,
  venueState,
  venueTimezone,
  activeYear = null,
  initialShows,
  initialYears,
  initialMonths,
  className,
}: VenuePastShowsProps) {
  const page = useArchivePage()

  const pageParams = venuePastShowsPageParams(page, activeYear)
  const offset = pageParams.offset ?? 0
  // Read from the params the LIST actually requested, not from the constant
  // behind them: the label walk maps row ordinals onto pages, so a page size
  // that ever diverged from the request would shift every label by the
  // difference — a wrong label, which is worse than a missing one. Named as a
  // primitive because `pageParams` is a fresh object each render.
  const pageLimit = pageParams.limit

  const yearsQuery = useVenueShowYears({
    venueId,
    timeFilter: 'past',
    initialData: initialYears,
  })

  // The histogram is the authority on counts. It is keyed on the venue alone,
  // so switching years never leaves it briefly describing the previous year the
  // way a page envelope does under `keepPreviousData`, and it is resolved
  // before the reader can click anything.
  //
  // Resolved BEFORE the row request because it GATES it: knowing the page count
  // up front means a stale or hand-edited `?page=` past the end is answered
  // without a round trip, instead of spending a 50,000-row offset scan to be
  // told there is nothing there.
  const yearScope = archiveYearScope({
    years: yearsQuery.data?.years,
    activeYear,
    page,
    pageSize: pageLimit,
  })

  const pastQuery = useVenueShows({
    venueId,
    timeFilter: 'past',
    limit: pageLimit,
    offset,
    year: pageParams.year,
    enabled: !yearScope.pageIsBeyondKnownEnd,
    // The old page stays on screen (dimmed) while the next one loads, so
    // paging does not collapse the section to a spinner and bounce the layout.
    keepPreviousPage: true,
    // ONLY on the page the server actually fetched. `initialData` attaches to
    // whatever key is current, so handing page 1's rows to a hook asking for
    // page 2 would seed page 2 with the wrong slice — and unlike a stale page
    // that would never correct itself, because it would look like a hit.
    initialData: page === 1 ? initialShows : undefined,
  })

  // Memoized on the response rather than recomputed: this array feeds the
  // table's month grouping and the archive's label memo, and a fresh `[]` on
  // every render would make both recompute forever.
  const pastData = pastQuery.data
  const rows: VenueShow[] = useMemo(() => pastData?.shows ?? [], [pastData])

  const scope = archiveScope(yearScope, {
    yearsSettled: yearsQuery.isSuccess,
    listSettled: pastQuery.isSuccess,
    listTotal: pastData?.total ?? 0,
    pageSize: pageLimit,
  })

  // What the pager labels its page links from (PSY-1769).
  //
  // On the VENUE page `monthsAreWorthFetching` is the only thing suppressing the
  // request, and it is doing most of the work: no server read happens there, and
  // most venues have a single-page archive that renders no pager at all. On the
  // YEAR-ARCHIVE route the histogram is fetched server-side unconditionally — the
  // count that would gate it is a sibling in the same `Promise.all`, and waiting
  // on it would put a round trip on the critical path of every cold render — so
  // there the seed has already arrived and this flag only decides whether to
  // revalidate.
  const monthsQuery = useVenueShowMonths({
    venueId,
    timeFilter: 'past',
    enabled: scope.monthsAreWorthFetching,
    initialData: initialMonths,
  })

  // One definition of the archive's URL space, shared with the sitemap and the
  // route that serves the year pages — see `venueArchiveHref`.
  const buildHref = useCallback(
    (year: number | null, targetPage: number) =>
      venueArchiveHref(venueSlug, year, targetPage),
    [venueSlug]
  )

  const zone: VenueShowZone = useMemo(
    () => ({ venueState, venueTimezone }),
    [venueState, venueTimezone]
  )
  // Memoized because the archive's label memo depends on it by reference.
  const zoneOf = useMemo(() => venueZoneResolver<VenueShow>(zone), [zone])

  const renderTable = useCallback(
    (pageRows: VenueShow[]) => (
      <VenueShowsTable
        shows={pageRows}
        zone={zone}
        ariaLabel="Past shows"
        groupByMonthHeadings
      />
    ),
    [zone]
  )

  return (
    <PastShowsArchive
      anchorId={VENUE_PAST_SHOWS_ANCHOR}
      entityName={venueName}
      activeYear={activeYear}
      page={page}
      pageSize={pageLimit}
      buildHref={buildHref}
      scope={scope}
      years={{
        counts: yearsQuery.data?.years ?? [],
        isError: yearsQuery.isError,
      }}
      months={monthsQuery.data?.months}
      list={archiveListState(pastQuery, rows)}
      zoneOf={zoneOf}
      renderTable={renderTable}
      // `/venues/{slug}/shows/{year}` is a real document the sitemap announces
      // (PSY-1756), which is what makes even a one-year strip load-bearing here:
      // suppressing it left that URL with no inbound link anywhere on the site,
      // next to a venue page carrying the identical rows.
      hasPerYearRoute
      className={className}
    />
  )
}
