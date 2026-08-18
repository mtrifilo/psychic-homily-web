import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { ArtistShow, ArtistShowsResponse } from '../types'

let queryPage = 1
const mockSetter = vi.fn()

vi.mock('nuqs', () => ({
  parseAsInteger: Object.assign({}, { withDefault: () => ({}) }),
  useQueryState: (key: string) =>
    key === 'page' ? [queryPage, mockSetter] : [null, mockSetter],
}))

const mockUseArtistShows = vi.fn()
const mockUseArtistShowYears = vi.fn()
vi.mock('../hooks/useArtists', () => ({
  useArtistShows: (options: unknown) => mockUseArtistShows(options),
  useArtistShowYears: (options: unknown) => mockUseArtistShowYears(options),
}))

// The table has its own suite; stub it to a row-count marker so this one is
// only about the two pagers.
vi.mock('./ArtistShowsTable', () => ({
  ArtistShowsTable: ({ shows }: { shows: ArtistShow[] }) => (
    <div data-testid="artist-shows-table">{shows.length} rows</div>
  ),
}))

// The barrel is stubbed to keep unrelated shared components out of this suite,
// but the pager IS the thing under test, so the real one (live region and
// all) is spliced back in from its own module.
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

function archive() {
  return (
    <ArtistPastShows artistId={3} artistSlug="glass-harbor" artistName="Glass Harbor" />
  )
}

describe('ArtistPastShows pagers', () => {
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
    mockUseArtistShows.mockImplementation((options: { offset?: number }) => ({
      data: showsResponse(options.offset ?? 0),
      isError: false,
      isFetching: false,
      isPending: false,
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
    // The venue twin carries the identical assertion. Fixing one archive and
    // not the other is exactly the asymmetry that lets this regress.
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
})
