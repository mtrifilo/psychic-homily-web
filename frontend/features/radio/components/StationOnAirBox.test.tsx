import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StationOnAirBox } from './StationOnAirBox'
import type {
  RadioStationDetail,
  RadioEpisodeDetail,
  RadioNowPlaying,
  RadioNowPlayingTrack,
} from '../types'

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

const mockUseStationNowPlaying = vi.fn()
vi.mock('../hooks/useStationNowPlaying', () => ({
  useStationNowPlaying: (...args: unknown[]) =>
    mockUseStationNowPlaying(...args),
}))

const mockUseShowLatestEpisode = vi.fn()
vi.mock('../hooks/useShowLatestEpisode', () => ({
  useShowLatestEpisode: (...args: unknown[]) => mockUseShowLatestEpisode(...args),
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
    network: null,
    sibling_stations: [],
    show_count: 2,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeTrack(
  overrides: Partial<RadioNowPlayingTrack> = {}
): RadioNowPlayingTrack {
  return {
    artist_name: 'CAN',
    track_title: 'Vitamin C',
    album_title: 'Ege Bamyasi',
    label_name: 'United Artists',
    release_year: 1972,
    rotation_status: null,
    dj_comment: null,
    artist_id: 7,
    artist_slug: 'can',
    release_id: null,
    release_slug: null,
    label_id: null,
    label_slug: null,
    ...overrides,
  }
}

function makeLiveNowPlaying(
  overrides: Partial<RadioNowPlaying> = {}
): RadioNowPlaying {
  return {
    source: 'live',
    on_air: true,
    show: {
      id: 1,
      name: 'The Night Owl Show',
      slug: 'night-owl',
      host_name: 'Pedro Santos',
    },
    show_name: 'The Night Owl Show',
    host_name: null,
    current_track: makeTrack(),
    recent_artists: [],
    episode_air_date: null,
    ...overrides,
  }
}

function makeArchiveNowPlaying(
  overrides: Partial<RadioNowPlaying> = {}
): RadioNowPlaying {
  return makeLiveNowPlaying({
    source: 'latest_archive',
    on_air: false,
    episode_air_date: '2026-06-08',
    ...overrides,
  })
}

function setNowPlaying(data: RadioNowPlaying | undefined) {
  mockUseStationNowPlaying.mockReturnValue({
    data,
    isLoading: false,
    error: null,
  })
}

function setEpisode(episode: RadioEpisodeDetail | undefined, isLoading = false) {
  mockUseShowLatestEpisode.mockReturnValue({
    episode,
    isLoading,
    error: null,
    hasEpisodes: !!episode,
  })
}

const latestEpisode = { air_date: '2026-06-08' } as RadioEpisodeDetail

describe('StationOnAirBox', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setEpisode(latestEpisode)
  })

  it('renders nothing while the payload is loading', () => {
    setNowPlaying(undefined)
    const { container } = render(<StationOnAirBox station={makeStation()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing for a station with no shows at all', () => {
    setNowPlaying(makeArchiveNowPlaying({ show: null, show_name: null, current_track: null }))
    setEpisode(undefined)
    const { container } = render(<StationOnAirBox station={makeStation()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a live broadcast with the ON AIR dot, show link, and track', () => {
    setNowPlaying(makeLiveNowPlaying())

    render(<StationOnAirBox station={makeStation()} />)

    expect(screen.getByText(/On air — WFMU/)).toBeInTheDocument()
    const showLink = screen.getByRole('link', { name: 'The Night Owl Show' })
    expect(showLink).toHaveAttribute('href', '/radio/wfmu/night-owl')
    expect(screen.getByText('w/ Pedro Santos')).toBeInTheDocument()

    const artistLink = screen.getByRole('link', { name: 'CAN' })
    expect(artistLink).toHaveAttribute('href', '/artists/can')
    expect(screen.getByText('— Vitamin C')).toBeInTheDocument()
    expect(
      screen.getByText('Ege Bamyasi · United Artists · 1972')
    ).toBeInTheDocument()

    // Matched show → its latest archived playlist deep-link.
    expect(mockUseShowLatestEpisode).toHaveBeenCalledWith('night-owl')
    const playlistLink = screen.getByRole('link', { name: 'Open latest playlist →' })
    expect(playlistLink).toHaveAttribute('href', '/radio/wfmu/night-owl/2026-06-08')
  })

  it('labels the latest-archive fallback honestly (no on-air claim)', () => {
    setNowPlaying(makeArchiveNowPlaying())

    render(<StationOnAirBox station={makeStation()} />)

    expect(screen.getByText(/Latest playlist — WFMU/)).toBeInTheDocument()
    expect(screen.queryByText(/On air/)).not.toBeInTheDocument()
    expect(screen.getByText('Latest: Jun 8')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'The Night Owl Show' })
    ).toBeInTheDocument()
  })

  it('renders an unmatched live show name as plain text with no playlist link', () => {
    setNowPlaying(
      makeLiveNowPlaying({
        show: null,
        show_name: 'Secret Canine Agents',
        host_name: 'DJ Perro Caliente',
        current_track: null,
      })
    )
    setEpisode(undefined)

    render(<StationOnAirBox station={makeStation()} />)

    expect(screen.getByText('Secret Canine Agents')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Secret Canine Agents' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('w/ DJ Perro Caliente')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Open latest playlist →' })
    ).not.toBeInTheDocument()
    // Unmatched show → no archive lookup target.
    expect(mockUseShowLatestEpisode).toHaveBeenCalledWith(undefined)
  })

  it('renders the unmatched current artist as plain text (no dead link)', () => {
    setNowPlaying(
      makeLiveNowPlaying({
        current_track: makeTrack({
          artist_name: 'Obscure Tape Act',
          artist_id: null,
          artist_slug: null,
        }),
      })
    )

    render(<StationOnAirBox station={makeStation()} />)

    expect(screen.getByText('Obscure Tape Act')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Obscure Tape Act' })
    ).not.toBeInTheDocument()
  })

  it('still renders show identity when no episode has loaded', () => {
    setNowPlaying(makeLiveNowPlaying())
    setEpisode(undefined)

    render(<StationOnAirBox station={makeStation()} />)

    expect(screen.getByRole('link', { name: 'The Night Owl Show' })).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Open latest playlist →' })
    ).not.toBeInTheDocument()
  })
})
