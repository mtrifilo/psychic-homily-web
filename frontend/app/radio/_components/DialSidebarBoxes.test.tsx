import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { NewReleaseRadarBox, DialStatsBox } from './DialSidebarBoxes'
import type { RadioNewReleaseRadarEntry, RadioStats } from '@/features/radio'

function makeEntry(
  overrides: Partial<RadioNewReleaseRadarEntry> = {}
): RadioNewReleaseRadarEntry {
  return {
    artist_name: 'Wet Leg',
    artist_id: 4,
    artist_slug: 'wet-leg',
    album_title: 'Moisturizer',
    label_name: 'Domino',
    release_id: 8,
    release_slug: 'wet-leg-moisturizer',
    label_id: 2,
    label_slug: 'domino',
    play_count: 24,
    station_count: 2,
    ...overrides,
  }
}

describe('NewReleaseRadarBox', () => {
  it('renders nothing when there are no releases', () => {
    const { container } = render(
      <NewReleaseRadarBox releases={[]} isLoading={false} />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('links a matched release to its release page with a mono subline', () => {
    render(<NewReleaseRadarBox releases={[makeEntry()]} isLoading={false} />)

    expect(screen.getByText('New release radar')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Wet Leg — Moisturizer' })
    ).toHaveAttribute('href', '/releases/wet-leg-moisturizer')
    expect(screen.getByText('Domino · 24 plays · 2 stations')).toBeInTheDocument()
  })

  it('falls back to the artist page, then plain text (no dead links)', () => {
    render(
      <NewReleaseRadarBox
        releases={[
          makeEntry({ release_slug: null }),
          makeEntry({
            artist_name: 'Florry',
            album_title: 'Sounds Like...',
            artist_slug: null,
            release_slug: null,
          }),
        ]}
        isLoading={false}
      />
    )

    expect(
      screen.getByRole('link', { name: 'Wet Leg — Moisturizer' })
    ).toHaveAttribute('href', '/artists/wet-leg')

    expect(screen.getByText('Florry — Sounds Like...')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Florry — Sounds Like...' })
    ).not.toBeInTheDocument()
  })
})

describe('DialStatsBox', () => {
  const stats: RadioStats = {
    total_stations: 6,
    total_shows: 2341,
    total_episodes: 1725,
    total_plays: 19071,
    matched_plays: 8210,
    unique_artists: 1148,
  }

  it('renders nothing without stats', () => {
    const { container } = render(<DialStatsBox stats={undefined} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders lifetime stats with honest all-time labels', () => {
    render(<DialStatsBox stats={stats} />)

    expect(screen.getByText('On the dial — all time')).toBeInTheDocument()
    expect(screen.getByText('playlists tracked')).toBeInTheDocument()
    expect(screen.getByText('1,725')).toBeInTheDocument()
    expect(screen.getByText('plays logged')).toBeInTheDocument()
    expect(screen.getByText('19,071')).toBeInTheDocument()
    expect(screen.getByText('plays matched')).toBeInTheDocument()
    expect(screen.getByText('unique artists')).toBeInTheDocument()
    // No "this week" claims — weekly windowed stats were not built.
    expect(screen.queryByText(/this week/i)).not.toBeInTheDocument()
  })
})
