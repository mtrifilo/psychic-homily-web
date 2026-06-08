import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RadioStationOverview } from './RadioStationOverview'
import type { StationOverview } from '../hooks/useStationOverview'
import type {
  RadioStationDetail,
  RadioShowDetail,
  RadioShowListItem,
  RadioPlay,
} from '../types'

vi.mock('next/link', () => ({
  default: ({ href, children, onClick, ...props }: { href: string; children: React.ReactNode; onClick?: () => void; [key: string]: unknown }) => (
    <a href={href} onClick={onClick} {...props}>{children}</a>
  ),
}))

// RecentShowRow fetches its own latest episode; stub it to a simple marker so
// this test isolates the overview shell + Now Playing card.
vi.mock('./RecentShowRow', () => ({
  RecentShowRow: ({ show }: { show: { name: string } }) => (
    <div data-testid="recent-show-row">{show.name}</div>
  ),
}))

const mockUseStationOverview = vi.fn<() => StationOverview>()
vi.mock('../hooks/useStationOverview', () => ({
  useStationOverview: () => mockUseStationOverview(),
}))

function makeStation(overrides: Partial<RadioStationDetail> = {}): RadioStationDetail {
  return {
    id: 1,
    name: 'KEXP',
    slug: 'kexp',
    description: 'Independent, host-curated radio — famously eclectic and deep.',
    city: 'Seattle',
    state: 'WA',
    country: 'USA',
    timezone: 'America/Los_Angeles',
    stream_url: null,
    stream_urls: null,
    website: 'https://kexp.org',
    donation_url: null,
    donation_embed_url: null,
    logo_url: null,
    social: null,
    broadcast_type: 'both',
    frequency_mhz: 90.3,
    playlist_source: null,
    playlist_config: null,
    last_playlist_fetch_at: null,
    is_active: true,
    network: null,
    sibling_stations: [],
    show_count: 5,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeShowDetail(overrides: Partial<RadioShowDetail> = {}): RadioShowDetail {
  return {
    id: 1,
    station_id: 1,
    station_name: 'KEXP',
    station_slug: 'kexp',
    name: 'Variety Mix',
    slug: 'variety-mix',
    host_name: 'Cheryl Waters',
    description: 'Eclectic host’s-choice mix — the KEXP signature.',
    schedule_display: null,
    schedule: null,
    genre_tags: null,
    archive_url: null,
    image_url: null,
    is_active: true,
    episode_count: 12,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeShowListItem(overrides: Partial<RadioShowListItem> = {}): RadioShowListItem {
  return {
    id: 1,
    station_id: 1,
    station_name: 'KEXP',
    name: 'Variety Mix',
    slug: 'variety-mix',
    host_name: 'Cheryl Waters',
    genre_tags: null,
    image_url: null,
    is_active: true,
    episode_count: 12,
    ...overrides,
  }
}

function makePlay(overrides: Partial<RadioPlay> = {}): RadioPlay {
  return {
    id: 1,
    episode_id: 100,
    position: 1,
    artist_name: 'Sleater-Kinney',
    track_title: 'Dig Me Out',
    album_title: 'Dig Me Out',
    label_name: 'Kill Rock Stars',
    release_year: 1997,
    is_new: false,
    rotation_status: null,
    dj_comment: null,
    is_live_performance: false,
    is_request: false,
    artist_id: 1,
    artist_slug: 'sleater-kinney',
    release_id: null,
    release_slug: null,
    label_id: 1,
    label_slug: 'kill-rock-stars',
    musicbrainz_artist_id: null,
    musicbrainz_recording_id: null,
    musicbrainz_release_id: null,
    air_timestamp: null,
    ...overrides,
  }
}

function baseOverview(overrides: Partial<StationOverview> = {}): StationOverview {
  return {
    station: makeStation(),
    nowPlayingShow: makeShowListItem(),
    nowPlayingShowDetail: makeShowDetail(),
    nowPlaying: {
      current: makePlay(),
      recentArtists: [
        { name: 'Wipers', slug: 'wipers' },
        { name: 'Unwound', slug: null },
      ],
    },
    recentShows: [makeShowListItem({ id: 2, name: 'Audioasis', slug: 'audioasis' })],
    isLoading: false,
    isEmpty: false,
    error: null,
    ...overrides,
  }
}

describe('RadioStationOverview', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the station identity header with a data-honest sub-line and Listen link', () => {
    mockUseStationOverview.mockReturnValue(baseOverview())
    render(<RadioStationOverview stationSlug="kexp" />)
    expect(screen.getByRole('link', { name: 'KEXP' })).toHaveAttribute('href', '/radio/kexp')
    // location + broadcast type (both from the model — not the Figma's
    // "listener-supported" editorial copy).
    expect(screen.getByText('Seattle, WA · FM/AM + Internet')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Listen/ })).toHaveAttribute('href', 'https://kexp.org')
  })

  it('renders the Now Playing card with show, host, current track, and entity hops', () => {
    mockUseStationOverview.mockReturnValue(baseOverview())
    render(<RadioStationOverview stationSlug="kexp" />)
    expect(screen.getByText('Now playing')).toBeInTheDocument()
    expect(screen.getByText('on air')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Variety Mix' })).toHaveAttribute(
      'href',
      '/radio/kexp/variety-mix'
    )
    expect(screen.getByText('with Cheryl Waters')).toBeInTheDocument()
    // current track artist is an entity hop
    expect(screen.getByRole('link', { name: 'Sleater-Kinney' })).toHaveAttribute(
      'href',
      '/artists/sleater-kinney'
    )
    // label is also a hop
    expect(screen.getByRole('link', { name: 'Kill Rock Stars' })).toHaveAttribute(
      'href',
      '/labels/kill-rock-stars'
    )
    // recently-played artists: linked + plain
    expect(screen.getByText('Recently:')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Wipers' })).toHaveAttribute('href', '/artists/wipers')
    expect(screen.getByText('Unwound')).toBeInTheDocument()
  })

  it('renders the recent shows section', () => {
    mockUseStationOverview.mockReturnValue(baseOverview())
    render(<RadioStationOverview stationSlug="kexp" />)
    expect(screen.getByText('Recent shows')).toBeInTheDocument()
    expect(screen.getByTestId('recent-show-row')).toHaveTextContent('Audioasis')
  })

  it('hides the description line when the station has none (WFMU/NTS may be null)', () => {
    mockUseStationOverview.mockReturnValue(
      baseOverview({ station: makeStation({ description: null }) })
    )
    render(<RadioStationOverview stationSlug="kexp" />)
    expect(
      screen.queryByText(/Independent, host-curated radio/)
    ).not.toBeInTheDocument()
  })

  it('skips the Now Playing card when there is no now-playing show', () => {
    mockUseStationOverview.mockReturnValue(
      baseOverview({ nowPlayingShow: null, nowPlayingShowDetail: undefined })
    )
    render(<RadioStationOverview stationSlug="kexp" />)
    expect(screen.queryByText('Now playing')).not.toBeInTheDocument()
  })

  it('shows an empty state when the station has no shows', () => {
    mockUseStationOverview.mockReturnValue(
      baseOverview({
        nowPlayingShow: null,
        nowPlayingShowDetail: undefined,
        recentShows: [],
        isEmpty: true,
      })
    )
    render(<RadioStationOverview stationSlug="kexp" />)
    expect(screen.getByText('No shows tracked for this station yet.')).toBeInTheDocument()
  })

  it('renders an error state when the station fails to load', () => {
    mockUseStationOverview.mockReturnValue(
      baseOverview({ station: undefined, error: new Error('boom') })
    )
    render(<RadioStationOverview stationSlug="kexp" />)
    expect(screen.getByText(/Couldn't load this station/)).toBeInTheDocument()
  })
})
