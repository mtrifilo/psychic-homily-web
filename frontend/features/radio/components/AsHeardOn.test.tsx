import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { RadioAsHeardOnResponse } from '../types'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

// AsHeardOn always calls both hooks (gated by `enabled`), then selects one by
// entityType. Mock both so each branch is independently controllable.
const mockUseArtistRadioPlays = vi.fn()
const mockUseReleaseRadioPlays = vi.fn()
vi.mock('../hooks/useArtistRadioPlays', () => ({
  useArtistRadioPlays: (...args: unknown[]) => mockUseArtistRadioPlays(...args),
}))
vi.mock('../hooks/useReleaseRadioPlays', () => ({
  useReleaseRadioPlays: (...args: unknown[]) => mockUseReleaseRadioPlays(...args),
}))

import { AsHeardOn } from './AsHeardOn'

function response(): RadioAsHeardOnResponse {
  return {
    stations: [
      {
        station_id: 1,
        station_name: 'KEXP',
        station_slug: 'kexp',
        show_id: 10,
        show_name: 'Morning Show',
        show_slug: 'morning-show',
        play_count: 3,
        last_played: '2026-05-01T00:00:00Z',
        latest_play_id: 77,
        latest_play_position: 4,
        recent_plays: [
          { air_date: '2026-05-01', play_id: 77, position: 4, track_title: 'Sunflower' },
          { air_date: '2026-04-24', play_id: 61, position: 9, track_title: null },
          { air_date: '2026-04-17', play_id: 40, position: 2, track_title: 'Words' },
        ],
      },
    ],
    count: 1,
  }
}

const empty = () => ({ data: { stations: [] as unknown[], count: 0 }, isLoading: false })
const loading = () => ({ data: undefined as unknown, isLoading: true })

describe('AsHeardOn', () => {
  beforeEach(() => {
    mockUseArtistRadioPlays.mockReset()
    mockUseReleaseRadioPlays.mockReset()
    mockUseArtistRadioPlays.mockReturnValue(empty())
    mockUseReleaseRadioPlays.mockReturnValue(empty())
  })

  /** Renders one artist row from `data` and returns its link. */
  function renderArtistRow(data = response()) {
    mockUseArtistRadioPlays.mockReturnValue({ data, isLoading: false })
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" />)
    return screen.getByText('Morning Show').closest('a')
  }

  it('links an artist row to the latest play on its episode playlist', () => {
    const link = renderArtistRow()
    expect(screen.getByText('As heard on')).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/radio/kexp/morning-show/2026-05-01#play-77')
  })

  it('links the episode without an anchor when no play is listed', () => {
    const data = response()
    data.stations[0].recent_plays = null
    expect(renderArtistRow(data)).toHaveAttribute('href', '/radio/kexp/morning-show/2026-05-01')
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('renders the station, pluralized play count and last-played date', () => {
    const link = renderArtistRow()!
    expect(link).toHaveTextContent('KEXP · 3 plays · last on May 1')
    expect(within(link).getByText('May 1')).toHaveClass('font-data')
  })

  it('singularizes a single play and offers no disclosure', () => {
    const data = response()
    data.stations[0].play_count = 1
    data.stations[0].recent_plays = data.stations[0].recent_plays!.slice(0, 1)
    expect(renderArtistRow(data)).toHaveTextContent('KEXP · 1 play · last on May 1')
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('falls back to the show page and names no date without last_played', () => {
    const data = response()
    data.stations[0].last_played = ''
    const link = renderArtistRow(data)
    expect(link).toHaveAttribute('href', '/radio/kexp/morning-show')
    expect(link?.textContent).toMatch(/KEXP · 3 plays$/)
  })

  it('discloses every recent play of a multi-play row as its own deep link', async () => {
    const user = userEvent.setup()
    renderArtistRow()

    const toggle = screen.getByRole('button', { name: 'Plays on Morning Show' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    const list = document.getElementById(toggle.getAttribute('aria-controls')!)
    expect(list).not.toBeVisible()

    await user.click(toggle)

    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(list).toBeVisible()
    const links = within(list!).getAllByRole('link')
    expect(links.map(a => a.getAttribute('href'))).toEqual([
      '/radio/kexp/morning-show/2026-05-01#play-77',
      '/radio/kexp/morning-show/2026-04-24#play-61',
      '/radio/kexp/morning-show/2026-04-17#play-40',
    ])
    expect(links[0]).toHaveTextContent('May 1Sunflower')
    expect(links[0]).toHaveClass('text-xs')
    expect(within(links[0]).getByText('May 1')).toHaveClass('font-data', 'text-link')
    // A play with no track title is still addressable by its date.
    expect(links[1]).toHaveTextContent(/^Apr 24$/)

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(list).not.toBeVisible()
  })

  it('selects the release query when entityType is release', () => {
    mockUseReleaseRadioPlays.mockReturnValue({ data: response(), isLoading: false })
    render(<AsHeardOn entityType="release" entitySlug="an-unkindness" />)
    expect(screen.getByText('As heard on')).toBeInTheDocument()
    expect(screen.getByText('Morning Show').closest('a')).toHaveAttribute(
      'href',
      '/radio/kexp/morning-show/2026-05-01#play-77'
    )
  })

  it('renders nothing while the active query is loading', () => {
    mockUseArtistRadioPlays.mockReturnValue(loading())
    const { container } = render(
      <AsHeardOn entityType="artist" entitySlug="gatecreeper" />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when there are no plays', () => {
    mockUseArtistRadioPlays.mockReturnValue(empty())
    const { container } = render(
      <AsHeardOn entityType="artist" entitySlug="gatecreeper" />
    )
    expect(container.firstChild).toBeNull()
  })

  it('gates the artist hook on entityType and enabled', () => {
    mockUseArtistRadioPlays.mockReturnValue(empty())
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" enabled={false} />)
    // enabled passed through as `enabled && entityType === 'artist'`.
    expect(mockUseArtistRadioPlays).toHaveBeenCalledWith('gatecreeper', false)
    expect(mockUseReleaseRadioPlays).toHaveBeenCalledWith('gatecreeper', false)
  })

  it('enables only the matching hook for the entity type', () => {
    mockUseReleaseRadioPlays.mockReturnValue(empty())
    render(<AsHeardOn entityType="release" entitySlug="an-unkindness" />)
    expect(mockUseArtistRadioPlays).toHaveBeenCalledWith('an-unkindness', false)
    expect(mockUseReleaseRadioPlays).toHaveBeenCalledWith('an-unkindness', true)
  })
})
