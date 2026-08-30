import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ShowResponse, VenueResponse } from '../types'
import type { AlsoTonightShow, ShowAlsoTonightResponse } from '../showRails'
import type { VenueShow, VenueShowsResponse } from '@/features/venues/types'

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

function makeVenue(overrides: Partial<VenueResponse> = {}): VenueResponse {
  return {
    id: 10,
    slug: 'salt-shed',
    name: 'Salt Shed',
    city: 'Chicago',
    state: 'IL',
    timezone: 'America/Chicago',
    verified: true,
    ...overrides,
  }
}

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'modest-mouse-califone',
    title: 'Modest Mouse',
    event_date: '2026-08-13T01:00:00Z',
    status: 'approved',
    is_sold_out: false,
    is_cancelled: false,
    venues: [makeVenue()],
    artists: [],
    created_at: '2026-07-12T12:00:00Z',
    updated_at: '2026-07-12T12:00:00Z',
    ...overrides,
  }
}

function alsoTonightShow(
  overrides: Partial<AlsoTonightShow> = {}
): AlsoTonightShow {
  return {
    id: 100,
    title: 'Dehd',
    slug: 'dehd-lifeguard',
    event_date: '2026-08-12',
    // 20:00 in Chicago on Aug 12.
    starts_at: '2026-08-13T01:00:00Z',
    artist_names: ['Dehd', 'Lifeguard'],
    venue_name: 'Empty Bottle',
    venue_state: 'IL',
    venue_timezone: 'America/Chicago',
    price: 15,
    is_cancelled: false,
    is_sold_out: false,
    ...overrides,
  }
}

function alsoTonightPayload(
  overrides: Partial<ShowAlsoTonightResponse> = {}
): ShowAlsoTonightResponse {
  return {
    city: 'Chicago',
    state: 'IL',
    scene_name: 'Chicago, IL',
    scene_slug: 'chicago-il',
    date: '2026-08-12',
    timezone: 'America/Chicago',
    is_tonight: true,
    show_count: 1,
    has_more: false,
    shows: [alsoTonightShow()],
    ...overrides,
  }
}

function venueShow(overrides: Partial<VenueShow> = {}): VenueShow {
  return {
    id: 200,
    slug: 'waxahatchee',
    title: 'Waxahatchee',
    // 20:00 in Chicago on Aug 15.
    event_date: '2026-08-16T01:00:00Z',
    city: 'Chicago',
    state: 'IL',
    price: null,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: true,
    artists: [
      {
        id: 1,
        name: 'Waxahatchee',
        slug: 'waxahatchee',
      } as VenueShow['artists'][number],
    ],
    ...overrides,
  }
}

function venuePayload(
  shows: VenueShow[],
  total = shows.length
): VenueShowsResponse {
  return { shows, venue_id: 10, total, limit: 4, offset: 0, year: 0 }
}

/** No rail has anything to say. */
function setEmpty() {
  useShowAlsoTonight.mockReturnValue({ data: undefined })
  useVenueShows.mockReturnValue({ data: undefined })
}

beforeEach(() => {
  vi.clearAllMocks()
  setEmpty()
})

describe('ShowDiscoveryRails', () => {
  it('renders nothing at all when neither rail has rows', () => {
    // Not merely empty rails: the ROW must go, or its bottom margin opens a
    // gap above the footer on every quiet page.
    const { container } = render(<ShowDiscoveryRails show={makeShow()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('draws the also-tonight rail in the mock’s register', () => {
    useShowAlsoTonight.mockReturnValue({ data: alsoTonightPayload() })
    render(<ShowDiscoveryRails show={makeShow()} />)

    expect(
      screen.getByRole('heading', { name: 'Also / Tonight · Chicago' })
    ).toBeInTheDocument()
    const row = screen.getByRole('link', { name: /Dehd, Lifeguard/ })
    expect(row).toHaveAttribute('href', '/shows/dehd-lifeguard')
    expect(row).toHaveTextContent('Empty Bottle')
    expect(row).toHaveTextContent('$15.00')
  })

  it('sets the also-tonight time on the VENUE’s clock, not the reader’s', () => {
    // 01:00 UTC Aug 13 is 8PM Aug 12 in Chicago. A reader anywhere must read
    // 8:00 PM, because the heading above the row names the Chicago night.
    useShowAlsoTonight.mockReturnValue({ data: alsoTonightPayload() })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(
      screen.getByRole('link', { name: /Dehd, Lifeguard/ })
    ).toHaveTextContent('8:00 PM')
  })

  it('falls back to the scene’s clock for a row with no venue zone', () => {
    useShowAlsoTonight.mockReturnValue({
      data: alsoTonightPayload({
        shows: [
          alsoTonightShow({ venue_timezone: undefined, venue_state: undefined }),
        ],
      }),
    })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(
      screen.getByRole('link', { name: /Dehd, Lifeguard/ })
    ).toHaveTextContent('8:00 PM')
  })

  it('names the night by its date when the scene says it is not tonight', () => {
    useShowAlsoTonight.mockReturnValue({
      data: alsoTonightPayload({ is_tonight: false }),
    })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(
      screen.getByRole('heading', { name: 'Also / Wed Aug 12 · Chicago' })
    ).toBeInTheDocument()
  })

  it('never lists the show being read in its own also-tonight rail', () => {
    useShowAlsoTonight.mockReturnValue({
      data: alsoTonightPayload({
        shows: [alsoTonightShow({ id: 1, artist_names: ['Modest Mouse'] })],
      }),
    })
    render(<ShowDiscoveryRails show={makeShow({ id: 1 })} />)
    expect(screen.queryByTestId('also-tonight-rail')).not.toBeInTheDocument()
  })

  it('offers see-all only when the night holds more than the rail drew', () => {
    useShowAlsoTonight.mockReturnValue({ data: alsoTonightPayload() })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(screen.queryByRole('link', { name: /See every show/ })).toBeNull()
  })

  it('points see-all at the scene’s page for that night', () => {
    useShowAlsoTonight.mockReturnValue({
      data: alsoTonightPayload({ has_more: true }),
    })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(
      screen.getByRole('link', { name: /See every show in Chicago on 2026-08-12/ })
    ).toHaveAttribute('href', '/scenes/chicago-il/2026-08-12')
  })

  it('withholds see-all when the backend withheld the scene slug', () => {
    useShowAlsoTonight.mockReturnValue({
      data: alsoTonightPayload({ has_more: true, scene_slug: undefined }),
    })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(screen.getByTestId('also-tonight-rail')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /See every show/ })).toBeNull()
  })

  it('draws the venue rail date-first, with the status where the mock puts it', () => {
    useVenueShows.mockReturnValue({ data: venuePayload([venueShow()]) })
    render(<ShowDiscoveryRails show={makeShow()} />)

    expect(
      screen.getByRole('heading', { name: 'More at / Salt Shed' })
    ).toBeInTheDocument()
    const row = screen.getByRole('link', { name: /Waxahatchee/ })
    expect(row).toHaveAttribute('href', '/shows/waxahatchee')
    expect(row).toHaveTextContent('Aug 15')
    expect(row).toHaveTextContent('SOLD OUT')
  })

  it('excludes the show being read from its own venue rail', () => {
    useVenueShows.mockReturnValue({
      data: venuePayload([venueShow({ id: 1, slug: 'modest-mouse-califone' })]),
    })
    render(<ShowDiscoveryRails show={makeShow({ id: 1 })} />)
    expect(screen.queryByTestId('more-at-venue-rail')).not.toBeInTheDocument()
  })

  it('does not offer see-all when the venue’s only other shows are on screen', () => {
    // total 2 = the one row drawn plus the show being read.
    useVenueShows.mockReturnValue({ data: venuePayload([venueShow()], 2) })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(screen.queryByRole('link', { name: /See every upcoming/ })).toBeNull()
  })

  it('offers see-all to the venue page once rows are hidden', () => {
    useVenueShows.mockReturnValue({ data: venuePayload([venueShow()], 9) })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(
      screen.getByRole('link', { name: /See every upcoming show at Salt Shed/ })
    ).toHaveAttribute('href', '/venues/salt-shed')
  })

  it('never links a venue with no slug — an empty slug resolves to the index', () => {
    useVenueShows.mockReturnValue({ data: venuePayload([venueShow()], 9) })
    render(
      <ShowDiscoveryRails
        show={makeShow({ venues: [makeVenue({ slug: '' })] })}
      />
    )
    expect(screen.getByTestId('more-at-venue-rail')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /See every upcoming/ })).toBeNull()
  })

  it('addresses a row by id when the row carries no slug', () => {
    useVenueShows.mockReturnValue({ data: venuePayload([venueShow({ slug: '' })]) })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(screen.getByRole('link', { name: /Waxahatchee/ })).toHaveAttribute(
      'href',
      '/shows/200'
    )
  })

  it('does not ask for venue shows when the show has no venue', () => {
    render(<ShowDiscoveryRails show={makeShow({ venues: [] })} />)
    expect(useVenueShows).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false })
    )
  })

  it('draws one rail without the other', () => {
    useShowAlsoTonight.mockReturnValue({ data: alsoTonightPayload() })
    render(<ShowDiscoveryRails show={makeShow()} />)
    expect(screen.getByTestId('also-tonight-rail')).toBeInTheDocument()
    expect(screen.queryByTestId('more-at-venue-rail')).not.toBeInTheDocument()
  })
})
