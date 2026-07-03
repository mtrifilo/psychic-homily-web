import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

const mockUseRadioStats = vi.fn()
const mockUseRadioStations = vi.fn()
const mockUseRecentRadioEpisodes = vi.fn()
const mockUseNewReleaseRadar = vi.fn()

const mockUseRadioGuide = vi.fn()

vi.mock('@/features/radio', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/radio')>()
  return {
    ...actual,
    useRadioStats: () => mockUseRadioStats(),
    useRadioStations: () => mockUseRadioStations(),
    useRecentRadioEpisodes: (...args: unknown[]) =>
      mockUseRecentRadioEpisodes(...args),
    useNewReleaseRadar: (...args: unknown[]) => mockUseNewReleaseRadar(...args),
    // PSY-1053: hub structure tests default to an empty guide (RadioGuide
    // has its own test file); the placement test overrides this mock.
    useRadioGuide: () => mockUseRadioGuide(),
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
    mockUseRadioGuide.mockReset().mockReturnValue({ data: undefined, isError: false })
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

  it('renders the guide between THE DIAL and LATEST PLAYLISTS (PSY-1053)', () => {
    mockUseRadioGuide.mockReturnValue({
      data: {
        on_now: [
          {
            station: { slug: 'wfmu', name: 'WFMU' },
            show: { id: 1, slug: 'wake', name: 'Wake', host_name: null },
            starts_at: new Date().toISOString(),
            ends_at: new Date(Date.now() + 2 * 3600e3).toISOString(),
            station_timezone: 'America/New_York',
          },
        ],
        up_next: [],
        generated_at: new Date().toISOString(),
      },
      isError: false,
    })
    render(<RadioHub />)

    const dial = screen.getByText('The dial — live now')
    const guide = screen.getByLabelText('Program guide')
    const playlists = screen.getByText('Latest playlists — across the dial')
    // DOM order: dial section → guide → playlists (the ticket's one
    // structural requirement).
    expect(
      dial.compareDocumentPosition(guide) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      guide.compareDocumentPosition(playlists) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it('links the full playlists feed under the latest-playlists table (PSY-1076)', () => {
    mockUseRecentRadioEpisodes.mockReturnValue({
      data: {
        episodes: [
          {
            id: 1,
            title: null,
            air_date: '2026-06-09',
            play_count: 24,
            archive_url: null,
            show_id: 3,
            show_name: 'The Night Owl Show',
            show_slug: 'night-owl',
            station_id: 2,
            station_name: 'WFMU',
            station_slug: 'wfmu',
            artist_preview: [],
          },
        ],
        total: 574,
      },
      isLoading: false,
      error: null,
    })
    render(<RadioHub />)

    expect(
      screen.getByRole('link', { name: 'all playlists →' })
    ).toHaveAttribute('href', '/radio/playlists')
  })

  it('omits the all-playlists link when the feed is empty', () => {
    render(<RadioHub />)
    expect(
      screen.queryByRole('link', { name: 'all playlists →' })
    ).not.toBeInTheDocument()
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
