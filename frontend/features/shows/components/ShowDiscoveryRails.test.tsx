import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import {
  makeAlsoTonightPayload,
  makeAlsoTonightShow,
  makeRailShow,
  makeRailVenue,
  makeVenueShow,
  makeVenueShowsResponse,
} from '../showRails.fixtures'
import { VENUE_RAIL_FETCH_LIMIT } from '../showRails'

// Both rails are fed by react-query hooks. Mocked at the hook boundary rather
// than mounting a QueryClientProvider and a fetch stub: what this component is
// responsible for is turning two payloads into two rails, and the hooks' own
// contracts (keys, endpoints, staleTime) are asserted where they live.
const useShowAlsoTonight = vi.fn()
const useVenueShows = vi.fn()

vi.mock('../hooks/useShows', () => ({
  useShowAlsoTonight: (...args: unknown[]) => useShowAlsoTonight(...args),
}))
vi.mock('@/features/venues/hooks/useVenues', () => ({
  useVenueShows: (...args: unknown[]) => useVenueShows(...args),
}))

import { ShowDiscoveryRails } from './ShowDiscoveryRails'

beforeEach(() => {
  vi.clearAllMocks()
  // Default: neither rail has anything. Each test opts into the payload it
  // needs, so a rail appearing in a test is always deliberate.
  useShowAlsoTonight.mockReturnValue({ data: undefined })
  useVenueShows.mockReturnValue({ data: undefined })
})

describe('ShowDiscoveryRails', () => {
  it('renders nothing at all when neither rail has rows', () => {
    // Not merely empty rails: the ROW must go, or its bottom margin opens a
    // gap above the footer on every quiet page.
    const { container } = render(<ShowDiscoveryRails show={makeRailShow()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('draws the also-tonight rail in the mock’s register', () => {
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    render(<ShowDiscoveryRails show={makeRailShow()} />)

    expect(
      screen.getByRole('heading', { name: 'Also / Tonight · Chicago' })
    ).toBeInTheDocument()
    const row = screen.getByRole('link', { name: /Dehd, Lifeguard/ })
    expect(row).toHaveAttribute('href', '/shows/dehd-lifeguard')
    // Venue-local: 01:00 UTC Aug 13 is 8PM Aug 12 in Chicago.
    expect(row).toHaveTextContent('8:00 PM')
    expect(row).toHaveTextContent('Empty Bottle')
    expect(row).toHaveTextContent('$15.00')
  })

  it('draws the venue rail date-first, with its status badge', () => {
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([makeVenueShow()]),
    })
    render(<ShowDiscoveryRails show={makeRailShow()} />)

    expect(
      screen.getByRole('heading', { name: 'More at / Salt Shed' })
    ).toBeInTheDocument()
    const row = screen.getByRole('link', { name: /Waxahatchee/ })
    expect(row).toHaveAttribute('href', '/shows/waxahatchee')
    expect(row).toHaveTextContent('AUG 15')
    expect(row).toHaveTextContent('SOLD OUT')
  })

  it('strikes through a cancelled bill and says so', () => {
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([
        makeVenueShow({ is_cancelled: true, is_sold_out: false }),
      ]),
    })
    render(<ShowDiscoveryRails show={makeRailShow()} />)
    const row = screen.getByRole('link', { name: /Waxahatchee/ })
    expect(row).toHaveTextContent('CANCELLED')
    expect(row.querySelector('.line-through')).not.toBeNull()
  })

  it('says CANCELLED rather than SOLD OUT when a show is both', () => {
    // A called-off show's ticket status is no longer the useful fact.
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([
        makeVenueShow({ is_cancelled: true, is_sold_out: true }),
      ]),
    })
    render(<ShowDiscoveryRails show={makeRailShow()} />)
    const row = screen.getByRole('link', { name: /Waxahatchee/ })
    expect(row).toHaveTextContent('CANCELLED')
    expect(row).not.toHaveTextContent('SOLD OUT')
  })

  it('puts also-tonight LEFT of more-at-venue, as the mock sets them', () => {
    // Which rail sits left is a design claim, and presence alone would pass
    // with the columns swapped.
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([makeVenueShow()]),
    })
    render(<ShowDiscoveryRails show={makeRailShow()} />)

    const alsoTonight = screen.getByTestId('also-tonight-rail')
    const moreAtVenue = screen.getByTestId('more-at-venue-rail')
    expect(
      alsoTonight.compareDocumentPosition(moreAtVenue) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it('names each rail as a landmark so the two can be told apart', () => {
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([makeVenueShow()]),
    })
    render(<ShowDiscoveryRails show={makeRailShow()} />)

    expect(
      screen.getByRole('region', { name: 'Also / Tonight · Chicago' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'More at / Salt Shed' })
    ).toBeInTheDocument()
  })

  it('badges a cancelled also-tonight row, not only a venue row', () => {
    useShowAlsoTonight.mockReturnValue({
      data: makeAlsoTonightPayload({
        shows: [makeAlsoTonightShow({ is_cancelled: true })],
      }),
    })
    render(<ShowDiscoveryRails show={makeRailShow()} />)
    expect(
      screen.getByRole('link', { name: /Dehd, Lifeguard/ })
    ).toHaveTextContent('CANCELLED')
  })

  it('reserves the lead column for a row with no usable instant', () => {
    // An undated row must not pull every bill beneath it out of line.
    useShowAlsoTonight.mockReturnValue({
      data: makeAlsoTonightPayload({
        shows: [makeAlsoTonightShow({ starts_at: 'not-a-date' })],
      }),
    })
    render(<ShowDiscoveryRails show={makeRailShow()} />)
    const row = screen.getByRole('link', { name: /Dehd, Lifeguard/ })
    expect(row.firstElementChild).toHaveClass('sm:w-16')
    expect(row.firstElementChild).toHaveTextContent('')
  })

  it('draws one rail without the other', () => {
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    render(<ShowDiscoveryRails show={makeRailShow()} />)
    expect(screen.getByTestId('also-tonight-rail')).toBeInTheDocument()
    expect(screen.queryByTestId('more-at-venue-rail')).not.toBeInTheDocument()
  })

  it('gives see-all an accessible name that survives leaving the heading', () => {
    useShowAlsoTonight.mockReturnValue({
      data: makeAlsoTonightPayload({ has_more: true }),
    })
    render(<ShowDiscoveryRails show={makeRailShow()} />)
    expect(
      screen.getByRole('link', { name: 'See all: Also, Tonight · Chicago' })
    ).toHaveAttribute('href', '/scenes/chicago-il/2026-08-12')
  })

  it('renders no bracket when the rail withheld its see-all', () => {
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    render(<ShowDiscoveryRails show={makeRailShow()} />)
    expect(screen.getByTestId('also-tonight-rail')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /^See all/ })).toBeNull()
  })

  it('addresses the also-tonight request by slug, preferring it to the id', () => {
    render(<ShowDiscoveryRails show={makeRailShow()} />)
    expect(useShowAlsoTonight).toHaveBeenCalledWith('modest-mouse-califone')
  })

  it('asks the venue for one row more than the rail can draw', () => {
    render(<ShowDiscoveryRails show={makeRailShow()} />)
    expect(useVenueShows).toHaveBeenCalledWith(
      expect.objectContaining({
        venueId: 10,
        timeFilter: 'upcoming',
        limit: VENUE_RAIL_FETCH_LIMIT,
        enabled: true,
      })
    )
  })

  it('does not ask for venue shows when the show has no venue', () => {
    render(<ShowDiscoveryRails show={makeRailShow({ venues: [] })} />)
    expect(useVenueShows).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false })
    )
    expect(screen.queryByTestId('more-at-venue-rail')).not.toBeInTheDocument()
  })

  it('keys rows by their target, so two rooms’ rows cannot collide', () => {
    // Regression guard for index keys: the two rails render into one grid and
    // a positional key would let a re-render reuse the wrong row's DOM.
    useShowAlsoTonight.mockReturnValue({
      data: makeAlsoTonightPayload({
        shows: [
          makeAlsoTonightShow({
            id: 1,
            slug: 'sen-morimoto',
            artist_names: ['Sen Morimoto'],
          }),
          makeAlsoTonightShow({
            id: 2,
            slug: 'hungry-brain',
            artist_names: ['Free-jazz Wednesdays'],
          }),
        ],
      }),
    })
    render(<ShowDiscoveryRails show={makeRailShow({ id: 999 })} />)
    expect(
      screen.getByRole('link', { name: /Sen Morimoto/ })
    ).toHaveAttribute('href', '/shows/sen-morimoto')
    expect(
      screen.getByRole('link', { name: /Free-jazz Wednesdays/ })
    ).toHaveAttribute('href', '/shows/hungry-brain')
  })

  it('never links a venue with no slug — an empty slug resolves to the index', () => {
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([makeVenueShow()], 9),
    })
    render(
      <ShowDiscoveryRails
        show={makeRailShow({ venues: [makeRailVenue({ slug: '' })] })}
      />
    )
    expect(screen.getByTestId('more-at-venue-rail')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /^See all/ })).toBeNull()
  })
})
