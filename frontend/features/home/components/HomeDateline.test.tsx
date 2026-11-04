import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { HomeDateline, formatDateline } from './HomeDateline'

let pulse: {
  data?: { shows_this_week: number; entities_in_graph: number }
  isError: boolean
}
vi.mock('../hooks/useCommunityPulse', () => ({
  useCommunityPulse: () => pulse,
}))

// A local date, so the assertion holds in any time zone the suite runs in.
const WED_NOV_4 = new Date(2026, 10, 4, 9, 30)
const PULSE = { shows_this_week: 1204, entities_in_graph: 25838 }

beforeEach(() => {
  pulse = { data: PULSE, isError: false }
})

describe('formatDateline', () => {
  it('writes the phone form: abbreviated date, the week, the entities', () => {
    expect(formatDateline(WED_NOV_4, PULSE, 'short')).toBe(
      'Wed Nov 4 · 1,204 shows in the next 7 days · 25,838 entities'
    )
  })

  it('writes the desktop form: the full date and what each count measures', () => {
    expect(formatDateline(WED_NOV_4, PULSE, 'long')).toBe(
      'Wednesday, November 4 · 1,204 shows in the next 7 days · 25,838 entities in the graph'
    )
  })

  it('agrees in number with a count of one', () => {
    const one = { shows_this_week: 1, entities_in_graph: 1 }
    expect(formatDateline(WED_NOV_4, one, 'short')).toBe(
      'Wed Nov 4 · 1 show in the next 7 days · 1 entity'
    )
    expect(formatDateline(WED_NOV_4, one, 'long')).toBe(
      'Wednesday, November 4 · 1 show in the next 7 days · 1 entity in the graph'
    )
  })

  it('is the date alone without counts', () => {
    expect(formatDateline(WED_NOV_4, undefined, 'short')).toBe('Wed Nov 4')
  })
})

describe('HomeDateline', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(WED_NOV_4)
    return () => vi.useRealTimers()
  })

  it('renders the line in the data face, uppercase and muted, at the form size', () => {
    render(<HomeDateline form="short" />)

    const line = screen.getByText('Wed Nov 4 · 1,204 shows in the next 7 days · 25,838 entities')
    expect(line).toHaveClass('font-data', 'uppercase', 'text-muted-foreground', 'text-xs')
  })

  it('holds the line with a placeholder until the counts arrive', () => {
    pulse = { data: undefined, isError: false }
    const { container } = render(<HomeDateline form="long" />)

    expect(container.querySelector('p')).toHaveTextContent('')
    expect(container.querySelector('p span[aria-hidden]')).toBeInTheDocument()
  })

  it('keeps the date when the counts fail to load', () => {
    pulse = { data: undefined, isError: true }
    render(<HomeDateline form="long" />)

    expect(screen.getByText('Wednesday, November 4')).toBeInTheDocument()
  })
})
