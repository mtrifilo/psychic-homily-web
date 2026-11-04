import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { installMatchMedia } from '@/test/mocks/matchMedia'
import type { RadioEpisodeDetail } from '@/features/radio'
import { localIso } from '@/features/radio/lib/localIso.testutil'
import { makeRadioPlay } from '@/features/radio/lib/radioPlay.testutil'

const mockUseRadioEpisode = vi.fn()
const mockUseEpisodeNeighbors = vi.fn()

vi.mock('@/features/radio', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/radio')>()
  return {
    ...actual,
    useRadioEpisode: (...args: unknown[]) => mockUseRadioEpisode(...args),
    useEpisodeNeighbors: (...args: unknown[]) => mockUseEpisodeNeighbors(...args),
  }
})

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

import EpisodeDateDetail from './EpisodeDateDetail'

function makeEpisode(overrides: Partial<RadioEpisodeDetail> = {}): RadioEpisodeDetail {
  return {
    id: 1,
    show_id: 3,
    show_name: 'The Night Owl Show',
    show_slug: 'night-owl',
    station_name: 'WFMU',
    station_slug: 'wfmu',
    title: null,
    air_date: '2026-06-08',
    air_time: '21:00',
    starts_at: null,
    ends_at: null,
    station_timezone: null,
    is_upcoming: false,
    duration_minutes: null,
    description: null,
    archive_url: null,
    mixcloud_url: null,
    genre_tags: null,
    mood_tags: null,
    play_count: 3,
    plays: [],
    created_at: '2026-06-08T00:00:00Z',
    ...overrides,
  }
}

function setEpisode(episode: RadioEpisodeDetail, dataUpdatedAt = 0) {
  mockUseRadioEpisode.mockReturnValue({
    data: episode,
    isLoading: false,
    error: null,
    dataUpdatedAt,
  })
  mockUseEpisodeNeighbors.mockReturnValue({ data: undefined })
}

/** A window covering `now` — the live regime (PSY-1511). */
function liveWindow() {
  return {
    starts_at: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
    ends_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
  }
}

const props = { stationSlug: 'wfmu', showSlug: 'night-owl', date: '2026-06-08' }

describe('EpisodeDateDetail (PSY-1306)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('keeps the H1 station-dated while the aired line is viewer-local (design decision 1)', () => {
    // Window's local day (Jun 9) deliberately differs from the station
    // air_date (Jun 8): the H1 must stay on the URL-keyed station date while
    // the aired line derives from the window.
    setEpisode(
      makeEpisode({
        starts_at: localIso(2026, 5, 9, 15),
        ends_at: localIso(2026, 5, 9, 18),
        station_timezone: 'Pacific/Kiritimati', // never the test machine's zone
      })
    )
    render(<EpisodeDateDetail {...props} />)

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('June 8, 2026')
    expect(screen.getByText(/aired Tue 3–6 PM your time \(/)).toBeInTheDocument()
  })

  it('falls back to the station-dated air_time line for windowless episodes', () => {
    setEpisode(makeEpisode())
    render(<EpisodeDateDetail {...props} />)
    // Jun 8 2026 is a Monday; 21:00 renders via formatTimeOfDay
    expect(screen.getByText(/aired Mon 9:00 PM/)).toBeInTheDocument()
  })

  it('says "airing" mid-window instead of claiming an in-progress show aired', () => {
    setEpisode(makeEpisode(liveWindow()))
    render(<EpisodeDateDetail {...props} />)
    expect(screen.getByText(/· airing .* your time/)).toBeInTheDocument()
  })
})

describe('EpisodeDateDetail live regime (PSY-1511)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // Matched plays (artist_id set); the match affordances themselves are
  // covered in PlaylistTable.test.tsx.
  const playFixture = makeRadioPlay({ artist_id: 5, artist_slug: 'can' })

  it('shows the ON AIR band with the updated-ago aside while the episode is live', () => {
    setEpisode(makeEpisode(liveWindow()), Date.now() - 40 * 1000)
    render(<EpisodeDateDetail {...props} />)
    expect(
      screen.getByText(/ON AIR NOW — the playlist is updating live/)
    ).toBeInTheDocument()
    // Seconds-granular but not exact-value: the fixture is Date.now()-relative
    // and the formatter reads its own clock, so pin the shape, not "40".
    expect(screen.getByText(/^updated \d+s ago$/)).toBeInTheDocument()
    expect(screen.getByText(/3 tracks so far/)).toBeInTheDocument()
    // A live episode with no plays yet gets the live waiting copy, not the
    // archive "No playlist data available" line under an ON AIR band.
    expect(screen.getByText(/Waiting for the first track/)).toBeInTheDocument()
    expect(
      screen.queryByText('No playlist data available for this episode')
    ).not.toBeInTheDocument()
  })

  it('renders the live ledger: newest-first with the ▸ now marker', () => {
    setEpisode(
      makeEpisode({
        ...liveWindow(),
        plays: [
          playFixture,
          { ...playFixture, id: 2, position: 2, artist_name: 'Neu!' },
        ],
      })
    )
    render(<EpisodeDateDetail {...props} />)
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows[0]).toHaveTextContent('▸ now')
    expect(rows[0]).toHaveTextContent('Neu!')
    expect(rows[1]).toHaveTextContent('CAN')
  })

  it('renders the archive page past ends_at: no band, chronological order', () => {
    setEpisode(
      makeEpisode({
        starts_at: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(),
        ends_at: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
        plays: [
          playFixture,
          { ...playFixture, id: 2, position: 2, artist_name: 'Neu!' },
        ],
      }),
      Date.now()
    )
    render(<EpisodeDateDetail {...props} />)
    expect(screen.queryByText(/ON AIR NOW/)).not.toBeInTheDocument()
    expect(screen.queryByText(/updated .* ago/)).not.toBeInTheDocument()
    expect(screen.queryByText(/so far/)).not.toBeInTheDocument()
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows[0]).toHaveTextContent('CAN')
    expect(rows[1]).toHaveTextContent('Neu!')
    expect(screen.queryByText('▸ now')).not.toBeInTheDocument()
  })

  it('keeps rendering cached data when a background poll errors (no false Not Found)', () => {
    // TanStack keeps `data` when a refetch fails: error and data coexist.
    // The ~60s live poll makes this state routine — it must not blank the
    // ledger to "Episode Not Found" over a transient blip.
    mockUseRadioEpisode.mockReturnValue({
      data: makeEpisode({ ...liveWindow(), plays: [playFixture] }),
      isLoading: false,
      error: new Error('transient poll failure'),
      dataUpdatedAt: Date.now() - 90 * 1000,
    })
    mockUseEpisodeNeighbors.mockReturnValue({ data: undefined })
    render(<EpisodeDateDetail {...props} />)
    expect(screen.queryByText('Episode Not Found')).not.toBeInTheDocument()
    expect(
      screen.getByText(/ON AIR NOW — the playlist is updating live/)
    ).toBeInTheDocument()
    expect(screen.getByText('Mother Sky')).toBeInTheDocument()
    // The poll stops on error and nothing restarts it in production — the
    // band must say so instead of counting a stale "updated Nm ago" up.
    expect(
      screen.getByText('live updates paused — reload to resume')
    ).toBeInTheDocument()
    expect(screen.queryByText(/^updated \d+/)).not.toBeInTheDocument()
  })

  it('still shows Not Found when the first fetch fails with no data', () => {
    mockUseRadioEpisode.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('404'),
      dataUpdatedAt: 0,
    })
    mockUseEpisodeNeighbors.mockReturnValue({ data: undefined })
    render(<EpisodeDateDetail {...props} />)
    expect(screen.getByText('Episode Not Found')).toBeInTheDocument()
  })

  it('leaves an upcoming episode untouched: "airs", no band (PSY-1205)', () => {
    setEpisode(
      makeEpisode({
        starts_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        ends_at: new Date(Date.now() + 3 * 60 * 60 * 1000).toISOString(),
        is_upcoming: true,
        play_count: 0,
        plays: [],
      })
    )
    render(<EpisodeDateDetail {...props} />)
    expect(screen.getByText(/airs .* your time/)).toBeInTheDocument()
    expect(screen.queryByText(/ON AIR NOW/)).not.toBeInTheDocument()
    expect(screen.queryByText(/so far/)).not.toBeInTheDocument()
  })

  // archive_url and mixcloud_url are written verbatim from a station's own feed,
  // so they are third-party strings reaching an href.
  it('renders both archive buttons for absolute http urls', () => {
    setEpisode(
      makeEpisode({
        archive_url: 'https://example.com/ep.mp3',
        mixcloud_url: 'https://www.mixcloud.com/x/y/',
      })
    )
    render(<EpisodeDateDetail {...props} />)
    expect(screen.getByRole('link', { name: /play archive/i })).toHaveAttribute(
      'href',
      'https://example.com/ep.mp3'
    )
    expect(screen.getByRole('link', { name: /mixcloud/i })).toHaveAttribute(
      'href',
      'https://www.mixcloud.com/x/y/'
    )
  })

  it.each([
    ['javascript:alert(1)'],
    ['data:text/html,<script>alert(1)</script>'],
    ['not a url'],
  ])('renders no archive button for a value the gate refuses (%s)', url => {
    setEpisode(makeEpisode({ archive_url: url, mixcloud_url: url }))
    render(<EpisodeDateDetail {...props} />)
    expect(
      screen.queryByRole('link', { name: /play archive/i })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /mixcloud/i })).not.toBeInTheDocument()
  })
})

describe('EpisodeDateDetail arrival at a #play-{id} anchor', () => {
  const REDUCED_MOTION = '(prefers-reduced-motion: reduce)'
  const scrollIntoView = vi.mocked(Element.prototype.scrollIntoView)
  let restoreMatchMedia: (() => void) | null = null

  const episodeWithPlays = () =>
    makeEpisode({
      plays: [
        makeRadioPlay({ id: 11, position: 1 }),
        makeRadioPlay({ id: 12, position: 2, artist_name: 'Neu!' }),
      ],
    })

  const anchoredRow = () => document.querySelector('[aria-current="true"]')

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    restoreMatchMedia?.()
    restoreMatchMedia = null
    window.history.replaceState(null, '', '/')
  })

  it('highlights the anchored row and scrolls it to the center', () => {
    window.history.replaceState(null, '', '/radio/wfmu/night-owl/2026-06-08#play-12')
    setEpisode(episodeWithPlays())
    render(<EpisodeDateDetail {...props} />)

    const row = document.getElementById('play-12')
    expect(anchoredRow()).toBe(row)
    expect(row?.className).toContain('bg-primary/5')
    expect(scrollIntoView).toHaveBeenCalledTimes(1)
    expect(scrollIntoView).toHaveBeenCalledWith({ block: 'center', behavior: 'smooth' })
    expect(scrollIntoView.mock.contexts[0]).toBe(row)
  })

  it('scrolls instantly under reduced motion', () => {
    restoreMatchMedia = installMatchMedia({ [REDUCED_MOTION]: true }).restore
    window.history.replaceState(null, '', '/radio/wfmu/night-owl/2026-06-08#play-12')
    setEpisode(episodeWithPlays())
    render(<EpisodeDateDetail {...props} />)

    expect(scrollIntoView).toHaveBeenLastCalledWith({ block: 'center', behavior: 'instant' })
  })

  it('renders the plain playlist when the anchored play is not on this episode', () => {
    window.history.replaceState(null, '', '/radio/wfmu/night-owl/2026-06-08#play-999')
    setEpisode(episodeWithPlays())
    render(<EpisodeDateDetail {...props} />)

    expect(screen.getByText('Neu!')).toBeInTheDocument()
    expect(anchoredRow()).toBeNull()
    expect(scrollIntoView).not.toHaveBeenCalled()
  })

  it('does not re-scroll when a poll re-renders the same anchored playlist', () => {
    window.history.replaceState(null, '', '/radio/wfmu/night-owl/2026-06-08#play-12')
    setEpisode(episodeWithPlays())
    const { rerender } = render(<EpisodeDateDetail {...props} />)
    setEpisode(episodeWithPlays(), 60_000)
    rerender(<EpisodeDateDetail {...props} />)

    expect(anchoredRow()).toBe(document.getElementById('play-12'))
    expect(scrollIntoView).toHaveBeenCalledTimes(1)
  })

  it('clears the highlight on the next navigation', () => {
    window.history.replaceState(null, '', '/radio/wfmu/night-owl/2026-06-08#play-12')
    setEpisode(episodeWithPlays())
    const { rerender } = render(<EpisodeDateDetail {...props} />)
    expect(anchoredRow()).not.toBeNull()

    // A client navigation to the next episode: pushState, no hash, no event.
    window.history.pushState(null, '', '/radio/wfmu/night-owl/2026-06-15')
    setEpisode(episodeWithPlays())
    rerender(<EpisodeDateDetail {...props} date="2026-06-15" />)

    expect(anchoredRow()).toBeNull()
  })

  it('follows a hash change to another play on the page', () => {
    window.history.replaceState(null, '', '/radio/wfmu/night-owl/2026-06-08#play-12')
    setEpisode(episodeWithPlays())
    render(<EpisodeDateDetail {...props} />)

    window.history.replaceState(null, '', '/radio/wfmu/night-owl/2026-06-08#play-11')
    act(() => {
      window.dispatchEvent(new HashChangeEvent('hashchange'))
    })

    expect(anchoredRow()).toBe(document.getElementById('play-11'))
    expect(scrollIntoView.mock.contexts.at(-1)).toBe(document.getElementById('play-11'))
  })
})
