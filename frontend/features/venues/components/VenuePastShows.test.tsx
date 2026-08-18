import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { ARCHIVE_MONTHS, ARCHIVE_PAGE_LABELS } from '@/test/archiveFixtures'
import type { VenueShow, VenueShowsResponse } from '../types'

/**
 * The venue archive's WIRING, not its behaviour.
 *
 * Since PSY-1842 the archive itself is one shared component
 * (`features/shows/components/PastShowsArchive`), and its behaviour — pager
 * placement, live-region ownership, label derivation, every degraded branch — is
 * locked once in that component's own suite. What only this file can see is
 * whether the VENUE wrapper hands it the right things: the venue's three
 * endpoints, the venue's URL space, and a zone resolver that reads every row on
 * the venue's calendar.
 *
 * The announce-once assertion below is deliberately duplicated with the artist
 * twin and the shared suite. It is the regression PSY-1768 fixed twice, and
 * asserting it at the mount point is what proves the extraction did not quietly
 * drop `announce={false}` on the way through.
 */

let queryPage = 1
const mockSetPage = vi.fn()

vi.mock('nuqs', () => ({
  parseAsInteger: { withDefault: () => ({}) },
  useQueryState: () => [queryPage, mockSetPage],
}))

const mockUseVenueShows = vi.fn()
const mockUseVenueShowYears = vi.fn()
const mockUseVenueShowMonths = vi.fn()
vi.mock('../hooks/useVenues', () => ({
  useVenueShows: (options: unknown) => mockUseVenueShows(options),
  useVenueShowYears: (options: unknown) => mockUseVenueShowYears(options),
  useVenueShowMonths: (options: unknown) => mockUseVenueShowMonths(options),
}))

// The table has its own suite; stub it to a row-count marker so this one is
// only about what the wrapper passes down.
vi.mock('./VenueShowsTable', () => ({
  VenueShowsTable: ({ shows }: { shows: VenueShow[] }) => (
    <div data-testid="venue-shows-table">{shows.length} rows</div>
  ),
}))

vi.mock('@/components/shared', async () => {
  const kit = await import('@/test/archiveFixtures')
  return kit.archiveSharedBarrelMock()
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

function archive() {
  return (
    <VenuePastShows
      venueId={7}
      venueSlug="the-rebel-lounge"
      venueName="The Rebel Lounge"
      venueState="AZ"
    />
  )
}

describe('VenuePastShows', () => {
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
    mockUseVenueShowMonths.mockReturnValue({
      data: { venue_id: 7, time_filter: 'past', months: [...ARCHIVE_MONTHS] },
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
      isSuccess: true,
      isPlaceholderData: false,
      refetch: vi.fn(),
    }))
  })

  // The mount smoke test: everything below asserts something SPECIFIC, and each
  // of them would also fail if the wrapper rendered nothing at all — this one
  // says which failure it was. The pager's own contract (one live region, both
  // instances showing the position) is locked in the shared component's suite,
  // not re-asserted here.
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
    // The artist twin carries the identical assertion, at its own mount point.
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

  it('labels every page link from the venue month histogram (PSY-1769)', () => {
    // The wrapper has to request the histogram, hand it down, and hand down the
    // page size the LIST asked for — a mismatch in any of the three shows up as
    // missing or shifted labels rather than as an error.
    renderWithProviders(archive())
    for (const [page, label] of ARCHIVE_PAGE_LABELS) {
      expect(
        screen.getAllByRole('link', { name: `Page ${page}, ${label}` }).length
      ).toBeGreaterThan(0)
    }
  })

  it('requests the month histogram under the same past filter as the rows', () => {
    // Counts taken under a different filter would describe a different set of
    // rows than the pager is paging, and the label walk would silently shift.
    renderWithProviders(archive())
    expect(mockUseVenueShowMonths).toHaveBeenCalledWith(
      expect.objectContaining({ venueId: 7, timeFilter: 'past', enabled: true })
    )
  })
})
