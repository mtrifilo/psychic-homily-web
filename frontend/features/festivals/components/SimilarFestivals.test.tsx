import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { SimilarFestivalsResponse } from '../types'

const mockUseSimilar = vi.fn()
vi.mock('../hooks/useFestivals', () => ({
  useSimilarFestivals: (opts: unknown) => mockUseSimilar(opts),
}))

import { SimilarFestivals } from './SimilarFestivals'

function makeSimilar(
  overrides: Partial<SimilarFestivalsResponse> = {}
): SimilarFestivalsResponse {
  return {
    similar: [
      {
        festival: {
          id: 2,
          name: 'Desert Daze',
          slug: 'desert-daze-2025',
          series_slug: 'desert-daze',
          edition_year: 2025,
          city: 'Lake Perris',
          state: 'CA',
        },
        shared_artist_count: 4,
        jaccard: 0.123,
        weighted_score: 0.5,
        top_shared: [
          {
            artist_id: 7,
            name: 'Shared Act',
            slug: 'shared-act',
            tier_at_source: 'mid_card',
            tier_at_target: 'headliner',
          },
        ],
      },
    ],
    ...overrides,
  }
}

describe('SimilarFestivals', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('requests a limit of 5 from the hook', () => {
    mockUseSimilar.mockReturnValue({ data: undefined, isLoading: true })
    render(<SimilarFestivals festivalIdOrSlug={1} />)
    expect(mockUseSimilar).toHaveBeenCalledWith({
      festivalIdOrSlug: 1,
      limit: 5,
      enabled: true,
    })
  })

  it('shows a loading spinner while fetching', () => {
    mockUseSimilar.mockReturnValue({ data: undefined, isLoading: true })
    const { container } = render(<SimilarFestivals festivalIdOrSlug={1} />)
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders nothing when there are no similar festivals', () => {
    mockUseSimilar.mockReturnValue({
      data: { similar: [] },
      isLoading: false,
    })
    const { container } = render(<SimilarFestivals festivalIdOrSlug={1} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders similar festivals with overlap stats and shared artists', () => {
    mockUseSimilar.mockReturnValue({ data: makeSimilar(), isLoading: false })
    render(<SimilarFestivals festivalIdOrSlug={1} />)

    expect(screen.getByText('Similar Festivals')).toBeInTheDocument()
    const festivalLink = screen.getByRole('link', { name: 'Desert Daze' })
    expect(festivalLink).toHaveAttribute('href', '/festivals/desert-daze-2025')
    expect(screen.getByText('4 shared')).toBeInTheDocument()
    expect(screen.getByText('12.3% overlap')).toBeInTheDocument()
    // shared artist badge links to the artist with the target-tier label
    expect(screen.getByText('Shared Act')).toBeInTheDocument()
    expect(screen.getByText('Headliner')).toBeInTheDocument()
  })
})
