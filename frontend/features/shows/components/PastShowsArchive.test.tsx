import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import {
  archiveScope,
  archiveYearScope,
  type ArchiveMonthCount,
  type ArchiveYearCount,
  type ShowZone,
} from '../showArchive'
import { PastShowsArchive, type ArchiveListState } from './PastShowsArchive'

/**
 * THE behaviour lock for the past-show archive, shared by the venue page, the
 * venue year-archive route and the artist page (PSY-1842).
 *
 * It lives here rather than being written twice because it was written twice.
 * The double-announcement fix (PSY-1768) had to land in both archives, and the
 * histogram page labels (PSY-1769) landed in only one — the two failure modes
 * this file exists to make impossible. A test that only exercised one archive
 * would leave exactly the gap PSY-1842 closed.
 *
 * It mounts the component with FIXTURE PROPS rather than through either wrapper,
 * so it needs no hooks mocked and no query cache primed: the wrappers' job is
 * fetching, and their own suites cover the wiring.
 */

// The barrel is stubbed to keep unrelated shared components out of this suite,
// but the pager IS the thing under test, so the real one (live region and all)
// is spliced back in from its own module.
vi.mock('@/components/shared', async () => {
  const actual = await vi.importActual<
    typeof import('@/components/shared/Pagination')
  >('@/components/shared/Pagination')
  const chrome = await vi.importActual<
    typeof import('@/components/shared/paginationChrome')
  >('@/components/shared/paginationChrome')
  return {
    Pagination: actual.Pagination,
    paginationWindow: actual.paginationWindow,
    usePaginationFocusTarget: actual.usePaginationFocusTarget,
    formatCount: chrome.formatCount,
    SectionHeader: ({
      title,
      action,
      headingProps,
    }: {
      title: string
      action?: React.ReactNode
      headingProps?: Record<string, unknown>
    }) => (
      <>
        <h2 {...headingProps}>{title}</h2>
        <div data-testid="section-action">{action}</div>
      </>
    ),
    YearStrip: ({ years }: { years: unknown[] }) => (
      <div data-testid="year-strip">{years.length} years</div>
    ),
  }
})

const PAGE_SIZE = 50

/** Phoenix does not observe DST, so every fixture means exactly what it says. */
const PHOENIX: ShowZone = { state: 'AZ', timezone: 'America/Phoenix' }
const zoneOf = () => PHOENIX

interface Row {
  id: number
  event_date: string
}

/**
 * 161 shows across five months of 2025, newest first — the same total the two
 * wrappers' suites use, so "Page 2 of 4" means the same thing in all three.
 *
 * The month boundaries deliberately do NOT line up with the page boundaries:
 * page 1 straddles Sep/Aug, page 2 straddles Jul/Jun. A histogram walk that
 * dropped or double-counted a bucket would produce a well-formed but wrong span,
 * which is the failure mode worth pinning.
 */
const MONTHS: ArchiveMonthCount[] = [
  { year: 2025, month: 9, count: 20 },
  { year: 2025, month: 8, count: 30 },
  { year: 2025, month: 7, count: 40 },
  { year: 2025, month: 6, count: 30 },
  { year: 2025, month: 5, count: 41 },
]
const TOTAL = 161

const YEARS: ArchiveYearCount[] = [{ year: 2025, count: TOTAL }]

/** A page of rows, all in September 2025 Phoenix-local. */
function rowsFor(offset: number, count = PAGE_SIZE): Row[] {
  return Array.from({ length: count }, (_, index) => ({
    id: offset + index + 1,
    // 02:00 UTC on the 14th is 19:00 on the 13th in Phoenix.
    event_date: '2025-09-14T02:00:00Z',
  }))
}

function listState(overrides: Partial<ArchiveListState<Row>> = {}): ArchiveListState<Row> {
  return {
    rows: rowsFor(0),
    total: TOTAL,
    isPending: false,
    isError: false,
    isFetching: false,
    isPlaceholderData: false,
    isSuccess: true,
    refetch: vi.fn(),
    ...overrides,
  }
}

function archive({
  page = 1,
  activeYear = null,
  years = YEARS,
  yearsError = false,
  // `null` rather than `undefined` for "the histogram has not landed": a default
  // parameter cannot distinguish an explicit `undefined` from an omitted one, so
  // the absent case has to be spelled with a value the default will not swallow.
  months = MONTHS,
  // The year request has ANSWERED. False is what an errored histogram looks
  // like, and it is load-bearing: `hasPastShows` then falls back to the row
  // envelope, which is the only reason the stranded-year branch is reachable.
  yearsSettled = !yearsError,
  list = listState(),
  yearStripForSingleYear = true,
}: {
  page?: number
  activeYear?: number | null
  years?: ArchiveYearCount[]
  yearsError?: boolean
  months?: ArchiveMonthCount[] | null
  yearsSettled?: boolean
  list?: ArchiveListState<Row>
  yearStripForSingleYear?: boolean
} = {}) {
  // The REAL derivation, not a hand-built scope: `totalPages`, `hasPastShows`
  // and the past-the-end branch are all functions of it, and a fixture that
  // bypassed it would pin the component against a scope the wrappers never
  // produce.
  const yearScope = archiveYearScope({
    years,
    activeYear,
    page,
    pageSize: PAGE_SIZE,
  })
  const scope = archiveScope(yearScope, {
    yearsSettled,
    listSettled: list.isSuccess,
    listTotal: list.total,
    pageSize: PAGE_SIZE,
  })

  return (
    <PastShowsArchive
      anchorId="test-past-shows"
      entityName="Glass Harbor"
      activeYear={activeYear}
      page={page}
      pageSize={PAGE_SIZE}
      offset={(page - 1) * PAGE_SIZE}
      buildHref={(year, targetPage) =>
        `/test${year === null ? '' : `?year=${year}`}${targetPage > 1 ? `${year === null ? '?' : '&'}page=${targetPage}` : ''}`
      }
      scope={scope}
      years={{ counts: years, isError: yearsError }}
      months={months ?? undefined}
      list={list}
      zoneOf={zoneOf}
      renderTable={rows => <div data-testid="shows-table">{rows.length} rows</div>}
      yearStripForSingleYear={yearStripForSingleYear}
    />
  )
}

describe('PastShowsArchive pagers', () => {
  it('renders a pager above and below the table', () => {
    renderWithProviders(archive())
    expect(
      screen.getByRole('navigation', {
        name: 'Past shows pagination, top of list',
      })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('navigation', {
        name: 'Past shows pagination, bottom of list',
      })
    ).toBeInTheDocument()
  })

  it('announces a page change exactly once, not once per pager (PSY-1768)', () => {
    // Two pagers each shipping their own live region meant a screen reader spoke
    // "Page 2 of 4" twice on every click. Exactly one instance owns the
    // announcement, and it owns it HERE — the fix used to live in two components
    // and either copy could have been dropped without the other noticing.
    const { rerender } = renderWithProviders(archive())

    rerender(archive({ page: 2, list: listState({ rows: rowsFor(50) }) }))

    const spoken = screen
      .getAllByRole('status')
      .filter(region => region.textContent !== '')
    expect(spoken).toHaveLength(1)
    expect(spoken[0]).toHaveTextContent('Page 2 of 4')
  })

  it('leaves exactly one live region in the tree at all', () => {
    // The opted-out pager drops its region entirely rather than shipping a
    // permanently empty one.
    renderWithProviders(archive())
    expect(screen.getAllByRole('status')).toHaveLength(1)
  })

  it('keeps the page position visible in both pagers', () => {
    // Opting out silences the SPOKEN duplicate; the bottom pager must still show
    // the reader where they are.
    renderWithProviders(archive())
    // Two pagers, each rendering a desktop caption and a mobile position line.
    expect(screen.getAllByText(/Page 1 of 4/).length).toBeGreaterThanOrEqual(4)
  })
})

describe('PastShowsArchive page labels', () => {
  it('labels EVERY visible page from the histogram, including pages never fetched', () => {
    // The PSY-1769 defect, and the state the artist archive was still in until
    // PSY-1842: labels derived from rows could only cover pages already in the
    // query cache, so a four-page archive showed one label and three bare
    // numerals on first paint.
    renderWithProviders(archive())

    for (const [page, label] of [
      [1, 'Aug–Sep 2025'],
      [2, 'Jun–Jul 2025'],
      [3, 'May–Jun 2025'],
      [4, 'May 2025'],
    ] as const) {
      expect(
        screen.getAllByRole('link', { name: `Page ${page}, ${label}` }).length
      ).toBeGreaterThan(0)
    }
  })

  it('prints the earlier month first, whichever way the list runs', () => {
    // The list is newest-first, but a date range reads left-to-right in time
    // everywhere else software prints one. A reversed "Sep–Aug" scans as a
    // wrap-around rather than a page span (locked user call, PSY-1769).
    renderWithProviders(archive())
    expect(
      screen.queryByRole('link', { name: /Page 1, Sep–Aug/ })
    ).not.toBeInTheDocument()
  })

  it('falls back to the current page rows when the histogram is unavailable', () => {
    // A failed histogram fetch must not strip the label from the page the reader
    // is on: below `sm` the pager renders no page links at all, so that label is
    // the only one there is.
    renderWithProviders(archive({ months: null }))

    expect(
      screen.getAllByRole('link', { name: 'Page 1, Sep 2025' }).length
    ).toBeGreaterThan(0)
    // ...and the pages it cannot speak for stay bare numerals rather than
    // borrowing this page's months.
    expect(
      screen.getAllByRole('link', { name: 'Page 3' }).length
    ).toBeGreaterThan(0)
  })

  it('withholds the current page label while placeholder rows are on screen', () => {
    // `keepPreviousData` holds the OUTGOING page while the next loads, and this
    // is the exact render on which the pager latches its live-region
    // announcement and never corrects it. A label taken from the wrong page's
    // rows would be spoken once and stand.
    renderWithProviders(
      archive({
        page: 2,
        months: null,
        list: listState({ isPlaceholderData: true, isFetching: true }),
      })
    )

    expect(
      screen.getAllByRole('link', { name: 'Page 2' }).length
    ).toBeGreaterThan(0)
    expect(
      screen.queryByRole('link', { name: /Page 2, / })
    ).not.toBeInTheDocument()
  })

  it('omits the caption range while placeholder rows are on screen', () => {
    // "Showing 51-100 of 161" over rows 1-50 is a wrong number, not a stale one.
    // The pager falls back to "Page 2 of 4", which stays true throughout.
    renderWithProviders(
      archive({
        page: 2,
        list: listState({ isPlaceholderData: true, isFetching: true }),
      })
    )
    expect(screen.queryByText(/Showing/)).not.toBeInTheDocument()
    expect(screen.getAllByText(/Page 2 of 4/).length).toBeGreaterThan(0)
  })

  it('scopes labels to the active year and drops the year from them', () => {
    // On a year-scoped pager the year is in the URL, the heading and every month
    // row, so repeating it in seven page labels is noise. On the all-years
    // archive it is the only thing telling two "Jun–Aug" pages apart.
    renderWithProviders(archive({ activeYear: 2025 }))
    expect(
      screen.getAllByRole('link', { name: 'Page 1, Aug–Sep' }).length
    ).toBeGreaterThan(0)
  })
})

describe('PastShowsArchive navigation safety', () => {
  it('says so, with a way back, when the URL asks past the end', () => {
    // The row request is never issued for such a page, so the query sits pending
    // forever and a spinner would be the terminal state.
    renderWithProviders(archive({ page: 9, list: listState({ isPending: true }) }))
    expect(
      screen.getByText(/That page is past the end of this archive/)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Back to the first page' })
    ).toBeInTheDocument()
  })

  it('offers a way out of a year scope whose histogram failed', () => {
    // The year strip is the only way OUT of a year scope and it is built from
    // the histogram, which can fail on its own while the rows succeed. Without
    // this the reader gets correct rows, a correct heading, and no control that
    // clears the filter.
    renderWithProviders(
      archive({ activeYear: 2025, years: [], yearsError: true })
    )
    expect(screen.getByText('Could not load the other years.')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Show every year' })
    ).toBeInTheDocument()
  })

  it('keeps the pager and a first-page link when the row request fails', () => {
    // A failed page must not take the navigation down with it.
    renderWithProviders(archive({ page: 2, list: listState({ isError: true }) }))
    expect(screen.getByText('Failed to load past shows')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Back to the first page' })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '[Try again]' })).toBeInTheDocument()
  })

  it('renders nothing at all for an entity with no past shows', () => {
    const { container } = renderWithProviders(
      archive({ years: [], list: listState({ rows: [], total: 0 }) })
    )
    expect(container).toBeEmptyDOMElement()
  })
})

describe('PastShowsArchive year strip', () => {
  const ONE_YEAR: ArchiveYearCount[] = [{ year: 2025, count: TOTAL }]
  const TWO_YEARS: ArchiveYearCount[] = [
    { year: 2025, count: 100 },
    { year: 2024, count: 61 },
  ]

  it('renders a single-year strip where the year is its own document (venues)', () => {
    // A one-year venue's archive still has its own URL and the sitemap announces
    // it; suppressing the strip left that URL with no inbound link (PSY-1756).
    renderWithProviders(archive({ years: ONE_YEAR, yearStripForSingleYear: true }))
    expect(screen.getByTestId('year-strip')).toBeInTheDocument()
  })

  it('suppresses a single-year strip where there is no per-year route (artists)', () => {
    renderWithProviders(archive({ years: ONE_YEAR, yearStripForSingleYear: false }))
    expect(screen.queryByTestId('year-strip')).not.toBeInTheDocument()
  })

  it('renders a multi-year strip either way', () => {
    renderWithProviders(archive({ years: TWO_YEARS, yearStripForSingleYear: false }))
    expect(screen.getByTestId('year-strip')).toHaveTextContent('2 years')
  })
})
