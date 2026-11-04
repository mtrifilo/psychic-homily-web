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
    render(<LatestRadioShows variant="cards" />)
    expect(
      screen.getByRole('heading', { name: /latest radio shows/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /browse all stations/i })
    ).toHaveAttribute('href', '/radio')
  })

  it('renders the three editorial station cards deep-linked to their /radio station tabs', () => {
    render(<LatestRadioShows variant="cards" />)
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
    render(<LatestRadioShows variant="cards" />)
    expect(screen.getByText('Strength Through Failure')).toBeInTheDocument()
    expect(screen.getByText('Stereolab · Broadcast · Pram')).toBeInTheDocument()
    expect(screen.queryByText(/Fourth Artist Never Shown/)).not.toBeInTheDocument()
  })

  it('degrades gracefully with no episode data: editorial shell only, no fictional content', () => {
    useStationEpisodes.mockReturnValue({ data: { episodes: [], total: 0 } })
    render(<LatestRadioShows variant="cards" />)
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
    render(<LatestRadioShows variant="cards" />)
    expect(screen.getAllByText('live')).toHaveLength(1)
    expect(screen.getByText('live').closest('a')).toHaveAttribute('href', '/radio/kexp')
  })

  it('shows no live dot while now-playing data is absent', () => {
    render(<LatestRadioShows variant="cards" />)
    expect(screen.queryByText('live')).not.toBeInTheDocument()
  })

  it('degrades to the editorial shell on a fetch ERROR (e.g. a renamed slug 404s)', () => {
    useStationEpisodes.mockReturnValue({ data: undefined, error: new Error('404') })
    useStationNowPlaying.mockReturnValue({ data: undefined, error: new Error('404') })
    render(<LatestRadioShows variant="cards" />)
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
    render(<LatestRadioShows variant="cards" />)
    expect(
      screen.getByRole('link', {
        name: 'WFMU · Jersey City — live now — latest: Strength Through Failure with Stereolab, Broadcast, Pram. Open the station page.',
      })
    ).toHaveAttribute('href', '/radio/wfmu')
  })

  describe('rows variant (signed-in home)', () => {
    function rowFor(name: RegExp) {
      return screen.getByRole('link', { name })
    }

    it('renders an "All stations" header link and three rows, no cards', () => {
      render(<LatestRadioShows variant="rows" />)
      expect(
        screen.getByRole('heading', { name: /latest radio shows/i })
      ).toBeInTheDocument()
      expect(screen.getByRole('link', { name: 'All stations →' })).toHaveAttribute(
        'href',
        '/radio'
      )
      expect(screen.queryByRole('link', { name: /browse all stations/i })).toBeNull()
      expect(screen.getAllByRole('listitem')).toHaveLength(3)
      expect(rowFor(/^KEXP, Seattle/)).toHaveAttribute('href', '/radio/kexp')
      expect(rowFor(/^WFMU, Jersey City/)).toHaveAttribute('href', '/radio/wfmu')
      expect(rowFor(/^NTS, London/)).toHaveAttribute('href', '/radio/nts-radio')
    })

    it('shows "station · show" over "city · live now" with a live dot when on air', () => {
      useStationEpisodes.mockImplementation(({ stationSlug }: { stationSlug: string }) =>
        stationSlug === 'wfmu'
          ? { data: { episodes: [episodeRow()], total: 1 } }
          : { data: { episodes: [], total: 0 } }
      )
      useStationNowPlaying.mockImplementation((slug: string) => ({
        data: { on_air: slug === 'wfmu' },
      }))
      render(<LatestRadioShows variant="rows" />)
      const wfmu = rowFor(/^WFMU · Strength Through Failure/)
      expect(wfmu).toHaveTextContent('WFMU · Strength Through Failure')
      expect(wfmu).toHaveTextContent('Jersey City · live now')
      expect(wfmu.querySelector('[data-live]')).not.toBeNull()
      expect(rowFor(/^KEXP, Seattle/).querySelector('[data-live]')).toBeNull()
      // The name opens with the visible text in visual order (WCAG 2.5.3);
      // the artists the row does not show follow it.
      expect(screen.queryByText('Stereolab · Broadcast · Pram')).toBeNull()
      expect(wfmu).toHaveAccessibleName(
        'WFMU · Strength Through Failure, Jersey City · live now. Latest show: Strength Through Failure with Stereolab, Broadcast, Pram. Open the station page.'
      )
    })

    it('keeps the "Latest show" qualifier when the episode has no artists, and omits it with no episode', () => {
      useStationEpisodes.mockImplementation(({ stationSlug }: { stationSlug: string }) =>
        stationSlug === 'wfmu'
          ? { data: { episodes: [episodeRow({ artist_preview: [] })], total: 1 } }
          : { data: { episodes: [], total: 0 } }
      )
      useStationNowPlaying.mockReturnValue({ data: { on_air: true } })
      render(<LatestRadioShows variant="rows" />)
      expect(rowFor(/^WFMU · Strength Through Failure/)).toHaveAccessibleName(
        'WFMU · Strength Through Failure, Jersey City · live now. Latest show: Strength Through Failure. Open the station page.'
      )
      expect(rowFor(/^KEXP, Seattle/)).toHaveAccessibleName(
        'KEXP, Seattle · live now. Open the station page.'
      )
    })

    it('falls back to "city · vibe" off air and the call sign alone with no episode', () => {
      useStationEpisodes.mockReturnValue({ data: { episodes: [], total: 0 } })
      render(<LatestRadioShows variant="rows" />)
      const kexp = rowFor(/^KEXP, Seattle/)
      expect(kexp).toHaveTextContent(/^KEXP\s*Seattle · Eclectic host’s-choice$/)
      expect(kexp).toHaveAccessibleName(
        'KEXP, Seattle · Eclectic host’s-choice. Open the station page.'
      )
      expect(kexp).not.toHaveTextContent(/live now/)
      expect(kexp.querySelector('[data-live]')).toBeNull()
    })

    it('degrades to the editorial shell on a fetch ERROR', () => {
      useStationEpisodes.mockReturnValue({ data: undefined, error: new Error('404') })
      useStationNowPlaying.mockReturnValue({ data: undefined, error: new Error('404') })
      render(<LatestRadioShows variant="rows" />)
      expect(rowFor(/^NTS, London/)).toHaveTextContent('London · Global, two live channels')
      expect(screen.queryByText(/live now/)).toBeNull()
    })
  })
})
