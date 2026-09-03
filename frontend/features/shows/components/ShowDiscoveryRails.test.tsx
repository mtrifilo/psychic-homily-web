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

/**
 * The clock every render here is staged at: an hour BEFORE the fixtures' 8PM
 * Chicago doors, so no fixture row counts as started and the live-night
 * ordering leaves the payload's clock order alone. Tests that care about the
 * ordering stage their own instant.
 */
const RAILS_NOW = new Date('2026-08-13T00:00:00Z')

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
    const { container } = render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('draws the also-tonight rail in the mock’s register', () => {
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)

    expect(
      screen.getByRole('heading', { name: 'Also / Tonight · Chicago' })
    ).toBeInTheDocument()
    const row = screen.getByRole('link', { name: /Dehd \+ Lifeguard/ })
    expect(row).toHaveAttribute('href', '/shows/dehd-lifeguard')
    // Venue-local: 01:00 UTC Aug 13 is 8PM Aug 12 in Chicago.
    expect(row).toHaveTextContent('8PM')
    expect(row).toHaveTextContent('Empty Bottle')
    expect(row).toHaveTextContent('$15')
  })

  it('draws the venue rail date-first, status in the figure column', () => {
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([makeVenueShow()]),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)

    expect(
      screen.getByRole('heading', { name: 'More at / Salt Shed' })
    ).toBeInTheDocument()
    const row = screen.getByRole('link', { name: /Waxahatchee/ })
    expect(row).toHaveAttribute('href', '/shows/waxahatchee')
    expect(row).toHaveTextContent('AUG 15')
    // Uppercased by the column, not by the policy — `formatPrice` still says
    // `Free` for the whole site.
    expect(row.lastElementChild).toHaveTextContent('Sold out')
    expect(row.lastElementChild).toHaveClass('uppercase')
  })

  it('strikes through a cancelled bill and says so', () => {
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([
        makeVenueShow({ is_cancelled: true, is_sold_out: false }),
      ]),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    const row = screen.getByRole('link', { name: /Waxahatchee/ })
    expect(row.lastElementChild).toHaveTextContent('Cancelled')
    expect(row.querySelector('.line-through')).not.toBeNull()
  })

  it('says CANCELLED rather than SOLD OUT when a show is both', () => {
    // A called-off show's ticket status is no longer the useful fact.
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([
        makeVenueShow({ is_cancelled: true, is_sold_out: true }),
      ]),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    const row = screen.getByRole('link', { name: /Waxahatchee/ })
    expect(row).toHaveTextContent('Cancelled')
    expect(row).not.toHaveTextContent('Sold out')
  })

  it('puts also-tonight LEFT of more-at-venue, as the mock sets them', () => {
    // Which rail sits left is a design claim, and presence alone would pass
    // with the columns swapped.
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([makeVenueShow()]),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)

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
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)

    expect(
      screen.getByRole('region', { name: 'Also / Tonight · Chicago' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'More at / Salt Shed' })
    ).toBeInTheDocument()
  })

  it('states cancellation on an also-tonight row, not only a venue row', () => {
    useShowAlsoTonight.mockReturnValue({
      data: makeAlsoTonightPayload({
        shows: [makeAlsoTonightShow({ is_cancelled: true })],
      }),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    const row = screen.getByRole('link', { name: /Dehd \+ Lifeguard/ })
    expect(row.lastElementChild).toHaveTextContent('Cancelled')
    expect(row.querySelector('.line-through')).not.toBeNull()
  })

  it('reserves the lead column for a row with no usable instant', () => {
    // An undated row must not pull every bill beneath it out of line.
    useShowAlsoTonight.mockReturnValue({
      data: makeAlsoTonightPayload({
        shows: [makeAlsoTonightShow({ starts_at: 'not-a-date' })],
      }),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    const row = screen.getByRole('link', { name: /Dehd \+ Lifeguard/ })
    // Width class, not text: the point is that the cell still OCCUPIES its
    // column so the bills beneath it stay in line. `sm:w-14` is the TIME
    // reservation, which is what this rail leads with.
    expect(row.firstElementChild).toHaveClass('sm:w-14')
    expect(row.firstElementChild).toHaveTextContent('')
  })

  it('draws one rail without the other', () => {
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    expect(screen.getByTestId('also-tonight-rail')).toBeInTheDocument()
    expect(screen.queryByTestId('more-at-venue-rail')).not.toBeInTheDocument()
  })

  it('gives see-all an accessible name that survives leaving the heading', () => {
    useShowAlsoTonight.mockReturnValue({
      data: makeAlsoTonightPayload({ has_more: true }),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    expect(
      screen.getByRole('link', { name: 'See every show Tonight, Chicago' })
    ).toHaveAttribute('href', '/scenes/chicago-il/2026-08-12')
  })

  it('renders no bracket when the rail withheld its see-all', () => {
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    expect(screen.getByTestId('also-tonight-rail')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /^See all/ })).toBeNull()
  })

  it('addresses the also-tonight request by slug, preferring it to the id', () => {
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    expect(useShowAlsoTonight).toHaveBeenCalledWith(
      'modest-mouse-califone',
      true,
      undefined
    )
  })

  it('asks the venue for one row more than the rail can draw', () => {
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)
    expect(useVenueShows).toHaveBeenCalledWith(
      expect.objectContaining({
        venueId: 10,
        timeFilter: 'upcoming',
        limit: VENUE_RAIL_FETCH_LIMIT,
        enabled: true,
      })
    )
  })

  // The two rails answer different questions and only one survives the show.
  // Also-tonight is scoped to THIS show's night, so on a past page it would
  // offer shows the reader equally cannot attend under a heading naming a date
  // that has gone by. More-at-venue queries the venue's UPCOMING window, so it
  // is forward-looking whatever the subject show's date.
  it('withholds the also-tonight rail on a past show and keeps more-at-venue', () => {
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([makeVenueShow()], 9),
    })

    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="past" />)

    expect(screen.queryByTestId('also-tonight-rail')).not.toBeInTheDocument()
    expect(screen.getByTestId('more-at-venue-rail')).toBeInTheDocument()
  })

  // Not fetched either: a rail that cannot render has no reason to cost a
  // request on every past show page.
  it('does not ask for also-tonight on a past show', () => {
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="past" />)

    expect(useShowAlsoTonight).toHaveBeenCalledWith(
      expect.anything(),
      false,
      undefined
    )
  })

  it('does not ask for venue shows when the show has no venue', () => {
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow({ venues: [] })} lifecycle="upcoming" />)
    expect(useVenueShows).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false })
    )
    expect(screen.queryByTestId('more-at-venue-rail')).not.toBeInTheDocument()
  })

  it('addresses each row by its own show, not by its position', () => {
    // NOT a key-reconciliation guard, which this cannot be: each rail renders
    // its own <ul>, so rows from the two rails are never siblings and React
    // scopes keys per list. What this pins is simpler and is what actually
    // broke in review — that two rows of one rail resolve to two different
    // shows rather than both inheriting the first row's target.
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
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow({ id: 999 })} lifecycle="upcoming" />)
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
        now={RAILS_NOW}
        show={makeRailShow({ venues: [makeRailVenue({ slug: '' })] })}
        lifecycle="upcoming"
      />
    )
    expect(screen.getByTestId('more-at-venue-rail')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /^See all/ })).toBeNull()
  })

  it('draws the mock’s age column on the night rail', () => {
    useShowAlsoTonight.mockReturnValue({
      data: makeAlsoTonightPayload({
        shows: [makeAlsoTonightShow({ age_requirement: 'all ages' })],
      }),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)

    const row = screen.getByRole('link', { name: /Dehd \+ Lifeguard/ })
    // Uppercased by the column, like the figure beside it, so `all ages`
    // reaches the mock's register without a second vocabulary.
    const age = row.children[3]
    expect(age).toHaveTextContent('all ages')
    expect(age).toHaveClass('uppercase')
    // The column is narrow enough to clip a long value at `xl`, so the whole
    // of it rides on `title` — the recovery a mouse has, in the only band
    // where the clipping happens.
    expect(age).toHaveAttribute('title', 'all ages')
  })

  it('reserves the age column when a row states no policy at all', () => {
    // Reserved-but-empty, not dropped: a row that omits a cell shifts every
    // cell after it, which is what stops a ledger reading as columns.
    useShowAlsoTonight.mockReturnValue({
      data: makeAlsoTonightPayload({
        shows: [
          makeAlsoTonightShow({ age_requirement: '', venue_age_policy: '' }),
        ],
      }),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)

    const row = screen.getByRole('link', { name: /Dehd \+ Lifeguard/ })
    expect(row.children).toHaveLength(5)
    expect(row.children[3]).toHaveTextContent('')
  })

  it('gives the venue rail no age column', () => {
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([makeVenueShow()]),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)

    // Lead, bill, figure. No room cell (the heading names it) and no age cell.
    const row = screen.getByRole('link', { name: /Waxahatchee/ })
    expect(row.children).toHaveLength(3)
  })

  it('reserves a WIDER lead column for the venue rail’s dates', () => {
    // The compact time register buys the night rail 24px it does not have to
    // give the room rail, whose lead can be as long as `SEP 04 '27`.
    useShowAlsoTonight.mockReturnValue({ data: makeAlsoTonightPayload() })
    useVenueShows.mockReturnValue({
      data: makeVenueShowsResponse([makeVenueShow()]),
    })
    render(<ShowDiscoveryRails now={RAILS_NOW} show={makeRailShow()} lifecycle="upcoming" />)

    expect(
      screen.getByRole('link', { name: /Dehd \+ Lifeguard/ }).firstElementChild
    ).toHaveClass('sm:w-14')
    expect(
      screen.getByRole('link', { name: /Waxahatchee/ }).firstElementChild
    ).toHaveClass('sm:w-20')
  })

  it('hands each hook the rows the server already fetched', () => {
    // The seeds are passed to the HOOKS, which build their own keys from their
    // own arguments, so a seeded URL and the key it lands on cannot describe
    // different requests.
    const alsoTonight = makeAlsoTonightPayload()
    const venueShows = makeVenueShowsResponse([makeVenueShow()])
    render(
      <ShowDiscoveryRails
        now={RAILS_NOW}
        show={makeRailShow()}
        lifecycle="upcoming"
        initialAlsoTonight={alsoTonight}
        initialVenueShows={venueShows}
      />
    )

    expect(useShowAlsoTonight).toHaveBeenCalledWith(
      'modest-mouse-califone',
      true,
      alsoTonight
    )
    expect(useVenueShows).toHaveBeenCalledWith(
      expect.objectContaining({ initialData: venueShows })
    )
  })

  it('withholds a seed from a rail it will not draw', () => {
    // `initialData` populates a cache entry even on a DISABLED query, so a past
    // show would otherwise hold a rail it refuses to render, and a venue-less
    // show a rail with no room.
    render(
      <ShowDiscoveryRails
        now={RAILS_NOW}
        show={makeRailShow({ venues: [] })}
        lifecycle="past"
        initialAlsoTonight={makeAlsoTonightPayload()}
        initialVenueShows={makeVenueShowsResponse([makeVenueShow()])}
      />
    )

    expect(useShowAlsoTonight).toHaveBeenCalledWith(
      expect.anything(),
      false,
      undefined
    )
    expect(useVenueShows).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false, initialData: undefined })
    )
  })
})
