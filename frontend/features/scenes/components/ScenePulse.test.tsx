import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ScenePulse } from './ScenePulse'
import type { ScenePulse as ScenePulseData } from '../types'

// PSY-690: ScenePulse renders three activity metrics (shows this month, new
// artists, active venues), a trend indicator vs last month, and a 6-month
// sparkline. The sparkline month labels derive from the real `Date`, so the
// label assertions pin the system clock to a known date.

function buildPulse(overrides: Partial<ScenePulseData> = {}): ScenePulseData {
  return {
    shows_this_month: 30,
    shows_prev_month: 25,
    shows_trend: 5,
    new_artists_30d: 8,
    active_venues_this_month: 10,
    shows_by_month: [20, 22, 25, 28, 30, 30],
    ...overrides,
  }
}

describe('ScenePulse', () => {
  it('renders the three activity counts with their metric labels', () => {
    const { container } = render(<ScenePulse pulse={buildPulse()} />)

    expect(screen.getByText('Scene Pulse')).toBeInTheDocument()

    expect(screen.getByText('Shows this month')).toBeInTheDocument()
    expect(screen.getByText('New artists')).toBeInTheDocument()
    expect(screen.getByText('Active venues')).toBeInTheDocument()

    // The three big metric values render in `.text-2xl` spans; the sparkline
    // bar labels reuse the same digits, so scope to the metric spans.
    const metricValues = Array.from(
      container.querySelectorAll('span.text-2xl')
    ).map((el) => el.textContent)
    expect(metricValues).toEqual(['30', '8', '10'])
  })

  it('renders the time-window labels for each metric', () => {
    render(<ScenePulse pulse={buildPulse()} />)

    // The new-artists window and the active-venues window.
    expect(screen.getByText('past 30 days')).toBeInTheDocument()
    expect(screen.getByText('this month')).toBeInTheDocument()
    // The shows-this-month metric carries a vs-last-month comparison.
    expect(screen.getByText(/vs 25 last month/)).toBeInTheDocument()
  })

  describe('trend indicator', () => {
    it('shows a positive trend with a leading plus sign', () => {
      render(<ScenePulse pulse={buildPulse({ shows_trend: 5 })} />)
      expect(screen.getByText('+5')).toBeInTheDocument()
    })

    it('shows a negative trend with its sign preserved', () => {
      render(<ScenePulse pulse={buildPulse({ shows_trend: -3 })} />)
      // The raw negative number already carries the minus sign.
      expect(screen.getByText('-3')).toBeInTheDocument()
    })

    it('shows a flat zero trend without a sign', () => {
      render(<ScenePulse pulse={buildPulse({ shows_trend: 0 })} />)
      expect(screen.getByText('0')).toBeInTheDocument()
    })
  })

  describe('sparkline', () => {
    beforeEach(() => {
      vi.useFakeTimers()
      // Pin to mid-month so the day=1 normalization in getMonthLabel can't
      // overflow regardless of month length. June 2026.
      vi.setSystemTime(new Date('2026-06-15T12:00:00Z'))
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it('renders the section heading and labels the last 6 months oldest-first', () => {
      // index 0 = oldest (5 months ago = Jan), last = current (Jun).
      render(<ScenePulse pulse={buildPulse({ shows_by_month: [1, 2, 3, 4, 5, 6] })} />)

      expect(
        screen.getByText('Shows per month (last 6 months)')
      ).toBeInTheDocument()

      // 6 months back from June 2026: Jan, Feb, Mar, Apr, May, Jun.
      for (const month of ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun']) {
        expect(screen.getByText(month)).toBeInTheDocument()
      }
    })

    it('renders each non-zero monthly value and suppresses zero-value labels', () => {
      const { container } = render(
        <ScenePulse pulse={buildPulse({ shows_by_month: [0, 7, 0, 9, 0, 11] })} />
      )

      // Non-zero values appear as bar labels.
      expect(screen.getByText('7')).toBeInTheDocument()
      expect(screen.getByText('9')).toBeInTheDocument()
      expect(screen.getByText('11')).toBeInTheDocument()

      // The sparkline value labels use the [10px] utility; a zero value
      // renders an empty label, so none of those spans should read "0".
      const valueLabels = Array.from(
        container.querySelectorAll('span.text-\\[10px\\]')
      )
      const zeroValueLabel = valueLabels.find((el) => el.textContent === '0')
      expect(zeroValueLabel).toBeUndefined()
    })

    it('hides the sparkline section when there is no monthly data', () => {
      render(<ScenePulse pulse={buildPulse({ shows_by_month: [] })} />)
      expect(
        screen.queryByText('Shows per month (last 6 months)')
      ).not.toBeInTheDocument()
    })
  })

  it('renders zero counts without crashing (empty scene)', () => {
    const { container } = render(
      <ScenePulse
        pulse={buildPulse({
          shows_this_month: 0,
          shows_prev_month: 0,
          shows_trend: 0,
          new_artists_30d: 0,
          active_venues_this_month: 0,
          shows_by_month: [],
        })}
      />
    )

    // All three big metric values render their zero (sparkline is hidden when
    // shows_by_month is empty, so these are the only `.text-2xl` spans).
    const metricValues = Array.from(
      container.querySelectorAll('span.text-2xl')
    ).map((el) => el.textContent)
    expect(metricValues).toEqual(['0', '0', '0'])
  })
})
