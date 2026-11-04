import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
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

  it('links an artist row to the latest episode it was played on', () => {
    mockUseArtistRadioPlays.mockReturnValue({ data: response(), isLoading: false })
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" />)

    expect(screen.getByText('As heard on')).toBeInTheDocument()
    const link = screen.getByText('Morning Show').closest('a')
    expect(link).toHaveAttribute('href', '/radio/kexp/morning-show/2026-05-01')
  })

  it('accepts a date-only last_played', () => {
    const data = response()
    data.stations[0].last_played = '2026-05-01'
    mockUseArtistRadioPlays.mockReturnValue({ data, isLoading: false })
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" />)
    expect(screen.getByText('Morning Show').closest('a')).toHaveAttribute(
      'href',
      '/radio/kexp/morning-show/2026-05-01'
    )
  })

  it('renders the station, pluralized play count and last-played date', () => {
    mockUseArtistRadioPlays.mockReturnValue({ data: response(), isLoading: false })
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" />)
    const link = screen.getByText('Morning Show').closest('a')
    expect(link).toHaveTextContent('KEXP · 3 plays · last on May 1')
    expect(link?.textContent).not.toMatch(/[\u2013\u2014]/)
    expect(screen.getByText('May 1')).toHaveClass('font-data')
  })

  it('singularizes a single play', () => {
    const data = response()
    data.stations[0].play_count = 1
    mockUseArtistRadioPlays.mockReturnValue({ data, isLoading: false })
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" />)
    expect(screen.getByText('Morning Show').closest('a')).toHaveTextContent(
      'KEXP · 1 play · last on May 1'
    )
  })

  it('falls back to the show page and names no date without last_played', () => {
    const data = response()
    data.stations[0].last_played = ''
    mockUseArtistRadioPlays.mockReturnValue({ data, isLoading: false })
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" />)
    const link = screen.getByText('Morning Show').closest('a')
    expect(link).toHaveAttribute('href', '/radio/kexp/morning-show')
    expect(link?.textContent).toMatch(/KEXP · 3 plays$/)
  })

  it('selects the release query when entityType is release', () => {
    mockUseReleaseRadioPlays.mockReturnValue({ data: response(), isLoading: false })
    render(<AsHeardOn entityType="release" entitySlug="an-unkindness" />)
    expect(screen.getByText('As heard on')).toBeInTheDocument()
    expect(screen.getByText('Morning Show').closest('a')).toHaveAttribute(
      'href',
      '/radio/kexp/morning-show/2026-05-01'
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
