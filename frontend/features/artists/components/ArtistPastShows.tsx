'use client'

import { useCallback, useEffect, useMemo, useRef, type ReactNode } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
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
import {
  archiveDocumentTitle,
  clampPage,
  monthRangeLabel,
  parseArchiveYear,
} from '@/features/shows/showArchive'
import { cn } from '@/lib/utils'
import { useArtistShowYears, useArtistShows } from '../hooks/useArtists'
import { artistQueryKeys, artistPastShowsPageParams } from '../api'
import { artistShowZone } from '../showArchive'
import { ArtistShowsTable } from './ArtistShowsTable'
import type { ArtistShow, ArtistShowsResponse } from '../types'

/**
 * Anchor the past-shows pager and year strip land on. Deliberately its own id
 * rather than one on the whole shows block, which would drop a reader who just
 * changed page at the top of the UPCOMING list.
 */
export const ARTIST_PAST_SHOWS_ANCHOR = 'artist-past-shows'

/**
 * Upper bound on the page a URL may ask for, so a hand-edited `?page=` becomes
 * a bounded empty page instead of an unbounded offset the backend has to
 * reject. At 50 rows a page this covers 50,000 shows for one artist, orders of
 * magnitude past the most-played artist observed.
 */
const MAX_PAGE = 1_000

export interface ArtistPastShowsProps {
  artistId: number
  /**
   * Used to build page/year hrefs, so they are absolute and shareable. May be
   * empty — see `basePath` below, which is where that is handled.
   */
  artistSlug: string
  /** Used in the document title while a year or page is active. */
  artistName: string
  className?: string
}

/**
 * The artist's past-show archive: a year strip, a paged table, and pagers above
 * and below it (PSY-1754).
 *
 * The venue twin is `VenuePastShows`, and the two are deliberately the same
 * reading surface pointed at a different entity — same URL scheme, same
 * accessibility contract, same treatment of a stale or hand-edited address.
 * Everything that is genuinely entity-independent (month grouping, page labels,
 * year parsing, the document title) lives in `@/features/shows/showArchive`.
 *
 * URL-driven and read-only about it. Every page and year is a real `<a href>`
 * built by this component, and navigation happens through Next's router when
 * one is clicked; nothing here WRITES the query string. That is deliberate —
 * calling a nuqs setter alongside a `<Link>` navigation puts two writers on the
 * same URL in one tick, which is the PSY-1388 failure. The nuqs hooks below
 * read the result; the hrefs decide it.
 *
 * The section renders nothing at all for an artist with no past shows, so the
 * page does not carry an empty archive.
 */
export function ArtistPastShows({
  artistId,
  artistSlug,
  artistName,
  className,
}: ArtistPastShowsProps) {
  const [rawYear] = useQueryState('year', parseAsInteger)
  const [rawPage] = useQueryState('page', parseAsInteger.withDefault(1))
  const activeYear = parseArchiveYear(rawYear)
  const page = clampPage(rawPage, MAX_PAGE)

  const pageParams = artistPastShowsPageParams(page, activeYear)
  const offset = pageParams.offset ?? 0

  const yearsQuery = useArtistShowYears({ artistId, timeFilter: 'past' })
  const yearCounts = yearsQuery.data?.years ?? []

  // The histogram is the authority on counts. It is keyed on the artist alone,
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

  const pastQuery = useArtistShows({
    artistId,
    timeFilter: 'past',
    limit: pageParams.limit,
    offset,
    year: pageParams.year,
    enabled: !pageIsBeyondKnownEnd,
    // The old page stays on screen (dimmed) while the next one loads, so
    // paging does not collapse the section to a spinner and bounce the layout.
    keepPreviousPage: true,
  })

  const { targetProps, focusTarget } =
    usePaginationFocusTarget<HTMLHeadingElement>()

  // Memoized on the response rather than recomputed: this array is a dependency
  // of the page-label memo below and of the table's month grouping, and a fresh
  // `[]` on every render would make both recompute forever.
  const pastData = pastQuery.data
  const rows: ArtistShow[] = useMemo(() => pastData?.shows ?? [], [pastData])

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

  // Every href starts from the params already on the URL, not from an empty
  // set, and then overwrites the two this section owns.
  //
  // The venue archive can build from scratch because it is the only query-param
  // writer on its page. This one is not: the connections graph pushes
  // `?center=<slug>` onto the same URL and leaves it there when its dialog
  // closes, and IT already preserves `year`/`page`. A fresh `URLSearchParams`
  // here would make that courtesy one-way — paging the archive would silently
  // drop the reader's graph center, and a shared link would lose it too.
  const searchParams = useSearchParams()
  // Falls back to the id, which resolves on the same route.
  //
  // `slug` is nullable in the DB and the API sends "" for a missing one — and
  // `GenerateSlug` returns "" for any name with no [a-z0-9] characters at all,
  // so a band called `!!!` or `少年ナイフ` reaches this page slugless. An
  // unguarded `/artists/${''}` is `/artists/`, which is not a 404 but the
  // artists INDEX: every year link, every page link and both "back to the
  // first page" links would silently eject the reader from the archive this
  // ticket exists to make navigable. Resolved HERE rather than at the caller so
  // no future caller can forget it.
  const basePath = `/artists/${artistSlug || artistId}`
  const buildHref = useCallback(
    (year: number | null, targetPage: number) => {
      const params = new URLSearchParams(searchParams)
      // Page 1 and "all years" are bare URLs: one canonical address per view,
      // so a shared link and the link the pager builds are the same string.
      // That rule is about OUR two params — anything else on the URL belongs to
      // another owner and is carried through untouched.
      if (year === null) params.delete('year')
      else params.set('year', String(year))
      if (targetPage > 1) params.set('page', String(targetPage))
      else params.delete('page')
      const query = params.toString()
      return `${basePath}${query ? `?${query}` : ''}#${ARTIST_PAST_SHOWS_ANCHOR}`
    },
    [basePath, searchParams]
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
  // prefetching six more 50-row pages on every artist load — too much for a
  // label. Bounded at seven lookups, because that is all the pager can render.
  //
  // A WRONG label is worse than a missing one (the pager announces it and never
  // corrects), so the current page contributes nothing until its own rows land.
  const queryClient = useQueryClient()
  const rangeLabels = useMemo(() => {
    const labels: Record<number, string> = {}
    for (const item of paginationWindow(page, totalPages)) {
      if (item === 'ellipsis') continue
      let pageRows: ArtistShow[] = []
      if (item === page) {
        pageRows = rowsAnswerCurrentRequest ? rows : []
      } else {
        pageRows =
          queryClient.getQueryData<ArtistShowsResponse>(
            artistQueryKeys.showsPage(
              artistId,
              artistPastShowsPageParams(item, activeYear)
            )
          )?.shows ?? []
      }
      const label = monthRangeLabel(pageRows, artistShowZone)
      if (label) labels[item] = label
    }
    return labels
  }, [
    queryClient,
    artistId,
    page,
    totalPages,
    activeYear,
    rows,
    rowsAnswerCurrentRequest,
  ])

  // An artist with no past shows carries no archive. Asked of the histogram,
  // not of the current page: a hand-typed year with nothing in it must still
  // render the section that says so, with the strip that leads back out of it.
  //
  // Resolved BEFORE the effects below rather than at the early return, because
  // both of them write to something outside this component (the document title,
  // the scroll position) and neither may do so on behalf of a section that is
  // not on the page.
  const hasPastShows = yearsQuery.isSuccess ? haveHistogram : envelopeTotal > 0

  // Reflect the active scope in the document title. The artist route reads no
  // `searchParams` on the server, so this is the only place the year and page
  // can reach the title. The brand suffix is carried over from what the route's
  // own metadata rendered rather than restated, so it cannot drift from the
  // root layout's title template.
  //
  // This is a SECOND writer of a global the framework already owns, so it only
  // ever touches what it put there:
  //  - nothing is written for the default view, whose title the route already
  //    renders correctly (and which is every artist page the reader opens);
  //  - the cleanup restores only while `document.title` is still this effect's
  //    own string. On a soft navigation away, React commits the next route's
  //    hoisted <title> in the mutation phase and flushes this destroy function
  //    AFTER it, so an unconditional restore would relabel the page the reader
  //    just opened — and Next's route announcer reads `document.title` later
  //    still, so a screen reader would hear the old artist's name as the new
  //    page.
  const baseTitleRef = useRef<string | null>(null)
  const writtenTitleRef = useRef<string | null>(null)
  useEffect(() => {
    if (!hasPastShows) return
    if (baseTitleRef.current === null) baseTitleRef.current = document.title
    const baseTitle = baseTitleRef.current
    const scopedTitle = archiveDocumentTitle({
      baseTitle,
      entityName: artistName,
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
  }, [hasPastShows, artistName, activeYear, page, totalPages])

  // Land a cold `#artist-past-shows` link on the archive.
  //
  // The browser resolves a fragment once, at load, and at that moment this
  // section does not exist: the artist route is prerendered without it, and the
  // archive only appears after its first client fetch. A shared or bookmarked
  // deep link would otherwise open at the top of the page, which is the one
  // place its `?year=`/`?page=` says nothing about.
  //
  // Fires ONCE per mount, and only for OUR fragment, so it can never fight a
  // reader who has already scrolled. The cost of that restraint is that it is
  // best-effort: anything ABOVE this section that settles later shifts the
  // archive down again afterwards. Re-honouring the fragment on every
  // subsequent layout change would land it every time and hijack the scroll of
  // anyone who moved in the meantime, which is the worse failure. Later page
  // changes need none of this: the fragment is already on the page, and the
  // pager moves focus to the heading.
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
    // `hasPastShows` is in the dep array and read nowhere in this body ON
    // PURPOSE, and eslint does not flag the reverse: its false -> true flip is
    // the ONLY thing that re-runs this effect once the histogram brings the
    // section into existence, which is what makes the null-ref bail above
    // recoverable rather than terminal. Removing it as "unused" silently lands
    // every deep link at the top of the page.
    if (hasHonoredAnchor.current || !archiveSettled || section === null) return
    hasHonoredAnchor.current = true
    if (window.location.hash === `#${ARTIST_PAST_SHOWS_ANCHOR}`) {
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

  // The year strip is the only way OUT of a `?year=` scope, and it is built
  // from the histogram — a separate request that can fail on its own while the
  // page request succeeds. Without this, a reader who opens a shared
  // `?year=2025` link during that failure gets correct rows, a correct heading,
  // and no control that clears the filter: the strip never renders (no years),
  // and the "Show every year" link lives in the zero-rows branch, which does
  // not run because there ARE rows. Same reasoning as the failed-page branch
  // below, applied to the request that branch does not cover.
  const yearFilterIsStranded =
    activeYear !== null && !haveHistogram && yearsQuery.isError

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
      id={ARTIST_PAST_SHOWS_ANCHOR}
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

      {yearFilterIsStranded && (
        <p className="mb-3 flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm text-muted-foreground">
          <span>Could not load the other years.</span>
          <Link
            href={buildHref(null, 1)}
            className="font-mono text-xs text-primary hover:underline"
          >
            Show every year
          </Link>
        </p>
      )}

      {yearEntries.length > 1 && (
        <YearStrip
          ariaLabel="Filter past shows by year"
          allYearsHref={buildHref(null, 1)}
          currentYear={activeYear}
          years={yearEntries}
          // Deep archives would otherwise turn the strip into a long
          // horizontal scroll on mobile; the tail stays in the DOM as real
          // links rather than being unmounted, so a reader who lands mid-strip
          // still has every year one tab away.
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
  rows: ArtistShow[]
}) {
  if (isError) {
    // A failed page must not take the navigation down with it. An artist with a
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
    // A real year this artist did not play is a legitimate view, reachable by
    // hand-editing the URL or by following a stale link. Say what is empty and
    // offer the way out instead of silently redirecting.
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
      <ArtistShowsTable
        shows={rows}
        ariaLabel="Past shows"
        groupByMonthHeadings
      />
      {pagerBottom}
    </div>
  )
}
