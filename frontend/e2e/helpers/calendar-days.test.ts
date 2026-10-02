import { describe, expect, it } from 'vitest'
import { daysOfMonth, firstUnlistedDay } from './calendar-days'

/** Days `from` through `to` inclusive, as two-digit segments. */
function dayRange(from: number, to: number): Set<string> {
  return new Set(
    Array.from({ length: to - from + 1 }, (_, index) =>
      String(from + index).padStart(2, '0')
    )
  )
}

describe('daysOfMonth', () => {
  it.each([
    [2026, 1, 31],
    [2026, 2, 28],
    [2028, 2, 29],
    [2100, 2, 28],
    [2026, 4, 30],
    [2026, 10, 31],
    [2026, 12, 31],
  ])('%i-%i has %i days', (year, month, length) => {
    const days = daysOfMonth(year, month)
    expect(days).toHaveLength(length)
    expect(days[0]).toBe('01')
    expect(days[days.length - 1]).toBe(String(length))
  })
})

describe('firstUnlistedDay', () => {
  it('finds a day past the 28th when the first four weeks are listed', () => {
    expect(firstUnlistedDay(2026, 9, dayRange(1, 28))).toBe('29')
  })

  it('returns undefined when every day of the month is listed', () => {
    expect(firstUnlistedDay(2026, 10, dayRange(1, 31))).toBeUndefined()
    expect(firstUnlistedDay(2026, 2, dayRange(1, 28))).toBeUndefined()
  })

  it('does not offer a day the month does not have', () => {
    expect(firstUnlistedDay(2026, 4, dayRange(1, 30))).toBeUndefined()
    expect(firstUnlistedDay(2028, 2, dayRange(1, 28))).toBe('29')
  })

  it('finds the 1st when only the month tail is listed', () => {
    expect(firstUnlistedDay(2026, 9, dayRange(29, 30))).toBe('01')
  })

  it('finds a gap in the middle of the month', () => {
    const listed = dayRange(1, 31)
    listed.delete('17')
    expect(firstUnlistedDay(2026, 10, listed)).toBe('17')
  })
})
