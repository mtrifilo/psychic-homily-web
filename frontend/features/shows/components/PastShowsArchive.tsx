'use client'

/**
 * THE past-show archive. One implementation, mounted by the venue page, the
 * venue year-archive route and the artist page (PSY-1842).
 *
 * It exists because there were two of it. `VenuePastShows` and `ArtistPastShows`
 * were ~460-line near-twins, and the duplication had already cost twice over: the
 * dual-pager double-announcement bug (PSY-1768) had to be fixed in both, and the
 * month-histogram page labels (PSY-1769) shipped to only one, leaving the artist
 * archive rendering bare numerals under every page the reader had not visited.
 * The wrappers around this now own what is genuinely entity-shaped — which
 * endpoints to read, what the URLs look like, how a row resolves its timezone —
 * and nothing else.
 *
 * WHAT IT DOES NOT DO IS FETCH. The three reads it needs are entity-namespaced
 * (`venueQueryKeys` / `artistQueryKeys`, invalidated from mutation sites all over
 * the app), and they interleave with the derivations: the year histogram decides
 * whether the row request is worth making, and the row envelope decides whether
 * the month histogram is. Hooks-as-props would put that back together at the cost
 * of a construct the react-compiler cannot see through, so instead the wrapper
 * calls its own hooks and the two shared pure functions that gate them
 * (`archiveYearScope`, `archiveScope` in ../showArchive) — no logic is
 * duplicated, only the calls, and their results are passed down rather than
 * recomputed here.
 *
 * URL-DRIVEN AND READ-ONLY ABOUT IT. Every page and year is a real `<a href>`
 * built by the wrapper's `buildHref`, and navigation happens through Next's
 * router when one is clicked; nothing here WRITES the query string. That is
 * deliberate — calling a nuqs setter alongside a `<Link>` navigation puts two
 * writers on the same URL in one tick, which is the PSY-1388 failure.
 * {@link useArchivePage} reads the result; the hrefs decide it.
 *
 * The section renders nothing at all for an entity with no past shows, so a page
 * does not carry an empty archive.
 */

import { useEffect, useMemo, useRef, type ReactNode } from 'react'
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
  archiveDocumentTitle,
  clampPage,
  MAX_ARCHIVE_PAGE,
  monthRangeLabel,
  monthRangeLabelsByPage,
  type ArchiveLabelScope,
  type ArchiveMonthCount,
  type ArchiveRow,
  type ArchiveScope,
  type ArchiveYearCount,
  type ShowZoneResolver,
} from '../showArchive'

/**
 * The `?page=` the archive is on, clamped.
 *
 * A hook rather than a prop because the WRAPPER needs it too — it goes into the
 * row request's offset — so it has to be read before this component mounts.
 *
 * `parseAsInteger.withDefault(1)` is also what the venue year-archive route
 * resolves `?page=` with server-side, to decide whether seeding page 1's rows is
 * worth a request (PSY-1770). The two cannot share one parser CONSTANT — `nuqs`
 * and `nuqs/server` ship structurally separate type declarations of the same
 * runtime, and a server module may not import the client entry point — so the
 * agreement is pinned by test instead: see the equivalence battery in
 * features/shows/showArchive.test.ts. A disagreement about what `?page=2abc`
 * names would show up as the canonical page rendering unseeded.
 */
export function useArchivePage(): number {
  const [rawPage] = useQueryState('page', parseAsInteger.withDefault(1))
  return clampPage(rawPage, MAX_ARCHIVE_PAGE)
}

/**
 * Gestures that MOVE the document, as opposed to gestures that merely touch it.
 *
 * The distinction is the whole point. `touchstart` and `pointerdown` fire on
 * every tap — including the one a first-time mobile visitor makes on the cookie
 * banner, which is `position: fixed` and moves the archive by exactly zero
 * pixels. Treating contact as scroll intent would end the settle window before
 * the upcoming list and the sidebar cards have landed, which is the failure this
 * whole mechanism exists to prevent, on its single most common scenario.
 *
 * Deliberately NOT 'scroll' either: our own `scrollIntoView` fires one. A reader
 * scroll that these miss — a scrollbar drag, a trackpad gesture the UA reports
 * oddly, a screen reader moving its virtual caret — is caught by comparing the
 * archive's position against where we put it. See `onScroll`.
 */
const READER_SCROLL_GESTURE_EVENTS = ['wheel', 'touchmove'] as const

/**
 * Keys that scroll. A keystroke into the search box is not a claim on the
 * viewport; Page Down is.
 */
const READER_SCROLL_KEYS = new Set([
  ' ',
  'ArrowDown',
  'ArrowLeft',
  'ArrowRight',
  'ArrowUp',
  'End',
  'Home',
  'PageDown',
  'PageUp',
])

/**
 * Register `onReaderScroll` for every way a reader moves the page themselves.
 * Returns the unregister.
 */
function watchForReaderScroll(onReaderScroll: () => void): () => void {
  const onKeyDown = (event: KeyboardEvent) => {
    if (READER_SCROLL_KEYS.has(event.key)) onReaderScroll()
  }
  for (const event of READER_SCROLL_GESTURE_EVENTS) {
    window.addEventListener(event, onReaderScroll, { passive: true })
  }
  window.addEventListener('keydown', onKeyDown, { passive: true })
  return () => {
    for (const event of READER_SCROLL_GESTURE_EVENTS) {
      window.removeEventListener(event, onReaderScroll)
    }
    window.removeEventListener('keydown', onKeyDown)
  }
}

/**
 * How far the archive may drift from where we put it before we conclude somebody
 * else moved it. Small on purpose: this measures the SECTION's offset, which a
 * reader's scroll changes by a lot and rounding changes by a fraction of a
 * pixel. It is NOT sized to absorb scroll anchoring — see `onScroll` for why
 * that would be the wrong axis to measure in the first place.
 */
const READER_SCROLL_TOLERANCE_PX = 4

/**
 * How long the archive keeps re-aligning itself on a cold deep link before
 * giving up, absent any sign the reader has taken over.
 *
 * Long enough to outlast the requests that move the archive after it settles —
 * the upcoming list, the sidebar's cards, the artist page's connections graph —
 * on a slow connection, which is the whole point. It is safe to be this generous
 * because it is not what protects the reader: any scroll gesture, scroll key, or
 * scroll we did not perform ends the window immediately.
 *
 * Per EFFECT PASS, not per mount. That bound holds today because both of the
 * effect's deps are monotonic — `isPending` cannot return to true under
 * `keepPreviousData`, and `hasPastShows` only flips up — so the window opens at
 * most twice. A future dep that toggles would grant a fresh window each time; if
 * one is ever added, hold the deadline in a ref instead.
 */
const ANCHOR_SETTLE_CEILING_MS = 10_000

/** The row request's state, as this component needs to read it. */
export interface ArchiveListState<T extends ArchiveRow> {
  /**
   * The current page's rows. MEMOIZED BY THE CALLER on the response — this feeds
   * the table's month grouping and a label memo here, and a fresh `[]` on every
   * render would make both recompute forever.
   */
  rows: T[]
  /**
   * The count that arrived WITH the rows, already scoped to whatever year was
   * requested. Zero when there is no response.
   */
  total: number
  isPending: boolean
  isError: boolean
  isFetching: boolean
  /** The rows on screen belong to a DIFFERENT query (`keepPreviousData`). */
  isPlaceholderData: boolean
  refetch: () => void
}

/**
 * The shape both entities' row hooks already have, narrowed to what the archive
 * reads. Structural rather than `UseQueryResult<T>` so this module does not take
 * a react-query dependency for one adapter.
 */
export interface ArchiveListQuery {
  data: { total: number } | undefined
  isPending: boolean
  isError: boolean
  isFetching: boolean
  isPlaceholderData: boolean
  refetch: () => unknown
}

/**
 * A row query, reshaped into what {@link PastShowsArchive} reads.
 *
 * One function so the two wrappers cannot drift on it. Hand-writing the same
 * seven-field literal at each mount point is the shape PSY-1842 exists to
 * remove, one layer up: a field added to {@link ArchiveListState} would be a
 * two-file edit today and an N-file edit at the next entity, with no compiler
 * help finding the sites.
 *
 * `rows` is passed separately, and must be memoized by the caller: only the
 * caller knows the row TYPE, and only the caller can memoize on the response
 * object react-query hands back.
 */
export function archiveListState<T extends ArchiveRow>(
  query: ArchiveListQuery,
  rows: T[]
): ArchiveListState<T> {
  return {
    rows,
    total: query.data?.total ?? 0,
    isPending: query.isPending,
    isError: query.isError,
    isFetching: query.isFetching,
    isPlaceholderData: query.isPlaceholderData,
    refetch: () => void query.refetch(),
  }
}

export interface PastShowsArchiveProps<T extends ArchiveRow> {
  /**
   * The section's DOM id, and the fragment its deep links carry. Per entity
   * (`venue-past-shows`, `artist-past-shows`) because a reader can have both a
   * venue page and an artist page open, and because the fragment appears in
   * shared URLs that must not start resolving to a different section.
   */
  anchorId: string
  /** The venue or artist this archive belongs to, for the document title. */
  entityName: string
  /** The year this archive is scoped to, or null for every year. */
  activeYear: number | null
  /** The current page, from {@link useArchivePage}. */
  page: number
  /**
   * Rows per page, as the row request ACTUALLY asked for them — not the constant
   * behind it. The label walk maps row ordinals onto pages, so a page size that
   * diverged from the request would shift every label by the difference, which is
   * a WRONG label rather than a missing one.
   *
   * The row OFFSET is derived from this and `page` rather than passed: it is
   * exactly `(page - 1) * pageSize` at both call sites, and a third prop that
   * must agree with two others is a way for the caption to say "Showing 51-100"
   * under a pager that says page 1.
   */
  pageSize: number
  /** THE address of one view of this archive. Owns the entire URL shape. */
  buildHref: (year: number | null, page: number) => string
  /** The counts, from {@link archiveScope}. */
  scope: ArchiveScope
  /** The year histogram, for the strip. */
  years: {
    counts: ArchiveYearCount[]
    isError: boolean
  }
  /** The month histogram, for the page labels. Undefined until it lands. */
  months: ArchiveMonthCount[] | undefined
  /** The current page of rows. */
  list: ArchiveListState<T>
  /**
   * How a row resolves the calendar it is read on.
   *
   * The one genuinely entity-shaped thing about the ROWS, and the reason this is
   * a function: a venue archive lists one venue's shows so every row shares a
   * zone, an artist archive lists shows ACROSS venues so each row carries its
   * own. Only used for the current page's fallback label — the histogram's
   * labels need no zone at all, because the backend bucketed them venue-side.
   */
  zoneOf: ShowZoneResolver<T>
  /** The table. Handed exactly the rows to render, in the order to render them. */
  renderTable: (rows: T[]) => ReactNode
  /**
   * Whether each of this entity's years is a crawlable DOCUMENT of its own.
   *
   * Named for the FACT rather than for what it renders, because it is the same
   * fact `buildHref` already encodes (`/venues/{slug}/shows/{year}` against
   * `?year=`) and the render rule follows from it. True on venues: a one-year
   * venue's archive still has its own URL and the sitemap announces it, so
   * suppressing the strip left that URL with no inbound link anywhere on the
   * site, next to a venue page carrying the identical rows (PSY-1756). False on
   * artists, which have no per-year route — there a single-year strip is a
   * control whose only option is the view already on screen.
   */
  hasPerYearRoute: boolean
  className?: string
}

/**
 * The archive section: a year strip, a paged table, and a pager above and below
 * it. See the module comment at the top of this file for why it is one component
 * and what its wrappers still own.
 */
export function PastShowsArchive<T extends ArchiveRow>({
  anchorId,
  entityName,
  activeYear,
  page,
  pageSize,
  buildHref,
  scope,
  years,
  months,
  list,
  zoneOf,
  renderTable,
  hasPerYearRoute,
  className,
}: PastShowsArchiveProps<T>) {
  const { allTimeTotal, hasPastShows, haveHistogram, scopedTotal, totalPages } =
    scope
  const { rows, isPlaceholderData } = list
  const offset = (page - 1) * pageSize

  const { targetProps, focusTarget } =
    usePaginationFocusTarget<HTMLHeadingElement>()

  // Whether `rows` answers the question the URL is currently asking.
  //
  // `keepPreviousData` deliberately holds the PREVIOUS page (or year) on screen
  // while the next one loads, and `isPlaceholderData` is exactly "these rows
  // belong to a different query". Anything derived from the rows — which is now
  // the caption range and the fallback label — must be suppressed until it
  // clears, or the surface states a fact about a slice the reader is not looking
  // at ("Showing 51-100 of 161" over rows 1-50). Dimming says stale; it does not
  // make a wrong number right.
  //
  // It matters most for the LABEL: the pager's live region latches its
  // announcement on the first render at a new page, so a label taken from the
  // outgoing rows is what a screen reader hears, and it is never corrected.
  const rowsAnswerCurrentRequest = !isPlaceholderData

  // The year is only safe to leave out of a label when the pager is already
  // scoped to one — see `monthSpanLabel`.
  const labelScope: ArchiveLabelScope =
    activeYear === null ? 'all-years' : 'one-year'

  // Month-range page labels: what is behind a page number, before the reader
  // spends a click on it (the Gazelle `451-500` label, on the time axis).
  //
  // Derived from the MONTH HISTOGRAM, not from rows. A page's span is a function
  // of the row ordinals it covers, so cumulative counts place every page at once
  // — which is what puts a label under page 6 on first paint. The shape this
  // replaced could only label pages already in the query cache, so an eight-page
  // archive showed one label and seven bare numerals until the reader had walked
  // the whole thing.
  //
  // The histogram is one request per entity and carries no year filter, so the
  // slice below is the whole cost of switching years. Reading the pager's own
  // window means at most seven labels are formatted, which is all it can render.
  //
  // Ordinal-for-ordinal with the list only WITHIN ONE ZONE. Both lists sort on
  // the absolute instant and both histograms bucket venue-locally, so on an
  // artist archive — whose rows span venues — a page boundary inside the ~1-day
  // cross-zone band around a month can have that end of its span named as the
  // adjacent month. Bounded, deliberate, and spelled out in full at
  // `monthRangeLabelsByPage` (PSY-1842). Do not "fix" it here.
  const rangeLabels = useMemo(() => {
    const buckets = months ?? []
    const labels = monthRangeLabelsByPage({
      // Newest first from the API, which is the order this archive pages in.
      months:
        activeYear === null
          ? buckets
          : buckets.filter(bucket => bucket.year === activeYear),
      pageSize,
      pages: paginationWindow(page, totalPages).filter(
        (item): item is number => item !== 'ellipsis'
      ),
      // The count that arrived WITH the rows, and only while those rows answer
      // the current request. Deliberately not `scopedTotal`, which may be the
      // YEAR histogram's sum: two aggregates agreeing with each other says
      // nothing about the rows on screen, and they age in separate caches, so
      // comparing them would blank every label whenever one revalidated before
      // the other. Omitted rather than guessed while a placeholder page is up.
      listTotal: rowsAnswerCurrentRequest ? list.total : undefined,
      scope: labelScope,
    })

    // The CURRENT page is only ever labelled from rows we can attest to. While
    // `keepPreviousData` holds the outgoing page, the histogram's label for this
    // page went unverified — `listTotal` was omitted above, so the premise check
    // could not run — and this is the exact render on which `Pagination` latches
    // its live-region announcement and never corrects it. Drop it and let the
    // fallback below decline too; the label returns a beat later, verified.
    if (!rowsAnswerCurrentRequest) delete labels[page]

    // The CURRENT page, from the rows already on screen, whenever the histogram
    // could not label it — it failed, or has not landed yet.
    //
    // Without this a failed histogram fetch strips the label from every page
    // link at once, which is strictly worse than the row-derived shape this
    // replaced: that always labelled at least the page being read. It matters
    // most below `sm`, where the pager renders NO page links and the current
    // page's label is the only one there is.
    if (!labels[page] && rowsAnswerCurrentRequest && rows.length > 0) {
      const fromRows = monthRangeLabel(rows, zoneOf, labelScope)
      if (fromRows) labels[page] = fromRows
    }

    return labels
  }, [
    months,
    activeYear,
    pageSize,
    page,
    totalPages,
    list.total,
    labelScope,
    rows,
    rowsAnswerCurrentRequest,
    zoneOf,
  ])

  // Reflect the active scope in the document title.
  //
  // What is left for this to do is the PAGE, and only the page. The year is in
  // the path on the venue year-archive route and in `?year=` on the artist page,
  // and every route renders a correct year-scoped title server-side already —
  // but none of them reads `searchParams`, by design: a `?page=` that reached
  // `generateMetadata` would make a canonical vary per page, and every page
  // canonicalizes to the scope root. So the page number can only reach the title
  // here.
  //
  // `archiveDocumentTitle` rebuilds the whole string from `entityName` + the
  // active scope, so passing the active year is correct on every surface and
  // cannot double it up: on the venue year route page 1 recomputes exactly the
  // title the server rendered, and the equality check below then writes nothing.
  // The brand suffix is carried over from what the route's own metadata rendered
  // rather than restated, so it cannot drift from the root layout's template.
  //
  // This is a SECOND writer of a global the framework already owns, so it only
  // ever touches what it put there:
  //  - nothing is written for the default view, whose title the route already
  //    renders correctly (and which is every entity page the reader opens);
  //  - the cleanup restores only while `document.title` is still this effect's
  //    own string. On a soft navigation away, React commits the next route's
  //    hoisted <title> in the mutation phase and flushes this destroy function
  //    AFTER it, so an unconditional restore would relabel the page the reader
  //    just opened — and Next's route announcer reads `document.title` later
  //    still, so a screen reader would hear the old entity's name as the new
  //    page.
  const baseTitleRef = useRef<string | null>(null)
  const writtenTitleRef = useRef<string | null>(null)
  useEffect(() => {
    if (!hasPastShows) return
    if (baseTitleRef.current === null) baseTitleRef.current = document.title
    const baseTitle = baseTitleRef.current
    const scopedTitle = archiveDocumentTitle({
      baseTitle,
      entityName,
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
  }, [hasPastShows, entityName, activeYear, page, totalPages])

  // Land a cold `#…-past-shows` link on the archive.
  //
  // The browser resolves a fragment once, at load, and on an ENTITY page this
  // section does not exist at that moment: the route is prerendered without it,
  // and the archive only appears after its first client fetch. A shared or
  // bookmarked deep link would otherwise open at the top of the page, which is
  // the one place its `?page=` says nothing about. (The venue year-archive route
  // needs none of this — it carries no fragment, because there the archive is
  // the page, and the guard below is what makes that free.)
  //
  // Only ever for OUR fragment, so it cannot fight a reader who arrived without
  // one, and once ended it is never reopened. Later page changes need none of
  // this: the fragment is already on the page, and the pager moves focus to the
  // heading.
  //
  // A single scrollIntoView is not enough (PSY-1769). This section is near the
  // bottom of a page whose height is still being decided when the archive first
  // renders: the upcoming-shows list directly above it is a client fetch that
  // expands from a spinner to as many as 200 rows; on mobile the sidebar stacks
  // ABOVE the main column, so its chart badge, genre profile and collections
  // cards all push the archive further down as they arrive; and on the artist
  // page the connections graph does the same. Landing on it once and never again
  // means landing on it before most of that has happened. Reserving space for
  // those is not an option — their heights are genuinely unknown until they
  // resolve, and a reserved gap would be the same guess with a hole in the page.
  //
  // So: re-align while the page above is still moving, and STOP the instant the
  // reader takes over. The abandon signals are what makes that safe — a
  // re-alignment after someone has started reading is the worse failure, and is
  // why the original took the one-shot restraint. Programmatic scrolling fires
  // none of these events, so nothing here can trip itself.
  const sectionRef = useRef<HTMLElement>(null)
  /** The window ran its course, or the reader ended it. Never reopened. */
  const hasAbandonedAnchor = useRef(false)
  // "This section has reached the state it is going to render in", which is NOT
  // the same as "the rows arrived". A `?page=` past the end never issues the row
  // request at all (`pageIsBeyondKnownEnd` disables it), and a disabled query
  // with no data stays `pending` forever — so waiting on the rows alone would
  // leave the anchor permanently unfired for the one deep-link shape that cannot
  // recover on its own: a shared link to a page the archive has since shrunk
  // past. That reader would be dropped at the top of a long page with the
  // explanation ("That page is past the end…") off-screen at the bottom.
  const archiveSettled = !list.isPending || scope.pageIsBeyondKnownEnd

  // Has the reader moved the page themselves, at any point since this mounted?
  //
  // Tracked from MOUNT rather than from the moment the archive settles, because
  // the gap between the two is a real second or more on a slow connection, and a
  // reader who scrolls during it has just as much claim to the viewport. Without
  // this they would be yanked back the instant the query landed.
  const readerHasMoved = useRef(false)
  useEffect(() => {
    // Only where a fragment could ever be honoured. Every other mount of this
    // component — the whole venue year-archive route, whose URLs carry no
    // fragment by design — would otherwise keep three window listeners alive for
    // its entire lifetime to feed a ref nothing there reads.
    if (window.location.hash !== `#${anchorId}`) return
    return watchForReaderScroll(() => {
      readerHasMoved.current = true
    })
  }, [anchorId])

  useEffect(() => {
    // Nothing is spent until the scroll can actually happen. The two queries
    // settle in either order, so on a deep link into an empty year the page
    // request can land first, leaving the section unrendered and this ref
    // null — closing the window there would strand the reader at the top of the
    // page once the histogram brought the section back.
    const section = sectionRef.current
    if (!archiveSettled || section === null) return
    if (window.location.hash !== `#${anchorId}`) return
    if (hasAbandonedAnchor.current) return

    // THE FIRST ALIGNMENT IS UNCONDITIONAL. It is what the fragment has always
    // done, and honouring a fragment on a cold load is not something a reader
    // opts out of by having touched the screen. Only the RE-ALIGNMENT window
    // below is withheld from someone who has already started moving the page —
    // gating the initial scroll on that too would mean a tap on the cookie
    // banner, or any keypress during the load, silently dropped the reader at
    // the top of the page, which is a regression on the one behaviour this
    // section already had.
    //
    // The helpers below are function DECLARATIONS because they refer to each
    // other in a cycle (realign -> abandon -> close -> onScroll -> abandon).
    // Nothing invokes any of them until after all of them exist, so this is not
    // load-bearing today — it just means the order of the definitions is not
    // something a later edit can get wrong.

    /**
     * Where WE last put the archive: its top relative to the viewport right
     * after aligning. Deliberately the SECTION's position and not `window.
     * scrollY` — see `onScroll`.
     */
    let expectedTop = 0
    let frame = 0
    let ceilingTimer = 0
    // Constructed up front, OBSERVED later. `close` reads it and can run on a
    // path that never started observing, so having the object always exist is
    // simpler than guarding every use — and disconnecting one that never
    // observed anything is a no-op. (`realign` is a hoisted declaration, so
    // referencing it here is fine.)
    const observer = new ResizeObserver(realign)

    function alignNow() {
      section!.scrollIntoView()
      expectedTop = section!.getBoundingClientRect().top
    }

    // Coalesced through a single frame. `scrollIntoView` lands in the same place
    // however many times it runs, but it is not FREE to run: it forces layout,
    // and doing that from inside a ResizeObserver callback is what produces the
    // "loop completed with undelivered notifications" churn. Several boxes
    // settling in one burst should cost one scroll, not one each.
    function realign() {
      if (frame !== 0) return
      frame = requestAnimationFrame(() => {
        frame = 0
        alignNow()
      })
    }

    // `settled` distinguishes the two ways this can end. A window that ran its
    // course, or a reader who took over, is DONE — `hasAbandonedAnchor` stops a
    // later effect pass from reopening it. React's own teardown (a dep change,
    // StrictMode's double-invoke in dev) is NOT: it must leave the flag alone so
    // the next pass can pick the window back up, or the whole re-align window
    // would be inert in development and a maintainer would reproduce the very
    // bug this fixes while running the fix.
    function close({ settled }: { settled: boolean }) {
      if (settled) hasAbandonedAnchor.current = true
      observer.disconnect()
      if (frame !== 0) cancelAnimationFrame(frame)
      window.clearTimeout(ceilingTimer)
      window.removeEventListener('scroll', onScroll)
      unwatchReaderScroll()
    }

    function abandon() {
      close({ settled: true })
    }

    // The BACKSTOP, and the one that matters most for the readers least able to
    // fight a moving viewport. The four input events are what a mouse, finger or
    // keyboard produces directly — but a screen reader moving its virtual caret,
    // a VoiceOver swipe the AT consumes, and the browser's own scroll
    // restoration all move the page while producing NONE of them.
    //
    // IT COMPARES THE SECTION'S POSITION, NOT `window.scrollY`, and that is the
    // whole correctness of it. Chrome has CSS scroll anchoring on by default and
    // nothing here sets `overflow-anchor: none`, so when the upcoming list above
    // expands from a spinner to 200 rows the browser SHIFTS scrollY by the full
    // height of that growth to keep the anchored content still — thousands of
    // pixels, and it fires a `scroll` event. The scroll steps run BEFORE
    // ResizeObserver delivery in the same frame, so a scrollY comparison would
    // see that delta and abandon before `realign` ever ran: the window would
    // collapse back to the one-shot this replaced, silently, on the browser
    // family with the largest mobile share.
    //
    // The section's own viewport offset has neither problem. An anchoring
    // adjustment is precisely the browser holding it still, so the offset does
    // not move. Growth that anchoring does NOT compensate moves the section but
    // changes no scroll offset, so no scroll event fires and the ResizeObserver
    // handles it. A reader scrolling moves the section and fires the event —
    // which is the one case that should end the window.
    function onScroll() {
      const drift = Math.abs(
        section!.getBoundingClientRect().top - expectedTop
      )
      if (drift > READER_SCROLL_TOLERANCE_PX) abandon()
    }

    alignNow()

    // A reader who was already moving the page before the archive settled gets
    // no re-align window at all — but they still got the alignment above, which
    // is the fragment doing what it has always done.
    if (readerHasMoved.current) {
      hasAbandonedAnchor.current = true
      return
    }

    // Observing the DOCUMENT rather than subscribing to the several queries that
    // happen to settle late: this section can see none of them, and enumerating
    // them here would mean editing this effect every time something new is added
    // above it. What it actually needs to know is "did the page above me get
    // taller", and that is one signal.
    observer.observe(document.body)

    window.addEventListener('scroll', onScroll, { passive: true })
    const unwatchReaderScroll = watchForReaderScroll(abandon)

    // ONE fixed ceiling, and no idle timer. An idle timer was the obvious shape
    // and is the wrong one: the late movers here are separate REQUESTS — the
    // 200-row upcoming list, the sidebar's cards — so the page is naturally
    // still between them, and any idle window short enough to be polite expires
    // in an ordinary gap between two responses. That is the exact failure this
    // mechanism is about. What actually protects the reader is not a short
    // window but the abandon signals above, which end it the instant anyone
    // touches the page; the ceiling only bounds a page that never settles at all.
    ceilingTimer = window.setTimeout(abandon, ANCHOR_SETTLE_CEILING_MS)

    return () => close({ settled: false })
    // `hasPastShows` is read nowhere in this body ON PURPOSE: its false -> true
    // flip is the ONLY thing that re-runs this effect once the histogram brings
    // the section into existence, which is what makes the null-ref bail above
    // recoverable rather than terminal. Removing it as "unused" silently lands
    // every deep link at the top of the page.
  }, [anchorId, archiveSettled, hasPastShows])

  if (!hasPastShows) return null

  const yearEntries: YearStripEntry[] = years.counts.map(entry => ({
    year: entry.year,
    count: entry.count,
    href: buildHref(entry.year, 1),
  }))

  // Dim only while the rows on screen answer a DIFFERENT question than the one
  // being awaited — `keepPreviousData` holding the previous page or year in
  // place. Raw `isFetching` would also dim a same-key background revalidation,
  // fading a list that is not changing (the ShowList.tsx form).
  const isUpdating = list.isFetching && isPlaceholderData

  const countLabel =
    activeYear !== null && haveHistogram
      ? `${formatCount(scopedTotal)} of ${formatCount(allTimeTotal)} all-time`
      : formatCount(scopedTotal)

  // The year strip is the only way OUT of a year scope, and it is built from the
  // histogram — a separate request that can fail on its own while the page
  // request succeeds. Without this, a reader who opens a shared year-scoped link
  // during that failure gets correct rows, a correct heading, and no control
  // that clears the filter: the strip never renders (no years), and the "Show
  // every year" link lives in the zero-rows branch, which does not run because
  // there ARE rows. Same reasoning as the failed-page branch below, applied to
  // the request that branch does not cover.
  const yearFilterIsStranded =
    activeYear !== null && !haveHistogram && years.isError

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
      // ONE of the two instances owns the announcement, or a screen reader
      // hears "Page 2 of 4" twice on every click: two identical live regions
      // updating in the same commit (PSY-1768). The top pager keeps it: it is
      // beside the heading the pager moves focus to, and it is first in DOM
      // order, so its region is the one adjacent to where the reader lands.
      //
      // Living HERE rather than at two call sites is the point of this component:
      // PSY-1768 had to be fixed in both archives independently, and either fix
      // could have been dropped without the other noticing.
      announce={position === 'top'}
      onNavigate={focusTarget}
    />
  )

  return (
    <section
      ref={sectionRef}
      id={anchorId}
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

      {yearEntries.length > (hasPerYearRoute ? 0 : 1) && (
        <YearStrip
          ariaLabel="Filter past shows by year"
          allYearsHref={buildHref(null, 1)}
          currentYear={activeYear}
          years={yearEntries}
          // Deep archives would otherwise turn the strip into a long
          // horizontal scroll on mobile; the tail stays in the DOM as real
          // links rather than being unmounted, so a reader who lands mid-strip
          // still has every year one tab away — and so does a crawler, which
          // reads the markup and never opens a disclosure. On the venue
          // year-archive route the whole strip IS in the served HTML (the
          // histogram is seeded server-side), which is what makes every year of
          // a venue's history reachable by following links (PSY-1756).
          collapseAfter={8}
          onNavigate={focusTarget}
          className="mb-3"
        />
      )}

      <PastShowsBody
        activeYear={activeYear}
        buildHref={buildHref}
        isError={list.isError}
        isPastEnd={page > totalPages}
        isPending={list.isPending}
        isUpdating={isUpdating}
        onRetry={list.refetch}
        pagerBottom={renderPager('bottom')}
        pagerTop={renderPager('top')}
        rows={rows}
        renderTable={renderTable}
      />
    </section>
  )
}

/**
 * The part of the section below the year strip. Split out so the section above
 * reads as a single sequence of decisions (what scope, what counts, what URLs)
 * rather than interleaving them with five render branches.
 */
function PastShowsBody<T>({
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
  renderTable,
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
  rows: T[]
  renderTable: (rows: T[]) => ReactNode
}) {
  if (isError) {
    // A failed page must not take the navigation down with it. An entity with a
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
    // A real year with nothing in it is a legitimate view, reachable by
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
      {renderTable(rows)}
      {pagerBottom}
    </div>
  )
}
