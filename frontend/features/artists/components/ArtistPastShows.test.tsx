import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { ArtistShow, ArtistShowsResponse } from '../types'

/**
 * The artist archive's WIRING, not its behaviour.
 *
 * Since PSY-1842 the archive itself is one shared component
 * (`features/shows/components/PastShowsArchive`), and its behaviour — pager
 * placement, live-region ownership, label derivation, every degraded branch — is
 * locked once in that component's own suite. What only this file can see is
 * whether the ARTIST wrapper hands it the right things: the artist's three
 * endpoints, hrefs that carry the graph's `?center=` through, and a zone
 * resolver that reads each row on its OWN venue's calendar.
 *
 * The label assertion is the one this ticket exists for. Before PSY-1842 this
 * archive derived labels from fetched ROWS, so every page the reader had not
 * visited rendered a bare numeral while the venue twin labelled all of them.
 */

let queryPage = 1
const mockSetter = vi.fn()

vi.mock('nuqs', () => ({
  parseAsInteger: Object.assign({}, { withDefault: () => ({}) }),
  useQueryState: (key: string) =>
    key === 'page' ? [queryPage, mockSetter] : [null, mockSetter],
}))

const mockUseArtistShows = vi.fn()
const mockUseArtistShowYears = vi.fn()
const mockUseArtistShowMonths = vi.fn()
vi.mock('../hooks/useArtists', () => ({
  useArtistShows: (options: unknown) => mockUseArtistShows(options),
  useArtistShowYears: (options: unknown) => mockUseArtistShowYears(options),
  useArtistShowMonths: (options: unknown) => mockUseArtistShowMonths(options),
}))

// The table has its own suite; stub it to a row-count marker so this one is
// only about what the wrapper passes down.
vi.mock('./ArtistShowsTable', () => ({
  ArtistShowsTable: ({ shows }: { shows: ArtistShow[] }) => (
    <div data-testid="artist-shows-table">{shows.length} rows</div>
  ),
}))

// The barrel is stubbed to keep unrelated shared components out of this suite,
// but the pager IS what the wiring is observed through, so the real one (live
// region and all) is spliced back in from its own module.
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
      headingProps,
    }: {
      title: string
      headingProps?: Record<string, unknown>
    }) => <h2 {...headingProps}>{title}</h2>,
    YearStrip: () => <div data-testid="year-strip" />,
  }
})

import { ArtistPastShows } from './ArtistPastShows'

function makeShow(id: number): ArtistShow {
  return {
    id,
    slug: `show-${id}`,
    title: `Show ${id}`,
    event_date: '2025-06-14T02:00:00Z',
    price: null,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: false,
    venue: null,
    artists: [],
  }
}

function showsResponse(offset: number): ArtistShowsResponse {
  return {
    shows: Array.from({ length: 50 }, (_, index) => makeShow(offset + index + 1)),
    artist_id: 3,
    total: 161,
    limit: 50,
    offset,
    year: 0,
  }
}

/**
 * 161 shows across five months of 2025, newest first. Four pages of 50, and the
 * SAME fixture the venue twin uses — the two archives must label a given page
 * identically, so the numbers they are labelled from are identical too.
 */
const MONTHS = [
  { year: 2025, month: 9, count: 20 },
  { year: 2025, month: 8, count: 30 },
  { year: 2025, month: 7, count: 40 },
  { year: 2025, month: 6, count: 30 },
  { year: 2025, month: 5, count: 41 },
]

function archive() {
  return (
    <ArtistPastShows artistId={3} artistSlug="glass-harbor" artistName="Glass Harbor" />
  )
}

describe('ArtistPastShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryPage = 1
    mockUseArtistShowYears.mockReturnValue({
      data: { artist_id: 3, time_filter: 'past', years: [{ year: 2025, count: 161 }] },
      isSuccess: true,
      isError: false,
      isFetching: false,
      isPending: false,
    })
    mockUseArtistShowMonths.mockReturnValue({
      data: { artist_id: 3, time_filter: 'past', months: MONTHS },
      isSuccess: true,
      isError: false,
      isFetching: false,
      isPending: false,
    })
    mockUseArtistShows.mockImplementation((options: { offset?: number }) => ({
      data: showsResponse(options.offset ?? 0),
      isError: false,
      isFetching: false,
      isPending: false,
      isSuccess: true,
      isPlaceholderData: false,
      refetch: vi.fn(),
    }))
  })

  it('renders the archive with a pager above and below the table', () => {
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
    // The venue twin carries the identical assertion, at its own mount point.
    // Fixing one archive and not the other is exactly the asymmetry that let this
    // regress before there was one component.
    const { rerender } = renderWithProviders(archive())

    queryPage = 2
    rerender(archive())

    const spoken = screen
      .getAllByRole('status')
      .filter(region => region.textContent !== '')
    expect(spoken).toHaveLength(1)
    expect(spoken[0]).toHaveTextContent('Page 2 of 4')
  })

  it('leaves exactly one live region in the tree at all', () => {
    renderWithProviders(archive())
    expect(screen.getAllByRole('status')).toHaveLength(1)
  })

  it('labels every page link from the artist month histogram (PSY-1842)', () => {
    // THE parity this ticket buys. Before it, only page 1 carried a label here —
    // it was the only page whose rows had been fetched — while the venue archive
    // labelled all four on first paint.
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

  it('requests the month histogram under the same past filter as the rows', () => {
    // Counts taken under a different filter would describe a different set of
    // rows than the pager is paging, and the label walk would silently shift.
    renderWithProviders(archive())
    expect(mockUseArtistShowMonths).toHaveBeenCalledWith(
      expect.objectContaining({ artistId: 3, timeFilter: 'past', enabled: true })
    )
  })
})
