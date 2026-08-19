'use client'

import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'next/navigation'
import { parseAsInteger, useQueryState } from 'nuqs'
// Deep import, not the barrel: `@/features/shows`'s barrel edge drags in
// ShowForm and the whole mutation graph, and pulls an artists -> shows ->
// artists value cycle in behind it (the same reason ArtistShowsTable
// deep-imports ShowBill).
import {
  archiveListState,
  PastShowsArchive,
  useArchivePage,
} from '@/features/shows/components/PastShowsArchive'
import {
  archiveScope,
  archiveYearScope,
  parseArchiveYear,
} from '@/features/shows/showArchive'
import {
  useArtistShowMonths,
  useArtistShowYears,
  useArtistShows,
} from '../hooks/useArtists'
import { artistPastShowsPageParams } from '../api'
import {
  artistArchiveHref,
  artistShowZone,
  ARTIST_PAST_SHOWS_FRAGMENT,
} from '../showArchive'
import { ArtistShowsTable } from './ArtistShowsTable'
import type { ArtistShow } from '../types'

/**
 * Anchor the past-shows pager and year strip land on. Deliberately its own id
 * rather than one on the whole shows block, which would drop a reader who just
 * changed page at the top of the UPCOMING list.
 *
 * Re-exported from `showArchive`, which owns the artist archive's URL space and
 * has to append this fragment when it builds hrefs.
 */
export const ARTIST_PAST_SHOWS_ANCHOR = ARTIST_PAST_SHOWS_FRAGMENT

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
 * A THIN WRAPPER over `PastShowsArchive` since PSY-1842, which is also when this
 * surface caught up with the venue archive. Everything a reader can see — pager
 * placement, which pager owns the live region, how a page link gets its
 * month-range label, what an empty or past-the-end page says — lives there and is
 * shared with the venue archive, because it had been duplicated between the two
 * and the duplication had already leaked a bug in each direction: PSY-1768's
 * double-announcement fix had to be applied twice, and PSY-1769's histogram page
 * labels shipped only to venues, leaving THIS archive rendering bare numerals
 * under every page the reader had not visited.
 *
 * What is left here is what is genuinely artist-shaped: which endpoints to read,
 * that the year lives in `?year=` rather than a path segment, that hrefs must
 * carry the graph's params through, and that each row is dated on its OWN
 * venue's calendar.
 */
export function ArtistPastShows({
  artistId,
  artistSlug,
  artistName,
  className,
}: ArtistPastShowsProps) {
  // `?year=`, not a path segment, and deliberately so: the artist archive has no
  // crawlable per-year route yet, so there is no second address for this query
  // form to duplicate. See `venueArchiveHref` for the venue's contrasting shape.
  const [rawYear] = useQueryState('year', parseAsInteger)
  const activeYear = parseArchiveYear(rawYear)
  const page = useArchivePage()

  const pageParams = artistPastShowsPageParams(page, activeYear)
  const offset = pageParams.offset ?? 0
  // Read from the params the LIST actually requested, not from the constant
  // behind them: the label walk maps row ordinals onto pages, so a page size
  // that ever diverged from the request would shift every label by the
  // difference — a wrong label, which is worse than a missing one.
  const pageLimit = pageParams.limit

  const yearsQuery = useArtistShowYears({ artistId, timeFilter: 'past' })

  // The histogram is the authority on counts. It is keyed on the artist alone,
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

  const pastQuery = useArtistShows({
    artistId,
    timeFilter: 'past',
    limit: pageLimit,
    offset,
    year: pageParams.year,
    enabled: !yearScope.pageIsBeyondKnownEnd,
    // The old page stays on screen (dimmed) while the next one loads, so
    // paging does not collapse the section to a spinner and bounce the layout.
    keepPreviousPage: true,
  })

  // Memoized on the response rather than recomputed: this array feeds the
  // table's month grouping and the archive's label memo, and a fresh `[]` on
  // every render would make both recompute forever.
  const pastData = pastQuery.data
  const rows: ArtistShow[] = useMemo(() => pastData?.shows ?? [], [pastData])

  const scope = archiveScope(yearScope, {
    yearsSettled: yearsQuery.isSuccess,
    listSettled: pastQuery.isSuccess,
    listTotal: pastData?.total ?? 0,
    pageSize: pageLimit,
  })

  // What the pager labels its page links from (PSY-1842). Nothing seeds it
  // server-side: the artist route renders this archive after its first client
  // fetch, so there is no pager in its document to label.
  const monthsQuery = useArtistShowMonths({
    artistId,
    timeFilter: 'past',
    enabled: scope.monthsAreWorthFetching,
  })

  // The archive's URL space lives in `artistArchiveHref`, beside the venue
  // archive's `venueArchiveHref` — including the slugless-artist fallback and the
  // rule that params this section does not own are carried through untouched.
  // `useSearchParams` is read HERE because only a component may.
  const searchParams = useSearchParams()
  const buildHref = useCallback(
    (year: number | null, targetPage: number) =>
      artistArchiveHref({
        artistSlug,
        artistId,
        currentParams: searchParams,
        year,
        page: targetPage,
      }),
    [artistSlug, artistId, searchParams]
  )

  const renderTable = useCallback(
    (pageRows: ArtistShow[]) => (
      <ArtistShowsTable
        shows={pageRows}
        ariaLabel="Past shows"
        groupByMonthHeadings
      />
    ),
    []
  )

  return (
    <PastShowsArchive
      anchorId={ARTIST_PAST_SHOWS_ANCHOR}
      entityName={artistName}
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
      // Each row carries its own zone: an artist's shows span venues, so there
      // is no single calendar to read them all on. A module-level function, so
      // it is referentially stable without a memo.
      zoneOf={artistShowZone}
      renderTable={renderTable}
      // No `/artists/{slug}/shows/{year}` route exists, so nothing here is a
      // document needing an inbound link the way a venue year archive is
      // (PSY-1756) — and a single-year strip would be a control whose only
      // option is the view already on screen.
      hasPerYearRoute={false}
      className={className}
    />
  )
}
