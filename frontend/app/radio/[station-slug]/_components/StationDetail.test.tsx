import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { STATION_PLAYLISTS_ANCHOR } from '@/features/radio/components/StationGraph'

// Control the station fetch; stub the heavy child components (they own their own
// data + canvas and aren't under test here).
const mockUseRadioStation = vi.fn()
vi.mock('@/features/radio', () => ({
  useRadioStation: () => mockUseRadioStation(),
  NetworkTabBar: () => <div data-testid="network-tab-bar" />,
  StationOnAirBox: () => <div data-testid="on-air" />,
  StationGraph: () => <div data-testid="station-graph" />,
  StationPlaylistsFeed: () => <div data-testid="station-playlists" />,
  StationShowsDirectory: () => <div data-testid="station-shows" />,
  StationSidebar: () => <div data-testid="station-sidebar" />,
  getBroadcastTypeLabel: () => 'Terrestrial',
}))

// Drive the url hash the cold-load scroll effect reads.
const mockHash = vi.fn(() => '')
vi.mock('@/lib/hooks/common/useUrlHash', () => ({
  useUrlHash: () => mockHash(),
  GRAPH_HASH: '#graph',
}))

import StationDetail from './StationDetail'

function makeStation(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    slug: 'kexp',
    name: 'KEXP',
    city: 'Seattle',
    state: 'WA',
    description: 'A radio station.',
    frequency_mhz: 90.3,
    broadcast_type: 'terrestrial',
    stream_url: null,
    website: null,
    donation_url: null,
    ...overrides,
  }
}

describe('StationDetail (PSY-1472)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockHash.mockReturnValue('')
    mockUseRadioStation.mockReturnValue({
      data: makeStation(),
      isLoading: false,
      error: null,
    })
    Element.prototype.scrollIntoView = vi.fn()
  })

  it('renders the #recent-playlists anchor the mobile graph teaser links to', () => {
    const { container } = render(<StationDetail stationSlug="kexp" />)
    // Same constant the teaser's linkHref uses — single source, can't drift.
    expect(container.querySelector(`#${STATION_PLAYLISTS_ANCHOR}`)).toBeInTheDocument()
  })

  it('scrolls to the playlists feed once when cold-loaded with the teaser hash', () => {
    mockHash.mockReturnValue(`#${STATION_PLAYLISTS_ANCHOR}`)
    render(<StationDetail stationSlug="kexp" />)
    // Client-fetched page: native fragment scroll fired too early, so the effect
    // scrolls once the station data lands.
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledTimes(1)
  })

  it('does not scroll for an unrelated hash', () => {
    mockHash.mockReturnValue('#graph')
    render(<StationDetail stationSlug="kexp" />)
    expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled()
  })

  it('re-arms the scroll when the station changes without a remount (sibling tab)', () => {
    mockHash.mockReturnValue(`#${STATION_PLAYLISTS_ANCHOR}`)
    const { rerender } = render(<StationDetail stationSlug="kexp" />)
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledTimes(1)

    // NetworkTabBar swaps sibling stations by URL change, NOT a remount — the
    // same instance receives new station data. A bare boolean ref would stay
    // latched; the slug-keyed ref re-arms.
    mockUseRadioStation.mockReturnValue({
      data: makeStation({ slug: 'kexp-2', name: 'KEXP 2' }),
      isLoading: false,
      error: null,
    })
    rerender(<StationDetail stationSlug="kexp-2" />)
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledTimes(2)
  })
})
