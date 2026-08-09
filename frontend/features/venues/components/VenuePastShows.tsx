'use client'

import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { parseAsInteger, useQueryState } from 'nuqs'
import {
  Pagination,
  paginationWindow,
  SectionHeader,
  YearStrip,
  usePaginationFocusTarget,
  type YearStripEntry,
} from '@/components/shared'
import { formatCount } from '@/components/shared/paginationChrome'
import { cn } from '@/lib/utils'
import { useVenueShowYears, useVenueShows } from '../hooks/useVenues'
import { venueQueryKeys, venuePastShowsPageParams } from '../api'
import {
  archiveDocumentTitle,
  clampPage,
  monthRangeLabel,
  parseArchiveYear,
  type ArchiveZone,
} from '../showArchive'
import { VenueShowsTable } from './VenueShowsTable'
import type { VenueShowsResponse } from '../types'

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
  const pastQuery = useVenueShows({
    venueId,
    timeFilter: 'past',
    timezone: pageParams.timezone,
    limit: pageParams.limit,
    offset,
    year: pageParams.year,
    // The old page stays on screen (dimmed) while the next one loads, so
    // paging does not collapse the section to a spinner and bounce the layout.
    keepPreviousPage: true,
  })

  const { targetProps, focusTarget } = usePaginationFocusTarget<HTMLHeadingElement>()

  const zone: ArchiveZone = useMemo(
    () => ({ venueState, venueTimezone }),
    [venueState, venueTimezone]
  )

  const yearCounts = yearsQuery.data?.years ?? []
  // Memoized on the response rather than recomputed: this array is a dependency
  // of the page-label memo below, and a fresh `[]` on every render would make
  // that memo recompute (and its cache lookups re-run) forever.
  const pastData = pastQuery.data
  const rows = useMemo(() => pastData?.shows ?? [], [pastData])

  // The histogram is the authority on counts: it is keyed on the venue alone,
  // so switching years never leaves it briefly describing the previous year the
  // way the page envelope does under `keepPreviousData`. Until it resolves, the
  // envelope is the only count there is — and it is already year-scoped, so the
  // fallback is right in both the filtered and unfiltered cases.
  const allTimeTotal = yearCounts.reduce((sum, entry) => sum + entry.count, 0)
  const haveHistogram = yearCounts.length > 0
  const scopedTotal = haveHistogram
    ? activeYear === null
      ? allTimeTotal
      : (yearCounts.find(entry => entry.year === activeYear)?.count ?? 0)
    : (pastQuery.data?.total ?? 0)

  const totalPages = Math.max(1, Math.ceil(scopedTotal / pageParams.limit))

  const basePath = `/venues/${venueSlug}`
  const buildHref = useMemo(
    () => (year: number | null, targetPage: number) => {
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
  // A span can only be computed from the rows themselves, and only ONE page's
  // rows are ever in hand — so the current page is labelled from `rows`, and
  // its neighbours from whatever the query cache still holds for them. Pages
  // the reader has already visited keep their label; the rest render as bare
  // numerals, which is what `Pagination` does with a missing entry. Labelling
  // every page up front would need a per-month histogram the API does not
  // serve. Scoped to the pages the pager can actually render, so the cache
  // lookups are bounded at seven.
  const queryClient = useQueryClient()
  const rangeLabels = useMemo(() => {
    const labels: Record<number, string> = {}
    for (const item of paginationWindow(page, totalPages)) {
      if (item === 'ellipsis') continue
      const pageRows =
        item === page
          ? rows
          : (queryClient.getQueryData<VenueShowsResponse>(
              venueQueryKeys.showsPage(
                venueId,
                venuePastShowsPageParams(item, activeYear)
              )
            )?.shows ?? [])
      const label = monthRangeLabel(pageRows, zone)
      if (label) labels[item] = label
    }
    return labels
  }, [queryClient, venueId, page, totalPages, activeYear, zone, rows])

  // Reflect the active scope in the document title. The venue route is ISR and
  // reads no `searchParams` on the server, so this is the only place the year
  // and page can reach the title. The brand suffix is carried over from what
  // the route's own metadata rendered rather than restated, so it cannot drift
  // from the root layout's title template.
  const baseTitleRef = useRef<string | null>(null)
  useEffect(() => {
    if (baseTitleRef.current === null) baseTitleRef.current = document.title
    const baseTitle = baseTitleRef.current
    document.title = archiveDocumentTitle({
      baseTitle,
      venueName,
      year: activeYear,
      page,
      totalPages,
    })
    return () => {
      document.title = baseTitle
    }
  }, [venueName, activeYear, page, totalPages])

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
    if (hasHonoredAnchor.current || !archiveSettled) return
    hasHonoredAnchor.current = true
    if (window.location.hash === `#${VENUE_PAST_SHOWS_ANCHOR}`) {
      sectionRef.current?.scrollIntoView()
    }
  }, [archiveSettled])

  // A venue with no past shows carries no archive. Asked of the histogram, not
  // of the current page: a hand-typed year with nothing in it must still render
  // the section that says so, with the strip that leads back out of it.
  const hasPastShows = yearsQuery.isSuccess
    ? haveHistogram
    : (pastQuery.data?.total ?? 0) > 0 || rows.length > 0
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

  const pager = (position: 'top' | 'bottom') => (
    <Pagination
      currentPage={page}
      totalPages={totalPages}
      pageHref={targetPage => buildHref(activeYear, targetPage)}
      ariaLabel={`Past shows pagination, ${position} of list`}
      rangeLabels={rangeLabels}
      captionRange={
        rows.length > 0
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
          // links so crawlers still reach every year.
          collapseAfter={8}
          onNavigate={focusTarget}
          className="mb-3"
        />
      )}

      <PastShowsBody
        activeYear={activeYear}
        allYearsHref={buildHref(null, 1)}
        firstPageHref={buildHref(activeYear, 1)}
        isError={pastQuery.isError}
        isPending={pastQuery.isPending}
        isUpdating={isUpdating}
        page={page}
        pager={pager}
        rows={rows}
        totalPages={totalPages}
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
  allYearsHref,
  firstPageHref,
  isError,
  isPending,
  isUpdating,
  page,
  pager,
  rows,
  totalPages,
  zone,
}: {
  activeYear: number | null
  allYearsHref: string
  firstPageHref: string
  isError: boolean
  isPending: boolean
  isUpdating: boolean
  page: number
  pager: (position: 'top' | 'bottom') => ReactNode
  rows: VenueShowsResponse['shows']
  totalPages: number
  zone: ArchiveZone
}) {
  if (isError) {
    return (
      <p className="py-3 text-sm text-destructive">Failed to load past shows</p>
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
    // Past the end of the archive: say so and offer the way back, rather than
    // silently rewriting the URL the reader typed or shared.
    if (page > totalPages) {
      return (
        <p className="py-3 text-sm text-muted-foreground">
          That page is past the end of this archive.{' '}
          <Link href={firstPageHref} className="text-primary hover:underline">
            Back to the first page
          </Link>
          .
        </p>
      )
    }
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
            <Link href={allYearsHref} className="text-primary hover:underline">
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
      {pager('top')}
      <VenueShowsTable
        shows={rows}
        zone={zone}
        ariaLabel="Past shows"
        groupByMonthHeadings
      />
      {pager('bottom')}
    </div>
  )
}
