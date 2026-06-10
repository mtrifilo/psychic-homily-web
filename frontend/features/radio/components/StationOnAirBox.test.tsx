import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StationOnAirBox } from './StationOnAirBox'
import type {
  RadioStationDetail,
  RadioShowListItem,
  RadioEpisodeDetail,
  RadioPlay,
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

const mockUseRadioShows = vi.fn()
vi.mock('../hooks/useRadioShows', () => ({
  useRadioShows: (...args: unknown[]) => mockUseRadioShows(...args),
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

function makeShow(overrides: Partial<RadioShowListItem> = {}): RadioShowListItem {
  return {
    id: 1,
    station_id: 1,
    station_name: 'WFMU',
    name: 'The Night Owl Show',
    slug: 'night-owl',
    host_name: 'Pedro Santos',
    schedule_display: 'Mon 9pm-12am',
    genre_tags: null,
    image_url: null,
    is_active: true,
    episode_count: 142,
    latest_air_date: '2026-06-09',
    ...overrides,
  }
}

function makePlay(overrides: Partial<RadioPlay> = {}): RadioPlay {
  return {
    id: 1,
    episode_id: 1,
    position: 1,
    artist_name: 'CAN',
    track_title: 'Vitamin C',
    album_title: 'Ege Bamyasi',
    label_name: 'United Artists',
    release_year: 1972,
    is_new: false,
    rotation_status: null,
    dj_comment: null,
    is_live_performance: false,
    is_request: false,
    artist_id: 7,
    artist_slug: 'can',
    release_id: null,
    release_slug: null,
    label_id: null,
    label_slug: null,
    musicbrainz_artist_id: null,
    musicbrainz_recording_id: null,
    musicbrainz_release_id: null,
    air_timestamp: null,
    ...overrides,
  }
}

function makeEpisode(overrides: Partial<RadioEpisodeDetail> = {}): RadioEpisodeDetail {
  return {
    id: 1,
    show_id: 1,
    show_name: 'The Night Owl Show',
    show_slug: 'night-owl',
    station_name: 'WFMU',
    station_slug: 'wfmu',
    title: null,
    air_date: '2026-06-09',
    air_time: null,
    duration_minutes: null,
    description: null,
    archive_url: null,
    mixcloud_url: null,
    genre_tags: null,
    mood_tags: null,
    play_count: 1,
    plays: [makePlay()],
    created_at: '2026-06-09T00:00:00Z',
    ...overrides,
  }
}

function setShows(shows: RadioShowListItem[]) {
  mockUseRadioShows.mockReturnValue({
    data: { shows, count: shows.length },
    isLoading: false,
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

describe('StationOnAirBox', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when the station has no shows', () => {
    setShows([])
    setEpisode(undefined)
    const { container } = render(<StationOnAirBox station={makeStation()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('surfaces the most-active show (v1 heuristic) with host and schedule', () => {
    const signature = makeShow({ id: 2, episode_count: 142 })
    const quieter = makeShow({
      id: 3,
      name: 'Techtonic',
      slug: 'techtonic',
      episode_count: 30,
    })
    setShows([quieter, signature])
    setEpisode(makeEpisode())

    render(<StationOnAirBox station={makeStation()} />)

    expect(mockUseShowLatestEpisode).toHaveBeenCalledWith('night-owl')
    const showLink = screen.getByRole('link', { name: 'The Night Owl Show' })
    expect(showLink).toHaveAttribute('href', '/radio/wfmu/night-owl')
    expect(screen.getByText('w/ Pedro Santos')).toBeInTheDocument()
    expect(screen.getByText('Mon 9pm-12am')).toBeInTheDocument()
    expect(screen.queryByText('Techtonic')).not.toBeInTheDocument()
  })

  it('renders the current track with an artist graph link and a playlist link', () => {
    setShows([makeShow()])
    setEpisode(makeEpisode())

    render(<StationOnAirBox station={makeStation()} />)

    const artistLink = screen.getByRole('link', { name: 'CAN' })
    expect(artistLink).toHaveAttribute('href', '/artists/can')
    expect(screen.getByText('— Vitamin C')).toBeInTheDocument()
    expect(
      screen.getByText('Ege Bamyasi · United Artists · 1972')
    ).toBeInTheDocument()

    const playlistLink = screen.getByRole('link', { name: 'Open latest playlist →' })
    expect(playlistLink).toHaveAttribute('href', '/radio/wfmu/night-owl/2026-06-09')
  })

  it('renders the unmatched current artist as plain text (no dead link)', () => {
    setShows([makeShow()])
    setEpisode(
      makeEpisode({
        plays: [makePlay({ artist_name: 'Obscure Tape Act', artist_id: null, artist_slug: null })],
      })
    )

    render(<StationOnAirBox station={makeStation()} />)

    expect(screen.getByText('Obscure Tape Act')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Obscure Tape Act' })
    ).not.toBeInTheDocument()
  })

  it('still renders show identity when no episode has loaded', () => {
    setShows([makeShow()])
    setEpisode(undefined)

    render(<StationOnAirBox station={makeStation()} />)

    expect(screen.getByRole('link', { name: 'The Night Owl Show' })).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Open latest playlist →' })
    ).not.toBeInTheDocument()
  })
})
