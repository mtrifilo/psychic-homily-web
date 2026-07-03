import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LatestRadioShows } from './LatestRadioShows'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

const useStationEpisodes = vi.fn()
const useStationNowPlaying = vi.fn()

vi.mock('@/features/radio/hooks', () => ({
  useStationEpisodes: (opts: { stationSlug: string }) => useStationEpisodes(opts),
  useStationNowPlaying: (slug: string) => useStationNowPlaying(slug),
}))

function episodeRow(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    title: null,
    air_date: '2026-07-03',
    starts_at: null,
    ends_at: null,
    play_count: 12,
    archive_url: null,
    show_id: 7,
    show_name: 'Strength Through Failure',
    show_slug: 'strength-through-failure',
    station_id: 3,
    station_name: 'WFMU',
    station_slug: 'wfmu',
    artist_preview: [
      { artist_name: 'Stereolab', artist_id: 1, artist_slug: 'stereolab' },
      { artist_name: 'Broadcast', artist_id: null, artist_slug: null },
      { artist_name: 'Pram', artist_id: null, artist_slug: null },
      { artist_name: 'Fourth Artist Never Shown', artist_id: null, artist_slug: null },
    ],
    ...overrides,
  }
}

beforeEach(() => {
  useStationEpisodes.mockReset().mockReturnValue({ data: undefined })
  useStationNowPlaying.mockReset().mockReturnValue({ data: undefined })
})

describe('LatestRadioShows', () => {
  it('renders the section heading and a "Browse all stations" link to /radio', () => {
    render(<LatestRadioShows />)
    expect(
      screen.getByRole('heading', { name: /latest radio shows/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /browse all stations/i })
    ).toHaveAttribute('href', '/radio')
  })

  it('renders the three editorial station cards deep-linked to their /radio station tabs', () => {
    render(<LatestRadioShows />)
    expect(screen.getByText('KEXP').closest('a')).toHaveAttribute('href', '/radio/kexp')
    expect(screen.getByText('WFMU').closest('a')).toHaveAttribute('href', '/radio/wfmu')
    expect(screen.getByText('NTS').closest('a')).toHaveAttribute('href', '/radio/nts-radio')
  })

  it("shows a station's real latest episode: show name + up to 3 preview artists", () => {
    useStationEpisodes.mockImplementation(({ stationSlug }: { stationSlug: string }) =>
      stationSlug === 'wfmu'
        ? { data: { episodes: [episodeRow()], total: 1 } }
        : { data: { episodes: [], total: 0 } }
    )
    render(<LatestRadioShows />)
    expect(screen.getByText('Strength Through Failure')).toBeInTheDocument()
    expect(screen.getByText('Stereolab · Broadcast · Pram')).toBeInTheDocument()
    expect(screen.queryByText(/Fourth Artist Never Shown/)).not.toBeInTheDocument()
  })

  it('degrades gracefully with no episode data: editorial shell only, no fictional content', () => {
    useStationEpisodes.mockReturnValue({ data: { episodes: [], total: 0 } })
    render(<LatestRadioShows />)
    // Editorial shells render (call sign + city + vibe), nothing invented.
    expect(screen.getByText('KEXP')).toBeInTheDocument()
    expect(screen.getByText('Seattle')).toBeInTheDocument()
    expect(screen.getByText('Eclectic host’s-choice')).toBeInTheDocument()
    // The old placeholder fictions must be gone.
    expect(screen.queryByText('Variety Mix')).not.toBeInTheDocument()
    expect(screen.queryByText('Wake and Bake')).not.toBeInTheDocument()
    expect(screen.queryByText('Charlie Bones')).not.toBeInTheDocument()
    expect(screen.queryByText(/Sleater-Kinney/)).not.toBeInTheDocument()
  })

  it('shows the live dot only for stations the now-playing endpoint reports on air', () => {
    useStationNowPlaying.mockImplementation((slug: string) => ({
      data: { on_air: slug === 'kexp' },
    }))
    render(<LatestRadioShows />)
    expect(screen.getAllByText('live')).toHaveLength(1)
    expect(screen.getByText('live').closest('a')).toHaveAttribute('href', '/radio/kexp')
  })

  it('shows no live dot while now-playing data is absent', () => {
    render(<LatestRadioShows />)
    expect(screen.queryByText('live')).not.toBeInTheDocument()
  })

  it('degrades to the editorial shell on a fetch ERROR (e.g. a renamed slug 404s)', () => {
    useStationEpisodes.mockReturnValue({ data: undefined, error: new Error('404') })
    useStationNowPlaying.mockReturnValue({ data: undefined, error: new Error('404') })
    render(<LatestRadioShows />)
    expect(screen.getByText('KEXP')).toBeInTheDocument()
    expect(screen.getByText('Eclectic host’s-choice')).toBeInTheDocument()
    expect(screen.queryByText('live')).not.toBeInTheDocument()
  })

  it('restates live status and artists in the accessible name (aria-label replaces content)', () => {
    useStationEpisodes.mockImplementation(({ stationSlug }: { stationSlug: string }) =>
      stationSlug === 'wfmu'
        ? { data: { episodes: [episodeRow()], total: 1 } }
        : { data: { episodes: [], total: 0 } }
    )
    useStationNowPlaying.mockImplementation((slug: string) => ({
      data: { on_air: slug === 'wfmu' },
    }))
    render(<LatestRadioShows />)
    expect(
      screen.getByRole('link', {
        name: 'WFMU · Jersey City — live now — latest: Strength Through Failure with Stereolab, Broadcast, Pram. Open the station page.',
      })
    ).toHaveAttribute('href', '/radio/wfmu')
  })
})
