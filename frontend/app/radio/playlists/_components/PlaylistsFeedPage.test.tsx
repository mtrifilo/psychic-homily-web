import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { RadioStationEpisodeRow } from '@/features/radio'

const mockUseRecentRadioEpisodes = vi.fn()

vi.mock('@/features/radio', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/radio')>()
  return {
    ...actual,
    useRecentRadioEpisodes: (...args: unknown[]) =>
      mockUseRecentRadioEpisodes(...args),
  }
})

import PlaylistsFeedPage from './PlaylistsFeedPage'

function makeRow(
  overrides: Partial<RadioStationEpisodeRow> = {}
): RadioStationEpisodeRow {
  return {
    id: 1,
    title: null,
    air_date: '2026-06-09',
    starts_at: null,
    ends_at: null,
    play_count: 24,
    archive_url: null,
    show_id: 3,
    show_name: 'The Night Owl Show',
    show_slug: 'night-owl',
    station_id: 2,
    station_name: 'WFMU',
    station_slug: 'wfmu',
    artist_preview: [
      { artist_name: 'CAN', artist_id: 9, artist_slug: 'can' },
      { artist_name: "it's all meat", artist_id: null, artist_slug: null },
    ],
    ...overrides,
  }
}

function setEpisodes(
  episodes: RadioStationEpisodeRow[],
  total = episodes.length
) {
  mockUseRecentRadioEpisodes.mockReturnValue({
    data: { episodes, total },
    isLoading: false,
    isFetching: false,
    error: null,
  })
}

describe('PlaylistsFeedPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the breadcrumb, heading, and feed rows via the hub table', () => {
    setEpisodes([makeRow()], 574)
    render(<PlaylistsFeedPage />)

    expect(screen.getByRole('link', { name: 'Radio' })).toHaveAttribute(
      'href',
      '/radio'
    )
    expect(
      screen.getByRole('heading', { level: 1, name: 'All playlists' })
    ).toBeInTheDocument()

    // Row anatomy comes from the shared LatestPlaylistsTable.
    expect(
      screen.getByRole('link', { name: 'The Night Owl Show' })
    ).toHaveAttribute('href', '/radio/wfmu/night-owl')
    expect(screen.getByText('Jun 9')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'CAN' })).toHaveAttribute(
      'href',
      '/artists/can'
    )
  })

  it('grows the in-place limit on "More playlists" and reports the total', () => {
    setEpisodes([makeRow()], 574)
    render(<PlaylistsFeedPage />)

    expect(screen.getByText('showing 1 of 574 playlists')).toBeInTheDocument()
    expect(mockUseRecentRadioEpisodes).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 20 })
    )

    fireEvent.click(screen.getByRole('button', { name: 'More playlists' }))
    expect(mockUseRecentRadioEpisodes).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 40 })
    )
  })

  it('hides the load-more control once every playlist is shown', () => {
    setEpisodes([makeRow()], 1)
    render(<PlaylistsFeedPage />)

    expect(screen.getByText('showing 1 of 1 playlists')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'More playlists' })
    ).not.toBeInTheDocument()
  })

  it('hides the load-more control at the API limit cap of 100', () => {
    // 100 rows shown out of 574: more exist, but the endpoint caps limit.
    setEpisodes(
      Array.from({ length: 100 }, (_, i) => makeRow({ id: i + 1 })),
      574
    )
    render(<PlaylistsFeedPage />)

    // 20 → 40 → 60 → 80 → 100: four clicks reach the cap.
    for (let i = 0; i < 4; i++) {
      fireEvent.click(screen.getByRole('button', { name: 'More playlists' }))
    }
    expect(mockUseRecentRadioEpisodes).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 100 })
    )

    expect(screen.getByText('showing 100 of 574 playlists')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'More playlists' })
    ).not.toBeInTheDocument()
  })

  it('renders the shared error state when the feed fails', () => {
    mockUseRecentRadioEpisodes.mockReturnValue({
      data: undefined,
      isLoading: false,
      isFetching: false,
      error: new Error('boom'),
    })
    render(<PlaylistsFeedPage />)

    expect(
      screen.getByText("Couldn't load the latest playlists.")
    ).toBeInTheDocument()
    expect(screen.queryByText(/showing/)).not.toBeInTheDocument()
  })
})
