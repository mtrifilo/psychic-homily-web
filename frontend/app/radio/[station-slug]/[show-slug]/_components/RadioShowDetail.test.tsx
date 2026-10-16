import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'

const mockUseRadioShow = vi.fn()
const mockUseRadioStation = vi.fn()

vi.mock('@/features/radio', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/radio')>()
  return {
    ...actual,
    useRadioShow: (slug: string) => mockUseRadioShow(slug),
    useRadioStation: (slug: string) => mockUseRadioStation(slug),
    useRadioEpisodes: () => ({
      data: { episodes: [], total: 0 },
      isLoading: false,
      isPlaceholderData: false,
    }),
    useRadioTopArtists: () => ({ data: undefined, isLoading: false }),
    useRadioTopLabels: () => ({ data: undefined, isLoading: false }),
  }
})

import RadioShowDetail from './RadioShowDetail'

const SHOW = {
  id: 1,
  station_id: 9,
  station_name: 'Give the Drummer Radio',
  station_slug: 'wfmu-drummer',
  name: 'Wake',
  slug: 'wake',
  host_name: null,
  description: null,
  schedule_display: null,
  schedule: null,
  genre_tags: null,
  archive_url: null,
  image_url: null,
  is_active: true,
  episode_count: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

const DRUMMER_STATION = {
  slug: 'wfmu-drummer',
  network: { slug: 'wfmu', name: 'WFMU', is_flagship: false },
}

function backLink(stationName: string) {
  return screen.getByRole('link', { name: `← ${stationName}` })
}

describe('RadioShowDetail station links', () => {
  beforeEach(() => {
    mockUseRadioShow
      .mockReset()
      .mockReturnValue({ data: SHOW, isLoading: false, error: null })
    mockUseRadioStation.mockReset().mockReturnValue({ data: DRUMMER_STATION })
  })

  it("links a sub-stream show's station to its channel URL", () => {
    render(<RadioShowDetail stationSlug="wfmu-drummer" showSlug="wake" />)

    expect(mockUseRadioStation).toHaveBeenCalledWith('wfmu-drummer')
    expect(backLink('Give the Drummer Radio')).toHaveAttribute(
      'href',
      '/radio/wfmu/channel/drummer'
    )
  })

  it('links the requested station slug until the station read lands', () => {
    mockUseRadioStation.mockReturnValue({ data: undefined })

    render(<RadioShowDetail stationSlug="wfmu-drummer" showSlug="wake" />)

    expect(backLink('Give the Drummer Radio')).toHaveAttribute(
      'href',
      '/radio/wfmu-drummer'
    )
  })

  it('links the slug URL of a station addressed by numeric id', () => {
    mockUseRadioShow.mockReturnValue({
      data: { ...SHOW, station_name: 'KEXP', station_slug: 'kexp' },
      isLoading: false,
      error: null,
    })
    mockUseRadioStation.mockReturnValue({
      data: { slug: 'kexp', network: null },
    })

    render(<RadioShowDetail stationSlug="7" showSlug="wake" />)

    expect(backLink('KEXP')).toHaveAttribute('href', '/radio/kexp')
  })

  it("links the not-found state's back button to the channel URL", () => {
    mockUseRadioShow.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('not found'),
    })

    render(<RadioShowDetail stationSlug="wfmu-drummer" showSlug="missing" />)

    expect(
      screen.getByRole('link', { name: 'Back to Station' })
    ).toHaveAttribute('href', '/radio/wfmu/channel/drummer')
  })
})
