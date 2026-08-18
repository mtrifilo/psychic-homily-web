import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { VenueShow, VenueShowsResponse } from '../types'

let queryPage = 1
const mockSetPage = vi.fn()

vi.mock('nuqs', () => ({
  parseAsInteger: { withDefault: () => ({}) },
  useQueryState: () => [queryPage, mockSetPage],
}))

const mockUseVenueShows = vi.fn()
const mockUseVenueShowYears = vi.fn()
vi.mock('../hooks/useVenues', () => ({
  useVenueShows: (options: unknown) => mockUseVenueShows(options),
  useVenueShowYears: (options: unknown) => mockUseVenueShowYears(options),
}))

// The table has its own suite; stub it to a row-count marker so this one is
// only about the two pagers.
vi.mock('./VenueShowsTable', () => ({
  VenueShowsTable: ({ shows }: { shows: VenueShow[] }) => (
    <div data-testid="venue-shows-table">{shows.length} rows</div>
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

import { VenuePastShows } from './VenuePastShows'

function makeShow(id: number): VenueShow {
  return {
    id,
    slug: `show-${id}`,
    title: `Show ${id}`,
    event_date: '2025-06-14T02:00:00Z',
    city: 'Phoenix',
    state: 'AZ',
    price: null,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: false,
    artists: [],
  }
}

function showsResponse(offset: number): VenueShowsResponse {
  return {
    shows: Array.from({ length: 50 }, (_, index) => makeShow(offset + index + 1)),
    venue_id: 7,
    total: 161,
    limit: 50,
    offset,
    year: 0,
  }
}

function renderArchive() {
  return renderWithProviders(
    <VenuePastShows
      venueId={7}
      venueSlug="the-rebel-lounge"
      venueName="The Rebel Lounge"
      venueState="AZ"
    />
  )
}

describe('VenuePastShows pagers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryPage = 1
    mockUseVenueShowYears.mockReturnValue({
      data: { venue_id: 7, time_filter: 'past', years: [{ year: 2025, count: 161 }] },
      isSuccess: true,
      isError: false,
      isFetching: false,
      isPending: false,
    })
    mockUseVenueShows.mockImplementation((options: { offset?: number }) => ({
      data: showsResponse(options.offset ?? 0),
      isError: false,
      isFetching: false,
      isPending: false,
      isPlaceholderData: false,
      refetch: vi.fn(),
    }))
  })

  it('renders the archive with a pager above and below the table', () => {
    renderArchive()
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
    // Two pagers each shipping their own live region meant a screen reader
    // spoke "Page 2 of 4" twice on every click. Exactly one instance owns the
    // announcement now.
    const { rerender } = renderArchive()

    queryPage = 2
    rerender(
      <VenuePastShows
        venueId={7}
        venueSlug="the-rebel-lounge"
        venueName="The Rebel Lounge"
        venueState="AZ"
      />
    )

    const spoken = screen
      .getAllByRole('status')
      .filter(region => region.textContent !== '')
    expect(spoken).toHaveLength(1)
    expect(spoken[0]).toHaveTextContent('Page 2 of 4')
  })

  it('leaves exactly one live region in the tree at all', () => {
    // The opted-out pager drops its region entirely rather than shipping a
    // permanently empty one.
    renderArchive()
    expect(screen.getAllByRole('status')).toHaveLength(1)
  })

  it('keeps the page position visible in both pagers', () => {
    // Opting out silences the SPOKEN duplicate; the bottom pager must still
    // show the reader where they are.
    renderArchive()
    const positions = screen.getAllByText(/Page 1 of 4/)
    // Two pagers, each rendering a desktop caption and a mobile position line.
    expect(positions.length).toBeGreaterThanOrEqual(4)
  })
})
