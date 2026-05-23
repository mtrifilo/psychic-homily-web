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

  it('renders the As Heard On header and show link for an artist', () => {
    mockUseArtistRadioPlays.mockReturnValue({ data: response(), isLoading: false })
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" />)

    expect(screen.getByText('As Heard On')).toBeInTheDocument()
    const link = screen.getByText('Morning Show').closest('a')
    expect(link).toHaveAttribute('href', '/radio/kexp/morning-show')
  })

  it('renders the station name and pluralized play count', () => {
    mockUseArtistRadioPlays.mockReturnValue({ data: response(), isLoading: false })
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" />)
    expect(screen.getByText(/KEXP - 3 plays/)).toBeInTheDocument()
  })

  it('singularizes a single play', () => {
    const data = response()
    data.stations[0].play_count = 1
    mockUseArtistRadioPlays.mockReturnValue({ data, isLoading: false })
    render(<AsHeardOn entityType="artist" entitySlug="gatecreeper" />)
    expect(screen.getByText(/KEXP - 1 play$/)).toBeInTheDocument()
  })

  it('selects the release query when entityType is release', () => {
    mockUseReleaseRadioPlays.mockReturnValue({ data: response(), isLoading: false })
    render(<AsHeardOn entityType="release" entitySlug="an-unkindness" />)
    expect(screen.getByText('As Heard On')).toBeInTheDocument()
    expect(screen.getByText('Morning Show')).toBeInTheDocument()
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
