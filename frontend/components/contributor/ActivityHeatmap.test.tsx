import React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
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

    // Max cell should have emerald-700 (level 4 dark)
    const maxCell = screen.getByTestId('heatmap-cell-2026-03-31')
    expect(maxCell.className).toContain('emerald-700')

    // Low cell should have emerald-200 (level 1)
    const lowCell = screen.getByTestId('heatmap-cell-2026-03-30')
    expect(lowCell.className).toContain('emerald-200')
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
})
