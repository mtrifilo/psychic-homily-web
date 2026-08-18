'use client'

import { useCallback, useEffect, useMemo, useRef, type ReactNode } from 'react'
import Link from 'next/link'
import { Loader2 } from 'lucide-react'
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
import {
  useVenueShowMonths,
  useVenueShowYears,
  useVenueShows,
} from '../hooks/useVenues'
import {
  venuePastShowsPageParams,
  VENUE_PAST_SHOWS_PAGE_LIMIT,
} from '../api'
// Both entity-agnostic and shared with the artist archive (PSY-1754); the year
// is no longer read from the URL here, so `parseArchiveYear` moved to the route
// that owns the `{year}` path segment (PSY-1756). `monthRangeLabelsByPage` needs
// no venue zone, deliberately — the histogram it reads was bucketed on the
// venue's own calendar server-side, so there is nothing left to resolve.
import {
  clampPage,
  monthRangeLabelsByPage,
} from '@/features/shows/showArchive'
import {
  archiveDocumentTitle,
  venueArchiveHref,
  VENUE_PAST_SHOWS_FRAGMENT,
} from '../showArchive'
import { VenueShowsTable } from './VenueShowsTable'
import type {
  VenueShow,
  VenueShowZone,
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
   * Page 1's rows and the year histogram, when the SERVER already fetched them
   * for this exact scope (the year-archive route does; the venue page does
   * not). Seeded into the query cache so the first render — server AND
   * client — has them, which is what puts the rows and the year links in the
   * served HTML.
   */
  initialShows?: VenueShowsResponse
  initialYears?: VenueShowYearsResponse
  className?: string
}

/**
 * The venue's past-show archive: a year strip, a paged table, and pagers above
 * and below it (PSY-1753).
 *
 * Mounted on TWO surfaces, and the difference between them is one prop
 * (PSY-1756): the venue page renders it with no `activeYear` (every year, paged)
 * and the year-archive route renders it scoped to a year, with the first page
 * and the histogram already fetched server-side. One component rather than two
 * so the archive cannot drift between the surface a reader browses and the
 * surface a crawler indexes.
 *
 * URL-driven and read-only about it. Every page and year is a real `<a href>`
 * built by `venueArchiveHref`, and navigation happens through Next's router when
 * one is clicked; nothing here WRITES the query string. That is deliberate —
 * calling a nuqs setter alongside a `<Link>` navigation puts two writers on the
 * same URL in one tick, which is the PSY-1388 failure. The nuqs hook below
 * reads the result; the hrefs decide it.
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
  activeYear = null,
  initialShows,
  initialYears,
  className,
}: VenuePastShowsProps) {
  const [rawPage] = useQueryState('page', parseAsInteger.withDefault(1))
  const page = clampPage(rawPage, MAX_PAGE)

  const pageParams = venuePastShowsPageParams(page, activeYear)
  const offset = pageParams.offset ?? 0

  const yearsQuery = useVenueShowYears({
    venueId,
    timeFilter: 'past',
    initialData: initialYears,
  })
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
    limit: pageParams.limit,
    offset,
    year: pageParams.year,
    enabled: !pageIsBeyondKnownEnd,
    // The old page stays on screen (dimmed) while the next one loads, so
    // paging does not collapse the section to a spinner and bounce the layout.
    keepPreviousPage: true,
    // ONLY on the page the server actually fetched. `initialData` attaches to
    // whatever key is current, so handing page 1's rows to a hook asking for
    // page 2 would seed page 2 with the wrong slice — and unlike a stale page
    // that would never correct itself, because it would look like a hit.
    initialData: page === 1 ? initialShows : undefined,
  })

  const { targetProps, focusTarget } = usePaginationFocusTarget<HTMLHeadingElement>()

  const zone: VenueShowZone = useMemo(
    () => ({ venueState, venueTimezone }),
    [venueState, venueTimezone]
  )

  // Memoized on the response rather than recomputed: this array feeds the
  // table's month grouping, and a fresh `[]` on every render would make it
  // recompute forever.
  const pastData = pastQuery.data
  const rows: VenueShow[] = useMemo(() => pastData?.shows ?? [], [pastData])

  // Whether `rows` answers the question the URL is currently asking.
  //
  // `keepPreviousData` deliberately holds the PREVIOUS page (or year) on screen
  // while the next one loads, and `isPlaceholderData` is exactly "these rows
  // belong to a different query". Anything derived from the rows — which is now
  // the caption range and nothing else — must be suppressed until it clears, or
  // the surface states a fact about a slice the reader is not looking at
  // ("Showing 51-100 of 161" over rows 1-50). Dimming says stale; it does not
  // make a wrong number right.
  //
  // The month-span labels used to need this too, and no longer do: they come
  // from a histogram keyed on the venue alone (PSY-1769), so they are not a
  // function of whichever page's rows happen to be on screen. That also removes
  // a sharper edge than the caption's — the pager's live region latches its
  // announcement on the first render at a new page, so a label taken from the
  // outgoing rows was what a screen reader heard, and it was never corrected.
  const rowsAnswerCurrentRequest = !pastQuery.isPlaceholderData

  // The envelope's own count, already scoped to whatever year was requested —
  // the only count there is until the histogram resolves.
  const envelopeTotal = pastData?.total ?? 0

  const scopedTotal = histogramTotal ?? envelopeTotal
  const totalPages = Math.max(1, Math.ceil(scopedTotal / pageParams.limit))

  // One definition of the archive's URL space, shared with the sitemap and the
  // route that serves the year pages — see `venueArchiveHref`.
  const buildHref = useCallback(
    (year: number | null, targetPage: number) =>
      venueArchiveHref(venueSlug, year, targetPage),
    [venueSlug]
  )

  // Month-range page labels: what is behind a page number, before the reader
  // spends a click on it (the Gazelle `451-500` label, on the time axis).
  //
  // Derived from the MONTH HISTOGRAM, not from rows (PSY-1769). A page's span is
  // a function of the row ordinals it covers, so cumulative counts place every
  // page at once — which is what puts a label under page 6 on first paint. The
  // shape this replaced could only label pages already in the query cache, so an
  // eight-page archive showed one label and seven bare numerals until the reader
  // had walked the whole thing.
  //
  // The histogram is one request per venue and carries no year filter, so the
  // slice below is the whole cost of switching years. Reading the pager's own
  // window means at most seven labels are formatted, which is all it can render.
  //
  // Not requested at all for an archive that fits on one page — which is also
  // every venue with no past shows, since `totalPages` floors at 1. That is
  // exactly when `Pagination` renders nothing, so the request would buy labels
  // for a control that is not on the page. It costs no waterfall: both routes
  // that mount this seed the year histogram server-side, so `totalPages` is
  // known on the first render.
  const monthsQuery = useVenueShowMonths({
    venueId,
    timeFilter: 'past',
    enabled: totalPages > 1,
  })
  const allMonthCounts = monthsQuery.data?.months
  const rangeLabels = useMemo(() => {
    const months = allMonthCounts ?? []
    return monthRangeLabelsByPage({
      // Newest first from the API, which is the order this archive pages in.
      months:
        activeYear === null
          ? months
          : months.filter(bucket => bucket.year === activeYear),
      pageSize: VENUE_PAST_SHOWS_PAGE_LIMIT,
      pages: paginationWindow(page, totalPages).filter(
        (item): item is number => item !== 'ellipsis'
      ),
    })
  }, [allMonthCounts, activeYear, page, totalPages])

  // A venue with no past shows carries no archive. Asked of the histogram, not
  // of the current page: a hand-typed year with nothing in it must still render
  // the section that says so, with the strip that leads back out of it.
  //
  // Resolved BEFORE the effects below rather than at the early return, because
  // both of them write to something outside this component (the document title,
  // the scroll position) and neither may do so on behalf of a section that is
  // not on the page.
  const hasPastShows = yearsQuery.isSuccess ? haveHistogram : envelopeTotal > 0

  // Reflect the active scope in the document title.
  //
  // What is left for this to do is the PAGE, and only the page. The year is in
  // the path, so both routes render a correct title server-side already
  // ("Venue" / "Venue shows in 2025") — but neither reads `searchParams`, by
  // design: a `?page=` that reached `generateMetadata` would make the year
  // archive's canonical vary per page, and every page canonicalizes to the year
  // root. So the page number can only reach the title here.
  //
  // `archiveDocumentTitle` rebuilds the whole string from `venueName` + the
  // active scope, so passing the path's year is correct on both surfaces and
  // cannot double it up: on the year route page 1 recomputes exactly the title
  // the server rendered, and the equality check below then writes nothing. The
  // brand suffix is carried over from what the route's own metadata rendered
  // rather than restated, so it cannot drift from the root layout's template.
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
  // The browser resolves a fragment once, at load, and on the VENUE page this
  // section does not exist at that moment: the route is prerendered without it,
  // and the archive only appears after its first client fetch. A shared or
  // bookmarked deep link would otherwise open at the top of the page, which is
  // the one place its `?page=` says nothing about. (The year-archive route
  // needs none of this — it carries no fragment, because there the archive is
  // the page.)
  //
  // Honoured ONCE per mount and only for OUR fragment, so it can never fight a
  // reader who arrived without one. Later page changes need none of this: the
  // fragment is already on the page, and the pager moves focus to the heading.
  //
  // A single scrollIntoView is not enough, which is what PSY-1769 fixes. This
  // section is near the bottom of a page whose height is still being decided
  // when the archive first renders: the upcoming-shows list directly above it is
  // a client fetch that expands from a spinner to as many as 200 rows, and on
  // mobile the sidebar stacks ABOVE the main column, so its chart badge, genre
  // profile and collections cards all push the archive further down as they
  // arrive. Landing on it once and never again means landing on it before most
  // of that has happened. Reserving space for those is not an option — their
  // heights are genuinely unknown until they resolve, and a reserved gap would
  // be the same guess with a hole in the page.
  //
  // So: re-align while the page above is still moving, and STOP the instant the
  // reader takes over. The abandon signals are what makes that safe — a
  // re-alignment after someone has started reading is the worse failure, and is
  // why the original took the one-shot restraint. Programmatic scrolling fires
  // none of these events, so nothing here can trip itself.
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
    if (window.location.hash !== `#${VENUE_PAST_SHOWS_ANCHOR}`) return

    section.scrollIntoView()

    // Every way a reader can start moving the page themselves. Deliberately NOT
    // 'scroll', which our own scrollIntoView fires — listening for that would
    // abandon on the first realignment and leave this a one-shot again.
    const readerTookOver = ['wheel', 'touchstart', 'keydown', 'pointerdown'] as const

    // A ResizeObserver on the document rather than subscriptions to the several
    // queries that happen to settle late: this section can see none of them, and
    // enumerating them here would mean editing this effect every time something
    // new is added above it. What it actually needs to know is "did the page
    // above me get taller", and that is one signal.
    //
    // Coalesced through a single frame. `scrollIntoView` lands on the same place
    // however many times it runs, but it is not FREE to run: it forces layout,
    // and doing that from inside a ResizeObserver callback is what produces the
    // "loop completed with undelivered notifications" churn. Several boxes
    // settling in one burst should cost one scroll, not one each.
    let frame = 0
    const realign = () => {
      if (frame !== 0) return
      frame = requestAnimationFrame(() => {
        frame = 0
        section.scrollIntoView()
      })
    }
    const observer = new ResizeObserver(realign)
    observer.observe(document.body)

    let settleTimer = 0
    const abandon = () => {
      observer.disconnect()
      if (frame !== 0) cancelAnimationFrame(frame)
      window.clearTimeout(settleTimer)
      for (const event of readerTookOver) {
        window.removeEventListener(event, abandon)
      }
    }

    for (const event of readerTookOver) {
      window.addEventListener(event, abandon, { passive: true, once: true })
    }

    // A hard ceiling, so a page that never stops resizing (an animation, a lazy
    // image with no intrinsic size) cannot hold the reader's scroll hostage.
    // Long enough to cover the fetches above settling on a slow connection,
    // short enough that nobody is mid-sentence when it lapses.
    settleTimer = window.setTimeout(abandon, 3000)

    // Also the cleanup: a dep change or an unmount ends the window early, which
    // degrades to exactly the one-shot behaviour this replaced rather than
    // leaving a live observer behind.
    return abandon
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

      {/* Rendered for a SINGLE year too, which it was not before PSY-1756.
          A one-year venue's archive is still a document with its own URL, and
          the sitemap announces it — so suppressing the strip left that URL with
          no inbound link anywhere on the site, next to a venue page carrying
          the identical rows. An orphaned near-duplicate is the worst of both,
          and one link is what resolves it. */}
      {yearEntries.length > 0 && (
        <YearStrip
          ariaLabel="Filter past shows by year"
          allYearsHref={buildHref(null, 1)}
          currentYear={activeYear}
          years={yearEntries}
          // Deep archives would otherwise turn the strip into a long
          // horizontal scroll on mobile; the tail stays in the DOM as real
          // links rather than being unmounted, so a reader who lands mid-strip
          // still has every year one tab away — and so does a crawler, which
          // reads the markup and never opens a disclosure. On the year-archive
          // route the whole strip IS in the served HTML (the histogram is
          // seeded server-side), which is what makes every year of a venue's
          // history reachable by following links (PSY-1756).
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
