import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PercentileRankings } from './PercentileRankings'
import type { PercentileRankings as PercentileRankingsType } from '@/features/auth'

// Mock hooks
const mockUsePercentileRankings = vi.fn()

vi.mock('@/features/auth', () => ({
  usePercentileRankings: (username: string) =>
    mockUsePercentileRankings(username),
}))

function makeRankings(
  overrides: Partial<PercentileRankingsType> = {}
): PercentileRankingsType {
  return {
    rankings: [
      {
        dimension: 'shows_submitted',
        label: 'Shows Submitted',
        percentile: 80,
        value: 15,
      },
      {
        dimension: 'venues_submitted',
        label: 'Venues Submitted',
        percentile: 60,
        value: 5,
      },
      {
        dimension: 'tags_applied',
        label: 'Tags Applied',
        percentile: 40,
        value: 8,
      },
      {
        dimension: 'edits_approved',
        label: 'Edits Approved',
        percentile: 20,
        value: 2,
      },
      {
        dimension: 'requests_fulfilled',
        label: 'Requests Fulfilled',
        percentile: 90,
        value: 12,
      },
    ],
    overall_score: 65,
    ...overrides,
  }
}

describe('PercentileRankings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders loading skeletons while loading', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    // Should have skeleton elements (card renders)
    expect(document.querySelectorAll('[class*="animate-pulse"], [data-slot="skeleton"]').length).toBeGreaterThan(0)
  })

  it('renders nothing when no data (error)', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: null,
      isLoading: false,
      error: new Error('Not found'),
    })

    const { container } = render(<PercentileRankings username="alice" />)
    expect(container.innerHTML).toBe('')
  })

  it('renders nothing when data is null', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: null,
      isLoading: false,
      error: null,
    })

    const { container } = render(<PercentileRankings username="alice" />)
    expect(container.innerHTML).toBe('')
  })

  it('renders overall score badge', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings(),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('Top 35% overall')).toBeInTheDocument()
  })

  it('renders all ranking dimension labels', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings(),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('Shows Submitted')).toBeInTheDocument()
    expect(screen.getByText('Venues Submitted')).toBeInTheDocument()
    expect(screen.getByText('Tags Applied')).toBeInTheDocument()
    expect(screen.getByText('Edits Approved')).toBeInTheDocument()
    expect(screen.getByText('Requests Fulfilled')).toBeInTheDocument()
  })

  it('renders "Top X%" for each dimension', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings(),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('Top 20%')).toBeInTheDocument() // 80 percentile
    expect(screen.getByText('Top 40%')).toBeInTheDocument() // 60 percentile
    expect(screen.getByText('Top 60%')).toBeInTheDocument() // 40 percentile
    expect(screen.getByText('Top 80%')).toBeInTheDocument() // 20 percentile
    expect(screen.getByText('Top 10%')).toBeInTheDocument() // 90 percentile
  })

  it('renders progress bars with correct widths', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings(),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    const progressBars = document.querySelectorAll('[style*="width"]')
    const widths = Array.from(progressBars).map(
      (bar) => (bar as HTMLElement).style.width
    )
    expect(widths).toContain('80%')
    expect(widths).toContain('60%')
    expect(widths).toContain('40%')
    expect(widths).toContain('20%')
    expect(widths).toContain('90%')
  })

  it('renders green color for high percentile (>= 75)', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings({
        rankings: [
          {
            dimension: 'shows_submitted',
            label: 'Shows Submitted',
            percentile: 85,
            value: 20,
          },
        ],
        overall_score: 85,
      }),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    const progressBar = document.querySelector('[style*="width: 85%"]')
    expect(progressBar?.className).toContain('bg-green-500')
  })

  it('renders red color for low percentile (< 25)', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings({
        rankings: [
          {
            dimension: 'shows_submitted',
            label: 'Shows Submitted',
            percentile: 10,
            value: 1,
          },
        ],
        overall_score: 10,
      }),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    const progressBar = document.querySelector('[style*="width: 10%"]')
    expect(progressBar?.className).toContain('bg-red-500')
  })

  it('renders heading text "Rankings"', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings(),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('Rankings')).toBeInTheDocument()
  })

  it('only shows overall badge when rankings array is empty (count_only privacy)', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings({
        rankings: [],
        overall_score: 50,
      }),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('Top 50% overall')).toBeInTheDocument()
    // No individual dimension labels
    expect(screen.queryByText('Shows Submitted')).not.toBeInTheDocument()
  })

  it('renders value counts for each dimension', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings(),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('15')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('8')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
  })

  it('formats large values with k suffix', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings({
        rankings: [
          {
            dimension: 'tags_applied',
            label: 'Tags Applied',
            percentile: 99,
            value: 1500,
          },
        ],
      }),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('1.5k')).toBeInTheDocument()
  })

  it('passes username to the hook', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: null,
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="testuser" />)
    expect(mockUsePercentileRankings).toHaveBeenCalledWith('testuser')
  })
})
