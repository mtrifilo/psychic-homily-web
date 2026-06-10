import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

const mockUseRadioStats = vi.fn()
const mockUseRadioStations = vi.fn()
const mockUseRecentRadioEpisodes = vi.fn()
const mockUseNewReleaseRadar = vi.fn()

vi.mock('@/features/radio', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/radio')>()
  return {
    ...actual,
    useRadioStats: () => mockUseRadioStats(),
    useRadioStations: () => mockUseRadioStations(),
    useRecentRadioEpisodes: (...args: unknown[]) =>
      mockUseRecentRadioEpisodes(...args),
    useNewReleaseRadar: (...args: unknown[]) => mockUseNewReleaseRadar(...args),
  }
})

// The strip fetches per-station data; stub it so the hub test stays focused
// on page structure (the strip has its own test file).
vi.mock('./DialStationStrip', () => ({
  DialStationStrip: ({ station }: { station: { name: string } }) => (
    <div data-testid="dial-strip">{station.name}</div>
  ),
}))

import RadioHub from './RadioHub'

const stats = {
  total_stations: 6,
  total_shows: 2341,
  total_episodes: 1725,
  total_plays: 19071,
  matched_plays: 8210,
  unique_artists: 1148,
}

function makeStation(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: 'KEXP',
    slug: 'kexp',
    city: 'Seattle',
    state: 'WA',
    country: 'USA',
    broadcast_type: 'both',
    frequency_mhz: 90.3,
    logo_url: null,
    is_active: true,
    network: null,
    sibling_stations: [],
    show_count: 40,
    ...overrides,
  }
}

describe('RadioHub', () => {
  beforeEach(() => {
    mockUseRadioStats.mockReset().mockReturnValue({ data: stats })
    mockUseRadioStations.mockReset().mockReturnValue({
      data: {
        stations: [
          makeStation(),
          makeStation({ id: 2, name: 'WFMU', slug: 'wfmu' }),
          // Non-flagship network sibling must NOT get its own strip.
          makeStation({
            id: 5,
            name: 'Give the Drummer Radio',
            slug: 'wfmu-drummer',
            network: { slug: 'wfmu', name: 'WFMU', is_flagship: false },
          }),
        ],
        count: 3,
      },
      isLoading: false,
      error: null,
    })
    mockUseRecentRadioEpisodes.mockReset().mockReturnValue({
      data: { episodes: [], total: 0 },
      isLoading: false,
      error: null,
    })
    mockUseNewReleaseRadar.mockReset().mockReturnValue({
      data: { releases: [], count: 0 },
      isLoading: false,
    })
  })

  it('renders the page head with mission and mono stats line', () => {
    render(<RadioHub />)

    expect(
      screen.getByRole('heading', { level: 1, name: 'Radio' })
    ).toBeInTheDocument()
    expect(screen.getByText(/wired into the knowledge graph/)).toBeInTheDocument()
    expect(
      screen.getByText(
        '6 stations · 2,341 shows · 1,725 playlists · 19,071 plays tracked'
      )
    ).toBeInTheDocument()
  })

  it('renders one dial strip per visible station, excluding non-flagship channels', () => {
    render(<RadioHub />)

    const strips = screen.getAllByTestId('dial-strip')
    expect(strips.map(s => s.textContent)).toEqual(['KEXP', 'WFMU'])
  })

  it('renders the dial and latest-playlists section headings', () => {
    render(<RadioHub />)

    expect(screen.getByText('The dial — live now')).toBeInTheDocument()
    expect(
      screen.getByText('Latest playlists — across the dial')
    ).toBeInTheDocument()
  })

  it('renders an error state when stations fail to load', () => {
    mockUseRadioStations.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    })
    render(<RadioHub />)
    expect(screen.getByText("Couldn't load radio stations.")).toBeInTheDocument()
  })
})
