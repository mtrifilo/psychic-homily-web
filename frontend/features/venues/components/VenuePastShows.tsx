'use client'

import { useCallback, useEffect, useMemo, useRef, type ReactNode } from 'react'
import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { parseAsInteger, useQueryState } from 'nuqs'
import {
  formatCount,
  Pagination,
  paginationWindow,
  SectionHeader,
  YearStrip,
  usePaginationFocusTarget,
  type YearStripEntry,
} from '@/components/shared'
import { cn } from '@/lib/utils'
import { useVenueShowYears, useVenueShows } from '../hooks/useVenues'
import { venueQueryKeys, venuePastShowsPageParams } from '../api'
import { clampPage, parseArchiveYear } from '@/features/shows/showArchive'
import { archiveDocumentTitle, monthRangeLabel } from '../showArchive'
import { VenueShowsTable } from './VenueShowsTable'
import type { VenueShow, VenueShowZone, VenueShowsResponse } from '../types'

/**
 * Anchor the past-shows pager and year strip land on. Deliberately its own id
 * rather than `VENUE_SHOWS_ANCHOR`, which sits on the whole shows block and
 * would drop a reader who just changed page at the top of the UPCOMING list.
 */
export const VENUE_PAST_SHOWS_ANCHOR = 'venue-past-shows'

/**
 * Upper bound on the page a URL may ask for, so a hand-edited `?page=` becomes
 * a bounded empty page instead of an unbounded offset the backend has to
 * reject. At 50 rows a page this covers 50,000 shows at one venue, roughly two
 * orders of magnitude past the busiest venue observed.
 */
const MAX_PAGE = 1_000

export interface VenuePastShowsProps {
  venueId: number
  /** Used to build page/year hrefs, so they are absolute and shareable. */
  venueSlug: string
  /** Used in the document title while a year or page is active. */
  venueName: string
  venueState: string
  venueTimezone?: string | null
  className?: string
}

/**
 * The venue's past-show archive: a year strip, a paged table, and pagers above
 * and below it (PSY-1753).
 *
 * URL-driven and read-only about it. Every page and year is a real `<a href>`
 * built by this component, and navigation happens through Next's router when
 * one is clicked; nothing here WRITES the query string. That is deliberate —
 * calling a nuqs setter alongside a `<Link>` navigation puts two writers on the
 * same URL in one tick, which is the PSY-1388 failure. The nuqs hooks below
 * read the result; the hrefs decide it.
 *
 * The section renders nothing at all for a venue with no past shows, so the
 * page does not carry an empty archive.
 */
export function VenuePastShows({
  venueId,
  venueSlug,
  venueName,
  venueState,
  venueTimezone,
  className,
}: VenuePastShowsProps) {
  const [rawYear] = useQueryState('year', parseAsInteger)
  const [rawPage] = useQueryState('page', parseAsInteger.withDefault(1))
  const activeYear = parseArchiveYear(rawYear)
  const page = clampPage(rawPage, MAX_PAGE)

  const pageParams = venuePastShowsPageParams(page, activeYear)
  const offset = pageParams.offset ?? 0

  const yearsQuery = useVenueShowYears({ venueId, timeFilter: 'past' })
  const yearCounts = yearsQuery.data?.years ?? []

  // The histogram is the authority on counts. It is keyed on the venue alone,
  // so switching years never leaves it briefly describing the previous year the
  // way a page envelope does under `keepPreviousData`, and it is resolved
  // before the reader can click anything.
  const haveHistogram = yearCounts.length > 0
  const allTimeTotal = yearCounts.reduce((sum, entry) => sum + entry.count, 0)
  const histogramTotal = !haveHistogram
    ? null
    : activeYear === null
      ? allTimeTotal
      : (yearCounts.find(entry => entry.year === activeYear)?.count ?? 0)

  // Knowing the page count up front means a stale or hand-edited `?page=` past
  // the end is answered without a round trip, instead of spending a 50,000-row
  // offset scan to be told there is nothing there. Skipped while the histogram
  // is still loading, when the page request is the only thing that knows.
  const pageIsBeyondKnownEnd =
    histogramTotal !== null &&
    page > Math.max(1, Math.ceil(histogramTotal / pageParams.limit))

  const pastQuery = useVenueShows({
    venueId,
    timeFilter: 'past',
    timezone: pageParams.timezone,
    limit: pageParams.limit,
    offset,
    year: pageParams.year,
    enabled: !pageIsBeyondKnownEnd,
    // The old page stays on screen (dimmed) while the next one loads, so
    // paging does not collapse the section to a spinner and bounce the layout.
    keepPreviousPage: true,
  })

  const { targetProps, focusTarget } = usePaginationFocusTarget<HTMLHeadingElement>()

  const zone: VenueShowZone = useMemo(
    () => ({ venueState, venueTimezone }),
    [venueState, venueTimezone]
  )

  // Memoized on the response rather than recomputed: this array is a dependency
  // of the page-label memo below and of the table's month grouping, and a fresh
  // `[]` on every render would make both recompute forever.
  const pastData = pastQuery.data
  const rows: VenueShow[] = useMemo(() => pastData?.shows ?? [], [pastData])

  // Whether `rows` answers the question the URL is currently asking.
  //
  // `keepPreviousData` deliberately holds the PREVIOUS page (or year) on screen
  // while the next one loads, and `isPlaceholderData` is exactly "these rows
  // belong to a different query". Anything derived from the rows — the caption
  // range, this page's month-span label — must be suppressed until it clears,
  // or the surface states a fact about a slice the reader is not looking at
  // ("Showing 51-100 of 161" over rows 1-50). Dimming says stale; it does not
  // make a wrong number right. It matters most for the label: the pager's live
  // region latches its announcement on the first render at the new page, so a
  // label taken from the outgoing rows is what a screen reader hears, and it is
  // never corrected.
  const rowsAnswerCurrentRequest = !pastQuery.isPlaceholderData

  // The envelope's own count, already scoped to whatever year was requested —
  // the only count there is until the histogram resolves.
  const envelopeTotal = pastData?.total ?? 0

  const scopedTotal = histogramTotal ?? envelopeTotal
  const totalPages = Math.max(1, Math.ceil(scopedTotal / pageParams.limit))

  const basePath = `/venues/${venueSlug}`
  const buildHref = useCallback(
    (year: number | null, targetPage: number) => {
      const params = new URLSearchParams()
      if (year !== null) params.set('year', String(year))
      // Page 1 and "all years" are bare URLs: one canonical address per view,
      // so a shared link and the link the pager builds are the same string.
      if (targetPage > 1) params.set('page', String(targetPage))
      const query = params.toString()
      return `${basePath}${query ? `?${query}` : ''}#${VENUE_PAST_SHOWS_ANCHOR}`
    },
    [basePath]
  )

  // Month-range page labels: what is behind a page number, before the reader
  // spends a click on it (the Gazelle `451-500` label, on the time axis).
  //
  // A span can only be computed from a page's own rows, so a page can only be
  // labelled once it has been fetched: the current page from `rows`, its
  // neighbours from whatever the query cache still holds. Pages the reader has
  // visited keep their label; the rest render as bare numerals, which is what
  // `Pagination` does with a missing entry. Labelling every page on first paint
  // would mean either a per-month histogram the API does not serve, or
  // prefetching six more 50-row pages on every venue load — too much for a
  // label. Bounded at seven lookups, because that is all the pager can render.
  //
  // A WRONG label is worse than a missing one (the pager announces it and never
  // corrects), so the current page contributes nothing until its own rows land.
  const queryClient = useQueryClient()
  const rangeLabels = useMemo(() => {
    const labels: Record<number, string> = {}
    for (const item of paginationWindow(page, totalPages)) {
      if (item === 'ellipsis') continue
      let pageRows: VenueShow[] = []
      if (item === page) {
        pageRows = rowsAnswerCurrentRequest ? rows : []
      } else {
        pageRows =
          queryClient.getQueryData<VenueShowsResponse>(
            venueQueryKeys.showsPage(
              venueId,
              venuePastShowsPageParams(item, activeYear)
            )
          )?.shows ?? []
      }
      const label = monthRangeLabel(pageRows, zone)
      if (label) labels[item] = label
    }
    return labels
  }, [
    queryClient,
    venueId,
    page,
    totalPages,
    activeYear,
    zone,
    rows,
    rowsAnswerCurrentRequest,
  ])

  // A venue with no past shows carries no archive. Asked of the histogram, not
  // of the current page: a hand-typed year with nothing in it must still render
  // the section that says so, with the strip that leads back out of it.
  //
  // Resolved BEFORE the effects below rather than at the early return, because
  // both of them write to something outside this component (the document title,
  // the scroll position) and neither may do so on behalf of a section that is
  // not on the page.
  const hasPastShows = yearsQuery.isSuccess ? haveHistogram : envelopeTotal > 0

  // Reflect the active scope in the document title. The venue route is ISR and
  // reads no `searchParams` on the server, so this is the only place the year
  // and page can reach the title. The brand suffix is carried over from what
  // the route's own metadata rendered rather than restated, so it cannot drift
  // from the root layout's title template.
  //
  // This is a SECOND writer of a global the framework already owns, so it only
  // ever touches what it put there:
  //  - nothing is written for the default view, whose title the route already
  //    renders correctly (and which is every venue page the reader opens);
  //  - the cleanup restores only while `document.title` is still this effect's
  //    own string. On a soft navigation away, React commits the next route's
  //    hoisted <title> in the mutation phase and flushes this destroy function
  //    AFTER it, so an unconditional restore would relabel the page the reader
  //    just opened — and Next's route announcer reads `document.title` later
  //    still, so a screen reader would hear the old venue's name as the new
  //    page.
  const baseTitleRef = useRef<string | null>(null)
  const writtenTitleRef = useRef<string | null>(null)
  useEffect(() => {
    if (!hasPastShows) return
    if (baseTitleRef.current === null) baseTitleRef.current = document.title
    const baseTitle = baseTitleRef.current
    const scopedTitle = archiveDocumentTitle({
      baseTitle,
      venueName,
      year: activeYear,
      page,
      totalPages,
    })
    if (scopedTitle === baseTitle) return

    document.title = scopedTitle
    writtenTitleRef.current = scopedTitle
    return () => {
      if (document.title === writtenTitleRef.current) {
        document.title = baseTitle
      }
      writtenTitleRef.current = null
    }
  }, [hasPastShows, venueName, activeYear, page, totalPages])

  // Land a cold `#venue-past-shows` link on the archive.
  //
  // The browser resolves a fragment once, at load, and at that moment this
  // section does not exist: the venue route is prerendered without it, and the
  // archive only appears after its first client fetch. A shared or bookmarked
  // deep link would otherwise open at the top of the page, which is the one
  // place its `?year=`/`?page=` says nothing about.
  //
  // Fires ONCE per mount, and only for OUR fragment, so it can never fight a
  // reader who has already scrolled. The cost of that restraint is that it is
  // best-effort: anything ABOVE this section that settles later (on mobile the
  // sidebar renders first, map card included) shifts the archive down again
  // afterwards. Re-honouring the fragment on every subsequent layout change
  // would land it every time and hijack the scroll of anyone who moved in the
  // meantime, which is the worse failure. Later page changes need none of
  // this: the fragment is already on the page, and the pager moves focus to
  // the heading.
  const sectionRef = useRef<HTMLElement>(null)
  const hasHonoredAnchor = useRef(false)
  const archiveSettled = !pastQuery.isPending
  useEffect(() => {
    // The one shot is spent only when the scroll can actually happen. The two
    // queries settle in either order, so on a deep link into an empty year the
    // page request can land first, leaving the section unrendered and this ref
    // null — burning the flag there would strand the reader at the top of the
    // page once the histogram brought the section back.
    const section = sectionRef.current
    if (hasHonoredAnchor.current || !archiveSettled || section === null) return
    hasHonoredAnchor.current = true
    if (window.location.hash === `#${VENUE_PAST_SHOWS_ANCHOR}`) {
      section.scrollIntoView()
    }
  }, [archiveSettled, hasPastShows])

  if (!hasPastShows) return null

  const yearEntries: YearStripEntry[] = yearCounts.map(entry => ({
    year: entry.year,
    count: entry.count,
    href: buildHref(entry.year, 1),
  }))

  // Dim only while the rows on screen answer a DIFFERENT question than the one
  // being awaited — `keepPreviousData` holding the previous page or year in
  // place. Raw `isFetching` would also dim a same-key background revalidation,
  // fading a list that is not changing (the ShowList.tsx form).
  const isUpdating = pastQuery.isFetching && pastQuery.isPlaceholderData

  const countLabel =
    activeYear !== null && haveHistogram
      ? `${formatCount(scopedTotal)} of ${formatCount(allTimeTotal)} all-time`
      : formatCount(scopedTotal)

  const renderPager = (position: 'top' | 'bottom') => (
    <Pagination
      currentPage={page}
      totalPages={totalPages}
      pageHref={targetPage => buildHref(activeYear, targetPage)}
      ariaLabel={`Past shows pagination, ${position} of list`}
      rangeLabels={rangeLabels}
      // Omitted while the rows on screen belong to the previous page or year:
      // the caption states an exact range, and "Showing 51-100" over rows 1-50
      // is a wrong number, not a stale one. The pager falls back to
      // "Page 2 of 4", which stays true throughout.
      captionRange={
        rowsAnswerCurrentRequest && rows.length > 0
          ? { start: offset + 1, end: offset + rows.length, total: scopedTotal }
          : undefined
      }
      onNavigate={focusTarget}
    />
  )

  return (
    <section
      ref={sectionRef}
      id={VENUE_PAST_SHOWS_ANCHOR}
      className={cn('scroll-mt-20', className)}
    >
      <SectionHeader
        title={activeYear === null ? 'Past shows' : `Past shows in ${activeYear}`}
        as="h2"
        size="md"
        headingProps={targetProps}
        action={
          <span className="font-mono text-xs text-muted-foreground">
            {countLabel}
          </span>
        }
      />

      {yearEntries.length > 1 && (
        <YearStrip
          ariaLabel="Filter past shows by year"
          allYearsHref={buildHref(null, 1)}
          currentYear={activeYear}
          years={yearEntries}
          // Deep archives would otherwise turn the strip into a long
          // horizontal scroll on mobile; the tail stays in the DOM as real
          // links rather than being unmounted, so a reader who lands mid-strip
          // still has every year one tab away. (Crawler reach is a separate
          // question this section does not answer today: it renders only after
          // its first client fetch, so the archive is not in the server HTML.
          // Server-rendered year archives are their own ticket.)
          collapseAfter={8}
          onNavigate={focusTarget}
          className="mb-3"
        />
      )}

      <PastShowsBody
        activeYear={activeYear}
        buildHref={buildHref}
        isError={pastQuery.isError}
        isPastEnd={page > totalPages}
        isPending={pastQuery.isPending}
        isUpdating={isUpdating}
        onRetry={() => void pastQuery.refetch()}
        pagerBottom={renderPager('bottom')}
        pagerTop={renderPager('top')}
        rows={rows}
        zone={zone}
      />
    </section>
  )
}

/**
 * The part of the section below the year strip. Split out so the section above
 * reads as a single sequence of decisions (what scope, what counts, what URLs)
 * rather than interleaving them with five render branches.
 */
function PastShowsBody({
  activeYear,
  buildHref,
  isError,
  isPastEnd,
  isPending,
  isUpdating,
  onRetry,
  pagerBottom,
  pagerTop,
  rows,
  zone,
}: {
  activeYear: number | null
  buildHref: (year: number | null, page: number) => string
  isError: boolean
  /** The URL asks for a page beyond the last one this scope has. */
  isPastEnd: boolean
  isPending: boolean
  isUpdating: boolean
  onRetry: () => void
  pagerBottom: ReactNode
  pagerTop: ReactNode
  rows: VenueShowsResponse['shows']
  zone: VenueShowZone
}) {
  if (isError) {
    // A failed page must not take the navigation down with it. A venue with a
    // single year renders no year strip, so without these two controls the only
    // way out of a failed page 2 is hand-editing the URL — on the one surface
    // whose whole premise is that the archive is navigable.
    return (
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 py-3 text-sm">
        <span className="text-destructive">Failed to load past shows</span>
        <button
          type="button"
          onClick={onRetry}
          className="font-mono text-xs text-primary hover:underline"
        >
          [Try again]
        </button>
        <Link
          href={buildHref(activeYear, 1)}
          className="font-mono text-xs text-primary hover:underline"
        >
          Back to the first page
        </Link>
      </div>
    )
  }

  // Checked BEFORE the pending branch: when the histogram already knows the URL
  // is past the end, the page request is never issued, so the query sits in
  // `pending` forever and a spinner would be the terminal state. Say so and
  // offer the way back, rather than silently rewriting the URL the reader typed
  // or shared.
  if (isPastEnd) {
    return (
      <p className="py-3 text-sm text-muted-foreground">
        That page is past the end of this archive.{' '}
        <Link
          href={buildHref(activeYear, 1)}
          className="text-primary hover:underline"
        >
          Back to the first page
        </Link>
        .
      </p>
    )
  }

  if (isPending) {
    return (
      <div className="flex justify-center py-6">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (rows.length === 0) {
    // A real year with nothing at this venue is a legitimate view, reachable
    // by hand-editing the URL or by following a stale link. Say what is empty
    // and offer the way out instead of silently redirecting.
    return (
      <p className="py-3 text-sm text-muted-foreground">
        {activeYear === null ? (
          'No past shows.'
        ) : (
          <>
            No past shows in {activeYear}.{' '}
            <Link
              href={buildHref(null, 1)}
              className="text-primary hover:underline"
            >
              Show every year
            </Link>
            .
          </>
        )}
      </p>
    )
  }

  return (
    <div className={cn('space-y-3', isUpdating && 'opacity-60')}>
      {pagerTop}
      <VenueShowsTable
        shows={rows}
        zone={zone}
        ariaLabel="Past shows"
        groupByMonthHeadings
      />
      {pagerBottom}
    </div>
  )
}
