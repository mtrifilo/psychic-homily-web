import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { SeriesComparison } from '../types'

const mockUseSeries = vi.fn()
vi.mock('../hooks/useFestivals', () => ({
  useSeriesComparison: (opts: unknown) => mockUseSeries(opts),
}))

import { SeriesHistory } from './SeriesHistory'

function makeComparison(
  overrides: Partial<SeriesComparison> = {}
): SeriesComparison {
  return {
    series_slug: 'form-arcosanti',
    editions: [
      { festival_id: 10, name: 'FORM 2023', slug: 'form-2023', year: 2023, artist_count: 30 },
      { festival_id: 11, name: 'FORM 2024', slug: 'form-2024', year: 2024, artist_count: 40 },
    ],
    returning_artists: [
      {
        artist: { id: 1, name: 'Returner', slug: 'returner' },
        years: [2023, 2024],
        tiers: { '2023': 'undercard', '2024': 'mid_card' },
      },
    ],
    newcomers: [
      { artist: { id: 2, name: 'Newbie', slug: 'newbie' }, tier: 'local' },
    ],
    retention_rate: 0.5,
    lineup_growth: 0.33,
    ...overrides,
  }
}

describe('SeriesHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when fewer than 2 editions exist', () => {
    mockUseSeries.mockReturnValue({ data: undefined, isLoading: false })
    const { container } = render(
      <SeriesHistory seriesSlug="form-arcosanti" editions={[{ year: 2024 }]} />
    )
    expect(container.firstChild).toBeNull()
  })

  it('disables the hook until at least two years are present', () => {
    mockUseSeries.mockReturnValue({ data: undefined, isLoading: false })
    render(
      <SeriesHistory
        seriesSlug="form-arcosanti"
        editions={[{ year: 2023 }, { year: 2024 }]}
      />
    )
    expect(mockUseSeries).toHaveBeenCalledWith({
      seriesSlug: 'form-arcosanti',
      years: [2023, 2024],
      enabled: true,
    })
  })

  it('shows a loading spinner while fetching with enough years', () => {
    mockUseSeries.mockReturnValue({ data: undefined, isLoading: true })
    const { container } = render(
      <SeriesHistory
        seriesSlug="form-arcosanti"
        editions={[{ year: 2023 }, { year: 2024 }]}
      />
    )
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders nothing when the comparison data is absent', () => {
    mockUseSeries.mockReturnValue({ data: null, isLoading: false })
    const { container } = render(
      <SeriesHistory
        seriesSlug="form-arcosanti"
        editions={[{ year: 2023 }, { year: 2024 }]}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders editions, metrics, returning artists, and newcomers', () => {
    mockUseSeries.mockReturnValue({ data: makeComparison(), isLoading: false })
    render(
      <SeriesHistory
        seriesSlug="form-arcosanti"
        editions={[{ year: 2023 }, { year: 2024 }]}
      />
    )

    // Edition links
    const editionLink = screen.getByRole('link', { name: /2023/ })
    expect(editionLink).toHaveAttribute('href', '/festivals/form-2023')

    // Metrics formatting
    expect(screen.getByText('50%')).toBeInTheDocument()
    expect(screen.getByText('+33%')).toBeInTheDocument()

    // Returning artists header reflects the count and lists the artist
    expect(screen.getByText('Returning Artists (1)')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Returner' })).toBeInTheDocument()

    // Newcomers header uses the latest year, and lists the newcomer
    expect(screen.getByText('2024 Newcomers (1)')).toBeInTheDocument()
    expect(screen.getByText('Newbie')).toBeInTheDocument()
  })

  it('prefixes negative lineup growth with no plus sign', () => {
    mockUseSeries.mockReturnValue({
      data: makeComparison({ lineup_growth: -0.2 }),
      isLoading: false,
    })
    render(
      <SeriesHistory
        seriesSlug="form-arcosanti"
        editions={[{ year: 2023 }, { year: 2024 }]}
      />
    )
    expect(screen.getByText('-20%')).toBeInTheDocument()
  })
})
