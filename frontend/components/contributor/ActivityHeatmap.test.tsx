import React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ActivityHeatmap } from './ActivityHeatmap'

// Mock the hook
const mockUseActivityHeatmap = vi.fn()

vi.mock('@/features/auth', () => ({
  useActivityHeatmap: (username: string) => mockUseActivityHeatmap(username),
}))

describe('ActivityHeatmap', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-31T12:00:00Z'))
    // Default mock: set window width to desktop
    Object.defineProperty(window, 'innerWidth', { value: 1024, writable: true })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows loading skeleton while fetching', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    render(<ActivityHeatmap username="alice" />)
    expect(screen.getByTestId('activity-heatmap-skeleton')).toBeInTheDocument()
  })

  it('renders empty state with 0 contributions', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: { days: [] },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)
    expect(screen.getByTestId('activity-heatmap')).toBeInTheDocument()
    expect(screen.getByText(/0 contributions/)).toBeInTheDocument()
  })

  it('renders grid with correct number of cells for active days', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: {
        days: [
          { date: '2026-03-31', count: 5 },
          { date: '2026-03-30', count: 2 },
          { date: '2026-01-15', count: 1 },
        ],
      },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)
    expect(screen.getByTestId('activity-heatmap')).toBeInTheDocument()
    expect(screen.getByText(/8 contributions/)).toBeInTheDocument()

    // Verify specific cells exist with correct data attributes
    const cell1 = screen.getByTestId('heatmap-cell-2026-03-31')
    expect(cell1).toBeInTheDocument()
    expect(cell1.getAttribute('data-count')).toBe('5')

    const cell2 = screen.getByTestId('heatmap-cell-2026-03-30')
    expect(cell2).toBeInTheDocument()
    expect(cell2.getAttribute('data-count')).toBe('2')
  })

  it('shows tooltip on hover', async () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: {
        days: [{ date: '2026-03-31', count: 3 }],
      },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)

    const cell = screen.getByTestId('heatmap-cell-2026-03-31')
    fireEvent.mouseEnter(cell)

    const tooltip = screen.getByTestId('heatmap-tooltip')
    expect(tooltip).toBeInTheDocument()
    expect(tooltip.textContent).toContain('3 contributions on')
    expect(tooltip.textContent).toContain('March 31, 2026')

    fireEvent.mouseLeave(cell)
    expect(screen.queryByTestId('heatmap-tooltip')).not.toBeInTheDocument()
  })

  it('tooltip shows singular "contribution" for count of 1', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: {
        days: [{ date: '2026-03-31', count: 1 }],
      },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)

    const cell = screen.getByTestId('heatmap-cell-2026-03-31')
    fireEvent.mouseEnter(cell)

    const tooltip = screen.getByTestId('heatmap-tooltip')
    expect(tooltip.textContent).toContain('1 contribution on')
    expect(tooltip.textContent).not.toContain('1 contributions')
  })

  it('tooltip shows "No contributions" for zero-count cell', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: { days: [] },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)

    // Find a cell that should have count=0
    const cell = screen.getByTestId('heatmap-cell-2026-03-31')
    fireEvent.mouseEnter(cell)

    const tooltip = screen.getByTestId('heatmap-tooltip')
    expect(tooltip.textContent).toContain('No contributions on')
  })

  it('renders color intensity scaled to max count', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: {
        days: [
          { date: '2026-03-31', count: 10 },  // max => level 4
          { date: '2026-03-30', count: 2 },   // 2/10 = 0.2 => level 1
          { date: '2026-03-29', count: 5 },   // 5/10 = 0.5 => level 2
          { date: '2026-03-28', count: 8 },   // 8/10 = 0.8 => level 4
        ],
      },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)

    // Max cell should have the full primary token (level 4)
    const maxCell = screen.getByTestId('heatmap-cell-2026-03-31')
    expect(maxCell.className).toMatch(/(?:^|\s)bg-primary(?:\s|$)/)

    // Low cell should have primary/25 (level 1)
    const lowCell = screen.getByTestId('heatmap-cell-2026-03-30')
    expect(lowCell.className).toContain('bg-primary/25')
  })

  it('renders legend with Less/More labels', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: { days: [] },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)
    expect(screen.getByText('Less')).toBeInTheDocument()
    expect(screen.getByText('More')).toBeInTheDocument()
  })

  it('renders day-of-week labels', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: { days: [] },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)
    expect(screen.getByText('Mon')).toBeInTheDocument()
    expect(screen.getByText('Wed')).toBeInTheDocument()
    expect(screen.getByText('Fri')).toBeInTheDocument()
  })

  it('renders month labels', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: { days: [] },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)
    // March 2026 should be shown in the month labels
    expect(screen.getByText('Mar')).toBeInTheDocument()
  })

  it('passes username to the hook', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: { days: [] },
      isLoading: false,
    })

    render(<ActivityHeatmap username="bob" />)
    expect(mockUseActivityHeatmap).toHaveBeenCalledWith('bob')
  })

  it('uses 26 weeks on mobile viewport', () => {
    Object.defineProperty(window, 'innerWidth', { value: 400, writable: true })

    mockUseActivityHeatmap.mockReturnValue({
      data: { days: [] },
      isLoading: false,
    })

    render(<ActivityHeatmap username="alice" />)
    expect(screen.getByText(/26 weeks/)).toBeInTheDocument()
  })

  // userEvent.hover is the closer-to-real-user mouse interaction;
  // existing tests use fireEvent.mouseEnter (which is enough to fire
  // the synthetic React event) but the ticket asks for hover coverage.
  it('shows tooltip via userEvent.hover and clears it on unhover', async () => {
    // Use today's date so the cell is guaranteed to be inside the visible
    // 52-week grid, regardless of when this test runs. A hardcoded historical
    // date would silently fall outside the grid after ~52 weeks and the
    // getByTestId would start throwing.
    const today = new Date().toISOString().slice(0, 10)

    mockUseActivityHeatmap.mockReturnValue({
      data: {
        days: [{ date: today, count: 4 }],
      },
      isLoading: false,
    })

    // userEvent's async API needs real timers to advance internal delays.
    vi.useRealTimers()
    const user = userEvent.setup()

    render(<ActivityHeatmap username="alice" />)
    const cell = screen.getByTestId(`heatmap-cell-${today}`)

    await user.hover(cell)
    const tooltip = screen.getByTestId('heatmap-tooltip')
    expect(tooltip.textContent).toContain('4 contributions on')

    await user.unhover(cell)
    expect(screen.queryByTestId('heatmap-tooltip')).not.toBeInTheDocument()
  })

  // INTENSITY-BOUNDARY COVERAGE
  // The intensity helper buckets ratio (count/max) into level 0..4.
  // The boundary points (0.25, 0.50, 0.75) are the load-bearing
  // thresholds — verify each one stays on the lower side.
  describe('intensity color mapping', () => {
    it('count === 0 maps to level 0 (bg-muted/40)', () => {
      mockUseActivityHeatmap.mockReturnValue({
        data: {
          days: [{ date: '2026-03-31', count: 0 }],
        },
        isLoading: false,
      })
      render(<ActivityHeatmap username="alice" />)
      const cell = screen.getByTestId('heatmap-cell-2026-03-31')
      expect(cell.className).toContain('bg-muted/40')
    })

    it('ratio === 0.25 boundary stays on level 1 (primary/25)', () => {
      mockUseActivityHeatmap.mockReturnValue({
        data: {
          days: [
            { date: '2026-03-31', count: 1 }, // 1/4 = 0.25 -> level 1
            { date: '2026-03-30', count: 4 }, // max
          ],
        },
        isLoading: false,
      })
      render(<ActivityHeatmap username="alice" />)
      const cell = screen.getByTestId('heatmap-cell-2026-03-31')
      expect(cell.className).toContain('bg-primary/25')
    })

    it('ratio === 0.50 boundary stays on level 2 (primary/50)', () => {
      mockUseActivityHeatmap.mockReturnValue({
        data: {
          days: [
            { date: '2026-03-31', count: 2 }, // 2/4 = 0.50 -> level 2
            { date: '2026-03-30', count: 4 },
          ],
        },
        isLoading: false,
      })
      render(<ActivityHeatmap username="alice" />)
      const cell = screen.getByTestId('heatmap-cell-2026-03-31')
      expect(cell.className).toContain('bg-primary/50')
    })

    it('ratio === 0.75 boundary stays on level 3 (primary/75)', () => {
      mockUseActivityHeatmap.mockReturnValue({
        data: {
          days: [
            { date: '2026-03-31', count: 3 }, // 3/4 = 0.75 -> level 3
            { date: '2026-03-30', count: 4 },
          ],
        },
        isLoading: false,
      })
      render(<ActivityHeatmap username="alice" />)
      const cell = screen.getByTestId('heatmap-cell-2026-03-31')
      expect(cell.className).toContain('bg-primary/75')
    })

    it('ratio > 0.75 maps to level 4 (bare primary)', () => {
      mockUseActivityHeatmap.mockReturnValue({
        data: {
          days: [
            { date: '2026-03-31', count: 10 }, // max -> level 4
          ],
        },
        isLoading: false,
      })
      render(<ActivityHeatmap username="alice" />)
      const cell = screen.getByTestId('heatmap-cell-2026-03-31')
      expect(cell.className).toMatch(/(?:^|\s)bg-primary(?:\s|$)/)
    })
  })

  // TOOLTIP DATE FORMAT
  // The tooltip uses Intl.DateTimeFormat (en-US) "weekday, month day, year".
  // Verify the user-visible string matches this expected format so a
  // future locale-switch can't regress accessibility text silently.
  it('tooltip text follows weekday/month/day/year format', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: {
        days: [{ date: '2026-03-31', count: 2 }],
      },
      isLoading: false,
    })
    render(<ActivityHeatmap username="alice" />)
    const cell = screen.getByTestId('heatmap-cell-2026-03-31')
    fireEvent.mouseEnter(cell)
    // 2026-03-31 is a Tuesday
    const tooltip = screen.getByTestId('heatmap-tooltip')
    expect(tooltip.textContent).toMatch(/Tuesday, March 31, 2026/)
  })

  // RECONSTRUCT THE GRID WHEN A NEW WEEK STARTS ON ANY DAY-OF-WEEK
  // The grid pads the first column with blank slots when start-day !=
  // Sunday. Confirm the cells use the same width/height so visual rows
  // stay aligned.
  it('renders day cells with stable width/height styling', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: {
        days: [{ date: '2026-03-31', count: 1 }],
      },
      isLoading: false,
    })
    render(<ActivityHeatmap username="alice" />)
    const cell = screen.getByTestId('heatmap-cell-2026-03-31')
    // Inline width/height pinned to 11px each (set via style prop).
    expect(cell.style.width).toBe('11px')
    expect(cell.style.height).toBe('11px')
  })

  it('renders all legend swatches (5 intensity classes)', () => {
    mockUseActivityHeatmap.mockReturnValue({
      data: { days: [] },
      isLoading: false,
    })
    const { container } = render(<ActivityHeatmap username="alice" />)
    // Legend swatches are 11x11px boxes between "Less" and "More" labels.
    // Each has a 2px-rounded shape via `rounded-[2px]`.
    const swatches = container.querySelectorAll(
      'div[class*="rounded-\\[2px\\]"][class*="w-\\[11px\\]"]'
    )
    expect(swatches.length).toBe(5)
  })
})
