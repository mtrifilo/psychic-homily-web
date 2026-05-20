import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { FestivalsListResponse } from '../types'

// next/navigation
const mockPush = vi.fn()
const mockGet = vi.fn<(key: string) => string | null>()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => ({ get: mockGet }),
}))

// Data hook
const mockUseFestivals = vi.fn()
vi.mock('../hooks/useFestivals', () => ({
  useFestivals: (opts: unknown) => mockUseFestivals(opts),
}))

// Density preference
vi.mock('@/lib/hooks/common/useDensity', () => ({
  useDensity: () => ({ density: 'comfortable', setDensity: vi.fn() }),
}))

// Tag faceting (pulls in tag query infra otherwise)
vi.mock('@/features/tags', () => ({
  TagFacetPanel: () => <div data-testid="tag-facet-panel" />,
  TagFacetSheet: () => <div data-testid="tag-facet-sheet" />,
  parseTagsParam: (raw: string | null) =>
    raw ? raw.split(',').filter(Boolean) : [],
  buildTagsParam: (tags: string[]) => tags.join(','),
}))

import { FestivalList } from './FestivalList'

function makeFestival(
  id: number,
  name: string
): FestivalsListResponse['festivals'][number] {
  return {
    id,
    name,
    slug: `${name.toLowerCase().replace(/\s+/g, '-')}-2025`,
    series_slug: name.toLowerCase().replace(/\s+/g, '-'),
    edition_year: 2025,
    city: 'Phoenix',
    state: 'AZ',
    start_date: '2025-05-09',
    end_date: '2025-05-11',
    status: 'confirmed',
    artist_count: 5,
    venue_count: 1,
  }
}

describe('FestivalList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockReturnValue(null)
  })

  it('shows the loading spinner on the initial load', () => {
    mockUseFestivals.mockReturnValue({
      data: undefined,
      isLoading: true,
      isFetching: true,
      error: null,
      refetch: vi.fn(),
    })
    const { container } = render(<FestivalList />)
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders an error state with a retry button that refetches', async () => {
    const refetch = vi.fn()
    mockUseFestivals.mockReturnValue({
      data: undefined,
      isLoading: false,
      isFetching: false,
      error: new Error('nope'),
      refetch,
    })
    render(<FestivalList />)

    expect(
      screen.getByText('Failed to load festivals. Please try again later.')
    ).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })

  it('renders the empty state when no festivals exist', () => {
    mockUseFestivals.mockReturnValue({
      data: { festivals: [], count: 0 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
    render(<FestivalList />)
    expect(
      screen.getByText('No festivals available at this time.')
    ).toBeInTheDocument()
  })

  it('renders festival cards and a count for a populated list', () => {
    mockUseFestivals.mockReturnValue({
      data: {
        festivals: [makeFestival(1, 'FORM'), makeFestival(2, 'Desert Daze')],
        count: 2,
      },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
    render(<FestivalList />)

    expect(screen.getByTestId('festival-count')).toHaveTextContent(
      '2 festivals'
    )
    expect(screen.getByRole('link', { name: 'FORM' })).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Desert Daze' })
    ).toBeInTheDocument()
  })

  it('pushes a status filter to the URL when a status chip is clicked', async () => {
    mockUseFestivals.mockReturnValue({
      data: { festivals: [makeFestival(1, 'FORM')], count: 1 },
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn(),
    })
    render(<FestivalList />)

    await userEvent.click(screen.getByRole('button', { name: 'Confirmed' }))
    expect(mockPush).toHaveBeenCalledWith('/festivals?status=confirmed', {
      scroll: false,
    })
  })
})
