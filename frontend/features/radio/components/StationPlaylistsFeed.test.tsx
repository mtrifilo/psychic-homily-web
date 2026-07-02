import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { StationPlaylistsFeed } from './StationPlaylistsFeed'
import type { RadioStationDetail, RadioStationEpisodeRow } from '../types'
import { localIso } from '../lib/localIso.testutil'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

const mockUseStationEpisodes = vi.fn()
vi.mock('../hooks/useStationEpisodes', () => ({
  useStationEpisodes: (...args: unknown[]) => mockUseStationEpisodes(...args),
}))

function makeStation(overrides: Partial<RadioStationDetail> = {}): RadioStationDetail {
  return {
    id: 1,
    name: 'WFMU',
    slug: 'wfmu',
    description: null,
    city: 'Jersey City',
    state: 'NJ',
    country: 'USA',
    timezone: null,
    stream_url: null,
    stream_urls: null,
    website: null,
    donation_url: null,
    donation_embed_url: null,
    logo_url: null,
    social: null,
    broadcast_type: 'both',
    frequency_mhz: 91.1,
    playlist_source: null,
    playlist_config: null,
    last_playlist_fetch_at: null,
    is_active: true,
    network: { slug: 'wfmu', name: 'WFMU', is_flagship: true },
    sibling_stations: [],
    show_count: 2,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeRow(overrides: Partial<RadioStationEpisodeRow> = {}): RadioStationEpisodeRow {
  return {
    id: 1,
    title: null,
    air_date: '2026-06-09',
    starts_at: null,
    ends_at: null,
    play_count: 24,
    archive_url: null,
    show_id: 1,
    show_name: 'The Night Owl Show',
    show_slug: 'night-owl',
    station_id: 1,
    station_name: 'WFMU',
    station_slug: 'wfmu',
    artist_preview: [
      { artist_name: 'CAN', artist_id: 7, artist_slug: 'can' },
      { artist_name: "it's all meat", artist_id: null, artist_slug: null },
    ],
    ...overrides,
  }
}

function setEpisodes(episodes: RadioStationEpisodeRow[], total = episodes.length) {
  mockUseStationEpisodes.mockReturnValue({
    data: { episodes, total },
    isLoading: false,
    isFetching: false,
  })
}

describe('StationPlaylistsFeed', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders date, show link, preview, and track count per row', () => {
    setEpisodes([makeRow()])
    render(<StationPlaylistsFeed station={makeStation()} />)

    const showLink = screen.getByRole('link', { name: 'The Night Owl Show' })
    expect(showLink).toHaveAttribute('href', '/radio/wfmu/night-owl')

    // aria-label matches the visible viewer-local rendering (label-in-name,
    // PSY-1298) while the href stays keyed on the station-dated air_date.
    const dateLink = screen.getByRole('link', { name: 'Playlist from Jun 9' })
    expect(dateLink).toHaveAttribute('href', '/radio/wfmu/night-owl/2026-06-09')
    expect(dateLink).toHaveTextContent('Jun 9')

    expect(screen.getByText('24')).toBeInTheDocument()
  })

  it('renders the viewer-local air-time block stacked under the date for windowed rows (PSY-1298)', () => {
    // air_date deliberately differs from the window's local day so the
    // assertion discriminates "date derives from starts_at" — a fallback to
    // air_date would render Jun 8 and fail.
    setEpisodes([
      makeRow({
        air_date: '2026-06-08',
        starts_at: localIso(2026, 5, 9, 15),
        ends_at: localIso(2026, 5, 9, 18),
      }),
    ])
    render(<StationPlaylistsFeed station={makeStation()} />)

    const dateLink = screen.getByRole('link', {
      name: 'Playlist from Jun 9, 3–6 PM',
    })
    expect(dateLink).toHaveTextContent('Jun 9')
    expect(dateLink).toHaveTextContent('3–6 PM')
    // deep-link stays station-dated even though the display is viewer-local
    expect(dateLink).toHaveAttribute('href', '/radio/wfmu/night-owl/2026-06-08')
  })

  it('renders NO time line for windowless rows (locked decision 4)', () => {
    setEpisodes([makeRow()]) // default fixture: null window
    render(<StationPlaylistsFeed station={makeStation()} />)
    const dateLink = screen.getByRole('link', { name: 'Playlist from Jun 9' })
    expect(dateLink.textContent).toBe('Jun 9')
  })

  it('links matched preview artists and renders unmatched ones as plain text', () => {
    setEpisodes([makeRow()])
    render(<StationPlaylistsFeed station={makeStation()} />)

    expect(screen.getByRole('link', { name: 'CAN' })).toHaveAttribute(
      'href',
      '/artists/can'
    )
    expect(screen.getByText("it's all meat")).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: "it's all meat" })
    ).not.toBeInTheDocument()
  })

  it('renders an em dash for an empty artist preview', () => {
    setEpisodes([makeRow({ artist_preview: [], play_count: 0 })])
    render(<StationPlaylistsFeed station={makeStation()} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('uses the per-station heading with no channel column on a network flagship (PSY-1074)', () => {
    // The station-scoped endpoint now returns only the requested station's
    // rows, so flagship tabs render the same single-station table as
    // everyone else — no "all channels" heading, no CHANNEL column.
    setEpisodes([makeRow()])
    render(<StationPlaylistsFeed station={makeStation()} />)

    expect(screen.getByText('Latest playlists')).toBeInTheDocument()
    expect(
      screen.queryByText('Latest playlists — all WFMU channels')
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Channel')).not.toBeInTheDocument()
  })

  it('builds row links from the row station on sub-channel pages', () => {
    setEpisodes([
      makeRow({
        show_name: 'Give the Drummer Some',
        show_slug: 'drummer-some',
        station_id: 9,
        station_name: 'Give the Drummer Radio',
        station_slug: 'wfmu-drummer',
      }),
    ])
    render(
      <StationPlaylistsFeed
        station={makeStation({
          name: 'Give the Drummer Radio',
          slug: 'wfmu-drummer',
          network: { slug: 'wfmu', name: 'WFMU', is_flagship: false },
        })}
      />
    )

    expect(screen.getByText('Latest playlists')).toBeInTheDocument()
    expect(screen.queryByText('Channel')).not.toBeInTheDocument()
    const showLink = screen.getByRole('link', { name: 'Give the Drummer Some' })
    expect(showLink).toHaveAttribute('href', '/radio/wfmu-drummer/drummer-some')
  })

  it('omits the channel column for standalone stations', () => {
    setEpisodes([makeRow({ station_name: 'KEXP', station_slug: 'kexp' })])
    render(
      <StationPlaylistsFeed
        station={makeStation({ name: 'KEXP', slug: 'kexp', network: null })}
      />
    )

    expect(screen.getByText('Latest playlists')).toBeInTheDocument()
    expect(screen.queryByText('Channel')).not.toBeInTheDocument()
  })

  it('grows the in-place limit on "More playlists" and reports the total', () => {
    setEpisodes([makeRow()], 574)
    render(<StationPlaylistsFeed station={makeStation()} />)

    expect(screen.getByText('showing 1 of 574 playlists')).toBeInTheDocument()
    expect(mockUseStationEpisodes).toHaveBeenLastCalledWith(
      expect.objectContaining({ stationSlug: 'wfmu', limit: 10 })
    )

    fireEvent.click(screen.getByRole('button', { name: 'More playlists' }))
    expect(mockUseStationEpisodes).toHaveBeenLastCalledWith(
      expect.objectContaining({ stationSlug: 'wfmu', limit: 30 })
    )
  })

  it('hides the load-more control once every playlist is shown', () => {
    setEpisodes([makeRow()], 1)
    render(<StationPlaylistsFeed station={makeStation()} />)
    expect(
      screen.queryByRole('button', { name: 'More playlists' })
    ).not.toBeInTheDocument()
  })

  it('renders the empty state when no playlists are logged', () => {
    setEpisodes([])
    render(<StationPlaylistsFeed station={makeStation()} />)
    expect(screen.getByText('No playlists logged yet.')).toBeInTheDocument()
  })
})
