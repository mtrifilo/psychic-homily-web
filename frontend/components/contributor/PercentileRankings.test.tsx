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
    expect(
      document.querySelectorAll('[class*="animate-pulse"], [data-slot="skeleton"]')
        .length
    ).toBeGreaterThan(0)
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

  it('renders the section header with the overall standing (board G)', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings(),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('Percentile rankings')).toBeInTheDocument()
    expect(screen.getByText('overall · top 35%')).toBeInTheDocument()
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

  it('renders "top X%" for each dimension', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings(),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('top 20%')).toBeInTheDocument() // 80 percentile
    expect(screen.getByText('top 40%')).toBeInTheDocument() // 60 percentile
    expect(screen.getByText('top 60%')).toBeInTheDocument() // 40 percentile
    expect(screen.getByText('top 80%')).toBeInTheDocument() // 20 percentile
    expect(screen.getByText('top 10%')).toBeInTheDocument() // 90 percentile
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

  it('renders every bar fill in the primary token (single-hue, board G)', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings(),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    const fills = document.querySelectorAll('div.h-full')
    expect(fills.length).toBe(5)
    fills.forEach(fill => {
      expect(fill.className).toContain('bg-primary')
    })
  })

  it('only shows overall standing when rankings array is empty (count_only privacy)', () => {
    mockUsePercentileRankings.mockReturnValue({
      data: makeRankings({
        rankings: [],
        overall_score: 50,
      }),
      isLoading: false,
      error: null,
    })

    render(<PercentileRankings username="alice" />)
    expect(screen.getByText('overall · top 50%')).toBeInTheDocument()
    // No individual dimension labels
    expect(screen.queryByText('Shows Submitted')).not.toBeInTheDocument()
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

  // MIN-WIDTH GUARD
  // Bars at percentile 0 or 1 would render as invisible slivers; the
  // helper applies Math.max(percentile, 2) so the bar is always at
  // least 2% wide. Verify both edges.
  describe('progress bar min-width guard', () => {
    it('renders width=2% (not 0%) for percentile=0', () => {
      mockUsePercentileRankings.mockReturnValue({
        data: makeRankings({
          rankings: [
            {
              dimension: 'shows_submitted',
              label: 'Shows Submitted',
              percentile: 0,
              value: 0,
            },
          ],
          overall_score: 0,
        }),
        isLoading: false,
        error: null,
      })

      render(<PercentileRankings username="alice" />)
      // The visible bar should be 2% wide (min), not 0%.
      const bar = document.querySelector('div.h-full.bg-primary') as HTMLElement | null
      expect(bar).not.toBeNull()
      expect(bar?.style.width).toBe('2%')
    })

    it('renders width=2% for percentile=1', () => {
      mockUsePercentileRankings.mockReturnValue({
        data: makeRankings({
          rankings: [
            {
              dimension: 'shows_submitted',
              label: 'Shows Submitted',
              percentile: 1,
              value: 1,
            },
          ],
          overall_score: 1,
        }),
        isLoading: false,
        error: null,
      })

      render(<PercentileRankings username="alice" />)
      const bar = document.querySelector('div.h-full.bg-primary') as HTMLElement | null
      expect(bar?.style.width).toBe('2%')
    })
  })
})
